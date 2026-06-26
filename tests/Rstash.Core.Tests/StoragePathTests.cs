using Rstash.Model;

namespace Rstash.Core.Tests;

/// <summary>Ported from legacy/internal/api/path_test.go.</summary>
public class StoragePathTests
{
    [Theory]
    [InlineData("/")]
    [InlineData("/file.txt")]
    [InlineData("/foo/bar/baz.txt")]
    [InlineData("/foo/bar/")]
    [InlineData("/données/日本語.txt")]
    [InlineData("/.hidden")]
    [InlineData("/foo/.gitignore")]
    [InlineData("/my..file.txt")]
    [InlineData("/what...no")]
    [InlineData("/my..dir/file.txt")]
    [InlineData(@"/foo\bar.txt")]
    public void Valid(string path)
    {
        Assert.True(StoragePath.TryValidate(path, out _));
    }

    [Theory]
    [InlineData("")]
    [InlineData("foo/bar")]
    [InlineData("/foo/\0bar")]
    [InlineData("/foo//bar")]
    [InlineData("/foo/./bar")]
    [InlineData("/foo/../bar")]
    [InlineData("/./foo")]
    [InlineData("/../foo")]
    [InlineData("/foo/.")]
    [InlineData("/..")]
    [InlineData("/.")]
    public void Invalid(string path)
    {
        Assert.False(StoragePath.TryValidate(path, out _));
    }

    [Fact]
    public void MaxLength_Boundary()
    {
        Assert.True(StoragePath.TryValidate("/" + new string('a', 511), out _));
        Assert.False(StoragePath.TryValidate("/" + new string('a', 512), out _));
    }
}
