namespace Rstash.Server;

/// <summary>Source-generated log messages for server startup.</summary>
internal static partial class ServerLog
{
    // Logged unconditionally at boot so the version is answerable from a *running* server —
    // a debugger session, or a container someone else started — and not only from the CLI.
    [LoggerMessage(
        EventId = 1100,
        Level = LogLevel.Information,
        Message = "rstash {Version} listening on {ListenUrl}")]
    public static partial void Starting(ILogger logger, string version, string listenUrl);
}
