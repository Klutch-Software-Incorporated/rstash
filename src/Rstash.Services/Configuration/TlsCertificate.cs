using System.Security.Cryptography;
using System.Security.Cryptography.X509Certificates;

namespace Rstash.Services.Configuration;

/// <summary>
/// Serves the operator's PEM certificate to Kestrel, re-reading it from disk after an
/// external renewer replaces it.
/// </summary>
/// <remarks>
/// <para>
/// Reloading is the point. Let's Encrypt's <c>tlsserver</c> profile is 45 days and its
/// <c>shortlived</c> profile is under seven, and the CA/Browser Forum baseline drops the
/// ceiling to 100 days in March 2027 and 47 days in March 2029. A certificate read once at
/// startup would mean a restart every renewal, forever. Instead the file's timestamp is
/// checked on handshake (at most once per <see cref="DefaultCheckInterval"/>) and a changed
/// file is picked up in place.
/// </para>
/// <para>
/// Timestamps rather than <c>FileSystemWatcher</c>: renewals land through symlink swaps
/// (certbot's <c>live/</c> directory), bind mounts, and network shares, and watchers are
/// unreliable across all three. Polling a stat on an already-established interval is dull
/// and works everywhere.
/// </para>
/// </remarks>
public sealed class TlsCertificate : IDisposable
{
    /// <summary>How often the files are stat'd, at most. Renewals are not urgent to the minute.</summary>
    public static readonly TimeSpan DefaultCheckInterval = TimeSpan.FromMinutes(5);

    private readonly string _certificatePath;
    private readonly string _keyPath;
    private readonly TimeProvider _time;
    private readonly TimeSpan _checkInterval;
    private readonly Action<Exception>? _onReloadFailure;
    private readonly Lock _gate = new();

    private X509Certificate2 _current;
    private (DateTime Certificate, DateTime Key) _loadedStamps;
    private long _nextCheckTicks;
    private bool _disposed;

    /// <param name="onReloadFailure">
    /// Invoked when a reload attempt fails. A half-written file during renewal is normal and
    /// transient, so failures keep the previous certificate in service rather than throwing
    /// mid-handshake; the callback exists so the host can log what it kept and why.
    /// </param>
    public TlsCertificate(
        string certificatePath,
        string keyPath,
        TimeProvider? timeProvider = null,
        TimeSpan? checkInterval = null,
        Action<Exception>? onReloadFailure = null)
    {
        _certificatePath = certificatePath;
        _keyPath = keyPath;
        _time = timeProvider ?? TimeProvider.System;
        _checkInterval = checkInterval ?? DefaultCheckInterval;
        _onReloadFailure = onReloadFailure;

        // Load eagerly: a bad path or an unreadable key is a configuration error, and it
        // should stop the server at boot rather than surface as a failed TLS handshake.
        _current = Load(certificatePath, keyPath);
        _loadedStamps = Stamps();
        _nextCheckTicks = _time.GetUtcNow().Ticks + _checkInterval.Ticks;
    }

    /// <summary>The certificate to present, reloaded first if the files have changed.</summary>
    public X509Certificate2 Current
    {
        get
        {
            ObjectDisposedException.ThrowIf(_disposed, this);

            var now = _time.GetUtcNow().Ticks;
            if (now < Interlocked.Read(ref _nextCheckTicks))
            {
                return _current;
            }

            lock (_gate)
            {
                Interlocked.Exchange(ref _nextCheckTicks, now + _checkInterval.Ticks);

                try
                {
                    var stamps = Stamps();
                    if (stamps != _loadedStamps)
                    {
                        // The previous instance is deliberately not disposed: a handshake in
                        // flight may still hold it, and leaking one certificate handle per
                        // renewal — a few times a year — is cheaper than a use-after-dispose.
                        _current = Load(_certificatePath, _keyPath);
                        _loadedStamps = stamps;
                    }
                }
                catch (Exception ex)
                {
                    _onReloadFailure?.Invoke(ex);
                }

                return _current;
            }
        }
    }

    /// <summary>
    /// Reads a PEM certificate and private key, failing with a message that names the
    /// setting at fault rather than a bare cryptographic error.
    /// </summary>
    public static X509Certificate2 Load(string certificatePath, string keyPath)
    {
        RequireFile(certificatePath, EnvVars.TlsCert);
        RequireFile(keyPath, EnvVars.TlsKey);

        X509Certificate2 certificate;
        try
        {
            certificate = X509Certificate2.CreateFromPemFile(certificatePath, keyPath);
        }
        catch (Exception ex) when (ex is UnauthorizedAccessException or IOException)
        {
            // The common Linux failure. certbot writes privkey.pem readable only by root, so
            // an rstash running as its own user gets here rather than anywhere informative.
            throw new InvalidOperationException(
                $"{EnvVars.TlsCert} ('{certificatePath}') and {EnvVars.TlsKey} ('{keyPath}') exist but could not be "
                + $"read: {ex.Message}. Check that the user rstash runs as can read both files.", ex);
        }
        catch (Exception ex) when (ex is CryptographicException or ArgumentException)
        {
            throw new InvalidOperationException(
                $"{EnvVars.TlsCert} ('{certificatePath}') and {EnvVars.TlsKey} ('{keyPath}') could not be read as a "
                + $"PEM certificate and private key: {ex.Message}", ex);
        }

        if (!OperatingSystem.IsWindows())
        {
            return certificate;
        }

        // Windows will not use a PEM-loaded key for server authentication: SChannel needs the
        // key associated through a certificate store, and the ephemeral key CreateFromPemFile
        // produces is not. Round-tripping through PKCS#12 makes the pair usable. Without this,
        // TLS binds cleanly at boot and then fails every handshake at runtime.
        try
        {
            return X509CertificateLoader.LoadPkcs12(certificate.Export(X509ContentType.Pfx), password: null);
        }
        finally
        {
            certificate.Dispose();
        }
    }

    public void Dispose()
    {
        if (_disposed)
        {
            return;
        }

        _disposed = true;
        _current.Dispose();
    }

    private static void RequireFile(string path, string envVar)
    {
        // Not just "missing": on Linux this also fires when the file is there but a parent
        // directory denies traversal, which is certbot's default (live/ and archive/ are
        // root-only). Reporting it as absent would send the operator hunting the wrong problem.
        if (!File.Exists(path))
        {
            throw new FileNotFoundException(
                $"{envVar} points at '{path}', which does not exist or is not readable by this user.", path);
        }
    }

    private (DateTime Certificate, DateTime Key) Stamps() =>
        (File.GetLastWriteTimeUtc(_certificatePath), File.GetLastWriteTimeUtc(_keyPath));
}
