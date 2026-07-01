using Azure.Storage.Blobs;
using Rstash.Storage;

namespace Rstash.Core.Tests;

/// <summary>
/// Round-trips the Azure Blob backend against the Azurite emulator. Mirrors
/// <see cref="FileSystemStorageTests"/> and <see cref="DatabaseStorageTests"/>
/// so every storage backend is held to the same behavioural contract. Self-skips
/// (via <see cref="AzuriteFactAttribute"/>) when no emulator is running.
/// </summary>
public sealed class AzureBlobStorageTests : IDisposable
{
    private readonly string _container = $"rstash-test-{Guid.NewGuid():N}";
    private readonly BlobContainerClient _admin;
    private readonly AzureBlobStorage _store;

    public AzureBlobStorageTests()
    {
        var connectionString =
            "DefaultEndpointsProtocol=http;"
            + $"AccountName={AzuriteEmulator.Account};AccountKey={AzuriteEmulator.Key};"
            + $"BlobEndpoint=http://{AzuriteEmulator.Host}:{AzuriteEmulator.BlobPort}/{AzuriteEmulator.Account};";

        _admin = new BlobContainerClient(connectionString, _container);
        _admin.CreateIfNotExists();

        _store = new AzureBlobStorage(AzuriteEmulator.BlobDsn(_container));
    }

    public void Dispose()
    {
        _admin.DeleteIfExists();
        _store.DisposeAsync().AsTask().GetAwaiter().GetResult();
    }

    [AzuriteFact]
    public async Task PutThenGet_RoundTrips()
    {
        var data = "hello, world"u8.ToArray();
        await _store.PutAsync(1, "docs/readme.txt", data);

        Assert.Equal(data, await ReadAllAsync(_store.GetAsync(1, "docs/readme.txt")));
    }

    [AzuriteFact]
    public async Task Put_Overwrites()
    {
        await _store.PutAsync(1, "file.txt", "v1"u8.ToArray());
        await _store.PutAsync(1, "file.txt", "v2"u8.ToArray());

        Assert.Equal("v2"u8.ToArray(), await ReadAllAsync(_store.GetAsync(1, "file.txt")));
    }

    [AzuriteFact]
    public async Task Delete_RemovesBlob()
    {
        await _store.PutAsync(1, "file.txt", "data"u8.ToArray());
        await _store.DeleteAsync(1, "file.txt");

        await Assert.ThrowsAnyAsync<IOException>(async () => await _store.GetAsync(1, "file.txt"));
    }

    [AzuriteFact]
    public async Task Delete_Missing_IsNotAnError()
    {
        await _store.DeleteAsync(1, "nonexistent.txt");
    }

    [AzuriteFact]
    public async Task Get_Missing_Throws()
    {
        await Assert.ThrowsAnyAsync<IOException>(async () => await _store.GetAsync(1, "nonexistent.txt"));
    }

    [AzuriteFact]
    public async Task DeleteTree_RemovesSubtree()
    {
        string[] files = ["photos/a.jpg", "photos/b.jpg", "photos/sub/c.jpg"];
        foreach (var file in files)
        {
            await _store.PutAsync(1, file, "img"u8.ToArray());
        }

        await _store.PutAsync(1, "keep.txt", "keep"u8.ToArray());

        await _store.DeleteTreeAsync(1, "photos/");

        foreach (var file in files)
        {
            await Assert.ThrowsAnyAsync<IOException>(async () => await _store.GetAsync(1, file));
        }

        // A sibling outside the subtree survives.
        Assert.Equal("keep"u8.ToArray(), await ReadAllAsync(_store.GetAsync(1, "keep.txt")));
    }

    [AzuriteFact]
    public async Task Users_AreIsolated()
    {
        await _store.PutAsync(1, "file.txt", "user1"u8.ToArray());
        await _store.PutAsync(2, "file.txt", "user2"u8.ToArray());

        Assert.Equal("user1"u8.ToArray(), await ReadAllAsync(_store.GetAsync(1, "file.txt")));
        Assert.Equal("user2"u8.ToArray(), await ReadAllAsync(_store.GetAsync(2, "file.txt")));
    }

    [AzuriteFact]
    public async Task Probe_Succeeds_AgainstLiveContainer()
    {
        await _store.ProbeAsync();
    }

    private static async Task<byte[]> ReadAllAsync(Task<Stream> streamTask)
    {
        await using var stream = await streamTask;
        using var buffer = new MemoryStream();
        await stream.CopyToAsync(buffer);
        return buffer.ToArray();
    }
}
