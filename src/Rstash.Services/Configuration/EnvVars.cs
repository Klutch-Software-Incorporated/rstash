namespace Rstash.Services.Configuration;

/// <summary>
/// Environment-variable names for boot-critical settings — the only settings
/// with env support. Everything else uses defaults and is runtime-managed.
/// </summary>
public static class EnvVars
{
    public const string Addr = "RSTASH_ADDR";
    public const string BaseUrl = "RSTASH_BASE_URL";
    public const string TrustProxy = "RSTASH_TRUST_PROXY";
    public const string Database = "RSTASH_DB";
    public const string Blob = "RSTASH_BLOB";
    public const string LogLevel = "RSTASH_LOG_LEVEL";
    public const string LogFile = "RSTASH_LOG_FILE";
    public const string TlsCert = "RSTASH_TLS_CERT";
    public const string TlsKey = "RSTASH_TLS_KEY";
    public const string TlsMode = "RSTASH_TLS_MODE";
    public const string Email = "RSTASH_EMAIL";
}
