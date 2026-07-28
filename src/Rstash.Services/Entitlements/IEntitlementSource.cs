namespace Rstash.Services.Entitlements;

/// <summary>
/// What a user is allowed to do. Resolved through this interface everywhere, so
/// enforcement is identical whether the limits were set by a local admin or handed
/// down by an external control plane.
/// </summary>
/// <remarks>
/// Deliberately says nothing about *usage* — bytes stored, egress consumed. Only
/// rstash can observe those, so only rstash tracks them. Enforcement compares what
/// this returns against what rstash has measured.
/// </remarks>
public interface IEntitlementSource
{
    /// <summary>
    /// Resolves the current entitlements for a storage user. Implementations must be
    /// cheap enough to call on every storage request; anything that talks to a remote
    /// provider has to cache.
    /// </summary>
    ValueTask<Entitlements> ResolveAsync(long storageUserId, CancellationToken cancellationToken = default);
}

/// <summary>
/// A user's limits at a point in time.
/// </summary>
/// <param name="MaxStorage">Storage cap in bytes; 0 = unlimited.</param>
/// <param name="MaxEgress">Monthly egress cap in bytes; 0 = unlimited.</param>
/// <param name="RateLimitOverride">Per-user request rate; null falls back to the global setting.</param>
/// <param name="Plan">Provider-assigned plan name; informational.</param>
/// <param name="Disabled">Whether the account is barred from storage entirely.</param>
public readonly record struct Entitlements(
    long MaxStorage,
    long MaxEgress,
    int? RateLimitOverride,
    string Plan,
    bool Disabled)
{
    /// <summary>
    /// What an unknown user gets: no access. Returned rather than throwing so callers
    /// on the storage path fail closed without special-casing a missing row.
    /// </summary>
    public static Entitlements None { get; } = new(0, 0, null, "", Disabled: true);
}
