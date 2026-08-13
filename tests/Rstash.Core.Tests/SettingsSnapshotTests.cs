using Rstash.Services;

namespace Rstash.Core.Tests;

public class SettingsSnapshotTests
{
    [Fact]
    public void Resolve_Null_UsesRegistryDefaults()
    {
        var snap = SettingsSnapshot.Resolve(null);

        Assert.Equal("closed", snap.RegistrationMode);
        Assert.Equal(10, snap.RateLimitRate);
        Assert.Equal(20, snap.RateLimitBurst);
        Assert.Equal(50L * 1024 * 1024, snap.MaxUploadSize);
        Assert.Equal(0, snap.TotalStorageLimit);
        Assert.Equal("30d", snap.TokenLifetime);
        Assert.Equal("90d", snap.RefreshTokenLifetime);
        Assert.Equal("rstash", snap.SiteName);
    }

    [Fact]
    public void Resolve_AppliesOverrides()
    {
        var overrides = new Dictionary<string, string>
        {
            ["registration_mode"] = "open",
            ["max_upload_size"] = "100MB",
            ["rate_limit_rate"] = "2.5",
            ["site_name"] = "My Cloud",
        };

        var snap = SettingsSnapshot.Resolve(overrides);

        Assert.Equal("open", snap.RegistrationMode);
        Assert.Equal(100L * 1024 * 1024, snap.MaxUploadSize);
        Assert.Equal(2.5, snap.RateLimitRate);
        Assert.Equal("My Cloud", snap.SiteName);
    }

    [Fact]
    public void Resolve_MalformedOverride_FallsBackToDefault()
    {
        var overrides = new Dictionary<string, string> { ["max_upload_size"] = "garbage" };

        var snap = SettingsSnapshot.Resolve(overrides);

        Assert.Equal(50L * 1024 * 1024, snap.MaxUploadSize);
    }

    /// <summary>
    /// "external" delegated registration to another service and was retired with
    /// the hosted plans. A server still holding it must land on the safe default,
    /// not on a value nothing recognises: the registration page only special-cases
    /// "closed", so an unrecognised mode renders an open signup form on a server
    /// whose operator had deliberately turned signups off.
    /// </summary>
    [Fact]
    public void Resolve_RetiredChoice_FallsBackToDefault()
    {
        var overrides = new Dictionary<string, string> { ["registration_mode"] = "external" };

        Assert.Equal("closed", SettingsSnapshot.Resolve(overrides).RegistrationMode);
    }

    [Theory]
    [InlineData("registration_mode", "closed")]
    [InlineData("metrics_mode", "public")]
    [InlineData("log_level", "info")]
    [InlineData("rate_limit", "enabled")]
    [InlineData("allow_public_writes", "enabled")]
    public void Resolve_UnknownChoice_FallsBackToDefault(string key, string expected)
    {
        var overrides = new Dictionary<string, string> { [key] = "no-such-value" };
        var snap = SettingsSnapshot.Resolve(overrides);

        var actual = key switch
        {
            "registration_mode" => snap.RegistrationMode,
            "metrics_mode" => snap.MetricsMode,
            "log_level" => snap.LogLevel,
            "rate_limit" => snap.RateLimit ? "enabled" : "disabled",
            "allow_public_writes" => snap.AllowPublicWrites ? "enabled" : "disabled",
            _ => throw new ArgumentOutOfRangeException(nameof(key)),
        };

        Assert.Equal(expected, actual);
    }
}
