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
    [InlineData("custom_links", "")]
    [InlineData("custom_links", "[{\"label\":\"Help\",\"url\":\"https://example.com\"}]")]
    [InlineData("custom_links", "[{\"label\":\"Home\",\"url\":\"/profile\"}]")]
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
    [InlineData("custom_links", "not json")]          // malformed JSON
    [InlineData("custom_links", "[{\"label\":\"\",\"url\":\"https://x.com\"}]")]   // missing label
    [InlineData("custom_links", "[{\"label\":\"X\",\"url\":\"http://x.com\"}]")]   // non-https
    public void Validate_Rejects(string key, string value)
    {
        Assert.Throws<SettingValidationException>(() => SettingValidator.Validate(key, value));
    }
}
