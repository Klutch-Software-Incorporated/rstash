using System.Net;
using Microsoft.AspNetCore.Mvc.Testing;

namespace Rstash.IntegrationTests;

/// <summary>
/// Logging out has to end both sessions. rstash is a relying party against its own
/// provider, so a signed-in person holds two cookies: the provider's Identity cookie
/// and the application's own. Either one left behind puts them straight back in.
/// </summary>
public sealed class LogoutTests
{
    [Fact]
    public async Task Logout_EndsBothSessions_SoAProtectedPageAsksForAPasswordAgain()
    {
        using var factory = new RstashAppFactory(new Dictionary<string, string>());
        var client = factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = true,
            MaxAutomaticRedirections = 12,
        });

        await CompleteSetupAsync(client, "logoutuser");

        // Both sessions are live at this point: setup established the provider's, and
        // fetching a protected page runs the challenge that establishes the other.
        var signedIn = await client.GetAsync("/admin/users");
        Assert.Equal(HttpStatusCode.OK, signedIn.StatusCode);
        Assert.Contains("Users — admin", await signedIn.Content.ReadAsStringAsync(), StringComparison.Ordinal);

        // The real form posts no antiforgery token, and the endpoint accordingly does
        // not ask for one — so this is the request a browser actually sends.
        var loggedOut = await client.PostAsync("/auth/logout", new StringContent(""));
        Assert.Equal(HttpStatusCode.OK, loggedOut.StatusCode);

        // Asking again must reach the password form, not the page. Landing back on the
        // page is what *both* half-logouts look like from the outside: leave the
        // application cookie and the request is still authorized; leave the provider
        // cookie and the challenge silently re-issues a code without prompting.
        var afterwards = await client.GetAsync("/admin/users");
        var body = await afterwards.Content.ReadAsStringAsync();

        Assert.Contains("id=\"login-password\"", body, StringComparison.Ordinal);
        Assert.DoesNotContain("Users — admin", body, StringComparison.Ordinal);
    }

    private static async Task CompleteSetupAsync(HttpClient client, string username)
    {
        var token = FormHelpers.AntiforgeryToken(await client.GetStringAsync("/setup"));
        var setup = await client.PostAsync("/setup", new FormUrlEncodedContent(
            new Dictionary<string, string>
            {
                ["_handler"] = "setup",
                ["__RequestVerificationToken"] = token,
                ["Input.Username"] = username,
                ["Input.Email"] = $"{username}@example.com",
                ["Input.Password"] = "Sup3r!secret",
            }));

        Assert.Equal(HttpStatusCode.OK, setup.StatusCode);
    }
}
