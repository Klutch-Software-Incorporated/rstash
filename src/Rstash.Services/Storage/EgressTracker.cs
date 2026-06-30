using System.Globalization;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Hosting;
using Rstash.Database;

namespace Rstash.Services.Storage;

/// <summary>
/// Meters outbound transfer (egress) per user per monthly period and enforces
/// the per-user quota (<see cref="ApplicationUser.EgressQuota"/>) plus the global
/// cap (<c>total_egress_limit</c> setting). Only document GET bodies are counted —
/// not HEAD, folder listings, or 304s.
///
/// Increments are buffered in memory and flushed to the DB on a timer (and on
/// shutdown) to keep write pressure off the read path. Enforcement counts both
/// persisted and not-yet-flushed bytes, so a burst can't slip past between flushes.
///
/// Uploads (ingress) are deliberately not tracked: they're bounded by storage
/// quota and <c>max_upload_size</c>, and inbound transfer is typically free.
/// </summary>
public sealed class EgressTracker(
    IDbContextFactory<RstashDbContext> contextFactory, SettingsService settings) : BackgroundService
{
    private static readonly TimeSpan FlushInterval = TimeSpan.FromSeconds(10);

    private readonly object _gate = new();
    private Dictionary<long, long> _pending = new();
    private long _pendingTotal;

    /// <summary>The current UTC billing period, formatted "YYYY-MM".</summary>
    public static string CurrentPeriod() =>
        DateTime.UtcNow.ToString("yyyy-MM", CultureInfo.InvariantCulture);

    /// <summary>Queues <paramref name="bytes"/> of egress for <paramref name="userId"/>.</summary>
    public void Record(long userId, long bytes)
    {
        if (bytes <= 0)
        {
            return;
        }

        lock (_gate)
        {
            _pending[userId] = _pending.GetValueOrDefault(userId) + bytes;
            _pendingTotal += bytes;
        }
    }

    /// <summary>
    /// Whether serving <paramref name="bytes"/> more to <paramref name="userId"/> stays
    /// within both the per-user quota (<paramref name="userLimit"/>; 0 = unlimited) and
    /// the global cap (0 = disabled) for the current period. Counts persisted usage plus
    /// pending in-memory bytes. With neither limit set, returns true without touching the DB.
    /// </summary>
    public async Task<bool> CanServeAsync(
        long userId, long bytes, long userLimit, CancellationToken cancellationToken = default)
    {
        if (bytes <= 0)
        {
            return true;
        }

        var totalLimit = settings.Current.TotalEgressLimit;
        if (userLimit <= 0 && totalLimit <= 0)
        {
            return true;
        }

        long pendingUser, pendingTotal;
        lock (_gate)
        {
            pendingUser = _pending.GetValueOrDefault(userId);
            pendingTotal = _pendingTotal;
        }

        var period = CurrentPeriod();
        await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);

        if (userLimit > 0)
        {
            var used = await db.EgressUsage
                .Where(e => e.UserId == userId && e.Period == period)
                .Select(e => e.BytesOut)
                .FirstOrDefaultAsync(cancellationToken);
            if (used + pendingUser + bytes > userLimit)
            {
                return false;
            }
        }

        if (totalLimit > 0)
        {
            var total = await db.EgressUsage
                .Where(e => e.Period == period)
                .SumAsync(e => e.BytesOut, cancellationToken);
            if (total + pendingTotal + bytes > totalLimit)
            {
                return false;
            }
        }

        return true;
    }

    /// <summary>
    /// Current-period egress for a user: persisted bytes plus any not-yet-flushed
    /// pending. Drives the dashboard bandwidth meter.
    /// </summary>
    public async Task<long> GetUsedAsync(long userId, CancellationToken cancellationToken = default)
    {
        long pending;
        lock (_gate)
        {
            pending = _pending.GetValueOrDefault(userId);
        }

        var period = CurrentPeriod();
        await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);
        var persisted = await db.EgressUsage
            .Where(e => e.UserId == userId && e.Period == period)
            .Select(e => e.BytesOut)
            .FirstOrDefaultAsync(cancellationToken);
        return persisted + pending;
    }

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        using var timer = new PeriodicTimer(FlushInterval);
        try
        {
            while (await timer.WaitForNextTickAsync(stoppingToken))
            {
                await FlushAsync(CancellationToken.None);
            }
        }
        catch (OperationCanceledException)
        {
            // Host is shutting down — fall through to the final flush.
        }
        finally
        {
            await FlushAsync(CancellationToken.None);
        }
    }

    /// <summary>
    /// Persists buffered increments. Single-writer (only the flush loop runs this), so a
    /// plain find-or-increment is race-free and provider-agnostic — no upsert SQL needed.
    /// On failure the whole batch is requeued for the next tick.
    /// </summary>
    private async Task FlushAsync(CancellationToken cancellationToken)
    {
        Dictionary<long, long> batch;
        lock (_gate)
        {
            if (_pending.Count == 0)
            {
                return;
            }

            batch = _pending;
            _pending = new Dictionary<long, long>();
            _pendingTotal = 0;
        }

        var period = CurrentPeriod();
        try
        {
            await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);
            foreach (var (userId, bytes) in batch)
            {
                var row = await db.EgressUsage
                    .FirstOrDefaultAsync(e => e.UserId == userId && e.Period == period, cancellationToken);
                if (row is null)
                {
                    db.EgressUsage.Add(new EgressUsage
                    {
                        UserId = userId,
                        Period = period,
                        BytesOut = bytes,
                        UpdatedAt = DateTimeOffset.UtcNow,
                    });
                }
                else
                {
                    row.BytesOut += bytes;
                    row.UpdatedAt = DateTimeOffset.UtcNow;
                }
            }

            await db.SaveChangesAsync(cancellationToken);
        }
        catch
        {
            lock (_gate)
            {
                foreach (var (userId, bytes) in batch)
                {
                    _pending[userId] = _pending.GetValueOrDefault(userId) + bytes;
                    _pendingTotal += bytes;
                }
            }
        }
    }
}
