using Microsoft.Extensions.DependencyInjection;
using Rstash.Services;
using Rstash.Services.Storage;

namespace Rstash.IntegrationTests;

public sealed class HostBootTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    [Fact]
    public async Task Healthz_ReturnsHealthy()
    {
        var client = factory.CreateClient();

        var response = await client.GetAsync("/healthz");

        response.EnsureSuccessStatusCode();
        Assert.Equal("Healthy", await response.Content.ReadAsStringAsync());
    }

    [Fact]
    public void ServiceGraph_Resolves()
    {
        using var scope = factory.Services.CreateScope();

        Assert.NotNull(scope.ServiceProvider.GetRequiredService<RemoteStorageService>());
        Assert.NotNull(scope.ServiceProvider.GetRequiredService<SettingsService>());
    }

    [Fact]
    public void Settings_LoadedWithDefaults()
    {
        using var scope = factory.Services.CreateScope();
        var settings = scope.ServiceProvider.GetRequiredService<SettingsService>();

        // Loaded at startup; defaults apply with an empty database.
        Assert.Equal("closed", settings.Current.RegistrationMode);
    }
}
