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

    /// <summary>
    /// Provisioning refuses to hand one person's storage to another, and the refusal
    /// stops the login rather than being logged and stepped over.
    /// </summary>
    /// <remarks>
    /// <para>
    /// This is the branch of provisioning that embedded mode can actually reach. Its
    /// creation branch cannot be: registration writes the storage record, and a login
    /// with no record is refused before provisioning ever runs, because entitlements
    /// resolve a missing row to disabled. So asserting "a record exists after login"
    /// proves nothing about provisioning at all — which is exactly what an earlier
    /// version of this test did.
    /// </para>
    /// <para>
    /// Repointing the record at a foreign subject reproduces what an external provider
    /// re-issuing a taken username would look like from here: provisioning finds no
    /// record for the subject, then finds the username already spoken for.
    /// </para>
    /// </remarks>
    [Fact]
    public async Task JitProvisioning_RefusesTheLogin_WhenTheUsernameBelongsToAnotherSubject()
    {
        using var factory = new RstashAppFactory(new Dictionary<string, string>());
        await CompleteSetupAsync(Following(factory), "claimsuser");

        var contextFactory = factory.Services.GetRequiredService<IDbContextFactory<RstashDbContext>>();
        await using (var seeded = await contextFactory.CreateDbContextAsync())
        {
            var row = await seeded.StorageUsers.SingleAsync(s => s.NormalizedUserName == "CLAIMSUSER");
            row.Subject = "a-different-persons-subject";
            await seeded.SaveChangesAsync();
        }

        // A fresh client, so this is a real login through the password form rather than
        // a session left over from setup.
        var client = Following(factory);
        var landed = await FormHelpers.SignInAsync(client, "claimsuser", "Sup3r!secret", "/admin/users");
        var body = await landed.Content.ReadAsStringAsync();

        Assert.NotEqual(HttpStatusCode.OK, landed.StatusCode);
        Assert.DoesNotContain("claimsuser@", body);

        // The rule itself has to be what refuses, named in the failure. Asserting only
        // that the login failed passes with the rule deleted, because the unique index
        // rejects the duplicate a moment later anyway — verified by removing the rule
        // and watching a weaker version of this test stay green.
        Assert.Contains("already belongs to a different account", body);

        // And the record still belongs to whoever it belonged to before.
        await using var db = await contextFactory.CreateDbContextAsync();
        Assert.Equal(
            "a-different-persons-subject",
            (await db.StorageUsers.SingleAsync(s => s.NormalizedUserName == "CLAIMSUSER")).Subject);
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

    private static HttpClient Following(RstashAppFactory factory) =>
        factory.CreateClient(new WebApplicationFactoryClientOptions
        {
            AllowAutoRedirect = true,
            MaxAutomaticRedirections = 12,
        });

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
