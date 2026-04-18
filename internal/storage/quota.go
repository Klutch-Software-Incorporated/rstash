package storage

import (
	"context"
	"errors"
	"sync"

	"rstash/internal/db"
)

// ErrQuotaExceeded is returned when a write would exceed the configured quota.
var ErrQuotaExceeded = errors.New("quota exceeded")

// QuotaConfig holds live quota enforcement settings.
//
// The per-user limit is not held here — it lives on User.StorageQuota and is
// consulted directly. The default_user_storage_limit setting is only read at
// user creation time (stamped onto the User row) and is not referenced here.
type QuotaConfig struct {
	TotalLimit int64 // global cap in bytes (0 = disabled)
}

// QuotaChecker enforces storage quotas.
type QuotaChecker struct {
	mu     sync.Mutex // serializes write transactions with quota checks
	config QuotaConfig
	repo   *db.Repository
}

// Lock serializes write transactions that include quota checks.
func (qc *QuotaChecker) Lock() { qc.mu.Lock() }

// Unlock releases the quota serialization lock.
func (qc *QuotaChecker) Unlock() { qc.mu.Unlock() }

// NewQuotaChecker creates a new QuotaChecker.
func NewQuotaChecker(cfg QuotaConfig, repo *db.Repository) *QuotaChecker {
	return &QuotaChecker{config: cfg, repo: repo}
}

// UpdateConfig replaces the quota configuration at runtime.
func (qc *QuotaChecker) UpdateConfig(cfg QuotaConfig) {
	qc.mu.Lock()
	defer qc.mu.Unlock()
	qc.config = cfg
}

// Check verifies that storing incomingBytes for userID would not exceed the
// per-user limit (from the User row, 0 = unlimited) or the global cap (from
// settings, 0 = disabled). incomingBytes should be the net delta (new size
// minus old size if overwriting). Use the given repo (which may be a
// transaction-scoped repo) for consistency — all reads happen on that
// connection to avoid deadlocks on single-writer dialects like SQLite.
//
// Callers that want the Check to be consistent against concurrent writes
// must hold Lock() for the enclosing transaction; Check itself does not
// re-acquire the mutex (it would deadlock with that usage pattern).
func (qc *QuotaChecker) Check(ctx context.Context, repo *db.Repository, userID int64, incomingBytes int64) error {
	totalLimit := qc.config.TotalLimit

	// Per-user check: User.StorageQuota is authoritative. 0 = unlimited.
	user, err := repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user != nil && user.StorageQuota > 0 {
		stats, err := repo.GetUserStorageStats(ctx, userID)
		if err != nil {
			return err
		}
		if stats.TotalBytes+incomingBytes > user.StorageQuota {
			return ErrQuotaExceeded
		}
	}

	// Global cap: enforced when > 0.
	if totalLimit > 0 {
		total, err := repo.GetTotalStorageUsed(ctx)
		if err != nil {
			return err
		}
		if total+incomingBytes > totalLimit {
			return ErrQuotaExceeded
		}
	}

	return nil
}

// GetUserQuota returns the per-user storage limit from the User row.
// 0 means unlimited. No fallback to any server-side default — the default is
// only consulted at account creation, not at check time. Uses the checker's
// base repo; for quota checks inside a transaction, prefer Check() which
// takes the tx repo as an argument.
func (qc *QuotaChecker) GetUserQuota(ctx context.Context, userID int64) int64 {
	user, err := qc.repo.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		return 0
	}
	return user.StorageQuota
}
