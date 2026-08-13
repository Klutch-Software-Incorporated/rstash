using System.Net;
using System.Net.Http.Headers;
using Microsoft.AspNetCore.Identity;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;
using Rstash.Services;

namespace Rstash.IntegrationTests;

/// <summary>
/// Covers the two things that make password guessing expensive: a per-IP limit on
/// how fast sign-in attempts can arrive, and an account lockout once enough of
/// them fail — plus the traffic that must never be caught in either net.
/// </summary>
/// <remarks>
/// Each test gets its own host, and the host trusts X-Forwarded-For so each test
/// can present its own client address. Rate-limit buckets are keyed by IP, so
/// sharing one host would make these tests draw down a common budget and pass or
/// fail on the order they happened to run in. An account is seeded first because
/// the setup guard redirects every non-exempt route to /setup until one exists —
/// and that redirect short-circuits ahead of the rate limiter, so without it
/// these tests would exercise nothing.
/// </remarks>
public sealed class RateLimitTests : IAsyncLifetime, IDisposable
{
    private const string Password = "Correct-Horse-1!";

    private readonly RstashAppFactory _factory =
        new(new Dictionary<string, string> { ["RSTASH_TRUST_PROXY"] = "true" });

    public async Task InitializeAsync() => await EnsureUserAsync("someone");

    public Task DisposeAsync() => Task.CompletedTask;

    public void Dispose() => _factory.Dispose();

    [Fact]
    public async Task SignInAttempts_ExceedingBurst_Get429()
    {
        var client = ClientFrom("203.0.113.10");
        var token = await AntiforgeryTokenAsync(client);

        // The default sign-in budget is a burst of 5, refilling one every five
        // seconds, so a rapid run past the burst has to be refused.
        var statuses = new List<HttpStatusCode>();
        for (var i = 0; i < 12; i++)
        {
            statuses.Add((await PostLoginAsync(client, token, "nobody", "wrong")).StatusCode);
        }

        Assert.Contains(HttpStatusCode.TooManyRequests, statuses);

        var rejected = statuses.FindIndex(s => s == HttpStatusCode.TooManyRequests);
        Assert.True(rejected >= 5, $"throttled after only {rejected} attempts; the burst should allow 5");
    }

    [Theory]
    [InlineData("/healthz")]
    [InlineData("/app.css")]
    [InlineData("/app.js")]
    [InlineData("/favicon.svg")]
    [InlineData("/site.webmanifest")]
    public async Task StaticAssetsAndHealth_AreNeverThrottled(string path)
    {
        // A distinct address per case so the theory's cases don't share a budget.
        var client = ClientFrom($"203.0.113.{20 + path.Length}");

        // Well past any configured burst: throttling these would break the UI and
        // hide whether the server is up. The exemption list used to name /css and
        // /js — directories this app doesn't have — so every page load quietly
        // spent five tokens of the caller's budget, and a throttled stylesheet
        // renders the UI unstyled rather than showing an error.
        for (var i = 0; i < 40; i++)
        {
            Assert.NotEqual(HttpStatusCode.TooManyRequests, (await client.GetAsync(path)).StatusCode);
        }
    }

    /// <summary>
    /// A stored document that happens to end in .css is storage traffic, not a
    /// static asset: the exemption covers root-level files out of wwwroot only.
    /// </summary>
    [Fact]
    public async Task StorageDocuments_WithAssetExtensions_AreNotExempt()
    {
        var client = ClientFrom("203.0.113.40");

        var statuses = new List<HttpStatusCode>();
        for (var i = 0; i < 40; i++)
        {
            statuses.Add((await client.GetAsync("/storage/someone/public/theme/site.css")).StatusCode);
        }

        Assert.Contains(HttpStatusCode.TooManyRequests, statuses);
    }

    /// <summary>
    /// Rendering the sign-in form must not spend the sign-in budget. At 0.2/s with
    /// a burst of 5, charging GETs meant one page view plus four bad passwords
    /// exhausted the bucket — so the fifth attempt, the one that trips the account
    /// lockout, returned a bare 429 and the user never saw the lockout message.
    /// </summary>
    [Fact]
    public async Task LoadingTheLoginPage_DoesNotSpendTheSignInBudget()
    {
        var client = ClientFrom("203.0.113.50");

        for (var i = 0; i < 10; i++)
        {
            Assert.NotEqual(HttpStatusCode.TooManyRequests, (await client.GetAsync("/login")).StatusCode);
        }

        // The whole burst is still there for attempts that actually guess.
        var token = await AntiforgeryTokenAsync(client);
        for (var i = 0; i < 5; i++)
        {
            var response = await PostLoginAsync(client, token, "nobody", "wrong");
            Assert.NotEqual(HttpStatusCode.TooManyRequests, response.StatusCode);
        }
    }

