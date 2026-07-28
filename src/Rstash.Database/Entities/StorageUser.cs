namespace Rstash.Database;

/// <summary>
/// The storage server's own user record, deliberately separate from the identity
/// tables even when both live in the same database.
/// </summary>
/// <remarks>
/// <para>
/// The schema is keyed on a local user id (<c>nodes.UserId</c>,
/// <c>oauth_tokens.UserId</c>, <c>audit_log.ActorId</c>) and the remoteStorage
/// protocol is keyed on a local username (<c>acct:user@host</c> in WebFinger,
/// <c>/storage/{user}/…</c> in every request), so the storage side needs a record of
/// its own regardless of who owns the accounts.
/// </para>
/// <para>
/// Keeping it separate from <see cref="ApplicationUser"/> is what lets the same
/// storage code run whether accounts come from the bundled provider or an external
/// one. Hanging these columns off the row that also holds <c>PasswordHash</c> would
/// mean tearing that table in half the day the provider moves out.
/// </para>
/// <para>
/// The entitlement fields are provider-owned. Under an external provider they are a
/// cache of the last values it gave us — never user-editable, never admin-editable —
/// which is why they carry <see cref="SyncedAt"/> and <see cref="SourceIssuer"/> for
/// provenance. Under the bundled provider the admin UI is the source, and those two
/// stay null. Nothing here records *usage*: only rstash can observe that, and it
/// lives in <c>egress_usage</c> and node sizes. Enforcement compares the two.
/// </para>
/// </remarks>
public class StorageUser
{
    /// <summary>
    /// Matches <see cref="ApplicationUser.Id"/> for accounts that existed before the
    /// split, which is what let the migration backfill without rewriting every
    /// foreign key. New rows get their own values; nothing should assume the two
    /// stay equal.
    /// </summary>
    public long Id { get; set; }

    /// <summary>The identity provider's <c>sub</c> claim — the durable join key.</summary>
    public string Subject { get; set; } = "";

    /// <summary>
    /// The remoteStorage identity. Appears in storage URLs and WebFinger, so it
    /// cannot change once any data exists: renaming is a data migration, not a claim
    /// update. Unique across the instance.
    /// </summary>
    public string UserName { get; set; } = "";

    /// <summary>
    /// Upper-cased <see cref="UserName"/>, mirroring Identity's own normalization so
    /// storage lookups stay case-insensitive after moving off <c>UserManager</c>.
    /// The uniqueness constraint lives here, not on <see cref="UserName"/>: two
    /// accounts differing only by case would collide in <c>/storage/{user}/…</c>.
    /// </summary>
    public string NormalizedUserName { get; set; } = "";

    /// <summary>Provider-assigned plan name; free-form, informational.</summary>
    public string Plan { get; set; } = "";

    /// <summary>Storage cap in bytes; 0 = unlimited.</summary>
    public long MaxStorage { get; set; }

    /// <summary>Monthly egress cap in bytes; 0 = unlimited.</summary>
    public long MaxEgress { get; set; }

    /// <summary>Per-user request-rate override; null falls back to the global setting.</summary>
    public int? RateLimitOverride { get; set; }

    /// <summary>Provider-driven kill switch, checked on every storage request.</summary>
    public bool Disabled { get; set; }

    /// <summary>When the entitlement fields were last refreshed from an external
    /// provider. Null under the bundled provider, where they are authored locally.</summary>
    public DateTimeOffset? SyncedAt { get; set; }

    /// <summary>Which issuer supplied the current entitlement values, so a stale or
    /// wrong-provider cache is visible rather than silent.</summary>
    public string? SourceIssuer { get; set; }

    public DateTimeOffset CreatedAt { get; set; }
}
