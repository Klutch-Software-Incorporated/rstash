namespace Rstash.Storage;

/// <summary>
/// A blob storage backend — raw byte content keyed by (userId, path). Folder
/// semantics live above this in the storage service; backends only move bytes.
/// </summary>
public interface IStorage : IAsyncDisposable
{
    /// <summary>Opens the blob for reading. The caller owns/disposes the stream.</summary>
    Task<Stream> GetAsync(long userId, string path, CancellationToken cancellationToken = default);

    /// <summary>Stores (or replaces) the blob at the given path.</summary>
    Task PutAsync(long userId, string path, ReadOnlyMemory<byte> data, CancellationToken cancellationToken = default);

    /// <summary>Removes the blob. A missing blob is not an error.</summary>
    Task DeleteAsync(long userId, string path, CancellationToken cancellationToken = default);

    /// <summary>Removes every blob under a folder path prefix.</summary>
    Task DeleteTreeAsync(long userId, string folderPath, CancellationToken cancellationToken = default);
}

/// <summary>
/// Optional capability: report the total number of stored blobs. Used for
/// consistency checks against metadata node counts.
/// </summary>
public interface IStorageCounter
{
    Task<long> CountAsync(CancellationToken cancellationToken = default);
}

/// <summary>
/// Optional capability: verify the backend is reachable and the credentials
/// work. Backends that talk to a remote service (e.g. Azure Blob) implement
/// this so <c>rstash check</c> can fail fast on bad config; local backends that
/// validate on open don't need it.
/// </summary>
public interface IStorageProbe
{
    Task ProbeAsync(CancellationToken cancellationToken = default);
}
