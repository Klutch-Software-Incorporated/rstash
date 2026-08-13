using System.Buffers.Text;
using System.Net;
using System.Net.Http.Headers;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using Microsoft.AspNetCore.Identity;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;
using Rstash.Services;
using Rstash.Web;

namespace Rstash.IntegrationTests;

public sealed class OAuthTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    [Fact]
    public async Task TokenEndpoint_ExchangesCodeWithPkce_AndIssuesWorkingToken()
    {
        const string verifier = "this-is-a-test-code-verifier-0123456789";
        const string redirectUri = "https://app.example.com/callback";
        var challenge = Pkce(verifier);

        var code = await SeedCodeAsync("oauthuser", "https://app.example.com", redirectUri, "*:rw", challenge);
        var client = factory.CreateClient();

        var response = await client.PostAsync("/oauth/token", TokenForm(code, verifier, redirectUri));
        Assert.Equal(HttpStatusCode.OK, response.StatusCode);
        Assert.Equal("no-store", response.Headers.CacheControl?.ToString());

        var json = JsonDocument.Parse(await response.Content.ReadAsStringAsync()).RootElement;
        var accessToken = json.GetProperty("access_token").GetString();
        Assert.False(string.IsNullOrEmpty(accessToken));
        Assert.Equal("bearer", json.GetProperty("token_type").GetString());
        Assert.Equal("*:rw", json.GetProperty("scope").GetString());

        // One-time: a second exchange of the same code fails.
        var second = await client.PostAsync("/oauth/token", TokenForm(code, verifier, redirectUri));
        Assert.Equal(HttpStatusCode.BadRequest, second.StatusCode);

        // The issued token works against the storage API.
        var put = new HttpRequestMessage(HttpMethod.Put, "/storage/oauthuser/docs/a.txt")
        {
            Content = new StringContent("hi"),
        };
        put.Headers.Authorization = new AuthenticationHeaderValue("Bearer", accessToken);
        Assert.Equal(HttpStatusCode.Created, (await client.SendAsync(put)).StatusCode);
    }

    [Fact]
    public async Task RefreshGrant_RotatesTokens_AndKeepsScopesWorking()
    {
        const string verifier = "refresh-flow-code-verifier-0123456789";
        const string redirectUri = "https://refresh.example.com/callback";

        var code = await SeedCodeAsync(
            "refreshuser", "https://refresh.example.com", redirectUri, "*:rw", Pkce(verifier));
        var client = factory.CreateClient();

        var first = JsonDocument.Parse(
            await (await client.PostAsync("/oauth/token", TokenForm(code, verifier, redirectUri)))
                .Content.ReadAsStringAsync()).RootElement;

        var oldAccess = first.GetProperty("access_token").GetString()!;
        var oldRefresh = first.GetProperty("refresh_token").GetString()!;
        Assert.False(string.IsNullOrEmpty(oldRefresh));

        var refreshed = await client.PostAsync(
            "/oauth/token", RefreshForm(oldRefresh, "https://refresh.example.com"));
        Assert.Equal(HttpStatusCode.OK, refreshed.StatusCode);

        var second = JsonDocument.Parse(await refreshed.Content.ReadAsStringAsync()).RootElement;
        var newAccess = second.GetProperty("access_token").GetString()!;
        var newRefresh = second.GetProperty("refresh_token").GetString()!;

        // Both secrets rotate, and the original scopes carry across.
        Assert.NotEqual(oldAccess, newAccess);
        Assert.NotEqual(oldRefresh, newRefresh);
        Assert.Equal("*:rw", second.GetProperty("scope").GetString());

        // The new access token works.
        var put = new HttpRequestMessage(HttpMethod.Put, "/storage/refreshuser/docs/b.txt")
        {
            Content = new StringContent("hi"),
        };
        put.Headers.Authorization = new AuthenticationHeaderValue("Bearer", newAccess);
        Assert.Equal(HttpStatusCode.Created, (await client.SendAsync(put)).StatusCode);

        // The superseded access token does not.
        var stale = new HttpRequestMessage(HttpMethod.Get, "/storage/refreshuser/docs/b.txt");
        stale.Headers.Authorization = new AuthenticationHeaderValue("Bearer", oldAccess);
        Assert.Equal(HttpStatusCode.Unauthorized, (await client.SendAsync(stale)).StatusCode);

        // And the spent refresh token cannot be replayed.
        var replay = await client.PostAsync(
            "/oauth/token", RefreshForm(oldRefresh, "https://refresh.example.com"));
        Assert.Equal(HttpStatusCode.BadRequest, replay.StatusCode);
    }

    [Fact]
    public async Task RefreshGrant_UnknownToken_Rejected()
    {
        var response = await factory.CreateClient().PostAsync(
            "/oauth/token", RefreshForm("deadbeef", "https://whoever.example.com"));

        Assert.Equal(HttpStatusCode.BadRequest, response.StatusCode);
    }

    /// <summary>
    /// A refresh token belongs to the app it was issued to. These are public
    /// clients with no secret, so if the grant doesn't check client_id, any app
    /// that gets hold of the token inherits the other app's scopes.
    /// </summary>
    [Fact]
    public async Task RefreshGrant_FromAnotherClient_Rejected()
    {
        const string verifier = "wrong-client-code-verifier-0123456789";
        const string redirectUri = "https://owner.example.com/callback";

        var code = await SeedCodeAsync(
            "wrongclientuser", "https://owner.example.com", redirectUri, "*:rw", Pkce(verifier));
        var client = factory.CreateClient();

        var issued = JsonDocument.Parse(
            await (await client.PostAsync("/oauth/token", TokenForm(code, verifier, redirectUri)))
                .Content.ReadAsStringAsync()).RootElement;
        var refresh = issued.GetProperty("refresh_token").GetString()!;

        var stolen = await client.PostAsync(
            "/oauth/token", RefreshForm(refresh, "https://attacker.example.com"));
        Assert.Equal(HttpStatusCode.BadRequest, stolen.StatusCode);

        // Refused, not consumed — the rightful owner can still redeem it.
        var rightful = await client.PostAsync(
            "/oauth/token", RefreshForm(refresh, "https://owner.example.com"));
        Assert.Equal(HttpStatusCode.OK, rightful.StatusCode);
    }

    [Fact]
    public async Task RefreshGrant_WithoutClientId_Rejected()
    {
        var response = await factory.CreateClient().PostAsync("/oauth/token", new FormUrlEncodedContent(
            new Dictionary<string, string>
            {
                ["grant_type"] = "refresh_token",
                ["refresh_token"] = "deadbeef",
            }));

        Assert.Equal(HttpStatusCode.BadRequest, response.StatusCode);
        Assert.Contains(
            "client_id", await response.Content.ReadAsStringAsync(), StringComparison.Ordinal);
    }

    [Fact]
    public async Task TokenEndpoint_BadVerifier_Rejected()
    {
        const string redirectUri = "https://app2.example.com/cb";
        var code = await SeedCodeAsync("oauthuser2", "https://app2.example.com", redirectUri, "*:rw", Pkce("correct-verifier-1234567890"));

        var response = await factory.CreateClient().PostAsync("/oauth/token", TokenForm(code, "WRONG-VERIFIER", redirectUri));

        Assert.Equal(HttpStatusCode.BadRequest, response.StatusCode);
    }

    [Fact]
    public async Task Authorize_RequiresLogin()
    {
        var client = factory.CreateClient(new WebApplicationFactoryClientOptions { AllowAutoRedirect = false });

        var response = await client.GetAsync(
            "/oauth/authorize?response_type=token&redirect_uri=https%3A%2F%2Fapp.example.com%2Fcb&scope=*%3Arw");

        Assert.Equal(HttpStatusCode.Redirect, response.StatusCode);
        Assert.Contains("/login?redirect=", response.Headers.Location?.OriginalString ?? string.Empty);
    }

    [Fact]
    public async Task AuthorizeDecision_PostResolvesToSingleEndpoint()
    {
        // Regression: the consent POST must not share a path with the GET consent
        // page (Blazor @page "/oauth/authorize"), or POST is an AmbiguousMatch (500).
        // A bare POST trips antiforgery -> 400, proving the route resolves uniquely.
        var response = await factory.CreateClient().PostAsync(
            "/oauth/authorize/decision", new FormUrlEncodedContent(new Dictionary<string, string>()));

        Assert.Equal(HttpStatusCode.BadRequest, response.StatusCode);
    }

    [Fact]
    public async Task Revoke_InvalidatesToken()
    {
        var token = await SeedTokenAsync("revokeuser", "*:rw");
        var client = factory.CreateClient();

        Assert.Equal(HttpStatusCode.OK, (await GetStorage(client, "/storage/revokeuser/", token)).StatusCode);

        var revoke = await client.PostAsync("/oauth/revoke",
            new FormUrlEncodedContent(new Dictionary<string, string> { ["token"] = token }));
        Assert.Equal(HttpStatusCode.OK, revoke.StatusCode);

        Assert.Equal(HttpStatusCode.Unauthorized,
            (await GetStorage(client, "/storage/revokeuser/docs/x.txt", token)).StatusCode);
    }

    private static string Pkce(string verifier) =>
        Base64Url.EncodeToString(SHA256.HashData(Encoding.ASCII.GetBytes(verifier)));

    private static FormUrlEncodedContent TokenForm(string code, string verifier, string redirectUri) =>
        new(new Dictionary<string, string>
        {
            ["grant_type"] = "authorization_code",
            ["code"] = code,
            ["code_verifier"] = verifier,
            ["redirect_uri"] = redirectUri,
        });

    private static FormUrlEncodedContent RefreshForm(string refreshToken, string clientId) =>
        new(new Dictionary<string, string>
        {
            ["grant_type"] = "refresh_token",
            ["refresh_token"] = refreshToken,
            ["client_id"] = clientId,
        });

    private static async Task<HttpResponseMessage> GetStorage(HttpClient client, string url, string token)
    {
        var request = new HttpRequestMessage(HttpMethod.Get, url);
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        return await client.SendAsync(request);
    }

    private async Task<string> SeedCodeAsync(
        string username, string clientId, string redirectUri, string scopes, string challenge)
    {
        using var scope = factory.Services.CreateScope();
        var userId = await EnsureUserAsync(scope.ServiceProvider, username);
        var tokens = scope.ServiceProvider.GetRequiredService<TokenStore>();
        var code = await tokens.CreateCodeAsync(userId, clientId, redirectUri, scopes, challenge, "S256");
        return code.Code;
    }

    private async Task<string> SeedTokenAsync(string username, string scopes)
    {
        using var scope = factory.Services.CreateScope();
        var userId = await EnsureUserAsync(scope.ServiceProvider, username);
        var tokens = scope.ServiceProvider.GetRequiredService<TokenStore>();
        return (await tokens.CreateAsync(userId, "client", scopes, lifetime: null, withRefreshToken: false)).Token;
    }

    private static async Task<long> EnsureUserAsync(IServiceProvider services, string username)
    {
        var users = services.GetRequiredService<UserManager<ApplicationUser>>();
        var user = await users.FindByNameAsync(username);
        if (user is null)
        {
            user = new ApplicationUser { UserName = username, CreatedAt = DateTimeOffset.UtcNow, Approved = true };
            var created = await users.CreateAsync(user, "Sup3r!secret");
            Assert.True(created.Succeeded);
        }

        return user.Id;
    }
}
