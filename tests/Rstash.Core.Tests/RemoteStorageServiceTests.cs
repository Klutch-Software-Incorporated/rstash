using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Rstash.Database;
using Rstash.Services;
using Rstash.Services.Storage;
using Rstash.Storage;

namespace Rstash.Core.Tests;

/// <summary>Ported from legacy/internal/storage/service_test.go.</summary>
public sealed class RemoteStorageServiceTests : IDisposable
{
    private const long UserId = 1;

    private readonly string _metaPath;
    private readonly string _blobDir;
    private readonly RemoteStorageService _service;

    public RemoteStorageServiceTests()
    {
        _metaPath = Path.Combine(Path.GetTempPath(), $"rstash-meta-{Guid.NewGuid():N}.sqlite");
        _blobDir = Path.Combine(Path.GetTempPath(), $"rstash-blob-{Guid.NewGuid():N}");

        IDbContextFactory<RstashDbContext> factory = new ContextFactory($"sqlite:{_metaPath}");
        using (var db = factory.CreateDbContext())
        {
            db.Database.Migrate();
        }

        _service = new RemoteStorageService(factory, new FileSystemStorage(_blobDir), new SettingsService(factory));
    }

    public void Dispose()
    {
        SqliteConnection.ClearAllPools();
        foreach (var p in new[] { _metaPath, _metaPath + "-wal", _metaPath + "-shm" })
        {
            TryDelete(p);
        }

        try
        {
            Directory.Delete(_blobDir, recursive: true);
        }
        catch (DirectoryNotFoundException)
        {
        }
    }

    [Fact]
    public async Task PutAndGet_RoundTripsAndUpdates()
    {
        var put = await _service.PutDocumentAsync(UserId, "/greeting.txt", Body("hello"), "text/plain", New());
        Assert.True(put.IsNew);
        Assert.NotEqual("", put.ETag);

        var get = await _service.GetDocumentAsync(UserId, "/greeting.txt", New());
        Assert.Equal("hello", await ReadText(get.Content));
        Assert.Equal("text/plain", get.ContentType);
        Assert.Equal(put.ETag, get.ETag);

        var update = await _service.PutDocumentAsync(UserId, "/greeting.txt", Body("world"), "text/plain", New());
        Assert.False(update.IsNew);
        Assert.NotEqual(put.ETag, update.ETag);
    }

    [Fact]
    public async Task Get_Missing_NotFound()
    {
        var ex = await Assert.ThrowsAsync<StorageException>(
            () => _service.GetDocumentAsync(UserId, "/nonexistent.txt", New()));
        Assert.Equal(StorageError.NotFound, ex.Error);
    }

    [Fact]
    public async Task Delete_RemovesDocument()
    {
        var put = await _service.PutDocumentAsync(UserId, "/file.txt", Body("data"), "text/plain", New());

        var del = await _service.DeleteDocumentAsync(UserId, "/file.txt", New());
        Assert.Equal(put.ETag, del.ETag);

        var ex = await Assert.ThrowsAsync<StorageException>(
            () => _service.GetDocumentAsync(UserId, "/file.txt", New()));
        Assert.Equal(StorageError.NotFound, ex.Error);
    }

    [Fact]
    public async Task FolderListing_DerivesItemsAndSubfolders()
    {
        await _service.PutDocumentAsync(UserId, "/docs/a.txt", Body("aaa"), "text/plain", New());
        await _service.PutDocumentAsync(UserId, "/docs/b.txt", Body("bbb"), "text/plain", New());

        var (desc, etag) = await _service.GetFolderAsync(UserId, "/docs/", New());
        Assert.Equal("http://remotestorage.io/spec/folder-description", desc.Context);
        Assert.Equal(2, desc.Items.Count);
        Assert.Contains("a.txt", desc.Items.Keys);
        Assert.Contains("b.txt", desc.Items.Keys);
        Assert.NotEqual("", etag);

        var (root, _) = await _service.GetFolderAsync(UserId, "/", New());
        Assert.Contains("docs/", root.Items.Keys);
    }

