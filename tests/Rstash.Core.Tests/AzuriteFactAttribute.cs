using System.Net.Sockets;

namespace Rstash.Core.Tests;

/// <summary>
/// A <see cref="FactAttribute"/> that self-skips when the Azurite storage
/// emulator isn't reachable at <c>127.0.0.1:10000</c>. Lets the Azure Blob
/// round-trip tests run locally/CI when an emulator is up, and report an honest
/// "Skipped" (not a false pass) when it isn't. The object-store backends share
/// this gate.
/// </summary>
public sealed class AzuriteFactAttribute : FactAttribute
{
    public AzuriteFactAttribute()
    {
        if (!AzuriteEmulator.IsReachable)
        {
            Skip = $"Azurite emulator not reachable at {AzuriteEmulator.Host}:{AzuriteEmulator.BlobPort}.";
        }
    }
}

/// <summary>Connection details for the well-known Azurite blob emulator.</summary>
public static class AzuriteEmulator
{
    public const string Host = "127.0.0.1";
    public const int BlobPort = 10000;
    public const string Account = "devstoreaccount1";

    // The fixed key Azurite ships with — not a secret, documented by Microsoft.
    public const string Key =
        "Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==";

    public static string BlobDsn(string container) =>
        $"azureblob://{Account}/{container}?endpoint={Host}:{BlobPort}&tls=false&key={Key}";

    private static readonly Lazy<bool> Reachable = new(Probe);

    public static bool IsReachable => Reachable.Value;

    private static bool Probe()
    {
        try
        {
            using var client = new TcpClient();
            return client.ConnectAsync(Host, BlobPort).Wait(TimeSpan.FromMilliseconds(500))
                && client.Connected;
        }
        catch
        {
            return false;
        }
    }
}
