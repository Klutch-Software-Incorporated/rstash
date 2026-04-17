package storage

import (
	"context"
	"errors"
	"sync"
	"time"

	"rstash/internal/db"
)

// ErrEgressExceeded is returned when serving a document would exceed the
// user's current-period egress quota.
var ErrEgressExceeded = errors.New("egress exceeded")

// EgressConfig holds egress tracking/enforcement settings.
type EgressConfig struct {
	Mode      string // "off" or "user"
	UserLimit int64  // default per-user monthly egress limit (bytes)
}

// EgressTracker buffers in-memory counter increments and flushes them to
// the DB every flushInterval. Read path: one mutex-protected map update;
// background goroutine flushes batched deltas. Zero cost when Mode=="off":
// Record() / CheckServe() calls return immediately.
//
// Only outbound transfer (egress) is tracked. Uploads (ingress) are bounded
// by storage quota and max_upload_size, not by this tracker.
type EgressTracker struct {
	repo     *db.Repository
	mu       sync.Mutex
	pending  map[int64]int64 // userID -> pending egress bytes for current period
	config   EgressConfig
	done     chan struct{}
	stop     sync.Once
	interval time.Duration
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
// No-op when egress_mode=off.
func (et *EgressTracker) Record(userID int64, bytes int64) {
	if bytes <= 0 {
		return
	}
	et.mu.Lock()
	defer et.mu.Unlock()
	if et.config.Mode == "off" {
		return
	}
	et.pending[userID] += bytes
}

// EffectiveUserQuota returns the per-user egress limit, using the user's
// override when set or the default otherwise.
func (et *EgressTracker) EffectiveUserQuota(userOverride int64) int64 {
	if userOverride > 0 {
		return userOverride
	}
	return et.config.UserLimit
}

// CheckServe decides whether serving an additional bytes to userID is permitted
// under the current-period egress quota. Returns ErrEgressExceeded when it is
// not. Returns nil when Mode=="off" (no enforcement).
func (et *EgressTracker) CheckServe(ctx context.Context, userID int64, bytes int64, userOverride int64) error {
	if et.config.Mode == "off" || bytes <= 0 {
		return nil
	}
	period := db.CurrentPeriod()
	usage, err := et.repo.GetEgressUsage(ctx, userID, period)
	if err != nil {
		return err
	}
	var used int64
	if usage != nil {
		used = usage.BytesOut
	}
	// Include the pending unflushed bytes for this user so rapid-fire reads
	// near the quota boundary don't over-serve.
	et.mu.Lock()
	used += et.pending[userID]
	et.mu.Unlock()

	limit := et.EffectiveUserQuota(userOverride)
	if limit > 0 && used+bytes > limit {
		return ErrEgressExceeded
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
	et.mu.Unlock()

	period := db.CurrentPeriod()
	for userID, bytes := range batch {
		if err := et.repo.AddEgressUsage(ctx, userID, period, bytes); err != nil {
			// Requeue so we try again next tick.
			et.mu.Lock()
			et.pending[userID] += bytes
			et.mu.Unlock()
		}
	}
}
