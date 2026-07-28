using Microsoft.AspNetCore.Mvc.Testing;

namespace Rstash.IntegrationTests;

/// <summary>
/// Behind a TLS-terminating proxy the app sees plain HTTP, so without
/// X-Forwarded-Proto handling the auth cookie ships without the Secure attribute over
/// what is really an HTTPS connection. Honouring the header is opt-in via
/// RSTASH_TRUST_PROXY, because anyone who can reach a directly-exposed rstash can
/// forge it.
/// </summary>
public sealed class ForwardedHeadersTests
{
    [Fact]
    public async Task TrustedProxy_ForwardedHttpsScheme_MarksTheAuthCookieSecure() =>
        Assert.True(await AuthCookieIsSecureAsync(trustProxy: true));

    [Fact]
    public async Task UntrustedProxy_ForwardedHttpsScheme_IsIgnored() =>
        Assert.False(await AuthCookieIsSecureAsync(trustProxy: false));

    /// <summary>
    /// Runs first-run setup — which signs the new admin in, and so emits the Identity
    /// cookie — over a request carrying <c>X-Forwarded-Proto: https</c>, and reports
    /// whether that cookie came back marked Secure.
    /// </summary>
    private static async Task<bool> AuthCookieIsSecureAsync(bool trustProxy)
    {
        using var factory = new RstashAppFactory(new Dictionary<string, string>
        {
            ["RSTASH_TRUST_PROXY"] = trustProxy ? "true" : "false",
        });

        // Redirects off so the Set-Cookie on the setup response is readable rather than
        // being swallowed by the redirect to /.
        var client = factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = false,
        });
        client.DefaultRequestHeaders.Add("X-Forwarded-Proto", "https");

        var token = FormHelpers.AntiforgeryToken(await client.GetStringAsync("/setup"));

        var response = await client.PostAsync("/setup", new FormUrlEncodedContent(
            new Dictionary<string, string>
            {
                ["_handler"] = "setup",
                ["__RequestVerificationToken"] = token,
                ["Input.Username"] = "root",
                ["Input.Email"] = "root@example.com",
                ["Input.Password"] = "Sup3r!secret",
            }));

        var authCookie = response.Headers
            .GetValues("Set-Cookie")
            .FirstOrDefault(c => c.StartsWith(".AspNetCore.Identity.Application", StringComparison.Ordinal));

        Assert.NotNull(authCookie); // setup must have signed the admin in
        return authCookie.Contains("secure", StringComparison.OrdinalIgnoreCase);
    }
}
