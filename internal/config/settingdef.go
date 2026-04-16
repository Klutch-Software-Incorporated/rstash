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
	EnvAddr     = "RSTASH_ADDR"
	EnvBaseURL  = "RSTASH_BASE_URL"
	EnvDB       = "RSTASH_DB"
	EnvBlob     = "RSTASH_BLOB"
	EnvLogLevel = "RSTASH_LOG_LEVEL"
	EnvLogFile  = "RSTASH_LOG_FILE"
	EnvTLSCert  = "RSTASH_TLS_CERT"
	EnvTLSKey   = "RSTASH_TLS_KEY"
	EnvTLSMode  = "RSTASH_TLS_MODE"
	EnvTLSCache = "RSTASH_TLS_CACHE"
	EnvEmail    = "RSTASH_EMAIL"
)

// SettingDef describes one configurable setting — the single source of truth
// for env-file generation, admin UI rendering, CLI listing, and validation.
type SettingDef struct {
	Key             string    // internal key, e.g. "registration_mode"
	EnvVar          string    // env var name, e.g. "RSTASH_ADDR" (empty = no env var)
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
			Help:            "The externally reachable URL of your rstash instance, set via the " + EnvBaseURL + " environment variable. This is used to generate WebFinger responses, OAuth redirect URIs, and links in the web UI. Must include the scheme (http or https) and host. Do not include a trailing slash. If you are behind a reverse proxy, this should be the proxy's public URL, not the internal address.",
			Default:         "http://localhost:8080",
			InputType:       InputText,
			RuntimeEditable: false,
		},
		{
			Key:             "database_dsn",
			EnvVar:          EnvDB,
			Group:           "Server",
			Label:           "Database DSN",
			Description:     "Metadata database DSN. Supported: sqlite:, postgres:, mysql:, mssql:.",
			Help:            "The Data Source Name for the metadata database, set via the " + EnvDB + " environment variable. Stores users, sessions, OAuth tokens, audit entries, and file metadata (nodes). Supported formats: sqlite:path (e.g. sqlite:rstash.sqlite), sqlite::memory:, postgres:host=localhost dbname=rstash user=rstash password=secret sslmode=disable, mysql:user:pass@tcp(host:3306)/dbname?parseTime=true, mssql:sqlserver://user:pass@host:1433?database=dbname. Changing this setting requires a server restart.",
			Default:         "sqlite:rstash.sqlite",
			InputType:       InputText,
			RuntimeEditable: false,
		},
		{
			Key:             "blob_dsn",
			EnvVar:          EnvBlob,
			Group:           "Server",
			Label:           "Blob store DSN",
			Description:     "Blob store DSN. Supported: sqlite:, fs:, postgres:, mysql:, mssql:, s3:.",
			Help:            "The Data Source Name for the blob (file content) store, set via the " + EnvBlob + " environment variable. Supported backends: sqlite:path stores blobs in a SQLite database (simple, single-file), fs:/path/to/dir stores blobs as individual files on the filesystem (better for large deployments), postgres:/mysql:/mssql: store blobs in the corresponding database (useful when running on a non-SQLite metadata database), and s3:bucket?region=us-east-1&endpoint=s3.amazonaws.com&prefix=optional/prefix stores blobs in an S3-compatible object store. S3 credentials can be set via access_key/secret_key query parameters or AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY environment variables. S3 examples: s3:my-bucket?region=us-west-2 (AWS), s3:my-space?region=nyc3&endpoint=nyc3.digitaloceanspaces.com (DigitalOcean Spaces), s3:data?endpoint=minio.local:9000&tls=false&access_key=minioadmin&secret_key=minioadmin (MinIO). Changing this setting requires a server restart.",
			Default:         "sqlite:rstash-blobs.sqlite",
			InputType:       InputText,
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
		{
			Key:             "email_dsn",
			EnvVar:          EnvEmail,
			Group:           "Server",
			Label:           "Email provider",
			Description:     "Email delivery DSN (e.g. resend:API_KEY?from=noreply@example.com). Empty = email disabled.",
			Help:            "The DSN for the email delivery provider, set via the " + EnvEmail + " environment variable. Currently supported: resend:API_KEY?from=sender@domain.com. When configured, enables email verification, password reset, and notification features. When empty, these features are disabled but email addresses are still collected. Changing this setting requires a server restart.",
			InputType:       InputText,
			RuntimeEditable: false,
		},

		// ── Branding (runtime-editable, no env var) ──
		{
			Key:             "site_name",
			Group:           "Branding",
			Label:           "Site name",
			Description:     "Display name shown in the header, footer, and page titles.",
			Help:            "Replaces the default \"rstash\" branding throughout the web UI. Useful for giving your instance a custom identity (e.g. \"My Cloud Storage\"). Changes take effect immediately.",
			Default:         "rstash",
			InputType:       InputText,
			RuntimeEditable: true,
		},
		{
			Key:             "home_subtitle",
			Group:           "Branding",
			Label:           "Home page subtitle",
			Description:     "Tagline shown to logged-out visitors on the home page.",
			Help:            "The subtitle displayed below the server name on the public home page. Shown to unauthenticated visitors alongside the login form. Keep it short — one sentence is ideal. Changes take effect immediately.",
			Default:         "A personal remoteStorage server.",
			InputType:       InputText,
			RuntimeEditable: true,
		},

		// ── Access (runtime-editable, no env var) ──
		{
			Key:             "cookie_domain",
			Group:           "Access",
			Label:           "Cookie domain",
			Description:     "Parent domain for session/CSRF cookies (e.g. \".example.com\"). Empty = host-only.",
			Help:            "When set, the session and CSRF cookies are scoped to this domain, making them visible to all subdomains. Useful for integrating companion apps on subdomains (e.g. a billing or admin app at billing.example.com). When empty, cookies are host-only (default, most secure). SECURITY: enabling this means any subdomain of the configured parent receives the session cookie — treat all subdomains as first-party-trusted. Format: include the leading dot for cross-subdomain sharing (\".example.com\"), or no dot for apex-only (\"example.com\"). Changes take effect on next login.",
			InputType:       InputText,
			RuntimeEditable: true,
		},
		{
			Key:             "registration_mode",
			Group:           "Access",
			Label:           "Registration mode",
			Description:     "Who can create new accounts.",
			Help:            "Controls how new users are onboarded. \"open\" lets anyone self-register. \"approval\" is like open but an admin must approve each account before login. \"closed\" restricts account creation to administrators (admin panel or admin API). \"external\" redirects /register to an external signup URL (for deployments where a companion app handles signup via the admin API — set registration_external_url). This setting takes effect immediately when changed at runtime.",
			Default:         "closed",
			ValidValues:     []string{"open", "approval", "closed", "external"},
			InputType:       InputSelect,
			RuntimeEditable: true,
		},
		{
			Key:             "registration_external_url",
			Group:           "Access",
			Label:           "External registration URL",
			Description:     "Where /register redirects when registration_mode is \"external\".",
			Help:            "When registration_mode is set to \"external\", requests to /register are redirected here (302). Typically this points to a companion app (e.g. billing.example.com/signup) that handles signup and calls the admin API to provision the account. Ignored when registration_mode is not \"external\". Must be a full URL including scheme.",
			InputType:       InputText,
			RuntimeEditable: true,
		},
		{
			Key:             "external_account_url",
			Group:           "Access",
			Label:           "External account management URL",
			Description:     "Where externally-managed users go to change their email or delete their account.",
			Help:            "For users flagged ExternallyManaged (created via admin API with provision=true), rstash disables local email editing and self-service account deletion. Instead, those UI surfaces link to this URL so the companion app (e.g. a billing portal) can own identity changes. Ignored when no externally-managed users exist.",
			InputType:       InputText,
			RuntimeEditable: true,
		},
		{
			Key:             "custom_links",
			Group:           "Branding",
			Label:           "Custom links",
			Description:     "JSON array of {label, url, description} entries shown in the user menu and profile page.",
			Help:            "Up to 10 admin-configurable external links rendered in each user's account dropdown and on their profile settings page under a \"Linked Services\" heading. Useful for pointing users at status pages, support contacts, or companion apps (e.g. a billing portal). Format: [{\"label\":\"Billing\",\"url\":\"https://billing.example.com\",\"description\":\"Manage your subscription\"}]. URLs must be https:// or start with / (relative). Leave empty to show no extra links.",
			InputType:       InputTextArea,
			RuntimeEditable: true,
		},
		{
			Key:             "disabled_account_message",
			Group:           "Access",
			Label:           "Disabled account message",
			Description:     "Shown on the login page when a disabled user tries to log in.",
			Help:            "Optional message (HTML allowed) displayed on the login error page when the user's account is disabled. Useful for pointing users at a billing portal or support contact (e.g. \"Your account is suspended — visit https://billing.example.com to update your payment method.\"). Leave empty for the default generic message.",
			InputType:       InputTextArea,
			RuntimeEditable: true,
		},
		{
			Key:             "audit_retention_days",
			Group:           "Legal",
			Label:           "Audit log retention (days)",
			Description:     "How many days of audit-log history to keep. 0 = forever.",
			Help:            "When > 0, a once-daily worker prunes audit entries older than this many days. Useful for GDPR data minimization and keeping the database bounded. 0 preserves the current behavior (no pruning). Changes take effect on the next daily tick.",
			Default:         "0",
			InputType:       InputNumber,
			RuntimeEditable: true,
			NumberMin:       "0",
		},
		{
			Key:             "log_client_ips",
			Group:           "Legal",
			Label:           "Client IP logging",
			Description:     "How to record client IPs in sessions and audit entries.",
			Help:            "Controls how IP addresses are stored. \"enabled\" records the raw IP (current behavior). \"hashed\" hashes the IP with SHA-256 and a daily-rotating salt before storage (still useful for same-day rate-limit / anomaly checks, but not back-convertible to an identity). \"disabled\" stores an empty string and is strongest for privacy. Does not retroactively change existing rows.",
			Default:         "enabled",
			ValidValues:     []string{"enabled", "hashed", "disabled"},
			InputType:       InputSelect,
			RuntimeEditable: true,
		},
		{
			Key:             "log_level",
			EnvVar:          EnvLogLevel,
			Group:           "Monitoring",
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
		{
			Key:             "bandwidth_mode",
			Group:           "Storage",
			Label:           "Bandwidth mode",
			Description:     "How monthly egress (download) bandwidth is enforced.",
			Help:            "Controls monthly egress limits per user. \"off\" disables both tracking and enforcement — no counter writes on the read path. \"user\" enforces per-user limits (configured via bandwidth_quota_user, with optional per-user overrides via the admin API). When a user's remaining quota for the current calendar month is zero, GetDocument returns 429 with a Retry-After header pointing to the next-month boundary. Uploads always succeed. Changes take effect immediately.",
			Default:         "user",
			ValidValues:     []string{"off", "user"},
			InputType:       InputSelect,
			RuntimeEditable: true,
		},
		{
			Key:             "bandwidth_quota_user",
			Group:           "Storage",
			Label:           "Per-user bandwidth",
			Description:     "Default monthly egress limit per user (e.g. 500GB).",
			Help:            "The default monthly egress (download) limit per user when bandwidth_mode=user. Individual users can be given higher or lower limits via the admin API. Accepts human-readable sizes: B, KB, MB, GB, TB. Must be greater than 0. Resets at the calendar-month boundary (UTC). Changes take effect immediately.",
			Default:         "500GB",
			InputType:       InputByteSize,
			RuntimeEditable: true,
		},
		// ── Monitoring (runtime-editable / env-only) ──
		{
			Key:             "metrics_mode",
			Group:           "Monitoring",
			Label:           "Metrics visibility",
			Description:     "Who can access the <a href=\"/metrics\">/metrics</a> endpoint.",
			Help:            "Controls access to the Prometheus <a href=\"/metrics\">/metrics</a> endpoint. \"public\" allows unauthenticated access (suitable when metrics are scraped from a private network). \"admin\" requires an authenticated admin session. \"off\" disables the endpoint entirely (returns 404). Changes take effect immediately.",
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
