using Microsoft.AspNetCore.Identity;

namespace Rstash.Database;

/// <summary>
/// The rstash user. Extends Identity's user (which owns UserName, Email,
/// PasswordHash, EmailConfirmed, lockout, etc.) with rstash-specific fields.
/// Identity's token providers handle email verification and password resets, so
/// those need no columns here.
/// </summary>
/// <remarks>
/// Strictly the *identity* half. Quotas, the disabled flag, and anything else a
/// provider owns live on <see cref="StorageUser"/> — see that type for why they are
/// not columns here, next to the password hash.
/// </remarks>
public class ApplicationUser : IdentityUser<long>
{
    public bool IsAdmin { get; set; }

    /// <summary>
    /// Local registration gate (invite/approval modes). Not an entitlement: there is
    /// no counterpart under an external provider, which is why the local entitlement
    /// source folds it into the effective disabled flag rather than storing it there.
    /// </summary>
    public bool Approved { get; set; } = true;

    public DateTimeOffset CreatedAt { get; set; }

    public DateTimeOffset? LastLoginAt { get; set; }

    public string? LastLoginIp { get; set; }

    public DateTimeOffset? TosAcceptedAt { get; set; }

    public DateTimeOffset? PrivacyAcceptedAt { get; set; }
}
