using Rstash.Services.Configuration;

namespace Rstash.Core.Tests;

/// <summary>
/// The TLS settings shipped for a full release advertising "set these to enable TLS" while
/// nothing read them — an operator could configure a certificate, see no error anywhere, and
/// serve plaintext believing otherwise. These tests pin the rule that replaced that: every
/// ambiguous or unsupported combination fails loudly rather than quietly resolving to HTTP.
/// </summary>
public sealed class TlsOptionsTests
{
    [Theory]
    [InlineData(null)]
    [InlineData("")]
    [InlineData("   ")]
    public void Resolve_IsOff_WhenNothingIsConfigured(string? mode)
    {
        Assert.True(TlsOptions.TryResolve(mode, null, null, out var options, out var error));
        Assert.Null(error);
        Assert.Equal(TlsMode.Off, options.Mode);
        Assert.False(options.Enabled);
    }

    [Fact]
    public void Resolve_AutoDetects_WhenBothPathsAreSet()
    {
        Assert.True(TlsOptions.TryResolve(null, "cert.pem", "key.pem", out var options, out var error));

        Assert.Null(error);
        Assert.Equal(TlsMode.Files, options.Mode);
        Assert.True(options.Enabled);
        Assert.Equal(("cert.pem", "key.pem"), options.RequirePaths());
    }

    [Theory]
    [InlineData("cert.pem", null)]
    [InlineData("cert.pem", "  ")]
    [InlineData(null, "key.pem")]
    public void Resolve_Fails_OnHalfAConfiguredPair(string? cert, string? key)
    {
        // The dangerous case: it reads as "TLS configured" to whoever set it and as "off"
        // to the server, which is precisely how plaintext gets served by accident.
        Assert.False(TlsOptions.TryResolve(null, cert, key, out var options, out var error));

        Assert.False(options.Enabled);
        Assert.Contains(cert is null ? EnvVars.TlsKey : EnvVars.TlsCert, error);
    }

    [Fact]
    public void Resolve_HonoursAnExplicitOff_EvenWithPathsSet()
    {
        Assert.True(TlsOptions.TryResolve("off", "cert.pem", "key.pem", out var options, out _));

        Assert.Equal(TlsMode.Off, options.Mode);
        Assert.False(options.Enabled);
    }

    [Theory]
    [InlineData("files")]
    [InlineData("FILES")]
    [InlineData("  Files  ")]
    public void Resolve_AcceptsFiles_CaseAndWhitespaceInsensitively(string mode)
    {
        Assert.True(TlsOptions.TryResolve(mode, " cert.pem ", " key.pem ", out var options, out _));

        Assert.Equal(TlsMode.Files, options.Mode);
        Assert.Equal(("cert.pem", "key.pem"), options.RequirePaths());
    }

    [Theory]
    [InlineData(null, "key.pem", EnvVars.TlsCert)]
    [InlineData("cert.pem", null, EnvVars.TlsKey)]
    [InlineData(null, null, EnvVars.TlsCert)]
    public void Resolve_Fails_WhenFilesModeIsMissingAPath(string? cert, string? key, string expected)
    {
        Assert.False(TlsOptions.TryResolve("files", cert, key, out var options, out var error));

        Assert.False(options.Enabled);
        Assert.Contains(expected, error);
    }

    [Fact]
    public void Resolve_Rejects_TheGoBuildsManualMode_WithItsNewName()
    {
        Assert.False(TlsOptions.TryResolve("manual", "cert.pem", "key.pem", out _, out var error));
        Assert.Contains("files", error);
    }

    [Fact]
    public void Resolve_Rejects_TheGoBuildsAutoMode_RatherThanSilentlyServingPlaintext()
    {
        // 'auto' meant autocert-managed Let's Encrypt. Treating it as "off" would leave an
        // upgrading operator on plain HTTP with no indication anything had changed.
        Assert.False(TlsOptions.TryResolve("auto", null, null, out var options, out var error));

        Assert.False(options.Enabled);
        Assert.Contains("certbot", error);
    }

    [Theory]
    [InlineData("on")]
    [InlineData("true")]
    [InlineData("acme")]
    public void Resolve_Rejects_UnknownModes_AndNamesTheValidOnes(string mode)
    {
        Assert.False(TlsOptions.TryResolve(mode, null, null, out _, out var error));

        Assert.Contains("off", error);
        Assert.Contains("files", error);
    }

    [Fact]
    public void ResolveOrThrow_NamesTheSettingAtFault()
    {
        var ex = Assert.Throws<InvalidOperationException>(
            () => TlsOptions.ResolveOrThrow("files", "cert.pem", null));

        Assert.Contains(EnvVars.TlsMode, ex.Message);
        Assert.Contains(EnvVars.TlsKey, ex.Message);
    }

    [Fact]
    public void RequirePaths_Throws_WhenTlsIsOff() =>
        Assert.Throws<InvalidOperationException>(() => TlsOptions.Disabled.RequirePaths());

    [Fact]
    public void ValidModes_MatchTheAdminUiChoices()
    {
        // The select in the admin UI and the parser here must not drift apart: a mode the UI
        // offers but the parser rejects would fail the server at its next restart.
        var registered = SettingDefinitions.ByKey("tls_mode");

        Assert.NotNull(registered);
        Assert.Equal(TlsOptions.ValidModes, registered.ValidValues);
    }
}
