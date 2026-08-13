namespace Rstash.Services.Configuration;

/// <summary>
/// Metadata for every configurable setting. Env vars are reserved for settings
/// that must be known before the database is available (listen address, DSNs,
/// TLS, log level); all others default and are runtime-managed.
/// </summary>
public static class SettingDefinitions
{
    private static readonly SettingDef[] AllDefs = BuildAll();

    private static readonly Dictionary<string, SettingDef> ByKeyMap =
        AllDefs.ToDictionary(d => d.Key, StringComparer.Ordinal);

    public static IReadOnlyList<SettingDef> All => AllDefs;

    public static IReadOnlyList<SettingDef> RuntimeEditable { get; } =
        Array.FindAll(AllDefs, d => d.RuntimeEditable);

    public static SettingDef? ByKey(string key) => ByKeyMap.GetValueOrDefault(key);

    private static SettingDef[] BuildAll() =>
    [
        // ── Server (env-only, needed at boot) ──
        new()
        {
            Key = "addr",
            EnvVar = EnvVars.Addr,
            Group = "Server",
            Label = "Listen address",
            Description = "Listen address (host:port). Host may be empty to listen on all interfaces.",
            Default = ":8080",
            InputType = SettingInputType.Text,
        },
        new()
        {
            Key = "base_url",
            EnvVar = EnvVars.BaseUrl,
            Group = "Server",
            Label = "Base URL",
            Description = "Public URL of the server. Used for WebFinger and OAuth redirects.",
            Default = "http://localhost:8080",
            InputType = SettingInputType.Text,
        },
        new()
        {
            Key = "trust_proxy",
            EnvVar = EnvVars.TrustProxy,
            Group = "Server",
            Label = "Trust reverse-proxy headers",
            Description = "Set true when running behind a reverse proxy (Caddy, nginx, Azure " +
                "Container Apps). Honours X-Forwarded-Proto/Host/For so the client IP and " +
                "scheme are the user's, not the proxy's. Leave false when rstash is exposed " +
                "directly — those headers are trivially spoofed by anyone who can reach it.",
            Default = "false",
            InputType = SettingInputType.Text,
        },
        new()
        {
            Key = "database_dsn",
            EnvVar = EnvVars.Database,
            Group = "Server",
            Label = "Database DSN",
            Description = "Metadata database DSN. sqlite:PATH (use sqlite::memory: for a clean, " +
                "wiped-on-restart in-memory database, handy for local dev), or postgres: + a " +
                "native Npgsql connection string (e.g. postgres:Host=…;Database=…;Username=…;" +
                "Ssl Mode=Require; append ;Auth=Entra for Azure managed-identity auth). " +
                "mysql:/mssql: not yet wired.",
            Default = "sqlite:rstash.sqlite",
            InputType = SettingInputType.Text,
        },
        new()
        {
            Key = "blob_dsn",
            EnvVar = EnvVars.Blob,
            Group = "Server",
            Label = "Blob store DSN",
            Description = "Blob store DSN. Supported: sqlite:, fs:, postgres:, mysql:, mssql:, s3:, azureblob:. " +
                "For an in-memory dev store use a NAMED sqlite::memory:blobs (distinct from RSTASH_DB).",
            Default = "sqlite:rstash-blobs.sqlite",
            InputType = SettingInputType.Text,
        },
        new()
        {
            Key = "tls_cert",
            EnvVar = EnvVars.TlsCert,
            Group = "Server",
            Label = "TLS certificate",
            Description = "Path to TLS certificate file. Both RSTASH_TLS_CERT and RSTASH_TLS_KEY must be set to enable TLS.",
            InputType = SettingInputType.Text,
        },
        new()
        {
            Key = "tls_key",
            EnvVar = EnvVars.TlsKey,
            Group = "Server",
            Label = "TLS private key",
            Description = "Path to TLS private key file. Both RSTASH_TLS_CERT and RSTASH_TLS_KEY must be set to enable TLS.",
            InputType = SettingInputType.Text,
        },
        new()
        {
            Key = "tls_mode",
            EnvVar = EnvVars.TlsMode,
            Group = "Server",
            Label = "TLS mode",
            Description = "TLS mode: off, manual, auto. Empty = auto-detect from TLS_CERT/TLS_KEY.",
            ValidValues = ["", "off", "manual", "auto"],
            InputType = SettingInputType.Select,
        },
        new()
        {
            Key = "tls_cache",
            EnvVar = EnvVars.TlsCache,
            Group = "Server",
            Label = "TLS cache directory",
            Description = "Directory for autocert certificate cache (used when TLS mode is auto).",
            Default = "./certs",
            InputType = SettingInputType.Text,
        },
        new()
        {
            Key = "email_dsn",
            EnvVar = EnvVars.Email,
            Group = "Server",
            Label = "Email provider",
            Description = "Email delivery DSN (e.g. resend:API_KEY?from=noreply@example.com). Empty = email disabled.",
            InputType = SettingInputType.Text,
        },

        // ── Branding (runtime-editable) ──
        new()
        {
            Key = "site_name",
            Group = "Branding",
            Label = "Site name",
            Description = "Display name shown in the header, footer, and page titles.",
            Default = "rstash",
            InputType = SettingInputType.Text,
            RuntimeEditable = true,
        },

        // ── Access (runtime-editable) ──
        new()
        {
            Key = "registration_mode",
            Group = "Access",
            Label = "Registration mode",
            Description = "Who can create new accounts: anyone (open), anyone pending your "
                + "approval, or nobody (closed — you create accounts from the admin pages).",
            Default = "closed",
            ValidValues = ["open", "approval", "closed"],
            InputType = SettingInputType.Select,
            RuntimeEditable = true,
        },

        // ── Rate Limiting (runtime-editable) ──
        new()
        {
            Key = "rate_limit",
            Group = "Rate Limiting",
            Label = "Rate limiting",
            Description = "Throttle abusive clients. Behind a reverse proxy this needs "
                + "<code>RSTASH_TRUST_PROXY=true</code>, or every request looks like it came "
                + "from the proxy and all users share one budget.",
            Default = "enabled",
            ValidValues = ["enabled", "disabled"],
            InputType = SettingInputType.Select,
            RuntimeEditable = true,
        },
        new()
        {
            Key = "auth_rate_limit_rate",
            Group = "Rate Limiting",
            Label = "Sign-in rate limit",
            Description = "Max sign-in, registration, and password-reset requests per second "
                + "per IP (0 = unlimited). Low on purpose — this is what makes password guessing "
                + "expensive. OAuth token requests are not counted here; their secrets are random "
                + "and not worth guessing.",
            Default = "0.2",
            InputType = SettingInputType.Number,
            RuntimeEditable = true,
            NumberMin = "0",
            NumberStep = "0.1",
        },
        new()
        {
            Key = "auth_rate_limit_burst",
            Group = "Rate Limiting",
            Label = "Sign-in rate limit burst",
            Description = "Attempts allowed back-to-back before sign-in throttling starts.",
            Default = "5",
            InputType = SettingInputType.Number,
            RuntimeEditable = true,
            NumberMin = "0",
        },
        new()
        {
            Key = "rate_limit_rate",
            Group = "Rate Limiting",
            Label = "Rate limit",
            Description = "Max requests per second per IP for everything else (0 = unlimited). "
                + "Static assets and health checks are never limited.",
            Default = "10",
            InputType = SettingInputType.Number,
            RuntimeEditable = true,
            NumberMin = "0",
            NumberStep = "0.1",
        },
        new()
        {
            Key = "rate_limit_burst",
            Group = "Rate Limiting",
            Label = "Rate limit burst",
            Description = "Max burst of requests allowed before throttling.",
            Default = "20",
            InputType = SettingInputType.Number,
            RuntimeEditable = true,
            NumberMin = "0",
        },
        new()
        {
            Key = "user_rate_limit_rate",
            Group = "Rate Limiting",
            Label = "Per-app rate limit",
            Description = "Max storage requests per second per connected app (0 = disabled). "
                + "Counted per app token rather than per IP, so several people behind one "
                + "home connection don't share a sync budget.",
            Default = "0",
            InputType = SettingInputType.Number,
            RuntimeEditable = true,
            NumberMin = "0",
            NumberStep = "0.1",
        },
        new()
        {
            Key = "user_rate_limit_burst",
            Group = "Rate Limiting",
            Label = "Per-app rate limit burst",
            Description = "Max burst of requests per connected app before throttling (0 = disabled).",
            Default = "20",
            InputType = SettingInputType.Number,
            RuntimeEditable = true,
            NumberMin = "0",
        },

        // ── Storage (runtime-editable) ──
        new()
        {
            Key = "total_storage_limit",
            Group = "Storage",
            Label = "Server storage limit",
            Description = "Global storage cap across all users (0 = unlimited).",
            Default = "0",
            InputType = SettingInputType.ByteSize,
            RuntimeEditable = true,
        },
        new()
        {
            Key = "default_user_storage_limit",
            Group = "Storage",
            Label = "Default user storage limit",
            Description = "Storage limit stamped onto new user accounts (0 = unlimited).",
            Default = "0",
            InputType = SettingInputType.ByteSize,
            RuntimeEditable = true,
        },
        new()
        {
            Key = "max_upload_size",
            Group = "Storage",
            Label = "Max upload size",
            Description = "Maximum file size for a single upload (e.g. 50MB).",
            Default = "50MB",
            InputType = SettingInputType.ByteSize,
            RuntimeEditable = true,
        },
        new()
        {
            Key = "allow_public_writes",
            Group = "Storage",
            Label = "Public writes",
            Description = "Whether the storage API may write or delete under <code>/public/</code>. "
                + "When disabled, public documents stay readable but apps can't add or change them — "
                + "users manage existing public files from the in-app file browser. "
                + "Limits anonymous mass-distribution of large files.",
            Default = "enabled",
            ValidValues = ["enabled", "disabled"],
            InputType = SettingInputType.Select,
            RuntimeEditable = true,
        },

        // ── Egress (runtime-editable) ──
        new()
        {
            Key = "total_egress_limit",
            Group = "Egress",
            Label = "Server egress limit",
            Description = "Global monthly download cap across all users (0 = unlimited).",
            Default = "0",
            InputType = SettingInputType.ByteSize,
            RuntimeEditable = true,
        },
        new()
        {
            Key = "default_user_egress_limit",
            Group = "Egress",
            Label = "Default user egress limit",
            Description = "Monthly download limit stamped onto new user accounts (0 = unlimited).",
            Default = "0",
            InputType = SettingInputType.ByteSize,
            RuntimeEditable = true,
        },

        // ── OAuth (runtime-editable) ──
        new()
        {
            Key = "token_lifetime",
            Group = "OAuth",
            Label = "Token lifetime",
            Description = "OAuth token expiry duration (e.g. 30d, 720h, 0 = no expiry).",
            Default = "30d",
            InputType = SettingInputType.Duration,
            RuntimeEditable = true,
        },
        new()
        {
            Key = "refresh_token_lifetime",
            Group = "OAuth",
            Label = "Refresh token lifetime",
            Description = "How long refresh tokens remain valid (e.g. 90d, 2160h, 0 = no expiry).",
            Default = "90d",
            InputType = SettingInputType.Duration,
            RuntimeEditable = true,
        },

        // ── Monitoring (runtime-editable / env-only) ──
        new()
        {
            Key = "metrics_mode",
            Group = "Monitoring",
            Label = "Metrics visibility",
            Description = "Who can access the <a href=\"/metrics\">/metrics</a> endpoint.",
            Default = "public",
            ValidValues = ["public", "admin", "off"],
            InputType = SettingInputType.Select,
            RuntimeEditable = true,
        },
        new()
        {
            Key = "log_level",
            EnvVar = EnvVars.LogLevel,
            Group = "Monitoring",
            Label = "Log level",
            Description = "Minimum severity for log output.",
            Default = "info",
            ValidValues = ["debug", "info", "warn", "error"],
            InputType = SettingInputType.Select,
            RuntimeEditable = true,
        },
        new()
        {
            Key = "log_file",
            EnvVar = EnvVars.LogFile,
            Group = "Monitoring",
            Label = "Log file",
            Description = "Path to log file. Empty = stderr only.",
            InputType = SettingInputType.Text,
        },

    ];
}
