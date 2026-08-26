namespace Rstash.Server;

/// <summary>Source-generated log messages for TLS certificate handling.</summary>
internal static partial class TlsLog
{
    [LoggerMessage(
        EventId = 1000,
        Level = LogLevel.Warning,
        Message = "Could not reload the TLS certificate from {CertificatePath}; "
            + "continuing with the one already loaded.")]
    public static partial void CertificateReloadFailed(
        ILogger logger,
        Exception exception,
        string? certificatePath);
}
