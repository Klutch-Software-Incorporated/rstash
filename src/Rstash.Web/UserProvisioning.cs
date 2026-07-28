using Microsoft.EntityFrameworkCore;
using Rstash.Database;
using Rstash.Services;

namespace Rstash.Web;

/// <summary>New-account provisioning shared by the setup and registration flows.</summary>
public static class UserProvisioning
{
    /// <summary>
    /// Creates the account's <see cref="StorageUser"/> record and stamps the current
    /// server default limits onto it.
    /// </summary>
    /// <remarks>
    /// <para>
    /// Without this row the account cannot serve a single document: the storage API
    /// resolves owners against <c>storage_users</c>, and the protocol addresses them
    /// by the username held there.
    /// </para>
    /// <para>
    /// Provisioning happens here, from claims-equivalent data, rather than as a
    /// foreign key off the Identity row — the same shape an external provider will use
    /// when it provisions on first login. Only non-zero defaults are applied (0 leaves
    /// that dimension unlimited), and they are captured at creation time, so changing
    /// <c>default_user_storage_limit</c> later does not retroactively affect existing
    /// accounts.
    /// </para>
    /// </remarks>
    public static async Task ProvisionStorageUserAsync(
        IDbContextFactory<RstashDbContext> contextFactory,
        ApplicationUser user,
        SettingsSnapshot settings)
    {
        await using var db = await contextFactory.CreateDbContextAsync();

        var subject = user.Id.ToString(System.Globalization.CultureInfo.InvariantCulture);
        if (await db.StorageUsers.AnyAsync(s => s.Subject == subject))
        {
            return;
        }

        var storageLimit = settings.DefaultUserStorageLimit;
        var egressLimit = settings.DefaultUserEgressLimit;

        db.StorageUsers.Add(new StorageUser
        {
            // Matches the Identity id for accounts created through the bundled
            // provider, which is also what /connect/authorize puts in the sub claim.
            Id = user.Id,
            Subject = subject,
            UserName = user.UserName ?? "",
            NormalizedUserName = user.NormalizedUserName ?? (user.UserName ?? "").ToUpperInvariant(),
            Plan = "",
            MaxStorage = storageLimit > 0 ? storageLimit : 0,
            MaxEgress = egressLimit > 0 ? egressLimit : 0,
            Disabled = false,
            CreatedAt = DateTimeOffset.UtcNow,
        });

        await db.SaveChangesAsync();
    }
}
