using Rstash.Services.Configuration;

namespace Rstash.Core.Tests;

/// <summary>Ported from legacy/internal/config/size_test.go.</summary>
public class ByteSizeTests
{
    [Theory]
    [InlineData("100MB", 100L * 1024 * 1024)]
    [InlineData("1GB", 1024L * 1024 * 1024)]
    [InlineData("1TB", 1024L * 1024 * 1024 * 1024)]
    [InlineData("500KB", 500L * 1024)]
    [InlineData("10B", 10L)]
    [InlineData("100mb", 100L * 1024 * 1024)]    // case insensitive
    [InlineData("1gb", 1024L * 1024 * 1024)]
    [InlineData("1Tb", 1024L * 1024 * 1024 * 1024)]
    [InlineData("1024", 1024L)]                  // plain integer = bytes
    [InlineData("0", 0L)]
    [InlineData(" 100MB ", 100L * 1024 * 1024)]  // outer whitespace
    [InlineData("100 MB", 100L * 1024 * 1024)]   // inner whitespace
    public void Parse_Valid(string input, long expected)
    {
        Assert.Equal(expected, ByteSize.Parse(input));
    }

    [Theory]
    [InlineData("")]
    [InlineData("abc")]
    [InlineData("MB")]
    [InlineData("-1GB")]
    [InlineData("-100")]
    public void Parse_Invalid_Throws(string input)
    {
        Assert.Throws<FormatException>(() => ByteSize.Parse(input));
    }

    [Theory]
    [InlineData(0L, "0 B")]
    [InlineData(512L, "512 B")]
    [InlineData(1024L, "1.0 KB")]
    [InlineData(1048576L, "1.0 MB")]
    [InlineData(1073741824L, "1.0 GB")]
    [InlineData(1099511627776L, "1.0 TB")]
    public void Format_Matches(long input, string expected)
    {
        Assert.Equal(expected, ByteSize.Format(input));
    }
}
