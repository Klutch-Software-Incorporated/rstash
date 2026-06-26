using Microsoft.AspNetCore.Identity;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;
using Rstash.Services;

namespace Rstash.IntegrationTests;

public sealed class RegisterTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    [Fact]
    public async Task OpenMode_RegistersUser()
    {
        await SeedAdminAndSetModeAsync("open");
        var client = factory.CreateClient();

        var token = FormHelpers.AntiforgeryToken(await client.GetStringAsync("/register"));
        var response = await client.PostAsync("/register", new FormUrlEncodedContent(new Dictionary<string, string>
        {
            ["_handler"] = "register",
            ["__RequestVerificationToken"] = token,
            ["Input.Username"] = "bob",
            ["Input.Password"] = "Sup3r!secret",
        }));

        response.EnsureSuccessStatusCode();

        using var scope = factory.Services.CreateScope();
        var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();
        var bob = await users.FindByNameAsync("bob");
        Assert.NotNull(bob);
        Assert.True(bob.Approved);
    }

    [Fact]
    public async Task ClosedMode_ShowsClosedMessage()
    {
        await SeedAdminAndSetModeAsync("closed");
        var client = factory.CreateClient();

        var html = await client.GetStringAsync("/register");

        Assert.Contains("closed", html, StringComparison.OrdinalIgnoreCase);
    }

    private async Task SeedAdminAndSetModeAsync(string mode)
    {
        using var scope = factory.Services.CreateScope();
        var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();
        if (await users.FindByNameAsync("admin") is null)
        {
            await users.CreateAsync(
                new ApplicationUser { UserName = "admin", CreatedAt = DateTimeOffset.UtcNow, IsAdmin = true, Approved = true },
                "Admin!12345");
        }

        await scope.ServiceProvider.GetRequiredService<SettingsService>().SetAsync("registration_mode", mode);
    }
}
