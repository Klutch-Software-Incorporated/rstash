package config

// InputType controls how a setting renders in the admin UI.
type InputType string

const (
	InputText     InputType = "text"
	InputNumber   InputType = "number"
	InputSelect   InputType = "select"
	InputByteSize InputType = "bytesize" // renders as text, validated as byte size
	InputDuration InputType = "duration" // renders as text, validated as duration
	InputTextArea InputType = "textarea" // renders as <textarea>
)

// Environment variable names for boot-critical settings.
// These are the only settings with env var support; all others use
// sane defaults and are managed at runtime via CLI or admin UI.
const (
	EnvAddr     = "GOSILO_ADDR"
	EnvBaseURL  = "GOSILO_BASE_URL"
	EnvDB       = "GOSILO_DB"
	EnvBlob     = "GOSILO_BLOB"
	EnvWebMode  = "GOSILO_WEB_MODE"
	EnvLogLevel = "GOSILO_LOG_LEVEL"
	EnvLogFile  = "GOSILO_LOG_FILE"
	EnvTLSCert  = "GOSILO_TLS_CERT"
	EnvTLSKey   = "GOSILO_TLS_KEY"
	EnvTLSMode  = "GOSILO_TLS_MODE"
	EnvTLSCache = "GOSILO_TLS_CACHE"
)

// SettingDef describes one configurable setting — the single source of truth
// for env-file generation, admin UI rendering, CLI listing, and validation.
type SettingDef struct {
	Key             string    // internal key, e.g. "registration_mode"
	EnvVar          string    // env var name, e.g. "GOSILO_ADDR" (empty = no env var)
	Group           string    // UI grouping, e.g. "Access", "Storage"
	Label           string    // human label for grid
	Description     string    // one-line description (grid + env file)
	Help            string    // extended docs for the detail page
	Default         string    // default value as display string
	ValidValues     []string  // for selects and validation
	InputType       InputType // controls HTML input rendering
	RuntimeEditable bool      // can be changed at runtime via DB
	RequiredWhen    string    // condition (for env file comments)
	NumberMin       string    // min attribute for number inputs
	NumberStep      string    // step attribute for number inputs
}

