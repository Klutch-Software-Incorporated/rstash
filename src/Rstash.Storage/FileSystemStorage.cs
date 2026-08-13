using System.Globalization;

namespace Rstash.Storage;

/// <summary>
/// Stores blobs as files under <c>{basePath}/{userId}/{path}</c>. Writes are
/// atomic (temp file + rename). All resolved paths are verified to stay under
/// the base directory to prevent traversal.
/// </summary>
public sealed class FileSystemStorage : IStorage, IStorageCounter, IStorageProbe
{
    private readonly string _basePath;

    public FileSystemStorage(string basePath)
    {
        _basePath = Path.TrimEndingDirectorySeparator(Path.GetFullPath(basePath));
        Directory.CreateDirectory(_basePath);
    }

    public Task<Stream> GetAsync(long userId, string path, CancellationToken cancellationToken = default)
    {
        try
        {
            var full = ResolveBlobPath(userId, path);
            Stream stream = new FileStream(
                full, FileMode.Open, FileAccess.Read, FileShare.Read, bufferSize: 4096, useAsync: true);
            return Task.FromResult(stream);
        }
        catch (Exception ex)
        {
            return Task.FromException<Stream>(ex);
        }
    }

    public async Task PutAsync(
        long userId, string path, ReadOnlyMemory<byte> data, CancellationToken cancellationToken = default)
    {
        var full = ResolveBlobPath(userId, path);
        var dir = Path.GetDirectoryName(full)!;
        Directory.CreateDirectory(dir);

        var temp = Path.Combine(dir, $".blob-{Guid.NewGuid():N}");
        try
        {
            await using (var stream = new FileStream(
                temp, FileMode.CreateNew, FileAccess.Write, FileShare.None, bufferSize: 4096, useAsync: true))
            {
                await stream.WriteAsync(data, cancellationToken);
            }

            File.Move(temp, full, overwrite: true);
        }
        catch
        {
            TryDeleteFile(temp);
            throw;
        }
    }

    public Task DeleteAsync(long userId, string path, CancellationToken cancellationToken = default)
    {
        TryDeleteFile(ResolveBlobPath(userId, path));
        return Task.CompletedTask;
    }

    public Task DeleteTreeAsync(long userId, string folderPath, CancellationToken cancellationToken = default)
    {
        var full = ResolveBlobPath(userId, folderPath);
        try
        {
            Directory.Delete(full, recursive: true);
        }
        catch (DirectoryNotFoundException)
        {
            // Missing tree is not an error.
        }

        return Task.CompletedTask;
    }

    public Task<long> CountAsync(CancellationToken cancellationToken = default) =>
        Task.FromResult(Directory.Exists(_basePath)
            ? Directory.EnumerateFiles(_basePath, "*", SearchOption.AllDirectories).LongCount()
            : 0L);

    public async Task ProbeAsync(CancellationToken cancellationToken = default)
    {
        await StorageRoundTrip.RunAsync(this, cancellationToken);

        // The round trip leaves behind the directory it wrote into.
        try
        {
            Directory.Delete(Path.Combine(_basePath, "0"));
        }
        catch (Exception ex) when (ex is IOException or UnauthorizedAccessException)
        {
            // Only tidiness — a leftover empty directory is not a failed probe.
        }
    }

    public ValueTask DisposeAsync() => ValueTask.CompletedTask;

    private static void TryDeleteFile(string path)
    {
        try
        {
            File.Delete(path);
        }
        catch (DirectoryNotFoundException)
        {
            // A missing parent directory means the blob isn't there — not an error.
        }
    }

    private string ResolveBlobPath(long userId, string path)
    {
        var relative = path
            .Replace('/', Path.DirectorySeparatorChar)
            .TrimStart(Path.DirectorySeparatorChar);

        var full = Path.GetFullPath(
            Path.Combine(_basePath, userId.ToString(CultureInfo.InvariantCulture), relative));

        if (full != _basePath
            && !full.StartsWith(_basePath + Path.DirectorySeparatorChar, StringComparison.Ordinal))
        {
            throw new StoragePathException($"path traversal rejected: {path}");
        }

        return full;
    }
}
