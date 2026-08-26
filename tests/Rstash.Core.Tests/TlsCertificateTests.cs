using System.Diagnostics.CodeAnalysis;
using System.Net;
using System.Net.Security;
using System.Net.Sockets;
using System.Security.Cryptography;
using System.Security.Cryptography.X509Certificates;
using Rstash.Services.Configuration;

namespace Rstash.Core.Tests;

/// <summary>
/// Covers the two things that decide whether HTTPS actually works: the certificate has to be
/// usable for server authentication on the platform rstash is running on, and a renewal
/// written by certbot has to take effect without a restart.
/// </summary>
public sealed class TlsCertificateTests : IDisposable
{
    private readonly string _directory = Directory.CreateTempSubdirectory("rstash-tls-").FullName;

    [Fact]
    public void Load_ReadsAPemPair_AndKeepsThePrivateKey()
    {
        var (certificatePath, keyPath) = WritePair();

        using var certificate = TlsCertificate.Load(certificatePath, keyPath);

        Assert.True(certificate.HasPrivateKey);
        Assert.Contains("rstash-test", certificate.Subject);
    }

    [Fact]
    public async Task Load_ProducesACertificateThatCompletesATlsHandshake()
    {
        // The guard that matters on Windows: SChannel will not use the ephemeral key that
        // CreateFromPemFile produces for server auth, so without the PKCS#12 round-trip in
        // Load the server binds cleanly at boot and then fails every single handshake.
        var (certificatePath, keyPath) = WritePair();
        using var certificate = TlsCertificate.Load(certificatePath, keyPath);

        var negotiated = await HandshakeAsync(certificate);

        Assert.Equal(certificate.GetCertHashString(), negotiated);
    }

    [Fact]
    public void Load_Throws_WhenTheCertificateIsMissing()
    {
        var (_, keyPath) = WritePair();
        var missing = Path.Combine(_directory, "absent.pem");

        var ex = Assert.Throws<FileNotFoundException>(() => TlsCertificate.Load(missing, keyPath));

        Assert.Contains(EnvVars.TlsCert, ex.Message);
    }

    [Fact]
    public void Load_Throws_WhenTheKeyIsMissing()
    {
        var (certificatePath, _) = WritePair();
        var missing = Path.Combine(_directory, "absent-key.pem");

        var ex = Assert.Throws<FileNotFoundException>(() => TlsCertificate.Load(certificatePath, missing));

        Assert.Contains(EnvVars.TlsKey, ex.Message);
    }

    [Fact]
    public void Load_Throws_WithBothPaths_WhenThePemCannotBeParsed()
    {
        var (certificatePath, keyPath) = WritePair();
        File.WriteAllText(certificatePath, "not a certificate");

        var ex = Assert.Throws<InvalidOperationException>(() => TlsCertificate.Load(certificatePath, keyPath));

        Assert.Contains(certificatePath, ex.Message);
        Assert.Contains(keyPath, ex.Message);
    }

    [Fact]
    public void Current_PicksUpARenewedCertificate_WithoutARestart()
    {
        var (certificatePath, keyPath) = WritePair(subject: "CN=rstash-before");
        using var reloading = new TlsCertificate(certificatePath, keyPath, checkInterval: TimeSpan.Zero);
        var original = reloading.Current.Thumbprint;

        Renew(certificatePath, keyPath, subject: "CN=rstash-after");

        Assert.NotEqual(original, reloading.Current.Thumbprint);
        Assert.Contains("rstash-after", reloading.Current.Subject);
    }

    [Fact]
    public void Current_KeepsServing_WhenAReloadFails()
    {
        // Renewal is not atomic: the certificate can be read while it is half-written. That
        // has to degrade to "keep using the old one", never to a dropped connection.
        var (certificatePath, keyPath) = WritePair();
        var failures = new List<Exception>();
        using var reloading = new TlsCertificate(
            certificatePath,
            keyPath,
            checkInterval: TimeSpan.Zero,
            onReloadFailure: failures.Add);
        var original = reloading.Current.Thumbprint;

        File.WriteAllText(certificatePath, "-----BEGIN CERTIFICATE-----\ntruncated");
        Touch(certificatePath);

        Assert.Equal(original, reloading.Current.Thumbprint);
        Assert.NotEmpty(failures);
    }

