using Microsoft.AspNetCore.Identity;
using Microsoft.Extensions.DependencyInjection;
using Rstash.Database;
using Rstash.Services;
using Rstash.Web;

namespace Rstash.IntegrationTests;

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
            using var scope = factory.Services.CreateScope();
            var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();

            var user = new ApplicationUser
            {
                UserName = "defaultsuser", CreatedAt = DateTimeOffset.UtcNow, Approved = true,
            };
            Assert.True((await users.CreateAsync(user, "Sup3r!secret")).Succeeded);
            await UserProvisioning.ApplyDefaultLimitsAsync(users, user, settings.Current);

            var created = await users.FindByNameAsync("defaultsuser");
            Assert.Equal(2L << 30, created!.StorageQuota);
            Assert.Equal(5L << 30, created.EgressQuota);

            // Raising the default afterwards must not retroactively change the existing user.
            await settings.SetAsync("default_user_storage_limit", "10GB");
            var reloaded = await users.FindByNameAsync("defaultsuser");
            Assert.Equal(2L << 30, reloaded!.StorageQuota);
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
        using var scope = factory.Services.CreateScope();
        var users = scope.ServiceProvider.GetRequiredService<UserManager<ApplicationUser>>();

        var user = new ApplicationUser
        {
            UserName = "nodefaultsuser", CreatedAt = DateTimeOffset.UtcNow, Approved = true,
        };
        Assert.True((await users.CreateAsync(user, "Sup3r!secret")).Succeeded);
        await UserProvisioning.ApplyDefaultLimitsAsync(users, user, settings.Current);

        var created = await users.FindByNameAsync("nodefaultsuser");
        Assert.Equal(0, created!.StorageQuota);
        Assert.Equal(0, created.EgressQuota);
    }
}
