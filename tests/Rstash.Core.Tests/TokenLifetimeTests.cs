using Rstash.Services.Configuration;

namespace Rstash.Core.Tests;

public class TokenLifetimeTests
{
    [Theory]
    [InlineData("", 0)]
    [InlineData("0", 0)]
    [InlineData("720h", 720 * 3600)]
    [InlineData("30m", 30 * 60)]
    [InlineData("1h30m", (3600 + 30 * 60))]
    [InlineData("30d", 30 * 24 * 3600)]
    [InlineData("90s", 90)]
    public void Parse_Valid(string input, int expectedSeconds)
    {
        Assert.Equal(TimeSpan.FromSeconds(expectedSeconds), TokenLifetime.Parse(input));
    }

    [Theory]
    [InlineData("abc")]
    [InlineData("-1h")]
    [InlineData("-30d")]
    [InlineData("12x")]
    public void Parse_Invalid_Throws(string input)
    {
        Assert.Throws<FormatException>(() => TokenLifetime.Parse(input));
    }
}