    [UnixFact]
    public void Current_FollowsASymlinkSwap_TheWayCertbotRenews()
    {
        // certbot does not rewrite the files it hands you. It writes a new pair into
        // archive/ and repoints the live/ symlinks at it, so reload detection has to see
        // through the link to the target — which is the deployment shape on Linux.
        var archive = Directory.CreateDirectory(Path.Combine(_directory, "archive")).FullName;
        var live = Directory.CreateDirectory(Path.Combine(_directory, "live")).FullName;

        var firstCertificate = Path.Combine(archive, "fullchain1.pem");
        var firstKey = Path.Combine(archive, "privkey1.pem");
        Write(firstCertificate, firstKey, subject: "CN=rstash-first", notAfter: null);

        var liveCertificate = Path.Combine(live, "fullchain.pem");
        var liveKey = Path.Combine(live, "privkey.pem");
        File.CreateSymbolicLink(liveCertificate, firstCertificate);
        File.CreateSymbolicLink(liveKey, firstKey);

        using var reloading = new TlsCertificate(liveCertificate, liveKey, checkInterval: TimeSpan.Zero);
        var original = reloading.Current.Thumbprint;

        var secondCertificate = Path.Combine(archive, "fullchain2.pem");
        var secondKey = Path.Combine(archive, "privkey2.pem");
        Write(secondCertificate, secondKey, subject: "CN=rstash-second", notAfter: null);
        Touch(secondCertificate);
        Touch(secondKey);
        File.Delete(liveCertificate);
        File.Delete(liveKey);
        File.CreateSymbolicLink(liveCertificate, secondCertificate);
        File.CreateSymbolicLink(liveKey, secondKey);

        Assert.NotEqual(original, reloading.Current.Thumbprint);
        Assert.Contains("rstash-second", reloading.Current.Subject);
    }

    [Fact]
    public void Current_DoesNotReload_BeforeTheCheckIntervalElapses()
    {
        var (certificatePath, keyPath) = WritePair();
        using var reloading = new TlsCertificate(certificatePath, keyPath, checkInterval: TimeSpan.FromHours(1));
        var original = reloading.Current.Thumbprint;

        Renew(certificatePath, keyPath, subject: "CN=rstash-ignored");

        Assert.Equal(original, reloading.Current.Thumbprint);
    }

    public void Dispose() => Directory.Delete(_directory, recursive: true);

    /// <summary>Runs a real TLS handshake against the certificate and returns what the client saw.</summary>
    [SuppressMessage(
        "Security",
        "CA5359:Do not disable certificate validation",
        Justification = "The certificate under test is self-signed by construction. What is being "
            + "verified is that the server can complete a handshake with it at all, not that the "
            + "chain validates.")]
    private static async Task<string?> HandshakeAsync(X509Certificate2 serverCertificate)
    {
        var listener = new TcpListener(IPAddress.Loopback, 0);
        listener.Start();
        try
        {
            var port = ((IPEndPoint)listener.LocalEndpoint).Port;
            var serving = Task.Run(async () =>
            {
                using var connection = await listener.AcceptTcpClientAsync();
                await using var stream = new SslStream(connection.GetStream(), leaveInnerStreamOpen: false);
                await stream.AuthenticateAsServerAsync(serverCertificate);
            });

            string? presented = null;
            using (var client = new TcpClient())
            {
                await client.ConnectAsync(IPAddress.Loopback, port);
                await using var stream = new SslStream(
                    client.GetStream(),
                    leaveInnerStreamOpen: false,
                    (_, certificate, _, _) =>
                    {
                        presented = certificate?.GetCertHashString();
                        return true; // self-signed by construction; the chain is not what's under test
                    });
                await stream.AuthenticateAsClientAsync("localhost");
            }

            await serving;
            return presented;
        }
        finally
        {
            listener.Stop();
        }
    }

    private (string Certificate, string Key) WritePair(
        string name = "server",
        string subject = "CN=rstash-test",
        DateTimeOffset? notAfter = null)
    {
        var certificatePath = Path.Combine(_directory, $"{name}.pem");
        var keyPath = Path.Combine(_directory, $"{name}-key.pem");
        Write(certificatePath, keyPath, subject, notAfter);
        return (certificatePath, keyPath);
    }

    /// <summary>Replaces the pair in place, the way an external renewer would.</summary>
    private static void Renew(string certificatePath, string keyPath, string subject)
    {
        Write(certificatePath, keyPath, subject, notAfter: null);
        Touch(certificatePath);
        Touch(keyPath);
    }

    private static void Write(string certificatePath, string keyPath, string subject, DateTimeOffset? notAfter)
    {
        using var key = RSA.Create(2048);
        var request = new CertificateRequest(subject, key, HashAlgorithmName.SHA256, RSASignaturePadding.Pkcs1);
        request.CertificateExtensions.Add(new X509BasicConstraintsExtension(false, false, 0, critical: true));
        request.CertificateExtensions.Add(
            new X509EnhancedKeyUsageExtension([new Oid("1.3.6.1.5.5.7.3.1")], critical: false)); // serverAuth

        var subjectAlternativeName = new SubjectAlternativeNameBuilder();
        subjectAlternativeName.AddDnsName("localhost");
        request.CertificateExtensions.Add(subjectAlternativeName.Build());

        using var certificate = request.CreateSelfSigned(
            DateTimeOffset.UtcNow.AddDays(-1),
            notAfter ?? DateTimeOffset.UtcNow.AddDays(30));

        File.WriteAllText(certificatePath, certificate.ExportCertificatePem());
        File.WriteAllText(keyPath, key.ExportPkcs8PrivateKeyPem());
    }

    /// <summary>
    /// Forces a distinct last-write time. Rewriting within the filesystem's timestamp
    /// granularity would otherwise look unchanged and make the reload tests vacuous.
    /// </summary>
    private static void Touch(string path) =>
        File.SetLastWriteTimeUtc(path, DateTime.UtcNow.AddSeconds(5));
}
