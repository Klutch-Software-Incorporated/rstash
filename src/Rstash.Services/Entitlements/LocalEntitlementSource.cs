using Microsoft.EntityFrameworkCore;
using Rstash.Database;

namespace Rstash.Services.Entitlements;

/// <summary>
/// Reads entitlements straight from the local <c>storage_users</c> row. This is what
/// a self-hoster runs: the admin UI is the source of truth, there is no external
/// provider, and nothing goes over the network.
/// </summary>
/// <remarks>
/// The external counterpart will implement the same interface by fetching from the
/// control plane and caching, so nothing downstream of <see cref="IEntitlementSource"/>
/// changes when the provider moves out.
/// </remarks>
public sealed class LocalEntitlementSource(IDbContextFactory<RstashDbContext> contextFactory)
    : IEntitlementSource
{
    public async ValueTask<Entitlements> ResolveAsync(
        long storageUserId, CancellationToken cancellationToken = default)
    {
        await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);

        // Approval is folded into Disabled here rather than stored that way, because
        // it is a local registration gate with no counterpart under an external
        // provider. Keeping the fold in this implementation means the storage path
        // checks exactly one flag, and the external implementation is not saddled with
        // a concept it does not have.
        var row = await db.StorageUsers
            .Where(s => s.Id == storageUserId)
            .Select(s => new
            {
                s.MaxStorage,
                s.MaxEgress,
                s.RateLimitOverride,
                s.Plan,
                s.Disabled,
                Approved = db.Users
                    .Where(u => u.Id == s.Id)
                    .Select(u => (bool?)u.Approved)
                    .FirstOrDefault(),
            })
            .FirstOrDefaultAsync(cancellationToken);

        if (row is null)
        {
            return Entitlements.None;
        }

        return new Entitlements(
            row.MaxStorage,
            row.MaxEgress,
            row.RateLimitOverride,
            row.Plan,
            Disabled: row.Disabled || row.Approved is false);
    }
}
