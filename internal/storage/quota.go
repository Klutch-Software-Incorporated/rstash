package storage

import (
	"context"
	"database/sql"
	"errors"
	"sync"

	"gosilo/internal/db"
)

// ErrQuotaExceeded is returned when a write would exceed the configured quota.
var ErrQuotaExceeded = errors.New("quota exceeded")

// QuotaConfig holds quota enforcement settings.
type QuotaConfig struct {
	Mode       string // "off", "total", "user"
	TotalLimit int64  // bytes (mode=total)
	UserLimit  int64  // default per-user bytes (mode=user)
}

// QuotaChecker enforces storage quotas.
type QuotaChecker struct {
	mu     sync.Mutex // serializes write transactions with quota checks
	config QuotaConfig
	db     *sql.DB
}

// Lock serializes write transactions that include quota checks.
func (qc *QuotaChecker) Lock()   { qc.mu.Lock() }

// Unlock releases the quota serialization lock.
func (qc *QuotaChecker) Unlock() { qc.mu.Unlock() }

// NewQuotaChecker creates a new QuotaChecker.
func NewQuotaChecker(cfg QuotaConfig, database *sql.DB) *QuotaChecker {
	return &QuotaChecker{config: cfg, db: database}
}

// UpdateConfig replaces the quota configuration at runtime.
func (qc *QuotaChecker) UpdateConfig(cfg QuotaConfig) {
	qc.mu.Lock()
	defer qc.mu.Unlock()
	qc.config = cfg
}

// Check verifies that storing incomingBytes for userID would not exceed quotas.
// incomingBytes should be the net delta (new size minus old size if overwriting).
// Use a Querier (tx or db) for transactional consistency.
func (qc *QuotaChecker) Check(ctx context.Context, q db.Querier, userID int64, incomingBytes int64) error {
	switch qc.config.Mode {
	case "off":
		return nil

	case "total":
		total, err := db.GetTotalStorageUsed(ctx, q)
		if err != nil {
			return err
		}
		if total+incomingBytes > qc.config.TotalLimit {
			return ErrQuotaExceeded
		}
		return nil

	case "user":
		limit := qc.GetUserQuota(ctx, userID)
		if limit == 0 {
			// Unlimited user.
			return nil
		}
		stats, err := db.GetUserStorageStats(ctx, q, userID)
		if err != nil {
			return err
		}
		if stats.TotalBytes+incomingBytes > limit {
			return ErrQuotaExceeded
		}
		return nil
	}

	return nil
}

// GetUserQuota returns the effective quota for a user.
// If the user has a DB override (storage_quota > 0), that is used.
// Otherwise, the server default (UserLimit) is returned.
// A return value of 0 means unlimited.
func (qc *QuotaChecker) GetUserQuota(ctx context.Context, userID int64) int64 {
	user, err := db.GetUserByID(ctx, qc.db, userID)
	if err != nil || user == nil {
		return qc.config.UserLimit
	}
	if user.StorageQuota > 0 {
		return user.StorageQuota
	}
	return qc.config.UserLimit
}
