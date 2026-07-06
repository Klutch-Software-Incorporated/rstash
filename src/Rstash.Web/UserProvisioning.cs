using Microsoft.AspNetCore.Identity;
using Rstash.Database;
using Rstash.Services;

namespace Rstash.Web;

/// <summary>New-account provisioning shared by the setup and registration flows.</summary>
public static class UserProvisioning
{
    /// <summary>
    /// Stamps the current server default quotas onto a freshly-created user. Only
    /// non-zero defaults are applied (0 leaves that dimension unlimited). The
    /// defaults are captured at creation time — changing <c>default_user_storage_limit</c>
    /// or <c>default_user_egress_limit</c> later does not retroactively affect
    /// existing accounts.
    /// </summary>
    public static async Task ApplyDefaultLimitsAsync(
        UserManager<ApplicationUser> users, ApplicationUser user, SettingsSnapshot settings)
    {
        var storage = settings.DefaultUserStorageLimit;
        var egress = settings.DefaultUserEgressLimit;
        if (storage <= 0 && egress <= 0)
        {
            return;
        }

        if (storage > 0)
        {
            user.StorageQuota = storage;
        }

        if (egress > 0)
        {
            user.EgressQuota = egress;
        }

        await users.UpdateAsync(user);
    }
}
