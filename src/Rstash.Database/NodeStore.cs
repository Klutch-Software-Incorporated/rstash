using Microsoft.EntityFrameworkCore;
using Rstash.Model;

namespace Rstash.Database;

/// <summary>
/// A direct child of a folder: either a document node or a virtual subfolder
/// derived from descendant document paths.
/// </summary>
public sealed record VirtualChild
{
    public required string Name { get; init; }
    public bool IsFolder { get; init; }
    public required string ETag { get; init; }
    public string ContentType { get; init; } = "";
    public long ContentLength { get; init; }
    public DateTimeOffset UpdatedAt { get; init; }
}

/// <summary>
/// Node (document metadata) queries over a <see cref="RstashDbContext"/>,
/// including the implicit-folder derivation. Folders are not stored — they
/// exist because documents exist beneath them. Operates on a supplied context
/// so the storage service can compose calls inside one transaction.
/// </summary>
public sealed class NodeStore(RstashDbContext context)
{
    public Task<Node?> GetNodeAsync(long userId, string path, CancellationToken cancellationToken = default) =>
        context.Nodes.AsNoTracking()
            .FirstOrDefaultAsync(n => n.UserId == userId && n.Path == path, cancellationToken);

    public async Task<Node> UpsertNodeAsync(
        long userId, string path, string contentType, long contentLength, string etag,
        CancellationToken cancellationToken = default)
    {
        var now = DateTimeOffset.UtcNow;
        var node = await context.Nodes
            .FirstOrDefaultAsync(n => n.UserId == userId && n.Path == path, cancellationToken);

        if (node is null)
        {
            node = new Node
            {
                UserId = userId,
                Path = path,
                ContentType = contentType,
                ContentLength = contentLength,
                ETag = etag,
                CreatedAt = now,
                UpdatedAt = now,
            };
            context.Nodes.Add(node);
        }
        else
        {
            node.ContentType = contentType;
            node.ContentLength = contentLength;
            node.ETag = etag;
            node.UpdatedAt = now;
        }

        await context.SaveChangesAsync(cancellationToken);
        return node;
    }

    public Task DeleteNodeAsync(long userId, string path, CancellationToken cancellationToken = default) =>
        context.Nodes.Where(n => n.UserId == userId && n.Path == path).ExecuteDeleteAsync(cancellationToken);

    /// <summary>
    /// Direct children of a folder: documents whose path has no further slash,
    /// plus virtual subfolders (with ETags hashed over all their descendants).
    /// </summary>
    public async Task<IReadOnlyList<VirtualChild>> ListVirtualChildrenAsync(
        long userId, string folderPath, CancellationToken cancellationToken = default)
    {
        var nodes = await context.Nodes.AsNoTracking()
            .Where(n => n.UserId == userId && n.Path != folderPath && n.Path.StartsWith(folderPath))
            .ToListAsync(cancellationToken);

        var result = new List<VirtualChild>();
        var folders = new Dictionary<string, Dictionary<string, string>>(StringComparer.Ordinal);

        foreach (var node in nodes)
        {
            var rest = node.Path[folderPath.Length..];
            var slashIndex = rest.IndexOf('/');

            if (slashIndex < 0)
            {
                result.Add(new VirtualChild
                {
                    Name = rest,
                    IsFolder = false,
                    ETag = node.ETag,
                    ContentType = node.ContentType,
                    ContentLength = node.ContentLength,
                    UpdatedAt = node.UpdatedAt,
                });
            }
            else
            {
                var subfolder = rest[..(slashIndex + 1)];
                if (!folders.TryGetValue(subfolder, out var descendants))
                {
                    descendants = new Dictionary<string, string>(StringComparer.Ordinal);
                    folders[subfolder] = descendants;
                }

                descendants[rest[(slashIndex + 1)..]] = node.ETag;
            }
        }

        foreach (var (name, descendants) in folders)
        {
            // A subfolder's ETag hashes ALL its descendants, so any deep change
            // propagates up. ETag.ForFolder is exactly that hash.
            result.Add(new VirtualChild { Name = name, IsFolder = true, ETag = ETag.ForFolder(descendants) });
        }

        return result;
    }

    /// <summary>
    /// True if creating a document at <paramref name="path"/> would collide with
    /// existing documents implying a folder there, or with a document occupying
    /// an ancestor segment.
    /// </summary>
    public async Task<bool> HasPathConflictAsync(
        long userId, string path, CancellationToken cancellationToken = default)
    {
        var childPrefix = path + "/";
        var shadowsFolder = await context.Nodes.AsNoTracking()
            .AnyAsync(n => n.UserId == userId && n.Path.StartsWith(childPrefix), cancellationToken);
        if (shadowsFolder)
        {
            return true;
        }

        var segments = path.TrimStart('/').Split('/');
        for (var i = 1; i < segments.Length; i++)
        {
            var ancestor = "/" + string.Join('/', segments[..i]);
            var occupied = await context.Nodes.AsNoTracking()
                .AnyAsync(n => n.UserId == userId && n.Path == ancestor, cancellationToken);
            if (occupied)
            {
                return true;
            }
        }

        return false;
    }

    public Task<List<Node>> ListDescendantFilesAsync(
        long userId, string folderPath, CancellationToken cancellationToken = default) =>
        context.Nodes.AsNoTracking()
            .Where(n => n.UserId == userId && n.Path != folderPath && n.Path.StartsWith(folderPath))
            .ToListAsync(cancellationToken);

    public Task DeleteSubtreeAsync(long userId, string folderPath, CancellationToken cancellationToken = default) =>
        context.Nodes
            .Where(n => n.UserId == userId && (n.Path == folderPath || n.Path.StartsWith(folderPath)))
            .ExecuteDeleteAsync(cancellationToken);
}
