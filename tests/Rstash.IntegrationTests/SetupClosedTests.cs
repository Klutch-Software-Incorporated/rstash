using System.Net;
using Microsoft.AspNetCore.Identity;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;

namespace Rstash.IntegrationTests;

/// <summary>
/// The setup wizard creates an administrator with no authentication in front of it,
/// which is correct exactly once. These cover the other side of that: after an account
/// exists, /setup must be shut — reachable only as a redirect, and inert if posted to.
/// </summary>
public sealed class SetupClosedTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    private async Task CreateFirstAdminAsync()
    {
        var client = factory.CreateClient();
        var token = FormHelpers.AntiforgeryToken(await client.GetStringAsync("/setup"));

        var response = await client.PostAsync("/setup", new FormUrlEncodedContent(
            new Dictionary<string, string>
            {
                ["_handler"] = "setup",
                ["__RequestVerificationToken"] = token,
                ["Input.Username"] = "owner",
                ["Input.Password"] = "Sup3r!secret",
            }));

        response.EnsureSuccessStatusCode();
    }

    [Fact]
    public async Task SetupPage_RedirectsAway_OnceAnAccountExists()
    {
        await CreateFirstAdminAsync();

        var client = factory.CreateClient(new WebApplicationFactoryClientOptions { AllowAutoRedirect = false });
        var response = await client.GetAsync("/setup");

        Assert.Equal(HttpStatusCode.Redirect, response.StatusCode);
        Assert.Equal("/", response.Headers.Location?.OriginalString);
    }

    [Fact]
    public async Task SetupPost_DoesNotCreateASecondAdmin()
    {
        await CreateFirstAdminAsync();

        // A fresh client: no cookie, no session, nothing that says who this is.
        var attacker = factory.CreateClient();

        // Take a valid antiforgery token off the sign-in page, which is served to
        // anonymous visitors. Posting without one proves nothing: antiforgery would
        // reject the request on its own and the test would pass over an open door.
        var token = FormHelpers.AntiforgeryToken(await attacker.GetStringAsync("/login"));

        var response = await attacker.PostAsync("/setup", new FormUrlEncodedContent(
            new Dictionary<string, string>
            {
                ["_handler"] = "setup",
                ["__RequestVerificationToken"] = token,
                ["Input.Username"] = "intruder",
                ["Input.Password"] = "Intrud3r!pass",
            }));

        Assert.NotEqual(HttpStatusCode.InternalServerError, response.StatusCode);

        using var scope = factory.Services.CreateScope();
        var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();

        Assert.Null(await users.FindByNameAsync("intruder"));
        Assert.NotNull(await users.FindByNameAsync("owner"));
    }
}
