using System.Net;
using System.Text.RegularExpressions;
using Microsoft.AspNetCore.Identity;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;

namespace Rstash.IntegrationTests;

/// <summary>
/// What the sign-in form is willing to say about an account, and when. Standing —
/// disabled, awaiting approval — is real information a blocked user needs, but it
/// only exists for accounts that exist, so handing it out before the password is
/// checked turns the form into a free username oracle.
/// </summary>
public sealed class LoginDisclosureTests : IAsyncLifetime, IDisposable
{
    private const string Password = "Sup3r!secret";
    private const string WrongPassword = "not-the-password";
    private const string Generic = "Invalid username or password.";

    private readonly RstashAppFactory _factory = new(new Dictionary<string, string>());

    public async Task InitializeAsync()
    {
        // An account has to exist or the setup guard redirects /login to /setup.
        await CreateUserAsync("regular", disabled: false, approved: true);
        await CreateUserAsync("disableduser", disabled: true, approved: true);
        await CreateUserAsync("pendinguser", disabled: false, approved: false);
    }

    public Task DisposeAsync() => Task.CompletedTask;

    public void Dispose() => _factory.Dispose();

    [Theory]
    [InlineData("disableduser")]
    [InlineData("pendinguser")]
    public async Task WrongPassword_SaysNothingAboutTheAccount(string username)
    {
        // The whole point: a guesser learns the same thing here as for a username
        // that was never registered.
        Assert.Equal(Generic, await AttemptAsync(username, WrongPassword));
    }

    [Fact]
    public async Task WrongPassword_ForARealAccount_ReadsTheSameAsAnUnknownOne()
    {
        var real = await AttemptAsync("disableduser", WrongPassword);
        var madeUp = await AttemptAsync("no-such-person", WrongPassword);

        Assert.Equal(Generic, real);
        Assert.Equal(real, madeUp);
    }

    /// <summary>
    /// Someone who proves the account is theirs still gets the real reason — the
    /// alternative leaves a blocked user with no way to tell a wrong password from
    /// an administrator who hasn't approved them yet.
    /// </summary>
    [Theory]
    [InlineData("disableduser", "disabled")]
    [InlineData("pendinguser", "pending approval")]
    public async Task CorrectPassword_TellsTheOwnerWhyTheyAreBlocked(string username, string expected)
    {
        var error = await AttemptAsync(username, Password);

        Assert.Contains(expected, error, StringComparison.OrdinalIgnoreCase);
    }

    /// <summary>
    /// Checking standing after the password means the password check runs first —
    /// so it must not be the kind that signs the user in on its way past. A
    /// disabled account that lands on the error page holding a session cookie is
    /// worse than the leak this reordering removed.
    /// </summary>
    [Theory]
    [InlineData("disableduser")]
    [InlineData("pendinguser")]
    public async Task CorrectPassword_ForABlockedAccount_DoesNotSignThemIn(string username)
    {
        var client = NewClient();
        await PostLoginAsync(client, username, Password);

        var protectedPage = await client.GetAsync("/account");
        var body = await protectedPage.Content.ReadAsStringAsync();

        Assert.Contains("id=\"login-password\"", body, StringComparison.Ordinal);
    }

    [Fact]
    public async Task CorrectPassword_ForAGoodAccount_SignsThemIn()
    {
        var client = NewClient();
        await PostLoginAsync(client, "regular", Password);

        var protectedPage = await client.GetAsync("/account");
        var body = await protectedPage.Content.ReadAsStringAsync();

        Assert.Equal(HttpStatusCode.OK, protectedPage.StatusCode);
        Assert.DoesNotContain("id=\"login-password\"", body, StringComparison.Ordinal);
    }

    /// <summary>
    /// Signs in and returns just the text of the form's error banner. Matching
    /// against the whole page would be worse than useless here — MudBlazor's inline
    /// theme defines <c>--mud-palette-text-disabled</c>, so "no mention of
    /// disabled" is true of no page this app serves.
    /// </summary>
    private async Task<string> AttemptAsync(string username, string password)
    {
        var client = NewClient();
        var response = await PostLoginAsync(client, username, password);
        var html = await response.Content.ReadAsStringAsync();

        var match = Regex.Match(
            html, "<div class=\"rs-auth-error\">(.*?)</div>", RegexOptions.Singleline);

        Assert.True(match.Success, "the sign-in form reported no error at all");
        return match.Groups[1].Value.Trim();
    }

    private static async Task<HttpResponseMessage> PostLoginAsync(
        HttpClient client, string username, string password)
    {
        var token = FormHelpers.AntiforgeryToken(await client.GetStringAsync("/login"));

        return await client.PostAsync("/login", new FormUrlEncodedContent(
            new Dictionary<string, string>
            {
                ["_handler"] = "login",
                ["__RequestVerificationToken"] = token,
                ["Input.Username"] = username,
                ["Input.Password"] = password,
            }));
    }

    private HttpClient NewClient() =>
        _factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = true,
            MaxAutomaticRedirections = 12,
        });

    private async Task CreateUserAsync(string username, bool disabled, bool approved)
    {
        using var scope = _factory.Services.CreateScope();
        var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();

        var created = await users.CreateAsync(
            new ApplicationUser
            {
                UserName = username,
                CreatedAt = DateTimeOffset.UtcNow,
                Disabled = disabled,
                Approved = approved,
            },
            Password);

        Assert.True(created.Succeeded, string.Join("; ", created.Errors.Select(e => e.Description)));
    }
}
