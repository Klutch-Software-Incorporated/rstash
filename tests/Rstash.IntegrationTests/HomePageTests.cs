using Microsoft.AspNetCore.Identity;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;

namespace Rstash.IntegrationTests;

public sealed class HomePageTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    [Fact]
    public async Task Home_RendersWelcomeFromSettings()
    {
        await SeedAdminAsync();

        var client = factory.CreateClient();
        var html = await client.GetStringAsync("/");

        // Server-prerendered Blazor content, with the site name/subtitle pulled
        // from the settings snapshot.
        Assert.Contains("Welcome to rstash", html);
        Assert.Contains("A personal remoteStorage server.", html);
    }

    private async Task SeedAdminAsync()
    {
        using var scope = factory.Services.CreateScope();
        var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();
        if (await users.FindByNameAsync("admin") is not null)
        {
            return;
        }

        var result = await users.CreateAsync(
            new ApplicationUser
            {
                UserName = "admin",
                CreatedAt = DateTimeOffset.UtcNow,
                IsAdmin = true,
                Approved = true,
            },
            "Admin!12345");

        Assert.True(result.Succeeded);
    }
}
