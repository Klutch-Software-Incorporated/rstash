namespace Rstash.Services.Storage;

/// <summary>Protocol-level outcomes that map to HTTP status codes at the edge.</summary>
public enum StorageError
{
    NotFound,           // 404
    PreconditionFailed, // 412
    Conflict,           // 409
    NotModified,        // 304
    PayloadTooLarge,    // 413
    ContentRejected,    // 4xx (scanner)
    QuotaExceeded,      // 507
}

/// <summary>
/// A storage operation failed with a protocol-level outcome. Carries the ETag
/// for <see cref="StorageError.NotModified"/> so the edge can echo it on a 304.
/// </summary>
public sealed class StorageException(StorageError error, string? message = null)
    : Exception(message ?? error.ToString())
{
    public StorageError Error { get; } = error;

    public string? ETag { get; init; }
}
