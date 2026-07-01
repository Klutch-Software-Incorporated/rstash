using Azure;
using Azure.Identity;
using Azure.Storage;
using Azure.Storage.Blobs;

namespace Rstash.Storage;

/// <summary>
/// Stores blobs in an Azure Blob Storage container, one blob per document keyed
/// as <c>{prefix/}{userId}/{path}</c>. The auth mode (shared key, SAS, or
/// <see cref="DefaultAzureCredential"/>) comes from the DSN — see
/// <see cref="AzureBlobDsn"/>.
///
/// This is the template the other object-store backends (S3, etc.) follow:
/// parse a typed DSN record, build the SDK client once in <c>CreateClient</c>,
/// share the <c>BlobName</c> key layout, and implement the <see cref="IStorage"/>
/// operations plus a connectivity <see cref="IStorageProbe"/>.
/// </summary>
public sealed class AzureBlobStorage : IStorage, IStorageProbe
{
    private readonly BlobContainerClient _container;
    private readonly string _prefix;

    public AzureBlobStorage(string dsn)
    {
        var parsed = AzureBlobDsn.Parse(dsn);
        _prefix = parsed.Prefix;
        _container = CreateClient(parsed);
    }

    public async Task<Stream> GetAsync(long userId, string path, CancellationToken cancellationToken = default)
    {
        try
        {
            var response = await _container
                .GetBlobClient(BlobName(userId, path))
                .DownloadStreamingAsync(cancellationToken: cancellationToken);

            // The streaming result owns the network stream; the caller disposes it.
            return response.Value.Content;
        }
        catch (RequestFailedException ex) when (ex.Status == 404)
        {
            throw new FileNotFoundException($"blob not found: {path}", path, ex);
        }
    }

    public Task PutAsync(
        long userId, string path, ReadOnlyMemory<byte> data, CancellationToken cancellationToken = default) =>
        _container
            .GetBlobClient(BlobName(userId, path))
            .UploadAsync(BinaryData.FromBytes(data), overwrite: true, cancellationToken);

    public Task DeleteAsync(long userId, string path, CancellationToken cancellationToken = default) =>
        _container
            .GetBlobClient(BlobName(userId, path))
            .DeleteIfExistsAsync(cancellationToken: cancellationToken);

    public async Task DeleteTreeAsync(long userId, string folderPath, CancellationToken cancellationToken = default)
    {
        var prefix = BlobName(userId, folderPath);
        await foreach (var item in _container.GetBlobsAsync(prefix: prefix, cancellationToken: cancellationToken))
        {
            await _container.GetBlobClient(item.Name).DeleteIfExistsAsync(cancellationToken: cancellationToken);
        }
    }

    /// <summary>Connectivity/credential check: fails fast on bad config for <c>rstash check</c>.</summary>
    public Task ProbeAsync(CancellationToken cancellationToken = default) =>
        _container.GetPropertiesAsync(cancellationToken: cancellationToken);

    public ValueTask DisposeAsync() => ValueTask.CompletedTask;

    private static BlobContainerClient CreateClient(AzureBlobDsn dsn)
    {
        if (dsn.Key is not null)
        {
            var credential = new StorageSharedKeyCredential(dsn.Account, dsn.Key);
            return new BlobContainerClient(dsn.ContainerUri, credential);
        }

        if (dsn.Sas is not null)
        {
            // A SAS token is appended to the container URL as query parameters;
            // the resulting URL is self-authorizing, so no credential object.
            var sasUri = new UriBuilder(dsn.ContainerUri) { Query = dsn.Sas.TrimStart('?') }.Uri;
            return new BlobContainerClient(sasUri);
        }

        return new BlobContainerClient(dsn.ContainerUri, new DefaultAzureCredential());
    }

    private string BlobName(long userId, string path)
    {
        var key = $"{userId}/{path.TrimStart('/')}";
        return _prefix.Length == 0 ? key : $"{_prefix}/{key}";
    }
}
