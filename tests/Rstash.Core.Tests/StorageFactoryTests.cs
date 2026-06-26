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
    [InlineData("azureblob")]
    [InlineData("ftp")]
    public void Open_UnsupportedOrDeferred_Throws(string scheme)
    {
        Assert.Throws<NotSupportedException>(() => StorageFactory.Open($"{scheme}:whatever"));
    }

    [Fact]
    public void SupportedSchemes_MatchesBlobDsnVocabulary()
    {
        Assert.True(StorageFactory.IsSupportedScheme("fs"));
        Assert.True(StorageFactory.IsSupportedScheme("sqlite"));
        Assert.False(StorageFactory.IsSupportedScheme("ftp"));
    }
}
