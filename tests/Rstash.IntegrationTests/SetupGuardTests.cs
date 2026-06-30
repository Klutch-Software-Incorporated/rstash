using System.Net;
using Microsoft.AspNetCore.Mvc.Testing;

namespace Rstash.IntegrationTests;

/// <summary>Own factory (no users seeded) so the first-run guard is active.</summary>
public sealed class SetupGuardTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    private HttpClient CreateClient() =>
        factory.CreateClient(new WebApplicationFactoryClientOptions { AllowAutoRedirect = false });

    [Fact]
    public async Task NoUsers_RootRedirectsToSetup()
    {
        var response = await CreateClient().GetAsync("/");

        Assert.Equal(HttpStatusCode.Redirect, response.StatusCode);
        Assert.Equal("/setup", response.Headers.Location?.OriginalString);
    }

    [Fact]
    public async Task SetupPage_IsReachable()
    {
        var response = await CreateClient().GetAsync("/setup");

        response.EnsureSuccessStatusCode();
        Assert.Contains("admin account", await response.Content.ReadAsStringAsync());
    }

    [Fact]
    public async Task Healthz_IsExemptFromGuard()
    {
        var response = await CreateClient().GetAsync("/healthz");

        response.EnsureSuccessStatusCode();
    }
}
