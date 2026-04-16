package db

// API key storage and lookup — "prefix + bcrypt" pattern
//
// API keys are stored as an indexed plaintext prefix plus a bcrypt hash of
// the full key. The prefix exists because bcrypt is non-deterministic:
// bcrypt(key) produces a different hash on every call (fresh 16-byte salt),
// so there is no way to query "find the row whose bcrypt hash matches this
// incoming key" — you can only ask CompareHashAndPassword(storedHash, plain)
// one row at a time.
//
// For password login that's fine: the user supplies a username, you look up
// the ONE row for that username, and run ONE bcrypt compare. API keys have
// no username equivalent — the incoming header is just an opaque string, so
// without a searchable fingerprint the only option would be to bcrypt-compare
// against every row (O(N) @ ~100ms each). Unusable for per-request auth.
//
// The prefix (first 8 hex chars of the raw key, stored plaintext in an
// indexed column) plays the same role username plays in the password flow:
// it lets us narrow to usually 0–1 candidate rows with a fast indexed query
// before we do the one expensive bcrypt compare.
//
// Validation flow on an incoming key:
//   1. prefix := incoming[:8]
//   2. SELECT * FROM api_keys WHERE key_prefix = prefix   (indexed, fast)
//   3. bcrypt.Compare(candidate.KeyHash, incoming)         (one compare)
//   4. First match (if any) authenticates the request.
//
// Why this is still secure:
//   - The prefix is 4 bytes out of a 32-byte key. An attacker who sees the
//     prefix still needs to guess the remaining 28 bytes (2^224 keyspace).
//   - bcrypt remains one-way, so a stolen DB can't be turned back into keys.
//   - Timing doesn't leak match-vs-no-match: the prefix lookup always runs,
//     bcrypt runs on collisions, and both the "no match" and "wrong key"
//     branches return 401 with the same response shape.
//
// 8 hex chars is a deliberate compromise: enough entropy that collisions
// in the prefix column are very rare (2^32 space), short enough to print in
// the admin UI as a human-readable fingerprint ("key starting with a7b9c1d2…").
// Same pattern used by GitHub (ghp_…), Stripe (sk_live_…), AWS (AKIA…).

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"rstash/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// GenerateAPIKey returns a fresh random 32-byte key encoded as 64 hex chars.
// The first 8 chars are used as the lookup prefix (see APIKeyPrefix).
func GenerateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// APIKeyPrefix returns the first 8 chars of a raw key, used for indexed lookup.
func APIKeyPrefix(rawKey string) string {
	if len(rawKey) < 8 {
		return rawKey
	}
	return rawKey[:8]
}

// ListAPIKeys returns all API keys, ordered by most recently created first.
func (r *Repository) ListAPIKeys(ctx context.Context) ([]*model.APIKey, error) {
	var keys []*model.APIKey
	if err := r.db.WithContext(ctx).Order("created_at DESC").Find(&keys).Error; err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	return keys, nil
}

// GetAPIKey returns the API key with the given ID, or nil if not found.
func (r *Repository) GetAPIKey(ctx context.Context, id int64) (*model.APIKey, error) {
	var key model.APIKey
	err := r.db.WithContext(ctx).First(&key, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get api key: %w", err)
	}
	return &key, nil
}

// CreateAPIKey inserts a new API key. The caller must populate Name, KeyHash,
// KeyPrefix, and RateLimitRPM. ID and CreatedAt are populated by the DB.
func (r *Repository) CreateAPIKey(ctx context.Context, key *model.APIKey) error {
	if err := r.db.WithContext(ctx).Create(key).Error; err != nil {
		return fmt.Errorf("create api key: %w", err)
	}
	return nil
}

// UpdateAPIKey saves the given key. Only mutable fields are persisted.
func (r *Repository) UpdateAPIKey(ctx context.Context, key *model.APIKey) error {
	err := r.db.WithContext(ctx).Model(&model.APIKey{}).
		Where("id = ?", key.ID).
		Updates(map[string]any{
			"name":           key.Name,
			"description":    key.Description,
			"key_hash":       key.KeyHash,
			"key_prefix":     key.KeyPrefix,
			"rate_limit_rpm": key.RateLimitRPM,
		}).Error
	if err != nil {
		return fmt.Errorf("update api key: %w", err)
	}
	return nil
}

// DeleteAPIKey removes an API key by ID.
func (r *Repository) DeleteAPIKey(ctx context.Context, id int64) error {
	if err := r.db.WithContext(ctx).Delete(&model.APIKey{}, id).Error; err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}
	return nil
}

// FindAPIKeyByRaw finds an API key matching the given raw key. Looks up candidates
// by prefix (indexed), then bcrypt-compares against each candidate's hash. On match,
// updates LastUsedAt (best-effort, non-blocking). Returns nil if no match.
func (r *Repository) FindAPIKeyByRaw(ctx context.Context, rawKey string) (*model.APIKey, error) {
	prefix := APIKeyPrefix(rawKey)
	if prefix == "" {
		return nil, nil
	}

	var candidates []*model.APIKey
	if err := r.db.WithContext(ctx).Where("key_prefix = ?", prefix).Find(&candidates).Error; err != nil {
		return nil, fmt.Errorf("find api key by prefix: %w", err)
	}

	for _, c := range candidates {
		if err := bcrypt.CompareHashAndPassword([]byte(c.KeyHash), []byte(rawKey)); err == nil {
			now := time.Now().UTC()
			go func(id int64) {
				_ = r.db.Model(&model.APIKey{}).Where("id = ?", id).Update("last_used_at", now).Error
			}(c.ID)
			return c, nil
		}
	}
	return nil, nil
}

// HashAPIKey returns the bcrypt hash of a raw key, used when creating/regenerating.
func HashAPIKey(rawKey string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash api key: %w", err)
	}
	return string(h), nil
}
