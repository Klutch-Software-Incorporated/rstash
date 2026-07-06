using Microsoft.AspNetCore.Identity;

namespace Rstash.Database;

/// <summary>
/// The rstash user. Extends Identity's user (which owns UserName, Email,
/// PasswordHash, EmailConfirmed, lockout, etc.) with rstash-specific fields.
/// Identity's token providers handle email verification and password resets, so
/// those need no columns here.
/// </summary>
public class ApplicationUser : IdentityUser<long>
{
    public bool IsAdmin { get; set; }

    /// <summary>Per-user storage cap in bytes; 0 = unlimited.</summary>
    public long StorageQuota { get; set; }

    /// <summary>Per-user monthly egress cap in bytes; 0 = unlimited.</summary>
    public long EgressQuota { get; set; }

    public bool Disabled { get; set; }

    public bool Approved { get; set; } = true;

    /// <summary>True for accounts provisioned by an external admin/billing app.</summary>
    public bool ExternallyManaged { get; set; }

    public DateTimeOffset CreatedAt { get; set; }

    public DateTimeOffset? LastLoginAt { get; set; }

    public string? LastLoginIp { get; set; }

    public DateTimeOffset? TosAcceptedAt { get; set; }

    public DateTimeOffset? PrivacyAcceptedAt { get; set; }
}
