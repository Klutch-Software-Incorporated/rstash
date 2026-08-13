using Rstash.Storage;

namespace Rstash.Core.Tests;

/// <summary>Ported from the Go implementation's internal/blob/fs_test.go.</summary>
public sealed class FileSystemStorageTests : IDisposable
{
    private readonly string _dir;
    private readonly FileSystemStorage _store;

    public FileSystemStorageTests()
    {
        _dir = Path.Combine(Path.GetTempPath(), $"rstash-fs-{Guid.NewGuid():N}");
        _store = new FileSystemStorage(_dir);
    }

    public void Dispose()
    {
        try
        {
            Directory.Delete(_dir, recursive: true);
        }
        catch (DirectoryNotFoundException)
        {
        }
    }

    [Fact]
    public async Task PutThenGet_RoundTrips()
    {
        var data = "hello, world"u8.ToArray();
        await _store.PutAsync(1, "docs/readme.txt", data);

        Assert.Equal(data, await ReadAllAsync(_store.GetAsync(1, "docs/readme.txt")));
    }

    [Fact]
    public async Task Put_Overwrites()
    {
        await _store.PutAsync(1, "file.txt", "v1"u8.ToArray());
        await _store.PutAsync(1, "file.txt", "v2"u8.ToArray());

        Assert.Equal("v2"u8.ToArray(), await ReadAllAsync(_store.GetAsync(1, "file.txt")));
    }

    [Fact]
    public async Task Delete_RemovesBlob()
    {
        await _store.PutAsync(1, "file.txt", "data"u8.ToArray());
        await _store.DeleteAsync(1, "file.txt");

        await Assert.ThrowsAnyAsync<IOException>(async () => await _store.GetAsync(1, "file.txt"));
    }

    [Fact]
    public async Task Delete_Missing_IsNotAnError()
    {
        await _store.DeleteAsync(1, "nonexistent.txt");
    }

    [Fact]
    public async Task Get_Missing_Throws()
    {
        await Assert.ThrowsAnyAsync<IOException>(async () => await _store.GetAsync(1, "nonexistent.txt"));
    }

    [Fact]
    public async Task DeleteTree_RemovesSubtree()
    {
        string[] files = ["photos/a.jpg", "photos/b.jpg", "photos/sub/c.jpg"];
        foreach (var file in files)
        {
            await _store.PutAsync(1, file, "img"u8.ToArray());
        }

        await _store.DeleteTreeAsync(1, "photos/");

        foreach (var file in files)
        {
            await Assert.ThrowsAnyAsync<IOException>(async () => await _store.GetAsync(1, file));
        }
    }

    [Fact]
    public async Task PathTraversal_IsRejected()
    {
        await Assert.ThrowsAsync<StoragePathException>(async () => await _store.GetAsync(1, "../../etc/passwd"));
        await Assert.ThrowsAsync<StoragePathException>(
            async () => await _store.PutAsync(1, "../../etc/evil", "bad"u8.ToArray()));
    }

    [Fact]
    public async Task Users_AreIsolated()
    {
        await _store.PutAsync(1, "file.txt", "user1"u8.ToArray());
        await _store.PutAsync(2, "file.txt", "user2"u8.ToArray());

        Assert.Equal("user1"u8.ToArray(), await ReadAllAsync(_store.GetAsync(1, "file.txt")));
        Assert.Equal("user2"u8.ToArray(), await ReadAllAsync(_store.GetAsync(2, "file.txt")));
    }

    private static async Task<byte[]> ReadAllAsync(Task<Stream> streamTask)
    {
        await using var stream = await streamTask;
        using var buffer = new MemoryStream();
        await stream.CopyToAsync(buffer);
        return buffer.ToArray();
    }
}
