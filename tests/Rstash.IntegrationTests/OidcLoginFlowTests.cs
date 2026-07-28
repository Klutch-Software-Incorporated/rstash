using System.Net;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;

namespace Rstash.IntegrationTests;

/// <summary>
/// The whole loop, end to end: an anonymous request challenges the provider, the
/// password form establishes the provider session, the code exchange establishes the
/// relying-party session, and JIT provisioning writes the storage record from claims.
/// </summary>
public sealed class OidcLoginFlowTests
{
    [Fact]
    public async Task Login_CompletesTheRoundTrip_AndEstablishesBothSessions()
    {
        using var factory = new RstashAppFactory(new Dictionary<string, string>());
        var client = factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = true,
        });

        // Setup creates the first admin and signs them in on the *provider* side.
        var token = FormHelpers.AntiforgeryToken(await client.GetStringAsync("/setup"));
        var setup = await client.PostAsync("/setup", new FormUrlEncodedContent(
            new Dictionary<string, string>
            {
                ["_handler"] = "setup",
                ["__RequestVerificationToken"] = token,
                ["Input.Username"] = "root",
                ["Input.Email"] = "root@example.com",
                ["Input.Password"] = "Sup3r!secret",
            }));
        Assert.Equal(HttpStatusCode.OK, setup.StatusCode);

        // Now a protected page. With a provider session already in hand, the challenge
        // round-trips without a second password prompt and returns the page itself —
        // this is the leg that would loop forever if both sessions shared one cookie.
        var page = await client.GetAsync("/admin/users");

        Assert.Equal(HttpStatusCode.OK, page.StatusCode);
        Assert.DoesNotContain("/login", page.RequestMessage!.RequestUri!.PathAndQuery);
    }

    [Fact]
    public async Task JitProvisioning_WritesTheStorageRecordFromClaims()
    {
        using var factory = new RstashAppFactory(new Dictionary<string, string>());
        var client = factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = true,
        });

        await CompleteSetupAsync(client, "claimsuser");
        await client.GetAsync("/admin/users"); // drives the OIDC round-trip

        var contextFactory = factory.Services.GetRequiredService<IDbContextFactory<RstashDbContext>>();
        await using var db = await contextFactory.CreateDbContextAsync();

        var row = await db.StorageUsers.SingleAsync(s => s.NormalizedUserName == "CLAIMSUSER");
        Assert.Equal("claimsuser", row.UserName);
        Assert.False(string.IsNullOrEmpty(row.Subject));
    }

    [Fact]
    public async Task SigningKey_IsTheSameMaterial_AcrossRestarts()
    {
        // Two hosts over the same database stand in for a restart — and, more to the
        // point, for two instances behind a load balancer, where a token minted by one
        // has to validate on the other.
        using var factory = new RstashAppFactory(new Dictionary<string, string>());
        var first = await SigningModulusAsync(factory.CreateClient());

        using var restarted = factory.WithWebHostBuilder(_ => { });
        var second = await SigningModulusAsync(restarted.CreateClient());

        Assert.False(string.IsNullOrEmpty(first));

        // The modulus, not the key id: kid is a constant here, so comparing it would
        // pass just as happily against freshly generated ephemeral keys.
        Assert.Equal(first, second);
    }

    private static async Task<string?> SigningModulusAsync(HttpClient client)
    {
        using var jwks = System.Text.Json.JsonDocument.Parse(
            await client.GetStringAsync("/.well-known/jwks"));

        return jwks.RootElement.GetProperty("keys")[0].GetProperty("n").GetString();
    }

    private static async Task CompleteSetupAsync(HttpClient client, string username)
    {
        var token = FormHelpers.AntiforgeryToken(await client.GetStringAsync("/setup"));
        await client.PostAsync("/setup", new FormUrlEncodedContent(
            new Dictionary<string, string>
            {
                ["_handler"] = "setup",
                ["__RequestVerificationToken"] = token,
                ["Input.Username"] = username,
                ["Input.Email"] = $"{username}@example.com",
                ["Input.Password"] = "Sup3r!secret",
            }));
    }
}
