using System.Diagnostics.CodeAnalysis;

namespace Rstash.Services.Configuration;

/// <summary>How rstash obtains the certificate it serves HTTPS with.</summary>
public enum TlsMode
{
    /// <summary>Serve plain HTTP. The default, and the right answer behind a TLS-terminating proxy.</summary>
    Off,

    /// <summary>Serve HTTPS using a certificate and key the operator keeps on disk.</summary>
    Files,
}

/// <summary>
/// Resolves and validates the <c>RSTASH_TLS_*</c> family into a decision Kestrel can act on.
/// </summary>
/// <remarks>
/// <para>
/// rstash does not obtain certificates itself. .NET ships no ACME client — not in the BCL,
/// not in any Microsoft-maintained package — so embedding one means adopting a third-party
/// dependency and owning renewal correctness under certificate lifetimes that keep
/// shrinking. <see cref="TlsMode.Files"/> instead consumes what an external renewer already
/// produces: certbot, acme.sh, Caddy, <c>tailscale cert</c>, and internal CAs all write a
/// PEM certificate and key to disk, and rstash picks up changes to them in place.
/// </para>
/// <para>
/// A misconfiguration here has to be loud. Earlier builds advertised these settings while
/// reading none of them, so an operator could set a certificate path, see no error, and
/// serve plaintext believing otherwise. Every failure below is therefore a boot-time throw
/// rather than a quiet fallback to HTTP.
/// </para>
/// </remarks>
public sealed record TlsOptions(TlsMode Mode, string? CertificatePath, string? KeyPath)
{
    /// <summary>Plain HTTP.</summary>
    public static readonly TlsOptions Disabled = new(TlsMode.Off, null, null);

    /// <summary>Values accepted by <c>RSTASH_TLS_MODE</c>; empty means auto-detect.</summary>
    public static readonly string[] ValidModes = ["", "off", "files"];

    /// <summary>True when Kestrel should bind an HTTPS endpoint.</summary>
    public bool Enabled => Mode != TlsMode.Off;

    /// <summary>
    /// The certificate and key paths, which every enabled mode is guaranteed to carry
    /// because <see cref="TryResolve"/> refuses to produce an enabled result without them.
    /// </summary>
    public (string Certificate, string Key) RequirePaths() =>
        Mode != TlsMode.Off && CertificatePath is not null && KeyPath is not null
            ? (CertificatePath, KeyPath)
            : throw new InvalidOperationException($"TLS is not enabled ({EnvVars.TlsMode} resolved to '{Mode}').");

    /// <summary>
    /// Applies <c>RSTASH_TLS_MODE</c> against the configured certificate paths. An empty
    /// mode auto-detects: supplying both a certificate and a key turns TLS on, supplying
    /// neither leaves it off.
    /// </summary>
    public static bool TryResolve(
        string? mode,
        string? certificatePath,
        string? keyPath,
        out TlsOptions options,
        [NotNullWhen(false)] out string? error)
    {
        options = Disabled;
        var cert = Trimmed(certificatePath);
        var key = Trimmed(keyPath);

        switch (Trimmed(mode)?.ToLowerInvariant())
        {
            case null:
                // Auto-detect. Half a pair is always a mistake worth stopping for: it reads
                // as "TLS configured" to the operator and as "off" to the server.
                if (cert is null && key is null)
                {
                    error = null;
                    return true;
                }

                if (cert is null || key is null)
                {
                    error = $"set both {EnvVars.TlsCert} and {EnvVars.TlsKey} to enable TLS, or neither to serve "
                        + $"plain HTTP (only {(cert is null ? EnvVars.TlsKey : EnvVars.TlsCert)} is set)";
                    return false;
                }

                options = new TlsOptions(TlsMode.Files, cert, key);
                error = null;
                return true;

            case "off":
                error = null;
                return true;

            case "files":
                if (cert is null || key is null)
                {
                    error = $"mode 'files' requires both {EnvVars.TlsCert} and {EnvVars.TlsKey} "
                        + $"(missing {(cert is null ? EnvVars.TlsCert : EnvVars.TlsKey)})";
                    return false;
                }

                options = new TlsOptions(TlsMode.Files, cert, key);
                error = null;
                return true;

            // The Go build accepted these, where 'auto' meant autocert-managed Let's Encrypt.
            // Neither exists here, and silently downgrading 'auto' to plain HTTP would be the
            // exact failure this type exists to prevent.
            case "manual":
                error = "mode 'manual' is now called 'files'";
                return false;

            case "auto":
                error = "mode 'auto' (automatic Let's Encrypt certificates) is not supported. Renew with "
                    + "certbot, acme.sh, or Caddy and point rstash at the resulting files with mode 'files' "
                    + "— renewals are picked up without a restart";
                return false;

            default:
                error = $"must be one of: {string.Join(", ", ValidModes.Where(v => v.Length > 0))}";
                return false;
        }
    }

    /// <summary>Resolve + validate for the boot path, where a bad value must stop the server.</summary>
    public static TlsOptions ResolveOrThrow(string? mode, string? certificatePath, string? keyPath)
    {
        if (!TryResolve(mode, certificatePath, keyPath, out var options, out var error))
        {
            throw new InvalidOperationException($"{EnvVars.TlsMode} is invalid: {error}.");
        }

        return options;
    }

    private static string? Trimmed(string? value) =>
        string.IsNullOrWhiteSpace(value) ? null : value.Trim();
}