    [Fact]
    public async Task RepeatedBadPasswords_LockTheAccount()
    {
        const string username = "lockoutuser";
        await EnsureUserAsync(username);

        using var scope = _factory.Services.CreateScope();
        var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();
        var signIn = scope.ServiceProvider.GetRequiredService<SignInManager<ApplicationUser>>();
        var user = await users.FindByNameAsync(username);

        // Five failures is the configured threshold.
        for (var i = 0; i < 5; i++)
        {
            await signIn.CheckPasswordSignInAsync(user!, "wrong-password", lockoutOnFailure: true);
        }

        var afterLockout = await signIn.CheckPasswordSignInAsync(user!, Password, lockoutOnFailure: true);

        // Even the right password is refused while the account is locked.
        Assert.True(afterLockout.IsLockedOut);
        Assert.False(afterLockout.Succeeded);
    }

    /// <summary>
    /// A user who mistypes their password five times must reach the account
    /// lockout through the browser rather than a bare 429 from the rate limiter.
    /// That is the point of not charging the sign-in budget for page loads: the
    /// GET that renders the form used to cost a token, so the fifth attempt — the
    /// one that locks the account — was refused before it ever reached Identity.
    /// </summary>
    [Fact]
    public async Task FiveBadPasswordsInABrowser_ShowTheLockoutMessage()
    {
        const string username = "browserlockout";
        await EnsureUserAsync(username);

        var client = ClientFrom("203.0.113.60");
        var token = await AntiforgeryTokenAsync(client);

        var body = "";
        for (var i = 0; i < 5; i++)
        {
            var response = await PostLoginAsync(client, token, username, "wrong-password");
            Assert.NotEqual(HttpStatusCode.TooManyRequests, response.StatusCode);
            body = await response.Content.ReadAsStringAsync();
        }

        Assert.Contains("locked", body, StringComparison.OrdinalIgnoreCase);
    }

    /// <summary>
    /// Storage traffic is budgeted per app token, not per address. This used to
    /// key on <c>context.User</c>, which is never populated for /storage — bearer
    /// tokens are read by hand inside the endpoint, so there is no authentication
    /// scheme and the principal is still anonymous when the limiter runs. Every
    /// app therefore fell through to the shared per-IP bucket, and the per-app
    /// settings did nothing.
    /// </summary>
    [Fact]
    public async Task StorageTraffic_IsBudgetedPerAppToken_NotPerAddress()
    {
        using (var scope = _factory.Services.CreateScope())
        {
            var settings = scope.ServiceProvider.GetRequiredService<SettingsService>();
            await settings.SetAsync("user_rate_limit_rate", "1");
            await settings.SetAsync("user_rate_limit_burst", "3");
        }

        var appA = await SeedTokenAsync("someone");
        var appB = await SeedTokenAsync("someone");

        // Both apps arrive from one address, so a budget keyed on IP would pool
        // them together and B would be throttled by A's traffic.
        var client = ClientFrom("203.0.113.70");

        var statuses = new List<HttpStatusCode>();
        for (var i = 0; i < 10; i++)
        {
            statuses.Add((await GetStorageAsync(client, appA)).StatusCode);
        }

        Assert.Contains(HttpStatusCode.TooManyRequests, statuses);

        // The second app still has its own budget, untouched by the first.
        Assert.NotEqual(
            HttpStatusCode.TooManyRequests, (await GetStorageAsync(client, appB)).StatusCode);
    }

    private async Task<string> SeedTokenAsync(string username)
    {
        using var scope = _factory.Services.CreateScope();
        var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();
        var tokens = scope.ServiceProvider.GetRequiredService<TokenStore>();
        var user = await users.FindByNameAsync(username);

        var token = await tokens.CreateAsync(
            user!.Id, "https://app.example.com", "*:rw", lifetime: null, withRefreshToken: false);
        return token.Token;
    }

    private static Task<HttpResponseMessage> GetStorageAsync(HttpClient client, string token)
    {
        var request = new HttpRequestMessage(HttpMethod.Get, "/storage/someone/notes/a.txt");
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        return client.SendAsync(request);
    }

    private async Task EnsureUserAsync(string username)
    {
        using var scope = _factory.Services.CreateScope();
        var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();
        if (await users.FindByNameAsync(username) is not null)
        {
            return;
        }

        var created = await users.CreateAsync(
            new ApplicationUser { UserName = username, Approved = true, CreatedAt = DateTimeOffset.UtcNow },
            Password);
        Assert.True(created.Succeeded);
    }

    private HttpClient ClientFrom(string ip)
    {
        var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Add("X-Forwarded-For", ip);
        return client;
    }

    /// <summary>
    /// One token, reused across a run of attempts: it stays valid for the client's
    /// session, and fetching a fresh one per attempt would add a page load per
    /// attempt and muddy what the test is measuring.
    /// </summary>
    private static async Task<string> AntiforgeryTokenAsync(HttpClient client) =>
        FormHelpers.AntiforgeryToken(await client.GetStringAsync("/login"));

    private static Task<HttpResponseMessage> PostLoginAsync(
        HttpClient client, string token, string username, string password) =>
        client.PostAsync("/login", new FormUrlEncodedContent(
            new Dictionary<string, string>
            {
                ["_handler"] = "login",
                ["__RequestVerificationToken"] = token,
                ["Input.Username"] = username,
                ["Input.Password"] = password,
            }));
}
