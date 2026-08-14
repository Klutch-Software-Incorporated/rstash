using Microsoft.AspNetCore.Identity;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;

namespace Rstash.IntegrationTests;

/// <summary>
/// The dashboard's storage address is the one string rstash asks a user to copy into
/// somebody else's software, and an app resolves WebFinger against exactly the authority
/// it is handed. So it has to carry the port, and it has to come from the configured
/// origin rather than from whatever the request happened to arrive on.
/// </summary>
public sealed class StorageAddressTests
{
    private const string Password = "Sup3r!secret";

    [Fact]
    public async Task Dashboard_ShowsTheAddressWithANonDefaultPort()
    {
        await using var factory = Factory("http://localhost:8080");

        var page = await SignedInDashboardAsync(factory);

        Assert.Contains("alice@localhost:8080", page, StringComparison.Ordinal);
    }

    [Fact]
    public async Task Dashboard_OmitsThePortWhenItIsTheSchemeDefault()
    {
        await using var factory = Factory("https://rstash.example.org");

        var page = await SignedInDashboardAsync(factory);

        Assert.Contains("alice@rstash.example.org", page, StringComparison.Ordinal);
        Assert.DoesNotContain("rstash.example.org:443", page, StringComparison.Ordinal);
    }

    [Fact]
    public async Task Dashboard_PrefersTheConfiguredOriginOverTheRequest()
    {
        // The request reaches the app on the test host; the deployment is public
        // somewhere else. A proxied server must advertise where users actually are.
        await using var factory = Factory("https://rstash.example.org:8443");

        var page = await SignedInDashboardAsync(factory);

        Assert.Contains("alice@rstash.example.org:8443", page, StringComparison.Ordinal);
        Assert.DoesNotContain("alice@localhost", page, StringComparison.Ordinal);
    }

    [Fact]
    public async Task Dashboard_HidesTheListenPortWhenProxiedOntoPort80()
    {
        // The common deployment: rstash listening on 8080, a reverse proxy serving the
        // public site on 80. The address is where users are, so neither the listen port
        // nor the proxy's own port belongs in it.
        await using var factory = Factory("http://rstash.example.org");

        var page = await SignedInDashboardAsync(factory);

        Assert.Contains("alice@rstash.example.org", page, StringComparison.Ordinal);
        Assert.DoesNotContain("rstash.example.org:80", page, StringComparison.Ordinal);
        Assert.DoesNotContain(":8080", page, StringComparison.Ordinal);
    }

    [Fact]
    public async Task TopBar_ShowsTheSameAddressOnEveryPage()
    {
        // The account pill builds the handle independently of the dashboard, and had
        // the same defect. Checked away from "/" so this cannot pass on the hero's work.
        await using var factory = Factory("http://localhost:8080");

        var client = await SignedInClientAsync(factory);
        var account = await client.GetAsync("/account");
        account.EnsureSuccessStatusCode();

        Assert.Contains("alice@localhost:8080", await account.Content.ReadAsStringAsync(), StringComparison.Ordinal);
    }

    private static RstashAppFactory Factory(string baseUrl) =>
        new(new Dictionary<string, string> { ["RSTASH_BASE_URL"] = baseUrl });

    private static async Task<string> SignedInDashboardAsync(RstashAppFactory factory)
    {
        var client = await SignedInClientAsync(factory);
        var home = await client.GetAsync("/");
        home.EnsureSuccessStatusCode();
        return await home.Content.ReadAsStringAsync();
    }

    private static async Task<HttpClient> SignedInClientAsync(RstashAppFactory factory)
    {
        using var scope = factory.Services.CreateScope();
        var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();
        var created = await users.CreateAsync(
            new ApplicationUser { UserName = "alice", CreatedAt = DateTimeOffset.UtcNow, Approved = true },
            Password);
        Assert.True(created.Succeeded);

        var client = factory.CreateClient();
        var signIn = await FormHelpers.SignInAsync(client, "alice", Password);
        signIn.EnsureSuccessStatusCode();
        return client;
    }
}
