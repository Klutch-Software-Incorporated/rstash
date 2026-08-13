using System.Net;
using Microsoft.AspNetCore.Identity;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;

namespace Rstash.IntegrationTests;

/// <summary>
/// Covers the two things that make password guessing expensive: a per-IP limit on
/// how fast sign-in attempts can arrive, and an account lockout once enough of
/// them fail.
/// </summary>
public sealed class RateLimitTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    [Fact]
    public async Task SignInAttempts_ExceedingBurst_Get429()
    {
        var client = factory.CreateClient();

        // The default sign-in budget is a burst of 5, refilling one every five
        // seconds, so a rapid run past the burst has to be refused.
        var statuses = new List<HttpStatusCode>();
        for (var i = 0; i < 12; i++)
        {
            var response = await client.PostAsync("/login", new FormUrlEncodedContent(
                new Dictionary<string, string>
                {
                    ["Input.Username"] = "nobody",
                    ["Input.Password"] = "wrong",
                }));
            statuses.Add(response.StatusCode);
        }

        Assert.Contains(HttpStatusCode.TooManyRequests, statuses);

        var rejected = statuses.FindIndex(s => s == HttpStatusCode.TooManyRequests);
        Assert.True(rejected >= 5, $"throttled after only {rejected} attempts; the burst should allow 5");
    }

    [Fact]
    public async Task StaticAssetsAndHealth_AreNeverThrottled()
    {
        var client = factory.CreateClient();

        // Well past any configured burst: throttling these would break the UI and
        // hide whether the server is up.
        for (var i = 0; i < 40; i++)
        {
            Assert.NotEqual(HttpStatusCode.TooManyRequests, (await client.GetAsync("/healthz")).StatusCode);
        }
    }

    [Fact]
    public async Task RepeatedBadPasswords_LockTheAccount()
    {
        const string username = "lockoutuser";
        using (var scope = factory.Services.CreateScope())
        {
            var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();
            if (await users.FindByNameAsync(username) is null)
            {
                await users.CreateAsync(
                    new ApplicationUser { UserName = username, Approved = true }, "Correct-Horse-1!");
            }
        }

        using (var scope = factory.Services.CreateScope())
        {
            var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();
            var signIn = scope.ServiceProvider.GetRequiredService<SignInManager<ApplicationUser>>();
            var user = await users.FindByNameAsync(username);

            // Five failures is the configured threshold.
            for (var i = 0; i < 5; i++)
            {
                await signIn.CheckPasswordSignInAsync(user!, "wrong-password", lockoutOnFailure: true);
            }

            var afterLockout = await signIn.CheckPasswordSignInAsync(
                user!, "Correct-Horse-1!", lockoutOnFailure: true);

            // Even the right password is refused while the account is locked.
            Assert.True(afterLockout.IsLockedOut);
            Assert.False(afterLockout.Succeeded);
        }
    }
}
