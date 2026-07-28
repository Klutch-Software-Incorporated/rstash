using Microsoft.AspNetCore.Identity;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;
using Rstash.Services;
using Rstash.Services.Entitlements;
using Rstash.Web;

namespace Rstash.IntegrationTests;

/// <summary>
/// Account creation has to produce a <see cref="StorageUser"/> row, not just an
/// Identity one: the storage API resolves owners against <c>storage_users</c>, so a
/// missing row means the account cannot serve a single document.
/// </summary>
public sealed class UserProvisioningTests(RstashAppFactory factory) : IClassFixture<RstashAppFactory>
{
    [Fact]
    public async Task NewUser_InheritsDefaultQuotas_AndLaterChangesDoNotAffectExisting()
    {
        var settings = factory.Services.GetRequiredService<SettingsService>();
        await settings.SetAsync("default_user_storage_limit", "2GB");
        await settings.SetAsync("default_user_egress_limit", "5GB");
        try
        {
            var user = await CreateUserAsync("defaultsuser", settings);
            var entitlements = factory.Services.GetRequiredService<IEntitlementSource>();

            var limits = await entitlements.ResolveAsync(user.Id);
            Assert.Equal(2L << 30, limits.MaxStorage);
            Assert.Equal(5L << 30, limits.MaxEgress);

            // Raising the default afterwards must not retroactively change the existing
            // user: defaults are captured at creation, not resolved on read.
            await settings.SetAsync("default_user_storage_limit", "10GB");
            Assert.Equal(2L << 30, (await entitlements.ResolveAsync(user.Id)).MaxStorage);
        }
        finally
        {
            await settings.DeleteAsync("default_user_storage_limit");
            await settings.DeleteAsync("default_user_egress_limit");
        }
    }

    [Fact]
    public async Task NewUser_WithNoDefaults_StaysUnlimited()
    {
        var settings = factory.Services.GetRequiredService<SettingsService>();
        var user = await CreateUserAsync("nodefaultsuser", settings);

        var limits = await factory.Services.GetRequiredService<IEntitlementSource>()
            .ResolveAsync(user.Id);

        Assert.Equal(0, limits.MaxStorage);
        Assert.Equal(0, limits.MaxEgress);
    }

    [Fact]
    public async Task Provisioning_WritesTheStorageRecord_KeyedOnSubjectAndUsername()
    {
        var settings = factory.Services.GetRequiredService<SettingsService>();
        var user = await CreateUserAsync("recorduser", settings);

        var contextFactory = factory.Services.GetRequiredService<IDbContextFactory<RstashDbContext>>();
        await using var db = await contextFactory.CreateDbContextAsync();
        var row = await db.StorageUsers.SingleAsync(s => s.Id == user.Id);

        Assert.Equal(user.Id.ToString(System.Globalization.CultureInfo.InvariantCulture), row.Subject);
        Assert.Equal("recorduser", row.UserName);

        // The storage path looks users up case-insensitively, so the normalized column
        // has to be populated or /storage/{user}/… 404s.
        Assert.Equal("RECORDUSER", row.NormalizedUserName);
        Assert.False(row.Disabled);
    }

    [Fact]
    public async Task Provisioning_IsIdempotent()
    {
        var settings = factory.Services.GetRequiredService<SettingsService>();
        var user = await CreateUserAsync("idempotentuser", settings);

        var contextFactory = factory.Services.GetRequiredService<IDbContextFactory<RstashDbContext>>();
        await UserProvisioning.ProvisionStorageUserAsync(contextFactory, user, settings.Current);

        await using var db = await contextFactory.CreateDbContextAsync();
        Assert.Equal(1, await db.StorageUsers.CountAsync(s => s.Id == user.Id));
    }

    private async Task<ApplicationUser> CreateUserAsync(string name, SettingsService settings)
    {
        using var scope = factory.Services.CreateScope();
        var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();

        var user = new ApplicationUser
        {
            UserName = name, CreatedAt = DateTimeOffset.UtcNow, Approved = true,
        };
        Assert.True((await users.CreateAsync(user, "Sup3r!secret")).Succeeded);

        await UserProvisioning.ProvisionStorageUserAsync(
            factory.Services.GetRequiredService<IDbContextFactory<RstashDbContext>>(),
            user,
            settings.Current);

        return user;
    }
}
