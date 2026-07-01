using System.Text.Json;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Services;
using Rstash.Services.Storage;

namespace Rstash.IntegrationTests;

public sealed class HostBootTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    [Fact]
    public async Task Healthz_ReturnsHealthy_WithPerDependencyBreakdown()
    {
        var client = factory.CreateClient();

        var response = await client.GetAsync("/healthz");

        response.EnsureSuccessStatusCode();

        using var doc = JsonDocument.Parse(await response.Content.ReadAsStringAsync());
        Assert.Equal("Healthy", doc.RootElement.GetProperty("status").GetString());

        // The database and storage connectivity checks are both reported by name.
        var checks = doc.RootElement.GetProperty("checks").EnumerateArray()
            .ToDictionary(c => c.GetProperty("name").GetString()!, c => c.GetProperty("status").GetString());
        Assert.Equal("Healthy", checks["database"]);
        Assert.Equal("Healthy", checks["storage"]);
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
