using System.Net;
using System.Net.Http.Headers;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using Microsoft.AspNetCore.Mvc.Testing;

namespace Rstash.IntegrationTests;

/// <summary>
/// The go/no-go on the bundled OpenID Connect provider: can rstash complete a real
/// authorization-code + PKCE round-trip against itself? Drives the flow by hand
/// rather than through the OIDC handler, so each leg fails in isolation and says why.
/// </summary>
public sealed class EmbeddedOidcTests
{
    [Fact]
    public async Task DiscoveryDocument_AdvertisesTheConnectEndpoints()
    {
        using var factory = new RstashAppFactory(new Dictionary<string, string>());
        var client = factory.CreateClient();

        var response = await client.GetAsync("/.well-known/openid-configuration");
        Assert.Equal(HttpStatusCode.OK, response.StatusCode);

        using var doc = JsonDocument.Parse(await response.Content.ReadAsStringAsync());
        var root = doc.RootElement;

        // OpenIddict normalises the issuer to a trailing slash; RSTASH_BASE_URL is
        // stored without one. Relying parties must compare the normalised form.
        Assert.Equal("http://localhost:8080/", root.GetProperty("issuer").GetString());
        Assert.EndsWith("/connect/authorize", root.GetProperty("authorization_endpoint").GetString());
        Assert.EndsWith("/connect/token", root.GetProperty("token_endpoint").GetString());

        // PKCE is required, not merely offered.
        var methods = root.GetProperty("code_challenge_methods_supported")
            .EnumerateArray().Select(e => e.GetString()).ToList();
        Assert.Contains("S256", methods);
    }

    [Fact]
    public async Task Authorize_WithNoSession_ChallengesToTheLoginPage()
    {
        using var factory = new RstashAppFactory(new Dictionary<string, string>());

        // Setup has to run first, or the first-run guard sends everything to /setup and
        // we would be asserting on that instead of on the provider's own challenge.
        await CompleteSetupAsync(factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = false,
        }));

        // A second client starts with an empty cookie container, i.e. no session.
        var anonymous = factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = false,
        });

        var response = await anonymous.GetAsync(AuthorizeUrl(Challenge(NewVerifier())));

        // The provider owns no password form; it defers to the Identity cookie scheme,
        // which lands the user on the existing /login page.
        Assert.Equal(HttpStatusCode.Redirect, response.StatusCode);
        Assert.Contains("/login", response.Headers.Location!.OriginalString);
    }

    [Fact]
    public async Task SignedInUser_CompletesTheCodeExchange_AndGetsAnIdToken()
    {
        using var factory = new RstashAppFactory(new Dictionary<string, string>());
        var client = factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = false,
        });

        await CompleteSetupAsync(client);

        var verifier = NewVerifier();
        var authorize = await client.GetAsync(AuthorizeUrl(Challenge(verifier)));

        Assert.Equal(HttpStatusCode.Redirect, authorize.StatusCode);
        var location = authorize.Headers.Location!;
        Assert.StartsWith("http://localhost:8080/signin-oidc", location.OriginalString);

        var code = System.Web.HttpUtility.ParseQueryString(location.Query)["code"];
        Assert.False(string.IsNullOrEmpty(code));

        // The back-channel leg. In production this is a real loopback HTTP call from
        // the server to its own token endpoint; here it goes through the test handler.
        var token = await client.PostAsync("/connect/token", new FormUrlEncodedContent(
            new Dictionary<string, string>
            {
                ["grant_type"] = "authorization_code",
                ["code"] = code!,
                ["client_id"] = "rstash-web",
                ["redirect_uri"] = "http://localhost:8080/signin-oidc",
                ["code_verifier"] = verifier,
            }));

        var tokenBody = await token.Content.ReadAsStringAsync();
        Assert.True(
            token.StatusCode == HttpStatusCode.OK && tokenBody.Length > 0,
            $"token endpoint returned {(int)token.StatusCode} "
            + $"content-type={token.Content.Headers.ContentType} "
            + $"len={tokenBody.Length} "
            + $"headers=[{string.Join("; ", token.Headers.Select(h => h.Key + "=" + string.Join(",", h.Value)))}] "
            + $"body='{tokenBody}'");

        using var payload = JsonDocument.Parse(tokenBody);
        var idToken = payload.RootElement.GetProperty("id_token").GetString();
        Assert.False(string.IsNullOrEmpty(idToken));

        // The claims that carry identity into the storage-side user record.
        var claims = DecodeJwtPayload(idToken!);
        Assert.Equal("root", claims.GetProperty("preferred_username").GetString());
        Assert.False(string.IsNullOrEmpty(claims.GetProperty("sub").GetString()));

        var accessToken = payload.RootElement.GetProperty("access_token").GetString();
        var userInfo = new HttpRequestMessage(HttpMethod.Get, "/connect/userinfo");
        userInfo.Headers.Authorization = new AuthenticationHeaderValue("Bearer", accessToken);

        var userInfoResponse = await client.SendAsync(userInfo);
        Assert.Equal(HttpStatusCode.OK, userInfoResponse.StatusCode);
    }

    private static async Task CompleteSetupAsync(HttpClient client)
    {
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

        // Setup signs the new admin in, which is the Identity cookie /connect/authorize
        // reads.
        Assert.Equal(HttpStatusCode.Redirect, response.StatusCode);
    }

    private static string AuthorizeUrl(string codeChallenge) =>
        "/connect/authorize"
        + "?client_id=rstash-web"
        + "&response_type=code"
        + "&redirect_uri=" + Uri.EscapeDataString("http://localhost:8080/signin-oidc")
        + "&scope=" + Uri.EscapeDataString("openid profile email")
        + "&code_challenge=" + codeChallenge
        + "&code_challenge_method=S256";

    private static string NewVerifier() =>
        Base64Url(RandomNumberGenerator.GetBytes(32));

    private static string Challenge(string verifier) =>
        Base64Url(SHA256.HashData(Encoding.ASCII.GetBytes(verifier)));

    private static string Base64Url(byte[] bytes) =>
        Convert.ToBase64String(bytes).TrimEnd('=').Replace('+', '-').Replace('/', '_');

    private static JsonElement DecodeJwtPayload(string jwt)
    {
        var segment = jwt.Split('.')[1];
        var padded = segment.Replace('-', '+').Replace('_', '/')
            .PadRight(segment.Length + ((4 - (segment.Length % 4)) % 4), '=');
        return JsonDocument.Parse(Convert.FromBase64String(padded)).RootElement.Clone();
    }
}
