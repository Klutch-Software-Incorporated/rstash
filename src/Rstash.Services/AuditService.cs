using Microsoft.EntityFrameworkCore;
using Rstash.Database;
using Rstash.Model;

namespace Rstash.Services;

/// <summary>Appends and reads audit-log entries for state-changing operations.</summary>
public sealed class AuditService(IDbContextFactory<RstashDbContext> contextFactory)
{
    public async Task RecordAsync(
        long actorId, string action, string targetType, string targetId,
        string details = "", CancellationToken cancellationToken = default)
    {
        await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);
        db.AuditLog.Add(new AuditEntry
        {
            ActorId = actorId,
            Action = action,
            TargetType = targetType,
            TargetId = targetId,
            Details = details,
            CreatedAt = DateTimeOffset.UtcNow,
        });
        await db.SaveChangesAsync(cancellationToken);
    }

    public async Task<IReadOnlyList<AuditEntry>> RecentAsync(
        int limit = 100, CancellationToken cancellationToken = default)
    {
        await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);
        // SQLite can't ORDER BY a DateTimeOffset; Id is autoincrement (insertion order).
        return await db.AuditLog
            .AsNoTracking()
            .OrderByDescending(entry => entry.Id)
            .Take(limit)
            .ToListAsync(cancellationToken);
    }
}
