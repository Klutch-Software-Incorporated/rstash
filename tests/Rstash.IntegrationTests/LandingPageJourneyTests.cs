using System.Net;
using Microsoft.AspNetCore.Identity;
using Microsoft.AspNetCore.Mvc.Testing;

namespace Rstash.IntegrationTests;

/// <summary>
/// The journey a real person takes: finish setup, land on the dashboard. Everything
/// else asserts on routes that carry <c>[Authorize]</c>, which challenge OpenID
/// Connect automatically; the home page does not, so it exercises a different path.
/// </summary>
public sealed class LandingPageJourneyTests
{
    [Fact]
    public async Task AfterSetup_TheHomePageRenders_RatherThanBouncingToLogin()
    {
        using var factory = new RstashAppFactory(new Dictionary<string, string>());
        var client = CreateFollowingClient(factory);
        var setup = await CompleteSetupAsync(client);

        Assert.Equal(HttpStatusCode.OK, setup.StatusCode);

        // Setup redirects to "/". Landing on the login form instead means the person
        // who just created the account is being asked to sign in again.
        Assert.DoesNotContain(
            "/login",
            setup.RequestMessage!.RequestUri!.PathAndQuery);
    }

    [Fact]
    public async Task AfterSetup_TheCurrentUserResolves_OnAPageThatChallenges()
    {
        using var factory = new RstashAppFactory(new Dictionary<string, string>());
        var client = CreateFollowingClient(factory);
        await CompleteSetupAsync(client);

        // /account challenges OpenID Connect. With query response mode the whole
        // round-trip is ordinary redirects, so following them completes the login.
        var account = await client.GetAsync("/account");
        var body = await account.Content.ReadAsStringAsync();

        Assert.Equal(HttpStatusCode.OK, account.StatusCode);
        Assert.DoesNotContain("/login", account.RequestMessage!.RequestUri!.PathAndQuery);

        // Guards against the flow stalling on an authorize response that was never
        // completed — a form_post page is a 200 that is not the login page, so weaker
        // assertions pass without anyone having logged in.
        Assert.False(FormPost.IsFormPost(body), "login stalled on an unsubmitted authorize response");

        // The page resolves the current user through CurrentUserAccessor, which asks
        // UserManager to map the principal back to an ApplicationUser. That mapping
        // depends on the claim types the handler emits.
        Assert.Contains("curtis", body);
    }

    /// <summary>
    /// The signed-in half of the application, reached the way a person reaches it.
    /// </summary>
    /// <remarks>
    /// Everything else asserts on anonymous requests, which only prove that a challenge
    /// happens. Nothing exercised an authenticated request carrying the cookie that the
    /// OpenID Connect round-trip actually issues — so when the principal turned out to
    /// carry no claim <see cref="UserManager{TUser}"/> could look a user up by, every
    /// admin gate silently failed closed and the whole signed-in application rendered
    /// empty, with a green suite.
    /// </remarks>
    [Fact]
    public async Task AfterLogin_AdminSurfacesResolveTheSignedInUser()
    {
        using var factory = new RstashAppFactory(new Dictionary<string, string>());
        var client = CreateFollowingClient(factory);
        await CompleteSetupAsync(client);

        // Each marker is rendered only on the far side of a successful
        // CurrentUserAccessor lookup — the storage address is inside "@if (_user is not
        // null)", the user table is behind an IsAdmin gate, and the address comes off
        // the loaded entity.
        //
        // Asserting merely that the username appears would not do it: the layout falls
        // back to Identity.Name straight off the principal, so the username renders
        // perfectly well on a page where the lookup returned null and every gated
        // section is missing. Verified by deleting the claim mapping and watching that
        // weaker assertion stay green.
        var surfaces = new[]
        {
            ("/", "rs-address"),
            ("/admin/users", "data-role="),
            ("/account", "curtis@example.com"),
        };

        foreach (var (path, marker) in surfaces)
        {
            var response = await client.GetAsync(path);
            var body = await response.Content.ReadAsStringAsync();

            Assert.Equal(HttpStatusCode.OK, response.StatusCode);
            Assert.DoesNotContain("/login", response.RequestMessage!.RequestUri!.PathAndQuery);
            Assert.False(FormPost.IsFormPost(body), $"{path} stalled mid-login");
            Assert.True(body.Contains(marker, StringComparison.Ordinal), $"{path} did not resolve the signed-in user");
        }
    }

    private static HttpClient CreateFollowingClient(RstashAppFactory factory) =>
        factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = true,
            MaxAutomaticRedirections = 12,
        });

    private static async Task<HttpResponseMessage> CompleteSetupAsync(HttpClient client)
    {
        var token = FormHelpers.AntiforgeryToken(await client.GetStringAsync("/setup"));
        return await client.PostAsync("/setup", new FormUrlEncodedContent(
            new Dictionary<string, string>
            {
                ["_handler"] = "setup",
                ["__RequestVerificationToken"] = token,
                ["Input.Username"] = "curtis",
                ["Input.Email"] = "curtis@example.com",
                ["Input.Password"] = "Sup3r!secret",
            }));
    }
}
