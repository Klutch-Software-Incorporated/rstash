namespace Rstash.Storage;

/// <summary>The kind of blob backend selected by a blob DSN.</summary>
public enum BlobBackendKind
{
    FileSystem,
    Database,
    S3,
    AzureBlob,
}