// SettingDefs returns metadata for every configurable setting.
//
// Env vars are reserved for settings that must be known before the database
// is available (listen address, DSNs, TLS paths, log level). All other
// settings use sane defaults and can be changed at runtime via the CLI or
// admin UI.
func SettingDefs() []SettingDef {
	return []SettingDef{
		// ── Server (env-only, needed at boot) ──
		{
			Key:             "addr",
			EnvVar:          EnvAddr,
			Group:           "Server",
			Label:           "Listen address",
			Description:     "Listen address (host:port). Host may be empty to listen on all interfaces.",
			Help:            "The network address the server binds to. Set via the " + EnvAddr + " environment variable. Use the format host:port, where host can be empty to listen on all interfaces (e.g. :8080), or a specific address like 127.0.0.1:8080 to restrict to localhost. Changing this setting requires a server restart.",
			Default:         ":8080",
			InputType:       InputText,
			RuntimeEditable: false,
		},
		{
			Key:             "base_url",
			EnvVar:          EnvBaseURL,
			Group:           "Server",
			Label:           "Base URL",
			Description:     "Public URL of the server. Used for WebFinger and OAuth redirects.",
			Help:            "The externally reachable URL of your Gosilo instance, set via the " + EnvBaseURL + " environment variable. This is used to generate WebFinger responses, OAuth redirect URIs, and links in the web UI. Must include the scheme (http or https) and host. Do not include a trailing slash. If you are behind a reverse proxy, this should be the proxy's public URL, not the internal address.",
			Default:         "http://localhost:8080",
			InputType:       InputText,
			RuntimeEditable: false,
		},
		{
			Key:             "database_dsn",
			EnvVar:          EnvDB,
			Group:           "Server",
			Label:           "Database DSN",
			Description:     "Metadata database DSN. Only SQLite is supported (sqlite:path or sqlite::memory:).",
			Help:            "The Data Source Name for the metadata database, set via the " + EnvDB + " environment variable. Stores users, sessions, OAuth tokens, audit entries, and file metadata (nodes). Only SQLite is supported. Use the format sqlite:path (e.g. sqlite:gosilo.db) for a file-based database, or sqlite::memory: for an in-memory database (data lost on restart). Changing this setting requires a server restart.",
			Default:         "sqlite:gosilo.db",
			InputType:       InputText,
			RuntimeEditable: false,
		},
		{
			Key:             "blob_dsn",
			EnvVar:          EnvBlob,
			Group:           "Server",
			Label:           "Blob store DSN",
			Description:     "Blob store DSN. Supported schemes: sqlite:path, fs:/path/to/dir.",
			Help:            "The Data Source Name for the blob (file content) store, set via the " + EnvBlob + " environment variable. Two backends are supported: sqlite:path stores blobs in a SQLite database (simple, single-file), and fs:/path/to/dir stores blobs as individual files on the filesystem (better for large deployments). Changing this setting requires a server restart.",
			Default:         "sqlite:gosilo-blobs.db",
			InputType:       InputText,
			RuntimeEditable: false,
		},
		{
			Key:             "web_mode",
			EnvVar:          EnvWebMode,
			Group:           "Server",
			Label:           "Web UI mode",
			Description:     "Web UI mode. full=all routes, oauth=login+OAuth only, off=API only.",
			Help:            "Controls which web UI routes are enabled, set via the " + EnvWebMode + " environment variable. \"full\" enables all routes including the file browser, admin panel, and user settings. \"oauth\" enables only the login page and OAuth consent flow (useful when you want API-only access with OAuth). \"off\" disables all web UI routes entirely, serving only the storage API and WebFinger. Changing this setting requires a server restart.",
			Default:         "full",
			ValidValues:     []string{"full", "oauth", "off"},
			InputType:       InputSelect,
			RuntimeEditable: false,
		},
		{
			Key:             "tls_cert",
			EnvVar:          EnvTLSCert,
			Group:           "Server",
			Label:           "TLS certificate",
			Description:     "Path to TLS certificate file. Both " + EnvTLSCert + " and " + EnvTLSKey + " must be set to enable TLS.",
			Help:            "The filesystem path to a PEM-encoded TLS certificate file, set via the " + EnvTLSCert + " environment variable. When both tls_cert and tls_key are set, the server uses HTTPS instead of HTTP. If you are behind a TLS-terminating reverse proxy, leave this empty and let the proxy handle TLS. Changing this setting requires a server restart.",
			InputType:       InputText,
			RuntimeEditable: false,
		},
		{
			Key:             "tls_key",
			EnvVar:          EnvTLSKey,
			Group:           "Server",
			Label:           "TLS private key",
			Description:     "Path to TLS private key file. Both " + EnvTLSCert + " and " + EnvTLSKey + " must be set to enable TLS.",
			Help:            "The filesystem path to a PEM-encoded TLS private key file, set via the " + EnvTLSKey + " environment variable. Must be set together with tls_cert. Keep this file secure and restrict its permissions. Changing this setting requires a server restart.",
			InputType:       InputText,
			RuntimeEditable: false,
		},
		{
			Key:             "tls_mode",
			EnvVar:          EnvTLSMode,
			Group:           "Server",
			Label:           "TLS mode",
			Description:     "TLS mode: off, manual, auto. Empty = auto-detect from TLS_CERT/TLS_KEY.",
			Help:            "Controls how TLS is configured. \"off\" disables TLS (plain HTTP). \"manual\" uses the certificate and key files specified by " + EnvTLSCert + " and " + EnvTLSKey + ". \"auto\" uses Let's Encrypt autocert to automatically obtain and renew certificates. When empty (not set), the mode is auto-detected: if both TLS_CERT and TLS_KEY are set, behaves as \"manual\"; otherwise behaves as \"off\". Changing this setting requires a server restart.",
			ValidValues:     []string{"", "off", "manual", "auto"},
			InputType:       InputSelect,
			RuntimeEditable: false,
		},
		{
			Key:             "tls_cache",
			EnvVar:          EnvTLSCache,
			Group:           "Server",
			Label:           "TLS cache directory",
			Description:     "Directory for autocert certificate cache (used when TLS mode is auto).",
			Help:            "The filesystem directory where Let's Encrypt certificates are cached when using TLS mode \"auto\". The directory is created if it does not exist. Must be writable by the server process. Changing this setting requires a server restart.",
			Default:         "./certs",
			InputType:       InputText,
			RuntimeEditable: false,
		},

		// ── Access (runtime-editable, no env var) ──
		{
			Key:             "registration_mode",
			Group:           "Access",
			Label:           "Registration mode",
			Description:     "Who can create new accounts.",
			Help:            "Controls whether new users can self-register. When set to \"open\", anyone can create an account through the web UI. When set to \"approval\", anyone can register but an admin must approve the account before login is allowed. When set to \"closed\", only administrators can create accounts (via the admin panel or CLI). This setting takes effect immediately when changed at runtime.",
			Default:         "closed",
			ValidValues:     []string{"open", "approval", "closed"},
			InputType:       InputSelect,
			RuntimeEditable: true,
		},
		{
			Key:             "log_level",
			EnvVar:          EnvLogLevel,
			Group:           "Access",
			Label:           "Log level",
			Description:     "Minimum severity for log output.",
			Help:            "Controls the minimum severity of log messages written to stderr. \"debug\" is the most verbose, showing detailed request/response information. \"info\" shows normal operational messages. \"warn\" shows only warnings and errors. \"error\" shows only errors. The " + EnvLogLevel + " environment variable is read at startup for early logging; runtime changes via the admin UI take effect immediately.",
			Default:         "info",
			ValidValues:     []string{"debug", "info", "warn", "error"},
			InputType:       InputSelect,
			RuntimeEditable: true,
		},

		// ── Rate Limiting (runtime-editable, no env var) ──
		{
			Key:             "rate_limit_rate",
			Group:           "Rate Limiting",
			Label:           "Rate limit",
			Description:     "Max requests per second per IP (0 = unlimited).",
			Help:            "The maximum number of requests per second allowed from a single IP address. Uses a token-bucket algorithm: each IP gets a bucket that refills at this rate. Set to 0 to disable rate limiting entirely. When rate limiting is active, requests that exceed the limit receive a 429 Too Many Requests response. Changes take effect immediately.",
			Default:         "10",
			InputType:       InputNumber,
			RuntimeEditable: true,
			NumberMin:       "0",
			NumberStep:      "0.1",
		},
		{
			Key:             "rate_limit_burst",
			Group:           "Rate Limiting",
			Label:           "Rate limit burst",
			Description:     "Max burst of requests allowed before throttling.",
			Help:            "The maximum number of requests that can be made in a short burst before rate limiting kicks in. This is the token-bucket capacity: a client can make up to this many requests instantly, then must wait for the bucket to refill at the rate_limit_rate. Should be at least 1 when rate limiting is enabled. Changes take effect immediately.",
			Default:         "20",
			InputType:       InputNumber,
			RuntimeEditable: true,
			NumberMin:       "0",
		},

		// ── Storage (runtime-editable, no env var) ──
		{
			Key:             "quota_mode",
			Group:           "Storage",
			Label:           "Quota mode",
			Description:     "How storage quotas are enforced.",
			Help:            "Controls how storage quotas are enforced. \"off\" disables all quota checking. \"total\" enforces a single global limit across all users (configured via quota_total). \"user\" enforces per-user limits (configured via quota_user, with optional per-user overrides in the admin panel). Changes take effect immediately.",
			Default:         "total",
			ValidValues:     []string{"off", "total", "user"},
			InputType:       InputSelect,
			RuntimeEditable: true,
		},
		{
			Key:             "quota_total",
			Group:           "Storage",
			Label:           "Total quota",
			Description:     "Global storage limit across all users (e.g. 10GB, 500MB).",
			Help:            "The maximum total storage allowed across all users combined. Only enforced when quota_mode is \"total\". Accepts human-readable sizes: B, KB, MB, GB, TB (binary units, 1 KB = 1024 bytes). Examples: 50GB, 500MB, 1TB. Must be greater than 0. Changes take effect immediately.",
			Default:         "50GB",
			InputType:       InputByteSize,
			RuntimeEditable: true,
		},
		{
			Key:             "quota_user",
			Group:           "Storage",
			Label:           "Per-user quota",
			Description:     "Default per-user storage quota (e.g. 500MB, 1GB). Admin can override per user.",
			Help:            "The default storage limit for each user. Only enforced when quota_mode is \"user\". Individual users can have custom quotas set by an administrator in the admin panel, which override this default. Accepts human-readable sizes: B, KB, MB, GB, TB. Must be greater than 0. Changes take effect immediately.",
			InputType:       InputByteSize,
			RuntimeEditable: true,
		},
		{
			Key:             "max_upload_size",
			Group:           "Storage",
			Label:           "Max upload size",
			Description:     "Maximum file size for a single upload (e.g. 50MB).",
			Help:            "The maximum size of a single PUT request body (i.e. a single file upload). Requests exceeding this limit are rejected with a 413 Payload Too Large response. Accepts human-readable sizes: B, KB, MB, GB. Must be greater than 0. Consider your available memory and storage when setting this value. Changes take effect immediately.",
			Default:         "50MB",
			InputType:       InputByteSize,
			RuntimeEditable: true,
		},

		// ── Monitoring (runtime-editable / env-only) ──
		{
			Key:             "metrics_mode",
			Group:           "Monitoring",
			Label:           "Metrics visibility",
			Description:     "Who can access the /metrics endpoint.",
			Help:            "Controls access to the Prometheus /metrics endpoint. \"public\" allows unauthenticated access (suitable when metrics are scraped from a private network). \"admin\" requires an authenticated admin session. \"off\" disables the endpoint entirely (returns 404). Changes take effect immediately.",
			Default:         "public",
			ValidValues:     []string{"public", "admin", "off"},
			InputType:       InputSelect,
			RuntimeEditable: true,
		},
		{
			Key:             "log_file",
			EnvVar:          EnvLogFile,
			Group:           "Monitoring",
			Label:           "Log file",
			Description:     "Path to log file. Empty = stderr only.",
			Help:            "When set, log output is written to both stderr and the specified file. This enables the admin log viewer at /admin/logs. The file is opened in append mode and created if it does not exist. Set via the " + EnvLogFile + " environment variable. Changing this setting requires a server restart.",
			InputType:       InputText,
			RuntimeEditable: false,
		},

		// ── OAuth (runtime-editable, no env var) ──
		{
			Key:             "token_lifetime",
			Group:           "OAuth",
			Label:           "Token lifetime",
			Description:     "OAuth token expiry duration (e.g. 30d, 720h, 0 = no expiry).",
			Help:            "How long OAuth access tokens remain valid after issuance. Accepts Go duration strings (e.g. \"720h\") and a convenience \"d\" suffix for days (e.g. \"30d\" = 30 days). Set to \"0\" for tokens that never expire. Expired tokens are automatically cleaned up. Changes apply only to newly issued tokens; existing tokens keep their original expiry. Changes take effect immediately.",
			Default:         "30d",
			InputType:       InputDuration,
			RuntimeEditable: true,
		},
		{
			Key:             "refresh_tokens",
			Group:           "OAuth",
			Label:           "Refresh tokens",
			Description:     "Whether to issue refresh tokens alongside access tokens.",
			Help:            "When enabled, the token endpoint issues a refresh_token alongside each access_token. Clients can exchange a refresh token for a new access token without requiring the user to re-authorize. Refresh tokens are rotated on each use (the old token is invalidated). Changes apply only to newly issued tokens.",
			Default:         "enabled",
			ValidValues:     []string{"enabled", "disabled"},
			InputType:       InputSelect,
			RuntimeEditable: true,
		},
		{
			Key:             "refresh_token_lifetime",
			Group:           "OAuth",
			Label:           "Refresh token lifetime",
			Description:     "How long refresh tokens remain valid (e.g. 90d, 2160h, 0 = no expiry).",
			Help:            "How long OAuth refresh tokens remain valid after issuance. Accepts Go duration strings and the convenience \"d\" suffix for days (e.g. \"90d\" = 90 days). Set to \"0\" for refresh tokens that never expire. Changes apply only to newly issued tokens.",
			Default:         "90d",
			InputType:       InputDuration,
			RuntimeEditable: true,
		},

		{
			Key:             "blocked_mime_types",
			Group:           "Storage",
			Label:           "Blocked MIME types",
			Description:     "Comma-separated MIME types to reject on upload (e.g. application/x-executable,video/*).",
			Help:            "A comma-separated list of MIME types that will be rejected when uploaded. Uses http.DetectContentType on the actual file content (not the Content-Type header). Supports wildcards like \"video/*\" to block all video types. Leave empty to allow all types. Changes take effect immediately.",
			InputType:       InputText,
			RuntimeEditable: true,
		},

		// ── Legal (runtime-editable, no env var) ──
		{
			Key:             "tos_mode",
			Group:           "Legal",
			Label:           "Terms of Service mode",
			Description:     "How the Terms of Service is provided: off, text, or url.",
			Help:            "Controls the Terms of Service page. \"off\" disables TOS entirely. \"text\" renders the tos_content setting as a page at /legal/terms. \"url\" redirects /legal/terms to an external URL specified in tos_content. When active (not off), registration requires TOS acceptance.",
			Default:         "text",
			ValidValues:     []string{"off", "text", "url"},
			InputType:       InputSelect,
			RuntimeEditable: true,
		},
		{
			Key:             "tos_content",
			Group:           "Legal",
			Label:           "Terms of Service content",
			Description:     "TOS text (when mode=text) or URL (when mode=url).",
			Help:            "The content of the Terms of Service. When tos_mode is \"text\", this is rendered as formatted text on the /legal/terms page. When tos_mode is \"url\", this should be a full URL that /legal/terms will redirect to.",
			Default:         "Terms of Service\n\n1. Acceptance. By creating an account or using this service, you agree to these terms.\n2. Acceptable Use. You shall not use this service to store, transmit, or distribute content that is unlawful, harmful, threatening, abusive, defamatory, or otherwise objectionable.\n3. Prohibited Content. You shall not store content that: (a) violates any applicable law or regulation; (b) infringes any intellectual property right; (c) contains child sexual abuse material; or (d) contains malware or other harmful code.\n4. Termination. The operator may suspend or terminate your account at any time, with or without cause or notice.\n5. No Warranty. This service is provided \"as is\" without warranty of any kind, express or implied.\n6. Limitation of Liability. The operator shall not be liable for any indirect, incidental, special, consequential, or punitive damages.\n7. Modifications. These terms may be updated at any time. Continued use constitutes acceptance.",
			InputType:       InputTextArea,
			RuntimeEditable: true,
		},
		{
			Key:             "privacy_mode",
			Group:           "Legal",
			Label:           "Privacy Policy mode",
			Description:     "How the Privacy Policy is provided: off, text, or url.",
			Help:            "Controls the Privacy Policy page. \"off\" disables the privacy policy. \"text\" renders the privacy_content setting as a page at /legal/privacy. \"url\" redirects /legal/privacy to an external URL specified in privacy_content. When active (not off), registration shows the privacy policy link.",
			Default:         "text",
			ValidValues:     []string{"off", "text", "url"},
			InputType:       InputSelect,
			RuntimeEditable: true,
		},
		{
			Key:             "privacy_content",
			Group:           "Legal",
			Label:           "Privacy Policy content",
			Description:     "Privacy Policy text (when mode=text) or URL (when mode=url).",
			Help:            "The content of the Privacy Policy. When privacy_mode is \"text\", this is rendered as formatted text on the /legal/privacy page. When privacy_mode is \"url\", this should be a full URL that /legal/privacy will redirect to.",
			Default:         "Privacy Policy\n\n1. Data Collected. This service stores account credentials (username, hashed password) and files you upload. Server logs may record IP addresses, timestamps, and user agents.\n2. Use of Data. Your data is used solely to provide the service. The operator does not sell, share, or disclose your data to third parties except as required by law.\n3. Data Access. The server operator has technical access to the infrastructure. Your files are not routinely accessed by the operator.\n4. Data Retention. Your data is retained for the duration of your account. Upon deletion, your data is permanently removed.\n5. Security. Reasonable measures are taken to protect your data. No method of storage or transmission is completely secure.\n6. Contact. For questions, contact the server operator.",
			InputType:       InputTextArea,
			RuntimeEditable: true,
		},

		// ── API (runtime-editable, no env var) ──
		{
			Key:             "json_api",
			Group:           "API",
			Label:           "JSON API",
			Description:     "Enable the /json/* management API for programmatic admin access.",
			Help:            "Controls the JSON management API at /json/*. When \"off\", all /json/* endpoints return 404. When \"admin\", the API is enabled and requires admin authentication via session cookie or Bearer token (obtained from POST /json/login). The API provides programmatic access to user management, settings, and audit log — mirroring CLI functionality. Changes take effect immediately.",
			Default:         "off",
			ValidValues:     []string{"off", "admin"},
			InputType:       InputSelect,
			RuntimeEditable: true,
		},
	}
}

// RuntimeSettingDefs returns only the settings that can be changed at runtime.
func RuntimeSettingDefs() []SettingDef {
	var out []SettingDef
	for _, d := range SettingDefs() {
		if d.RuntimeEditable {
			out = append(out, d)
		}
	}
	return out
}

// SettingDefByKey returns the SettingDef for the given key, or nil if not found.
func SettingDefByKey(key string) *SettingDef {
	for _, d := range SettingDefs() {
		if d.Key == key {
			return &d
		}
	}
	return nil
}
