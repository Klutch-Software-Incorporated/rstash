using Microsoft.AspNetCore.Identity;

namespace Rstash.Database;

/// <summary>
/// The rstash user. Extends Identity's user (which owns UserName, Email,
/// PasswordHash, EmailConfirmed, lockout, etc.) with rstash-specific fields.
/// Identity's token providers handle email verification and password resets, so
/// those need no columns here.
/// </summary>
/// <remarks>
/// One row per account, quotas included. rstash owns its users outright — there is no
/// external provider to keep in sync with — so splitting the storage-side columns into
/// a second table would buy nothing but a join and a write to keep consistent.
/// </remarks>
public class ApplicationUser : IdentityUser<long>
{
    public bool IsAdmin { get; set; }

    /// <summary>Local registration gate (invite/approval modes).</summary>
    public bool Approved { get; set; } = true;

    /// <summary>Storage cap in bytes; 0 = unlimited.</summary>
    public long StorageQuota { get; set; }

    /// <summary>Monthly egress cap in bytes; 0 = unlimited.</summary>
    public long EgressQuota { get; set; }

    /// <summary>
    /// Admin kill switch, checked on every storage request. Distinct from
    /// <see cref="Approved"/>, which gates a brand-new account; both bar access.
    /// </summary>
    public bool Disabled { get; set; }

    public DateTimeOffset CreatedAt { get; set; }

    public DateTimeOffset? LastLoginAt { get; set; }

    public string? LastLoginIp { get; set; }
}
