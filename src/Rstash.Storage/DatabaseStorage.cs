using Microsoft.EntityFrameworkCore;
using Rstash.Database;

namespace Rstash.Storage;

/// <summary>
/// Stores blobs as rows in a database (the blob DSN's database — by default a
/// separate SQLite file). The schema is ensured on construction. The blob DSN shares
/// the <see cref="RstashDbContextOptionsExtensions.UseRstashDatabase"/> opener with
/// the metadata database, so every wired dialect (sqlite/postgres, incl. Postgres
/// <c>Auth=Entra</c>) works here identically and for free.
/// </summary>
public sealed class DatabaseStorage : IStorage, IStorageCounter
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
        context.Database.EnsureCreated();
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

    public ValueTask DisposeAsync() => ValueTask.CompletedTask;
}
