package settings

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"gosilo/internal/config"
	"gosilo/internal/db"
)

// Snapshot holds the current resolved settings (DB overrides merged with env defaults).
type Snapshot struct {
	MetricsMode      string
	JSONApi          string
	RegistrationMode string
	LogLevel         string
	RateLimitRate    float64
	RateLimitBurst   int
	QuotaMode        string
	QuotaTotal       int64
	QuotaUser        int64
	MaxUploadSize    int64
	TokenLifetime        string // duration string: "30d", "24h", "0" (no expiry)
	RefreshTokens        string // "enabled" or "disabled"
	RefreshTokenLifetime string // duration string: "90d", "0" (no expiry)
	PublicWrites         string
	BlockedMIMETypes     string
	TOSMode              string // "off", "text", "url"
	TOSContent           string
	PrivacyMode          string // "off", "text", "url"
	PrivacyContent       string
}

// Settings provides runtime-configurable settings backed by SQLite.
// Reads are lock-free via atomic pointer; writes go through the DB.
type Settings struct {
	current  atomic.Pointer[Snapshot]
	db       *sql.DB
	defaults *config.Config
	mu       sync.Mutex // serializes writes
	onChange []func(*Snapshot)
}

// New creates a Settings service and loads the initial snapshot from DB + env defaults.
func New(database *sql.DB, defaults *config.Config) *Settings {
	s := &Settings{
		db:       database,
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
	return db.GetSetting(ctx, s.db, key)
}

// Set validates and writes a setting to the DB, then reloads the snapshot.
func (s *Settings) Set(ctx context.Context, key, value string) error {
	if err := validateSetting(key, value); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := db.SetSetting(ctx, s.db, key, value); err != nil {
		return err
	}
	return s.reloadLocked(ctx)
}

// Delete removes a DB override (reverting to the env default), then reloads.
func (s *Settings) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := db.DeleteSetting(ctx, s.db, key); err != nil {
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
	overrides, err := db.ListSettings(ctx, s.db)
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
	return db.ListSettings(ctx, s.db)
}

// ValueMap returns the current runtime-editable setting values as a
// key→display-string map, keyed by SettingDef.Key.
func (snap *Snapshot) ValueMap() map[string]string {
	return map[string]string{
		"metrics_mode":      snap.MetricsMode,
		"json_api":          snap.JSONApi,
		"registration_mode": snap.RegistrationMode,
		"log_level":         snap.LogLevel,
		"rate_limit_rate":   fmt.Sprintf("%g", snap.RateLimitRate),
		"rate_limit_burst":  fmt.Sprintf("%d", snap.RateLimitBurst),
		"quota_mode":        snap.QuotaMode,
		"quota_total":       config.FormatByteSize(snap.QuotaTotal),
		"quota_user":        config.FormatByteSize(snap.QuotaUser),
		"max_upload_size":   config.FormatByteSize(snap.MaxUploadSize),
		"token_lifetime":         snap.TokenLifetime,
		"refresh_tokens":         snap.RefreshTokens,
		"refresh_token_lifetime": snap.RefreshTokenLifetime,
		"public_writes":          snap.PublicWrites,
		"blocked_mime_types":     snap.BlockedMIMETypes,
		"tos_mode":               snap.TOSMode,
		"tos_content":            snap.TOSContent,
		"privacy_mode":           snap.PrivacyMode,
		"privacy_content":        snap.PrivacyContent,
	}
}

// buildSnapshot merges DB overrides on top of env defaults.
func (s *Settings) buildSnapshot(overrides map[string]string) *Snapshot {
	snap := &Snapshot{
		MetricsMode:          s.defaults.MetricsMode,
		JSONApi:              s.defaults.JSONApi,
		RegistrationMode:     s.defaults.RegistrationMode,
		LogLevel:             s.defaults.LogLevel,
		RateLimitRate:        s.defaults.RateLimitRate,
		RateLimitBurst:       s.defaults.RateLimitBurst,
		QuotaMode:            s.defaults.QuotaMode,
		QuotaTotal:           s.defaults.QuotaTotal,
		QuotaUser:            s.defaults.QuotaUser,
		MaxUploadSize:        s.defaults.MaxUploadSize,
		PublicWrites:         s.defaults.PublicWrites,
		TokenLifetime:        s.defaults.TokenLifetime,
		RefreshTokens:        s.defaults.RefreshTokens,
		RefreshTokenLifetime: s.defaults.RefreshTokenLifetime,
		TOSMode:              s.defaults.TOSMode,
		TOSContent:           s.defaults.TOSContent,
		PrivacyMode:          s.defaults.PrivacyMode,
		PrivacyContent:       s.defaults.PrivacyContent,
	}

	if overrides == nil {
		return snap
	}

	if v, ok := overrides["metrics_mode"]; ok {
		snap.MetricsMode = v
	}
	if v, ok := overrides["json_api"]; ok {
		snap.JSONApi = v
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
	if v, ok := overrides["quota_mode"]; ok {
		snap.QuotaMode = v
	}
	if v, ok := overrides["quota_total"]; ok {
		if n, err := config.ParseByteSize(v); err == nil {
			snap.QuotaTotal = n
		}
	}
	if v, ok := overrides["quota_user"]; ok {
		if n, err := config.ParseByteSize(v); err == nil {
			snap.QuotaUser = n
		}
	}
	if v, ok := overrides["max_upload_size"]; ok {
		if n, err := config.ParseByteSize(v); err == nil {
			snap.MaxUploadSize = n
		}
	}
	if v, ok := overrides["public_writes"]; ok {
		snap.PublicWrites = v
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

	return snap
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
		if n <= 0 {
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
