using Microsoft.Data.Sqlite;
using Rstash.Storage;

namespace Rstash.Core.Tests;

/// <summary>Database blob backend against a real SQLite file.</summary>
public sealed class DatabaseStorageTests : IDisposable
{
    private readonly string _path;
    private readonly DatabaseStorage _store;

    public DatabaseStorageTests()
    {
        _path = Path.Combine(Path.GetTempPath(), $"rstash-blobdb-{Guid.NewGuid():N}.sqlite");
        _store = new DatabaseStorage($"sqlite:{_path}");
    }

    public void Dispose()
    {
        SqliteConnection.ClearAllPools();
        foreach (var p in new[] { _path, _path + "-wal", _path + "-shm" })
        {
            try
            {
                if (File.Exists(p))
                {
                    File.Delete(p);
                }
            }
            catch (IOException)
            {
            }
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

    [Fact]
    public async Task Count_ReflectsStoredBlobs()
    {
        Assert.Equal(0, await _store.CountAsync());
        await _store.PutAsync(1, "a.txt", "a"u8.ToArray());
        await _store.PutAsync(2, "a.txt", "a"u8.ToArray());
        Assert.Equal(2, await _store.CountAsync());
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