    [Fact]
    public async Task Conditional_IfNoneMatch()
    {
        var put = await _service.PutDocumentAsync(UserId, "/test.txt", Body("data"), "text/plain", New());

        var ex = await Assert.ThrowsAsync<StorageException>(
            () => _service.GetDocumentAsync(UserId, "/test.txt", new StorageConditions { IfNoneMatch = [put.ETag] }));
        Assert.Equal(StorageError.NotModified, ex.Error);

        var get = await _service.GetDocumentAsync(UserId, "/test.txt", new StorageConditions { IfNoneMatch = ["bogus"] });
        await get.Content.DisposeAsync();
    }

    [Fact]
    public async Task Conditional_IfMatch()
    {
        var put = await _service.PutDocumentAsync(UserId, "/test.txt", Body("v1"), "text/plain", New());

        await _service.PutDocumentAsync(UserId, "/test.txt", Body("v2"), "text/plain",
            new StorageConditions { IfMatch = put.ETag });

        var ex = await Assert.ThrowsAsync<StorageException>(() => _service.PutDocumentAsync(
            UserId, "/test.txt", Body("v3"), "text/plain", new StorageConditions { IfMatch = put.ETag }));
        Assert.Equal(StorageError.PreconditionFailed, ex.Error);
    }

    [Fact]
    public async Task Conditional_IfNoneMatchStar()
    {
        await _service.PutDocumentAsync(UserId, "/new.txt", Body("data"), "text/plain",
            new StorageConditions { IfNoneMatch = ["*"] });

        var ex = await Assert.ThrowsAsync<StorageException>(() => _service.PutDocumentAsync(
            UserId, "/new.txt", Body("other"), "text/plain", new StorageConditions { IfNoneMatch = ["*"] }));
        Assert.Equal(StorageError.PreconditionFailed, ex.Error);
    }

    [Fact]
    public async Task Delete_CleansUpImplicitFolders()
    {
        await _service.PutDocumentAsync(UserId, "/a/b/c.txt", Body("data"), "text/plain", New());

        var (folder, _) = await _service.GetFolderAsync(UserId, "/a/b/", New());
        Assert.Single(folder.Items);

        await _service.DeleteDocumentAsync(UserId, "/a/b/c.txt", New());

        var (root, _) = await _service.GetFolderAsync(UserId, "/", New());
        Assert.Empty(root.Items);
    }

    [Fact]
    public async Task Head_ReturnsMetadataOnly()
    {
        var put = await _service.PutDocumentAsync(UserId, "/head.txt", Body("content"), "text/plain", New());

        var head = await _service.HeadDocumentAsync(UserId, "/head.txt", New());
        Assert.Equal(put.ETag, head.ETag);
        Assert.Equal("text/plain", head.ContentType);
        Assert.Equal(7, head.ContentLength);
    }

    [Fact]
    public async Task FolderETag_ChangesWhenChildAdded()
    {
        await _service.PutDocumentAsync(UserId, "/folder/a.txt", Body("aaa"), "text/plain", New());
        var (_, etag1) = await _service.GetFolderAsync(UserId, "/folder/", New());

        await _service.PutDocumentAsync(UserId, "/folder/b.txt", Body("bbb"), "text/plain", New());
        var (_, etag2) = await _service.GetFolderAsync(UserId, "/folder/", New());

        Assert.NotEqual(etag1, etag2);
    }

    private static StorageConditions New() => new();

    private static MemoryStream Body(string text) => new(System.Text.Encoding.UTF8.GetBytes(text));

    private static async Task<string> ReadText(Stream stream)
    {
        await using (stream)
        {
            using var reader = new StreamReader(stream);
            return await reader.ReadToEndAsync();
        }
    }

    private static void TryDelete(string path)
    {
        try
        {
            if (File.Exists(path))
            {
                File.Delete(path);
            }
        }
        catch (IOException)
        {
        }
    }

    private sealed class ContextFactory(string dsn) : IDbContextFactory<RstashDbContext>
    {
        public RstashDbContext CreateDbContext() =>
            new(new DbContextOptionsBuilder<RstashDbContext>().UseRstashDatabase(dsn).Options);
    }
}
