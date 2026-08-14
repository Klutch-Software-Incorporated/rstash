using Rstash.Services.Configuration;

namespace Rstash.Core.Tests;

/// <summary>
/// RSTASH_BASE_URL is the origin every absolute URL rstash emits is built from —
/// WebFinger hrefs, OAuth redirects, password-reset links, and (once OIDC lands) the
/// token issuer. A malformed value does not fail loudly at the point of use; it ships
/// links nobody can follow, so it is validated at boot instead.
/// </summary>
public sealed class BaseUrlTests
{
    [Fact]
    public void Resolve_AppliesTheDefault_WhenUnset()
    {
        Assert.Equal(BaseUrl.Default, BaseUrl.Resolve(null));
        Assert.Equal(BaseUrl.Default, BaseUrl.Resolve(""));
        Assert.Equal(BaseUrl.Default, BaseUrl.Resolve("   "));
    }

    [Theory]
    [InlineData("https://rstash.example.com/", "https://rstash.example.com")]
    [InlineData("https://rstash.example.com///", "https://rstash.example.com")]
    [InlineData("  https://rstash.example.com  ", "https://rstash.example.com")]
    public void Resolve_StripsTrailingSlashesAndWhitespace(string configured, string expected) =>
        Assert.Equal(expected, BaseUrl.Resolve(configured));

    [Theory]
    [InlineData("http://localhost:8080")]
    [InlineData("https://rstash.cloud")]
    [InlineData("https://rstash.example.com:8443")]
    public void TryValidate_AcceptsAnHttpOrigin(string value)
    {
        Assert.True(BaseUrl.TryValidate(value, out var error));
        Assert.Null(error);
    }

    [Theory]
    [InlineData("rstash.cloud")]              // the common mistake: no scheme
    [InlineData("/storage")]
    [InlineData("ftp://rstash.cloud")]
    [InlineData("https://rstash.cloud/base")] // a path segment corrupts an OIDC issuer
    [InlineData("https://rstash.cloud?a=b")]
    [InlineData("https://rstash.cloud#frag")]
    public void TryValidate_RejectsAnythingThatIsNotABareOrigin(string value)
    {
        Assert.False(BaseUrl.TryValidate(value, out var error));
        Assert.NotNull(error);
    }

    [Fact]
    public void ResolveOrThrow_NamesTheEnvVarAndTheReason()
    {
        var ex = Assert.Throws<InvalidOperationException>(() => BaseUrl.ResolveOrThrow("rstash.cloud"));

        Assert.Contains(EnvVars.BaseUrl, ex.Message);
        Assert.Contains("rstash.cloud", ex.Message);
        Assert.Contains("absolute URL", ex.Message);
    }

    [Fact]
    public void ResolveOrThrow_AcceptsTheBuiltInDefault() =>
        Assert.Equal(BaseUrl.Default, BaseUrl.ResolveOrThrow(null));

    [Theory]
    // The port is the point: an app resolves WebFinger against exactly the authority it
    // is handed, so a storage address missing ':8080' points at port 80 and finds nothing.
    [InlineData("http://localhost:8080", "localhost:8080")]
    [InlineData("https://rstash.example.org:8443", "rstash.example.org:8443")]
    // Default ports are implied by the scheme and would be noise in the address.
    [InlineData("https://rstash.example.org", "rstash.example.org")]
    [InlineData("http://rstash.example.org:80", "rstash.example.org")]
    [InlineData("https://rstash.example.org:443", "rstash.example.org")]
    public void AddressHost_KeepsThePortUnlessItIsTheSchemeDefault(string baseUrl, string expected) =>
        Assert.Equal(expected, BaseUrl.AddressHost(baseUrl));

    [Fact]
    public void AddressHost_MatchesTheDefaultDevelopmentOrigin() =>
        Assert.Equal("localhost:8080", BaseUrl.AddressHost(BaseUrl.Resolve(null)));
}
