package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"rstash/internal/config"
	"rstash/internal/db"
)

// CustomLink is a single entry in the custom_links list rendered in the user
// dropdown and profile settings page.
type CustomLink struct {
	Label       string `json:"label"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

// maxCustomLinks caps the size of the list to keep the UI surface bounded.
const maxCustomLinks = 10

// Snapshot holds the current resolved settings (DB overrides merged with env defaults).
type Snapshot struct {
	MetricsMode      string
	RegistrationMode string
	LogLevel         string
	RateLimitRate      float64
	RateLimitBurst     int
	UserRateLimitRate  float64 // per-user storage request rate (0 = disabled)
	UserRateLimitBurst int     // per-user burst capacity
	TotalStorageLimit       int64 // global storage cap (0 = disabled)
	DefaultUserStorageLimit int64 // stamped onto new users (0 = unlimited)
	MaxUploadSize           int64
	TokenLifetime        string // duration string: "30d", "24h", "0" (no expiry)
	RefreshTokens        string // "enabled" or "disabled"
	RefreshTokenLifetime string // duration string: "90d", "0" (no expiry)
	SiteName             string
	HomeSubtitle         string
	BlockedMIMETypes     string
	TOSMode              string // "off", "text", "url"
	TOSContent           string
	PrivacyMode          string // "off", "text", "url"
	PrivacyContent       string
	CookieDomain         string // domain attribute for session/CSRF cookies; empty = host-only
	RegistrationExternalURL string // target for /register redirect when mode=external
	ExternalAccountURL      string // where externally-managed users manage email/delete
	CustomLinks             []CustomLink // admin-configured links for user menu/profile
	DisabledAccountMessage  string       // HTML shown on login-error for disabled users
	TotalEgressLimit       int64 // global monthly egress cap (0 = disabled)
	DefaultUserEgressLimit int64 // stamped onto new users (0 = unlimited)
	AuditRetentionDays      int          // 0 = forever; > 0 = prune entries older than N days
	LogClientIPs            string       // "enabled", "hashed", or "disabled"
}

// Settings provides runtime-configurable settings backed by the database.
// Reads are lock-free via atomic pointer; writes go through the DB.
type Settings struct {
	current  atomic.Pointer[Snapshot]
	repo     *db.Repository
	defaults *config.Config
	mu       sync.Mutex // serializes writes
	onChange []func(*Snapshot)
}

// New creates a Settings service and loads the initial snapshot from DB + env defaults.
func New(repo *db.Repository, defaults *config.Config) *Settings {
	s := &Settings{
		repo:     repo,
		defaults: defaults,
	}

	// Build initial snapshot.
	snap := s.buildSnapshot(nil)
	s.current.Store(snap)

	// Try to load from DB (non-fatal on error since we have defaults).
	if err := s.Reload(context.Background()); err != nil {
		slog.Error("failed to load settings from database, using defaults", "error", err)
	}

	return s
}

// Load returns the current settings snapshot. Lock-free.
func (s *Settings) Load() *Snapshot {
	return s.current.Load()
}

// Get returns the raw DB value for a key, or "" if not overridden.
func (s *Settings) Get(ctx context.Context, key string) (string, error) {
	return s.repo.GetSetting(ctx, key)
}

// Set validates and writes a setting to the DB, then reloads the snapshot.
func (s *Settings) Set(ctx context.Context, key, value string) error {
	if err := validateSetting(key, value); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.repo.SetSetting(ctx, key, value); err != nil {
		return err
	}
	return s.reloadLocked(ctx)
}

// Delete removes a DB override (reverting to the env default), then reloads.
func (s *Settings) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.repo.DeleteSetting(ctx, key); err != nil {
		return err
	}
	return s.reloadLocked(ctx)
}

// Reload reads all settings from the DB, merges with defaults, swaps the
// atomic pointer, and runs onChange callbacks.
func (s *Settings) Reload(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reloadLocked(ctx)
}

func (s *Settings) reloadLocked(ctx context.Context) error {
	overrides, err := s.repo.ListSettings(ctx)
	if err != nil {
		return fmt.Errorf("reload settings: %w", err)
	}

	snap := s.buildSnapshot(overrides)
	s.current.Store(snap)

	for _, fn := range s.onChange {
		fn(snap)
	}

	return nil
}

// OnChange registers a callback that fires whenever the snapshot is swapped.
func (s *Settings) OnChange(fn func(*Snapshot)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = append(s.onChange, fn)
}

// Overrides returns the raw DB overrides map (for admin UI display).
func (s *Settings) Overrides(ctx context.Context) (map[string]string, error) {
	return s.repo.ListSettings(ctx)
}

// ValueMap returns the current runtime-editable setting values as a
// key→display-string map, keyed by SettingDef.Key.
func (snap *Snapshot) ValueMap() map[string]string {
	return map[string]string{
		"metrics_mode":      snap.MetricsMode,
		"registration_mode": snap.RegistrationMode,
		"log_level":         snap.LogLevel,
		"rate_limit_rate":        fmt.Sprintf("%g", snap.RateLimitRate),
		"rate_limit_burst":       fmt.Sprintf("%d", snap.RateLimitBurst),
		"user_rate_limit_rate":   fmt.Sprintf("%g", snap.UserRateLimitRate),
		"user_rate_limit_burst":  fmt.Sprintf("%d", snap.UserRateLimitBurst),
		"total_storage_limit":        config.FormatByteSize(snap.TotalStorageLimit),
		"default_user_storage_limit": config.FormatByteSize(snap.DefaultUserStorageLimit),
		"total_egress_limit":         config.FormatByteSize(snap.TotalEgressLimit),
		"default_user_egress_limit":  config.FormatByteSize(snap.DefaultUserEgressLimit),
		"max_upload_size":            config.FormatByteSize(snap.MaxUploadSize),
		"token_lifetime":         snap.TokenLifetime,
		"refresh_tokens":         snap.RefreshTokens,
		"refresh_token_lifetime": snap.RefreshTokenLifetime,
		"site_name":              snap.SiteName,
		"home_subtitle":          snap.HomeSubtitle,
		"blocked_mime_types":     snap.BlockedMIMETypes,
		"tos_mode":               snap.TOSMode,
		"tos_content":            snap.TOSContent,
		"privacy_mode":           snap.PrivacyMode,
		"privacy_content":        snap.PrivacyContent,
		"cookie_domain":          snap.CookieDomain,
		"registration_external_url":   snap.RegistrationExternalURL,
		"external_account_url":        snap.ExternalAccountURL,
		"custom_links":                snap.customLinksRaw(),
		"disabled_account_message":    snap.DisabledAccountMessage,
		"audit_retention_days":        strconv.Itoa(snap.AuditRetentionDays),
		"log_client_ips":              snap.LogClientIPs,
	}
}

// customLinksRaw returns the JSON-encoded list for display/round-tripping.
func (snap *Snapshot) customLinksRaw() string {
	if len(snap.CustomLinks) == 0 {
		return ""
	}
	b, err := json.Marshal(snap.CustomLinks)
	if err != nil {
		return ""
	}
	return string(b)
}

// buildSnapshot merges DB overrides on top of env defaults.
func (s *Settings) buildSnapshot(overrides map[string]string) *Snapshot {
	snap := &Snapshot{
		MetricsMode:          s.defaults.MetricsMode,
		RegistrationMode:     s.defaults.RegistrationMode,
		LogLevel:             s.defaults.LogLevel,
		RateLimitRate:        s.defaults.RateLimitRate,
		RateLimitBurst:       s.defaults.RateLimitBurst,
		UserRateLimitRate:    s.defaults.UserRateLimitRate,
		UserRateLimitBurst:   s.defaults.UserRateLimitBurst,
		TotalStorageLimit:       s.defaults.TotalStorageLimit,
		DefaultUserStorageLimit: s.defaults.DefaultUserStorageLimit,
		MaxUploadSize:           s.defaults.MaxUploadSize,
		SiteName:             s.defaults.SiteName,
		HomeSubtitle:         s.defaults.HomeSubtitle,
		TokenLifetime:        s.defaults.TokenLifetime,
		RefreshTokens:        s.defaults.RefreshTokens,
		RefreshTokenLifetime: s.defaults.RefreshTokenLifetime,
		TOSMode:              s.defaults.TOSMode,
		TOSContent:           s.defaults.TOSContent,
		PrivacyMode:          s.defaults.PrivacyMode,
		PrivacyContent:       s.defaults.PrivacyContent,
		CookieDomain:         s.defaults.CookieDomain,
		TotalEgressLimit:       s.defaults.TotalEgressLimit,
		DefaultUserEgressLimit: s.defaults.DefaultUserEgressLimit,
		AuditRetentionDays:   s.defaults.AuditRetentionDays,
		LogClientIPs:         s.defaults.LogClientIPs,
	}

	if overrides == nil {
		return snap
	}

	if v, ok := overrides["metrics_mode"]; ok {
		snap.MetricsMode = v
	}
	if v, ok := overrides["registration_mode"]; ok {
		snap.RegistrationMode = v
	}
	if v, ok := overrides["log_level"]; ok {
		snap.LogLevel = v
	}
	if v, ok := overrides["rate_limit_rate"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			snap.RateLimitRate = f
		}
	}
	if v, ok := overrides["rate_limit_burst"]; ok {
		if i, err := strconv.Atoi(v); err == nil {
			snap.RateLimitBurst = i
		}
	}
	if v, ok := overrides["user_rate_limit_rate"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			snap.UserRateLimitRate = f
		}
	}
	if v, ok := overrides["user_rate_limit_burst"]; ok {
		if i, err := strconv.Atoi(v); err == nil {
			snap.UserRateLimitBurst = i
		}
	}
	if v, ok := overrides["total_storage_limit"]; ok {
		if n, err := config.ParseByteSize(v); err == nil {
			snap.TotalStorageLimit = n
		}
	}
	if v, ok := overrides["default_user_storage_limit"]; ok {
		if n, err := config.ParseByteSize(v); err == nil {
			snap.DefaultUserStorageLimit = n
		}
	}
	if v, ok := overrides["total_egress_limit"]; ok {
		if n, err := config.ParseByteSize(v); err == nil {
			snap.TotalEgressLimit = n
		}
	}
	if v, ok := overrides["default_user_egress_limit"]; ok {
		if n, err := config.ParseByteSize(v); err == nil {
			snap.DefaultUserEgressLimit = n
		}
	}
	if v, ok := overrides["max_upload_size"]; ok {
		if n, err := config.ParseByteSize(v); err == nil {
			snap.MaxUploadSize = n
		}
	}
	if v, ok := overrides["token_lifetime"]; ok {
		snap.TokenLifetime = v
	}
	if v, ok := overrides["refresh_tokens"]; ok {
		snap.RefreshTokens = v
	}
	if v, ok := overrides["refresh_token_lifetime"]; ok {
		snap.RefreshTokenLifetime = v
	}
	if v, ok := overrides["site_name"]; ok {
		snap.SiteName = v
	}
	if v, ok := overrides["home_subtitle"]; ok {
		snap.HomeSubtitle = v
	}
	if v, ok := overrides["blocked_mime_types"]; ok {
		snap.BlockedMIMETypes = v
	}
	if v, ok := overrides["tos_mode"]; ok {
		snap.TOSMode = v
	}
	if v, ok := overrides["tos_content"]; ok {
		snap.TOSContent = v
	}
	if v, ok := overrides["privacy_mode"]; ok {
		snap.PrivacyMode = v
	}
	if v, ok := overrides["privacy_content"]; ok {
		snap.PrivacyContent = v
	}
	if v, ok := overrides["cookie_domain"]; ok {
		snap.CookieDomain = v
	}
	if v, ok := overrides["registration_external_url"]; ok {
		snap.RegistrationExternalURL = v
	}
	if v, ok := overrides["external_account_url"]; ok {
		snap.ExternalAccountURL = v
	}
	if v, ok := overrides["custom_links"]; ok && v != "" {
		if links, err := parseCustomLinks(v); err == nil {
			snap.CustomLinks = links
		}
	}
	if v, ok := overrides["disabled_account_message"]; ok {
		snap.DisabledAccountMessage = v
	}
	if v, ok := overrides["audit_retention_days"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			snap.AuditRetentionDays = n
		}
	}
	if v, ok := overrides["log_client_ips"]; ok {
		snap.LogClientIPs = v
	}

	return snap
}

// parseCustomLinks decodes and validates the custom_links JSON array.
// Invalid entries are rejected with an error so admin UI can surface them.
func parseCustomLinks(raw string) ([]CustomLink, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var links []CustomLink
	if err := json.Unmarshal([]byte(raw), &links); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if len(links) > maxCustomLinks {
		return nil, fmt.Errorf("too many links (max %d)", maxCustomLinks)
	}
	for i, l := range links {
		if strings.TrimSpace(l.Label) == "" {
			return nil, fmt.Errorf("link %d: label is required", i+1)
		}
		if err := validateLinkURL(l.URL); err != nil {
			return nil, fmt.Errorf("link %d (%q): %w", i+1, l.Label, err)
		}
	}
	return links, nil
}

// validateLinkURL accepts https:// absolute URLs or relative paths starting with /.
// Other schemes are rejected to avoid javascript:/data: injection or accidental
// http:// in a TLS deployment.
func validateLinkURL(raw string) error {
	if strings.HasPrefix(raw, "/") {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("URL must be https:// or a relative path starting with /")
	}
	if u.Host == "" {
		return fmt.Errorf("URL must include a host")
	}
	return nil
}

// validateSetting checks that a key is known, runtime-editable, and its value is valid.
// Validation rules are derived from the SettingDefs registry.
func validateSetting(key, value string) error {
	def := config.SettingDefByKey(key)
	if def == nil {
		return fmt.Errorf("unknown setting: %q", key)
	}
	if !def.RuntimeEditable {
		return fmt.Errorf("setting %q cannot be changed at runtime", key)
	}

	// Keys with structured values get special-cased validators so Set() rejects
	// malformed input before the bad value ever lands in the DB.
	switch key {
	case "custom_links":
		if strings.TrimSpace(value) == "" {
			return nil
		}
		_, err := parseCustomLinks(value)
		return err
	}

	switch def.InputType {
	case config.InputSelect:
		for _, v := range def.ValidValues {
			if value == v {
				return nil
			}
		}
		return fmt.Errorf("%s must be one of: %s — got %q", key, strings.Join(def.ValidValues, ", "), value)

	case config.InputNumber:
		if def.NumberStep != "" && strings.Contains(def.NumberStep, ".") {
			f, err := strconv.ParseFloat(value, 64)
			if err != nil || f < 0 {
				return fmt.Errorf("%s must be a non-negative number — got %q", key, value)
			}
		} else {
			i, err := strconv.Atoi(value)
			if err != nil || i < 0 {
				return fmt.Errorf("%s must be a non-negative integer — got %q", key, value)
			}
		}
		return nil

	case config.InputByteSize:
		n, err := config.ParseByteSize(value)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		if n < 0 {
			return fmt.Errorf("%s must be >= 0", key)
		}
		// max_upload_size of 0 would break all uploads; reject explicitly.
		if key == "max_upload_size" && n == 0 {
			return fmt.Errorf("%s must be > 0", key)
		}
		return nil

	case config.InputDuration:
		_, err := config.ParseTokenLifetime(value)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		return nil

	default:
		return nil
	}
}
