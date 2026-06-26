using System.Globalization;
using Microsoft.EntityFrameworkCore;
using Rstash.Database;
using Rstash.Model;
using Rstash.Storage;

namespace Rstash.Services.Storage;

/// <summary>
/// Orchestrates remoteStorage document operations across blob content and node
/// metadata: ETags, implicit folders, conditional requests, and transactions.
/// Quota and egress enforcement are layered on later (currently no-op).
/// </summary>
public sealed class RemoteStorageService(
    IDbContextFactory<RstashDbContext> contextFactory, IStorage storage, SettingsService settings)
{
    public async Task<PutResult> PutDocumentAsync(
        long userId, string path, Stream content, string contentType,
        StorageConditions conditions, CancellationToken cancellationToken = default)
    {
        if (path.EndsWith('/'))
        {
            throw new StorageException(StorageError.Conflict);
        }

        var data = await ReadAllAsync(content, cancellationToken);
        var etag = ETag.ForDocument(data);

        // Write the blob first (outside the metadata transaction).
        await storage.PutAsync(userId, path, data, cancellationToken);

        try
        {
            await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);
            await using var transaction = await db.Database.BeginTransactionAsync(cancellationToken);
            var nodes = new NodeStore(db);

            var existing = await nodes.GetNodeAsync(userId, path, cancellationToken);

            if (await nodes.HasPathConflictAsync(userId, path, cancellationToken))
            {
                throw new StorageException(StorageError.Conflict);
            }

            if (!string.IsNullOrEmpty(conditions.IfMatch)
                && (existing is null || existing.ETag != conditions.IfMatch))
            {
                throw new StorageException(StorageError.PreconditionFailed);
            }

            if (conditions.IfNoneMatchStar && existing is not null)
            {
                throw new StorageException(StorageError.PreconditionFailed);
            }

            await EnsureWithinQuotaAsync(
                db, userId, data.LongLength - (existing?.ContentLength ?? 0), cancellationToken);

            var isNew = existing is null;
            await nodes.UpsertNodeAsync(userId, path, contentType, data.LongLength, etag, cancellationToken);
            await transaction.CommitAsync(cancellationToken);

            return new PutResult(etag, isNew);
        }
        catch (Exception ex)
        {
            // Best-effort blob cleanup on metadata failure, except for the
            // conflict/precondition cases (matching the Go reference).
            if (ex is not StorageException { Error: StorageError.Conflict or StorageError.PreconditionFailed })
            {
                try
                {
                    await storage.DeleteAsync(userId, path, cancellationToken);
                }
                catch
                {
                    // Orphaned blob cleanup is best-effort.
                }
            }

            throw;
        }
    }

    public async Task<GetResult> GetDocumentAsync(
        long userId, string path, StorageConditions conditions, CancellationToken cancellationToken = default)
    {
        await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);
        var node = await new NodeStore(db).GetNodeAsync(userId, path, cancellationToken)
            ?? throw new StorageException(StorageError.NotFound);

        if (conditions.IfNoneMatchContains(node.ETag))
        {
            throw new StorageException(StorageError.NotModified) { ETag = node.ETag };
        }

        var content = await storage.GetAsync(userId, path, cancellationToken);
        return new GetResult(content, node.ContentType, node.ContentLength, node.ETag);
    }

    public async Task<HeadResult> HeadDocumentAsync(
        long userId, string path, StorageConditions conditions, CancellationToken cancellationToken = default)
    {
        await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);
        var node = await new NodeStore(db).GetNodeAsync(userId, path, cancellationToken)
            ?? throw new StorageException(StorageError.NotFound);

        if (conditions.IfNoneMatchContains(node.ETag))
        {
            throw new StorageException(StorageError.NotModified) { ETag = node.ETag };
        }

        return new HeadResult(node.ContentType, node.ContentLength, node.ETag);
    }

    public async Task<DeleteResult> DeleteDocumentAsync(
        long userId, string path, StorageConditions conditions, CancellationToken cancellationToken = default)
    {
        await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);

        string deletedETag;
        await using (var transaction = await db.Database.BeginTransactionAsync(cancellationToken))
        {
            var nodes = new NodeStore(db);
            var node = await nodes.GetNodeAsync(userId, path, cancellationToken)
                ?? throw new StorageException(StorageError.NotFound);

            if (!string.IsNullOrEmpty(conditions.IfMatch) && node.ETag != conditions.IfMatch)
            {
                throw new StorageException(StorageError.PreconditionFailed);
            }

            deletedETag = node.ETag;
            await nodes.DeleteNodeAsync(userId, path, cancellationToken);
            await transaction.CommitAsync(cancellationToken);
        }

        // Remove the blob after the metadata commit (best-effort).
        try
        {
            await storage.DeleteAsync(userId, path, cancellationToken);
        }
        catch
        {
            // Blob cleanup is best-effort once metadata is gone.
        }

        return new DeleteResult(deletedETag);
    }

    public async Task<int> DeleteFolderAsync(
        long userId, string folderPath, CancellationToken cancellationToken = default)
    {
        if (!folderPath.EndsWith('/') || folderPath == "/")
        {
            throw new StorageException(StorageError.Conflict);
        }

        await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);

        int fileCount;
        await using (var transaction = await db.Database.BeginTransactionAsync(cancellationToken))
        {
            var nodes = new NodeStore(db);
            fileCount = (await nodes.ListDescendantFilesAsync(userId, folderPath, cancellationToken)).Count;
            await nodes.DeleteSubtreeAsync(userId, folderPath, cancellationToken);
            await transaction.CommitAsync(cancellationToken);
        }

        try
        {
            await storage.DeleteTreeAsync(userId, folderPath, cancellationToken);
        }
        catch
        {
            // Blob tree cleanup is best-effort once metadata is gone.
        }

        return fileCount;
    }

    public async Task<(FolderDescription Description, string ETag)> GetFolderAsync(
        long userId, string path, StorageConditions conditions, CancellationToken cancellationToken = default)
    {
        await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);
        var children = await new NodeStore(db).ListVirtualChildrenAsync(userId, path, cancellationToken);

        var items = new Dictionary<string, FolderItem>(StringComparer.Ordinal);
        var childETags = new Dictionary<string, string>(StringComparer.Ordinal);

        foreach (var child in children)
        {
            items[child.Name] = child.IsFolder
                ? new FolderItem { ETag = child.ETag }
                : new FolderItem
                {
                    ETag = child.ETag,
                    ContentType = child.ContentType,
                    ContentLength = child.ContentLength,
                    LastModified = child.UpdatedAt.UtcDateTime.ToString("R", CultureInfo.InvariantCulture),
                };
            childETags[child.Name] = child.ETag;
        }

        var etag = ETag.ForFolder(childETags);
        if (conditions.IfNoneMatchContains(etag))
        {
            throw new StorageException(StorageError.NotModified) { ETag = etag };
        }

        var description = new FolderDescription { Items = items };
        return (description, etag);
    }

    /// <summary>Total bytes stored for a user plus a per-top-folder breakdown (largest first).</summary>
    public async Task<StorageUsage> GetUsageAsync(long userId, CancellationToken cancellationToken = default)
    {
        await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);
        var rows = await db.Nodes
            .Where(n => n.UserId == userId)
            .Select(n => new { n.Path, n.ContentLength })
            .ToListAsync(cancellationToken);

        var folders = rows
            .GroupBy(r => TopFolder(r.Path))
            .Select(group => new FolderUsage(group.Key, group.Sum(r => r.ContentLength)))
            .OrderByDescending(folder => folder.Bytes)
            .ToList();

        return new StorageUsage(rows.Sum(r => r.ContentLength), folders);
    }

    /// <summary>
    /// UI-oriented listing of a folder's immediate children. Files carry their own size;
    /// subfolders carry the recursive total of every document beneath them. Folders sort
    /// first, then files, each alphabetically.
    /// </summary>
    public async Task<IReadOnlyList<BrowserEntry>> ListFolderAsync(
        long userId, string folderPath, CancellationToken cancellationToken = default)
    {
        await using var db = await contextFactory.CreateDbContextAsync(cancellationToken);
        var rows = await db.Nodes
            .Where(n => n.UserId == userId && n.Path.StartsWith(folderPath))
            .Select(n => new { n.Path, n.ContentLength, n.ContentType })
            .ToListAsync(cancellationToken);

        var files = new List<BrowserEntry>();
        var folderSizes = new Dictionary<string, long>(StringComparer.Ordinal);

        foreach (var row in rows)
        {
            var remainder = row.Path[folderPath.Length..];
            var slash = remainder.IndexOf('/');
            if (slash < 0)
            {
                files.Add(new BrowserEntry(remainder, IsFolder: false, row.ContentLength, row.ContentType));
            }
            else
            {
                var name = remainder[..slash];
                folderSizes[name] = folderSizes.GetValueOrDefault(name) + row.ContentLength;
            }
        }

        var entries = folderSizes
            .Select(folder => new BrowserEntry(folder.Key + "/", IsFolder: true, folder.Value, ""))
            .OrderBy(entry => entry.Name, StringComparer.Ordinal)
            .ToList();
        entries.AddRange(files.OrderBy(file => file.Name, StringComparer.Ordinal));
        return entries;
    }

    /// <summary>The top-level module folder of a path, e.g. <c>/photos/2026/x.jpg</c> → <c>/photos</c>.</summary>
    private static string TopFolder(string path)
    {
        var trimmed = path.TrimStart('/');
        var slash = trimmed.IndexOf('/');
        return "/" + (slash < 0 ? trimmed : trimmed[..slash]);
    }

    /// <summary>
    /// Enforces the per-user storage quota (ApplicationUser.StorageQuota; 0 =
    /// unlimited) and the global cap (settings; 0 = disabled). <paramref name="delta"/>
    /// is the net byte change (new size minus any replaced size).
    /// </summary>
    private async Task EnsureWithinQuotaAsync(
        RstashDbContext db, long userId, long delta, CancellationToken cancellationToken)
    {
        var userQuota = await db.Users
            .Where(u => u.Id == userId)
            .Select(u => u.StorageQuota)
            .FirstOrDefaultAsync(cancellationToken);

        if (userQuota > 0)
        {
            var used = await db.Nodes
                .Where(n => n.UserId == userId)
                .SumAsync(n => n.ContentLength, cancellationToken);
            if (used + delta > userQuota)
            {
                throw new StorageException(StorageError.QuotaExceeded);
            }
        }

        var totalLimit = settings.Current.TotalStorageLimit;
        if (totalLimit > 0)
        {
            var total = await db.Nodes.SumAsync(n => n.ContentLength, cancellationToken);
            if (total + delta > totalLimit)
            {
                throw new StorageException(StorageError.QuotaExceeded);
            }
        }
    }

    private static async Task<byte[]> ReadAllAsync(Stream content, CancellationToken cancellationToken)
    {
        if (content is MemoryStream alreadyBuffered)
        {
            return alreadyBuffered.ToArray();
        }

        using var buffer = new MemoryStream();
        await content.CopyToAsync(buffer, cancellationToken);
        return buffer.ToArray();
    }
}

/// <summary>Aggregate storage usage for a user: total bytes and a per-top-folder breakdown.</summary>
public sealed record StorageUsage(long TotalBytes, IReadOnlyList<FolderUsage> Folders);

/// <summary>Bytes stored under one top-level module folder (e.g. <c>/photos</c>).</summary>
public sealed record FolderUsage(string Folder, long Bytes);

/// <summary>One row in the file browser: a file (own size) or a subfolder (recursive size).</summary>
public sealed record BrowserEntry(string Name, bool IsFolder, long Bytes, string ContentType);
