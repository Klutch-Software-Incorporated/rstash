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
        Assert.Equal("rstash", snap.SiteName);
        Assert.Empty(snap.CustomLinks);
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
            ["custom_links"] = "[{\"label\":\"Billing\",\"url\":\"https://billing.example.com\"}]",
        };

        var snap = SettingsSnapshot.Resolve(overrides);

        Assert.Equal("open", snap.RegistrationMode);
        Assert.Equal(100L * 1024 * 1024, snap.MaxUploadSize);
        Assert.Equal(2.5, snap.RateLimitRate);
        Assert.Equal("My Cloud", snap.SiteName);
        Assert.Single(snap.CustomLinks);
        Assert.Equal("Billing", snap.CustomLinks[0].Label);
    }

    [Fact]
    public void Resolve_MalformedOverride_FallsBackToDefault()
    {
        var overrides = new Dictionary<string, string> { ["max_upload_size"] = "garbage" };

        var snap = SettingsSnapshot.Resolve(overrides);

        Assert.Equal(50L * 1024 * 1024, snap.MaxUploadSize);
    }
}
