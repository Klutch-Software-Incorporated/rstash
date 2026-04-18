package storage

import (
	"context"
	"errors"
	"sync"
	"time"

	"rstash/internal/db"
)

// ErrEgressExceeded is returned when serving a document would exceed either
// the per-user or global monthly egress limit.
var ErrEgressExceeded = errors.New("egress exceeded")

// EgressConfig holds live egress enforcement settings.
//
// The per-user limit is not held here — it lives on User.EgressQuota and is
// consulted directly. The default_user_egress_limit setting is only read at
// user creation time (stamped onto the User row) and is not referenced here.
type EgressConfig struct {
	TotalLimit int64 // global monthly cap in bytes (0 = disabled)
}

// EgressTracker buffers in-memory counter increments and flushes them to
// the DB every flushInterval. Read path: one mutex-protected map update;
// background goroutine flushes batched deltas.
//
// Only outbound transfer (egress) is tracked. Uploads (ingress) are bounded
// by storage quota and max_upload_size, not by this tracker.
type EgressTracker struct {
	repo         *db.Repository
	mu           sync.Mutex
	pending      map[int64]int64 // userID -> pending egress bytes for current period
	pendingTotal int64           // sum of pending across all users (for global cap check)
	config       EgressConfig
	done         chan struct{}
	stop         sync.Once
	interval     time.Duration
}

// NewEgressTracker returns a tracker. Call Start to begin the flush loop.
func NewEgressTracker(cfg EgressConfig, repo *db.Repository) *EgressTracker {
	return &EgressTracker{
		repo:     repo,
		pending:  make(map[int64]int64),
		config:   cfg,
		done:     make(chan struct{}),
		interval: 10 * time.Second,
	}
}

// UpdateConfig swaps the tracker's config. Pending counters keep their prior
// routing (they flush under whatever mode they were recorded in).
func (et *EgressTracker) UpdateConfig(cfg EgressConfig) {
	et.mu.Lock()
	defer et.mu.Unlock()
	et.config = cfg
}

// Start launches the flush goroutine. Safe to call once.
func (et *EgressTracker) Start(ctx context.Context) {
	go et.flushLoop(ctx)
}

// Stop shuts down the flush goroutine after a final flush.
func (et *EgressTracker) Stop(ctx context.Context) {
	et.stop.Do(func() {
		close(et.done)
		et.flushNow(ctx)
	})
}

// Record queues bytes of egress for userID to be flushed later.
func (et *EgressTracker) Record(userID int64, bytes int64) {
	if bytes <= 0 {
		return
	}
	et.mu.Lock()
	defer et.mu.Unlock()
	et.pending[userID] += bytes
	et.pendingTotal += bytes
}

// CheckServe decides whether serving an additional bytes to userID is permitted
// under the current-period egress limits. Returns ErrEgressExceeded when it is
// not.
//
// Per-user enforcement consults User.EgressQuota (0 = unlimited). Global
// enforcement consults EgressConfig.TotalLimit (0 = disabled) against the sum
// of all users' recorded egress for the current period plus any pending
// in-memory bytes.
func (et *EgressTracker) CheckServe(ctx context.Context, userID int64, bytes int64, userLimit int64) error {
	if bytes <= 0 {
		return nil
	}
	period := db.CurrentPeriod()

	et.mu.Lock()
	totalLimit := et.config.TotalLimit
	pendingUser := et.pending[userID]
	pendingTotal := et.pendingTotal
	et.mu.Unlock()

	// Per-user check: User.EgressQuota is authoritative. 0 = unlimited.
	if userLimit > 0 {
		usage, err := et.repo.GetEgressUsage(ctx, userID, period)
		if err != nil {
			return err
		}
		var used int64
		if usage != nil {
			used = usage.BytesOut
		}
		used += pendingUser
		if used+bytes > userLimit {
			return ErrEgressExceeded
		}
	}

	// Global cap: enforced when > 0.
	if totalLimit > 0 {
		total, err := et.repo.GetTotalEgressUsage(ctx, period)
		if err != nil {
			return err
		}
		total += pendingTotal
		if total+bytes > totalLimit {
			return ErrEgressExceeded
		}
	}

	return nil
}

// UsagePeriod returns the current UTC period string ("YYYY-MM").
func (et *EgressTracker) UsagePeriod() string { return db.CurrentPeriod() }

func (et *EgressTracker) flushLoop(ctx context.Context) {
	ticker := time.NewTicker(et.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			et.flushNow(ctx)
			return
		case <-et.done:
			return
		case <-ticker.C:
			et.flushNow(ctx)
		}
	}
}

func (et *EgressTracker) flushNow(ctx context.Context) {
	et.mu.Lock()
	if len(et.pending) == 0 {
		et.mu.Unlock()
		return
	}
	batch := et.pending
	et.pending = make(map[int64]int64)
	et.pendingTotal = 0
	et.mu.Unlock()

	period := db.CurrentPeriod()
	for userID, bytes := range batch {
		if err := et.repo.AddEgressUsage(ctx, userID, period, bytes); err != nil {
			// Requeue so we try again next tick.
			et.mu.Lock()
			et.pending[userID] += bytes
			et.pendingTotal += bytes
			et.mu.Unlock()
		}
	}
}
