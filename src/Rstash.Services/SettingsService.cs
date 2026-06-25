using Microsoft.EntityFrameworkCore;
using Rstash.Database;
using Rstash.Services.Configuration;

namespace Rstash.Services;

/// <summary>
/// Runtime-configurable settings backed by the database. Reads are lock-free
/// via an atomically-swapped <see cref="SettingsSnapshot"/>; writes are
/// serialized, validated, persisted, then trigger a reload + change callbacks.
/// </summary>
public sealed class SettingsService : IDisposable
{
    private readonly IDbContextFactory<RstashDbContext> _contextFactory;
    private readonly SemaphoreSlim _writeLock = new(1, 1);
    private readonly List<Action<SettingsSnapshot>> _onChange = [];

    private volatile SettingsSnapshot _current;

    public SettingsService(IDbContextFactory<RstashDbContext> contextFactory)
    {
        _contextFactory = contextFactory;
        // Defaults until the first ReloadAsync pulls overrides from the DB.
        _current = SettingsSnapshot.Resolve(null);
    }

    /// <summary>The current resolved settings. Lock-free.</summary>
    public SettingsSnapshot Current => _current;

    /// <summary>Re-reads overrides from the DB and swaps the snapshot.</summary>
    public async Task ReloadAsync(CancellationToken cancellationToken = default)
    {
        await _writeLock.WaitAsync(cancellationToken);
        try
        {
            await ReloadLockedAsync(cancellationToken);
        }
        finally
        {
            _writeLock.Release();
        }
    }

    /// <summary>The raw DB override for a key, or null if not overridden.</summary>
    public async Task<string?> GetOverrideAsync(string key, CancellationToken cancellationToken = default)
    {
        await using var db = await _contextFactory.CreateDbContextAsync(cancellationToken);
        var setting = await db.Settings.FindAsync([key], cancellationToken);
        return setting?.Value;
    }

    /// <summary>All raw DB overrides (for admin display).</summary>
    public async Task<IReadOnlyDictionary<string, string>> GetOverridesAsync(
        CancellationToken cancellationToken = default)
    {
        await using var db = await _contextFactory.CreateDbContextAsync(cancellationToken);
        return await db.Settings.ToDictionaryAsync(s => s.Key, s => s.Value, StringComparer.Ordinal, cancellationToken);
    }

    /// <summary>Validates and persists a setting, then reloads the snapshot.</summary>
    /// <exception cref="SettingValidationException">If the key/value is invalid.</exception>
    public async Task SetAsync(string key, string value, CancellationToken cancellationToken = default)
    {
        SettingValidator.Validate(key, value);

        await _writeLock.WaitAsync(cancellationToken);
        try
        {
            await using var db = await _contextFactory.CreateDbContextAsync(cancellationToken);
            var existing = await db.Settings.FindAsync([key], cancellationToken);
            if (existing is null)
            {
                db.Settings.Add(new Setting { Key = key, Value = value, UpdatedAt = DateTimeOffset.UtcNow });
            }
            else
            {
                existing.Value = value;
                existing.UpdatedAt = DateTimeOffset.UtcNow;
            }

            await db.SaveChangesAsync(cancellationToken);
            await ReloadLockedAsync(cancellationToken);
        }
        finally
        {
            _writeLock.Release();
        }
    }

    /// <summary>Removes a DB override (reverting to the default), then reloads.</summary>
    public async Task DeleteAsync(string key, CancellationToken cancellationToken = default)
    {
        await _writeLock.WaitAsync(cancellationToken);
        try
        {
            await using var db = await _contextFactory.CreateDbContextAsync(cancellationToken);
            var existing = await db.Settings.FindAsync([key], cancellationToken);
            if (existing is not null)
            {
                db.Settings.Remove(existing);
                await db.SaveChangesAsync(cancellationToken);
            }

            await ReloadLockedAsync(cancellationToken);
        }
        finally
        {
            _writeLock.Release();
        }
    }

    /// <summary>Registers a callback fired whenever the snapshot is swapped.</summary>
    public void OnChange(Action<SettingsSnapshot> callback)
    {
        lock (_onChange)
        {
            _onChange.Add(callback);
        }
    }

    public void Dispose() => _writeLock.Dispose();

    private async Task ReloadLockedAsync(CancellationToken cancellationToken)
    {
        await using var db = await _contextFactory.CreateDbContextAsync(cancellationToken);
        var overrides = await db.Settings.ToDictionaryAsync(
            s => s.Key, s => s.Value, StringComparer.Ordinal, cancellationToken);

        var snapshot = SettingsSnapshot.Resolve(overrides);
        _current = snapshot;

        Action<SettingsSnapshot>[] callbacks;
        lock (_onChange)
        {
            callbacks = [.. _onChange];
        }

        foreach (var callback in callbacks)
        {
            callback(snapshot);
        }
    }
}
