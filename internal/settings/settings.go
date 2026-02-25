package settings

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"

	"gosilo/internal/config"
	"gosilo/internal/db"
)

// Snapshot holds the current resolved settings (DB overrides merged with env defaults).
type Snapshot struct {
	RegistrationMode string
	LogLevel         string
	RateLimitRate    float64
	RateLimitBurst   int
	QuotaMode        string
	QuotaTotal       int64
	QuotaUser        int64
	MaxUploadSize    int64
	TokenLifetime    string // duration string: "30d", "24h", "0" (no expiry)
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

// buildSnapshot merges DB overrides on top of env defaults.
func (s *Settings) buildSnapshot(overrides map[string]string) *Snapshot {
	snap := &Snapshot{
		RegistrationMode: s.defaults.RegistrationMode,
		LogLevel:         s.defaults.LogLevel,
		RateLimitRate:    s.defaults.RateLimitRate,
		RateLimitBurst:   s.defaults.RateLimitBurst,
		QuotaMode:        s.defaults.QuotaMode,
		QuotaTotal:       s.defaults.QuotaTotal,
		QuotaUser:        s.defaults.QuotaUser,
		MaxUploadSize:    s.defaults.MaxUploadSize,
		TokenLifetime:    s.defaults.TokenLifetime,
	}

	if overrides == nil {
		return snap
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
	if v, ok := overrides["token_lifetime"]; ok {
		snap.TokenLifetime = v
	}

	return snap
}

// validateSetting checks that a key is known and its value is valid.
func validateSetting(key, value string) error {
	switch key {
	case "registration_mode":
		switch value {
		case "open", "closed":
			return nil
		default:
			return fmt.Errorf("registration_mode must be one of: open, closed — got %q", value)
		}

	case "log_level":
		switch value {
		case "debug", "info", "warn", "error":
			return nil
		default:
			return fmt.Errorf("log_level must be one of: debug, info, warn, error — got %q", value)
		}

	case "rate_limit_rate":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil || f < 0 {
			return fmt.Errorf("rate_limit_rate must be a non-negative number — got %q", value)
		}
		return nil

	case "rate_limit_burst":
		i, err := strconv.Atoi(value)
		if err != nil || i < 0 {
			return fmt.Errorf("rate_limit_burst must be a non-negative integer — got %q", value)
		}
		return nil

	case "quota_mode":
		switch value {
		case "off", "total", "user":
			return nil
		default:
			return fmt.Errorf("quota_mode must be one of: off, total, user — got %q", value)
		}

	case "quota_total":
		n, err := config.ParseByteSize(value)
		if err != nil {
			return fmt.Errorf("quota_total: %w", err)
		}
		if n <= 0 {
			return fmt.Errorf("quota_total must be > 0")
		}
		return nil

	case "quota_user":
		n, err := config.ParseByteSize(value)
		if err != nil {
			return fmt.Errorf("quota_user: %w", err)
		}
		if n <= 0 {
			return fmt.Errorf("quota_user must be > 0")
		}
		return nil

	case "max_upload_size":
		n, err := config.ParseByteSize(value)
		if err != nil {
			return fmt.Errorf("max_upload_size: %w", err)
		}
		if n <= 0 {
			return fmt.Errorf("max_upload_size must be > 0")
		}
		return nil

	case "token_lifetime":
		_, err := config.ParseTokenLifetime(value)
		if err != nil {
			return fmt.Errorf("token_lifetime: %w", err)
		}
		return nil

	default:
		return fmt.Errorf("unknown setting: %q", key)
	}
}
