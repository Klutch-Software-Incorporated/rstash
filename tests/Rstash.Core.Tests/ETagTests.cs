using Rstash.Model;

namespace Rstash.Core.Tests;

/// <summary>
/// Ported from the Go reference (the Go implementation's internal/storage/etag_test.go) — the
/// behavioral oracle for ETag computation.
/// </summary>
public class ETagTests
{
    [Fact]
    public void ForDocument_Produces16HexChars()
    {
        Assert.Equal(16, ETag.ForDocument("hello"u8).Length);
    }

    [Fact]
    public void ForDocument_IsDeterministic()
    {
        Assert.Equal(ETag.ForDocument("hello"u8), ETag.ForDocument("hello"u8));
    }

    [Fact]
    public void ForDocument_DiffersByContent()
    {
        Assert.NotEqual(ETag.ForDocument("hello"u8), ETag.ForDocument("world"u8));
    }

    [Fact]
    public void ForFolder_Is16HexChars_AndDeterministic()
    {
        var children = new Dictionary<string, string>
        {
            ["a.txt"] = "abc",
            ["b.txt"] = "def",
        };

        var etag = ETag.ForFolder(children);

        Assert.Equal(16, etag.Length);
        Assert.Equal(etag, ETag.ForFolder(children));
    }

    [Fact]
    public void ForFolder_OrderIndependent()
    {
        var ascending = new Dictionary<string, string> { ["a.txt"] = "abc", ["b.txt"] = "def" };
        var descending = new Dictionary<string, string> { ["b.txt"] = "def", ["a.txt"] = "abc" };

        Assert.Equal(ETag.ForFolder(ascending), ETag.ForFolder(descending));
    }

    [Fact]
    public void ForFolder_Empty_Is16HexChars()
    {
        Assert.Equal(16, ETag.ForFolder(new Dictionary<string, string>()).Length);
    }

    [Fact]
    public void Quote_WrapsInDoubleQuotes()
    {
        Assert.Equal("\"abc123\"", ETag.Quote("abc123"));
    }

    [Fact]
    public void Unquote_StripsDoubleQuotes()
    {
        Assert.Equal("abc123", ETag.Unquote("\"abc123\""));
    }

    [Fact]
    public void Unquote_LeavesBareValueUnchanged()
    {
        Assert.Equal("abc123", ETag.Unquote("abc123"));
    }
}
