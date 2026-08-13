using System.Data.Common;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Infrastructure;
using Microsoft.EntityFrameworkCore.Storage;
using Rstash.Database;

namespace Rstash.Storage;

/// <summary>
/// Stores blobs as rows in a database (the blob DSN's database — by default a
/// separate SQLite file). The schema is ensured on construction. The blob DSN shares
/// the <see cref="RstashDbContextOptionsExtensions.UseRstashDatabase"/> opener with
/// the metadata database, so every wired dialect (sqlite/postgres, including Postgres
/// <c>Auth=Entra</c>) works here without any blob-specific code.
/// </summary>
public sealed class DatabaseStorage : IStorage, IStorageCounter, IStorageProbe
{
    private readonly DbContextOptions<BlobDbContext> _options;

    public DatabaseStorage(string dsn)
    {
        // Built once and reused for every context; the options capture the provider
        // (and, for Postgres Entra, a long-lived NpgsqlDataSource + pool). Note this
        // is a separate pool/data source from the metadata DB even when both DSNs
        // point at the same server.
        _options = new DbContextOptionsBuilder<BlobDbContext>()
            .UseRstashDatabase(dsn)
            .Options;

        using var context = new BlobDbContext(_options);
        EnsureBlobsTable(context);
    }

    /// <summary>Creates the <c>blobs</c> table if it is not already there.</summary>
    /// <remarks>
    /// Deliberately not <c>EnsureCreated</c>: that asks whether the *database*
    /// exists, so it does nothing at all once the database holds any table. When
    /// the blob DSN names the metadata database, the metadata migrations have
    /// already run by the time anything resolves this backend, so the table would
    /// never be created and the first upload would fail on a missing relation.
    /// </remarks>
    private static void EnsureBlobsTable(BlobDbContext context)
    {
        var creator = (RelationalDatabaseCreator)context.GetService<IDatabaseCreator>();

        if (!creator.Exists())
        {
            creator.Create();
        }

        if (!BlobsTableExists(context))
        {
            // The context maps exactly one table, so this creates only "blobs".
            creator.CreateTables();
        }
    }

    /// <summary>Asks the provider a question only an existing table can answer.</summary>
    private static bool BlobsTableExists(BlobDbContext context)
    {
        try
        {
            _ = context.Blobs.Any();
            return true;
        }
        catch (DbException)
        {
            return false;
        }
    }

    public async Task<Stream> GetAsync(long userId, string path, CancellationToken cancellationToken = default)
    {
        await using var context = new BlobDbContext(_options);
        var blob = await context.Blobs
            .AsNoTracking()
            .FirstOrDefaultAsync(b => b.UserId == userId && b.Path == path, cancellationToken)
            ?? throw new FileNotFoundException($"blob not found: {path}");

        return new MemoryStream(blob.Data, writable: false);
    }

    public async Task PutAsync(
        long userId, string path, ReadOnlyMemory<byte> data, CancellationToken cancellationToken = default)
    {
        await using var context = new BlobDbContext(_options);
        var existing = await context.Blobs
            .FirstOrDefaultAsync(b => b.UserId == userId && b.Path == path, cancellationToken);

        if (existing is null)
        {
            context.Blobs.Add(new Blob { UserId = userId, Path = path, Data = data.ToArray() });
        }
        else
        {
            existing.Data = data.ToArray();
        }

        await context.SaveChangesAsync(cancellationToken);
    }

    public async Task DeleteAsync(long userId, string path, CancellationToken cancellationToken = default)
    {
        await using var context = new BlobDbContext(_options);
        await context.Blobs
            .Where(b => b.UserId == userId && b.Path == path)
            .ExecuteDeleteAsync(cancellationToken);
    }

    public async Task DeleteTreeAsync(long userId, string folderPath, CancellationToken cancellationToken = default)
    {
        await using var context = new BlobDbContext(_options);
        // EF translates StartsWith to a LIKE 'prefix%' (with escaping). Case
        // sensitivity follows the connection collation; the case-sensitive-LIKE
        // pragma is applied alongside the metadata path queries in P2b/P2c.
        await context.Blobs
            .Where(b => b.UserId == userId && b.Path.StartsWith(folderPath))
            .ExecuteDeleteAsync(cancellationToken);
    }

    public async Task<long> CountAsync(CancellationToken cancellationToken = default)
    {
        await using var context = new BlobDbContext(_options);
        return await context.Blobs.LongCountAsync(cancellationToken);
    }

    public Task ProbeAsync(CancellationToken cancellationToken = default) =>
        StorageRoundTrip.RunAsync(this, cancellationToken);

    public ValueTask DisposeAsync() => ValueTask.CompletedTask;
}
