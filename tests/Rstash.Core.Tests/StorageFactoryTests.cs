using Rstash.Storage;

namespace Rstash.Core.Tests;

public class StorageFactoryTests
{
    [Fact]
    public async Task Open_Fs_CreatesFileSystemBackend()
    {
        var dir = Path.Combine(Path.GetTempPath(), $"rstash-factory-{Guid.NewGuid():N}");
        try
        {
            await using var store = StorageFactory.Open($"fs:{dir}");
            Assert.IsType<FileSystemStorage>(store);
        }
        finally
        {
            try
            {
                Directory.Delete(dir, recursive: true);
            }
            catch (DirectoryNotFoundException)
            {
            }
        }
    }

    [Theory]
    [InlineData("s3")]
    [InlineData("ftp")]
    public void Open_UnsupportedOrDeferred_Throws(string scheme)
    {
        Assert.Throws<NotSupportedException>(() => StorageFactory.Open($"{scheme}:whatever"));
    }

    [Fact]
    public async Task Open_AzureBlob_WellFormed_CreatesBackend()
    {
        // Shared-key DSN so construction is deterministic (the managed-identity
        // path builds a DefaultAzureCredential, whose chain depends on ambient
        // Azure env vars). No network I/O either way — connectivity is checked
        // separately via ProbeAsync.
        await using var store = StorageFactory.Open("azureblob://acct/container?key=dGVzdA==");
        Assert.IsType<AzureBlobStorage>(store);
    }

    [Fact]
    public void Open_AzureBlob_Malformed_Throws()
    {
        Assert.Throws<FormatException>(() => StorageFactory.Open("azureblob://acct")); // no container
    }

    [Fact]
    public void SupportedSchemes_MatchesBlobDsnVocabulary()
    {
        Assert.True(StorageFactory.IsSupportedScheme("fs"));
        Assert.True(StorageFactory.IsSupportedScheme("sqlite"));
        Assert.False(StorageFactory.IsSupportedScheme("ftp"));
    }
}
