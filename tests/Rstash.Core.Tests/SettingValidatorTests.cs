using Rstash.Services.Configuration;

namespace Rstash.Core.Tests;

public class SettingValidatorTests
{
    [Theory]
    [InlineData("registration_mode", "open")]
    [InlineData("registration_mode", "closed")]
    [InlineData("max_upload_size", "100MB")]
    [InlineData("rate_limit_rate", "2.5")]      // fractional (NumberStep has '.')
    [InlineData("rate_limit_burst", "20")]      // integer
    [InlineData("token_lifetime", "30d")]
    [InlineData("token_lifetime", "0")]
    public void Validate_Accepts(string key, string value)
    {
        SettingValidator.Validate(key, value); // must not throw
    }

    [Theory]
    [InlineData("nonexistent_key", "x")]              // unknown
    [InlineData("addr", ":9090")]                     // not runtime-editable
    [InlineData("registration_mode", "bogus")]        // bad select value
    [InlineData("rate_limit_burst", "-1")]            // negative integer
    [InlineData("rate_limit_burst", "1.5")]           // non-integer
    [InlineData("max_upload_size", "0")]              // zero upload forbidden
    [InlineData("max_upload_size", "abc")]            // bad size
    [InlineData("token_lifetime", "nope")]            // bad duration
    public void Validate_Rejects(string key, string value)
    {
        Assert.Throws<SettingValidationException>(() => SettingValidator.Validate(key, value));
    }
}
