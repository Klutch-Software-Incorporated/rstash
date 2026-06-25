namespace Rstash.Model;

/// <summary>
/// A stored remoteStorage document. Folders are implicit — derived from
/// document paths — so only documents are persisted as nodes. A node is
/// unique per (<see cref="UserId"/>, <see cref="Path"/>).
/// </summary>
public class Node
{
    public long Id { get; set; }

    public long UserId { get; set; }

    /// <summary>Storage path, never ending in '/' (documents only).</summary>
    public string Path { get; set; } = string.Empty;

    public string ContentType { get; set; } = string.Empty;

    public long ContentLength { get; set; }

    public string ETag { get; set; } = string.Empty;

    public DateTimeOffset CreatedAt { get; set; }

    public DateTimeOffset UpdatedAt { get; set; }
}
