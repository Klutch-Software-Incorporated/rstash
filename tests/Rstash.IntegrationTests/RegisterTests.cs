using Microsoft.AspNetCore.Identity;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;
using Rstash.Services;
using Rstash.Services.Entitlements;

namespace Rstash.IntegrationTests;

public sealed class RegisterTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    /// <summary>
    /// The storage record is the assertion that matters. <c>Approved</c> is written by
    /// Identity itself, so checking it proves the form posted and nothing more: drop the
    /// provisioning call from the page and a test that stops there stays green while
    /// every account it creates is locked out of storage for good.
    /// </summary>
    [Fact]
    public async Task OpenMode_RegistersUser_AndProvisionsStorage()
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

        var contextFactory = factory.Services.GetRequiredService<IDbContextFactory<RstashDbContext>>();
        await using var db = await contextFactory.CreateDbContextAsync();
        var storageUser = await db.StorageUsers.SingleOrDefaultAsync(s => s.Id == bob.Id);

        Assert.NotNull(storageUser);
        Assert.Equal("BOB", storageUser.NormalizedUserName);
        Assert.False(storageUser.Disabled);

        // And the account can actually be used: entitlements resolve a missing row to
        // disabled, so this is the check the storage path itself performs.
        var limits = await factory.Services.GetRequiredService<IEntitlementSource>()
            .ResolveAsync(bob.Id);
        Assert.False(limits.Disabled);
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
