package storage

import (
	"context"
	"errors"
	"sync"
	"time"

	"rstash/internal/db"
)

// ErrBandwidthExceeded is returned when serving a document would exceed the
// user's current-period egress quota.
var ErrBandwidthExceeded = errors.New("bandwidth exceeded")

// BandwidthConfig holds egress tracking/enforcement settings.
type BandwidthConfig struct {
	Mode      string // "off" or "user"
	UserLimit int64  // default per-user monthly limit (bytes)
}

// BandwidthTracker buffers in-memory counter increments and flushes them to
// the DB every flushInterval. Read path: one mutex-protected map update;
// background goroutine flushes batched deltas. Zero cost when Mode=="off":
// all Record() / ShouldServe() calls return immediately.
type BandwidthTracker struct {
	repo     *db.Repository
	mu       sync.Mutex
	pending  map[int64]int64 // userID -> pending bytes for current period
	config   BandwidthConfig
	done     chan struct{}
	stop     sync.Once
	interval time.Duration
}

// NewBandwidthTracker returns a tracker. Call Start to begin the flush loop.
func NewBandwidthTracker(cfg BandwidthConfig, repo *db.Repository) *BandwidthTracker {
	return &BandwidthTracker{
		repo:     repo,
		pending:  make(map[int64]int64),
		config:   cfg,
		done:     make(chan struct{}),
		interval: 10 * time.Second,
	}
}

// UpdateConfig swaps the tracker's config. Pending counters keep their prior
// routing (they flush under whatever mode they were recorded in).
func (bt *BandwidthTracker) UpdateConfig(cfg BandwidthConfig) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.config = cfg
}

// Start launches the flush goroutine. Safe to call once.
func (bt *BandwidthTracker) Start(ctx context.Context) {
	go bt.flushLoop(ctx)
}

// Stop shuts down the flush goroutine after a final flush.
func (bt *BandwidthTracker) Stop(ctx context.Context) {
	bt.stop.Do(func() {
		close(bt.done)
		bt.flushNow(ctx)
	})
}

// Record queues bytes of egress for userID to be flushed later.
// No-op when bandwidth_mode=off.
func (bt *BandwidthTracker) Record(userID int64, bytes int64) {
	if bytes <= 0 {
		return
	}
	bt.mu.Lock()
	defer bt.mu.Unlock()
	if bt.config.Mode == "off" {
		return
	}
	bt.pending[userID] += bytes
}

// EffectiveUserQuota returns the per-user egress limit, using the user's
// override when set or the default otherwise.
func (bt *BandwidthTracker) EffectiveUserQuota(userOverride int64) int64 {
	if userOverride > 0 {
		return userOverride
	}
	return bt.config.UserLimit
}

// CheckServe decides whether serving an additional bytes to userID is permitted
// under the current-period quota. Returns ErrBandwidthExceeded when it is not.
// Returns nil when Mode=="off" (no enforcement).
func (bt *BandwidthTracker) CheckServe(ctx context.Context, userID int64, bytes int64, userOverride int64) error {
	if bt.config.Mode == "off" || bytes <= 0 {
		return nil
	}
	period := db.CurrentPeriod()
	usage, err := bt.repo.GetBandwidthUsage(ctx, userID, period)
	if err != nil {
		return err
	}
	var used int64
	if usage != nil {
		used = usage.BytesOut
	}
	// Include the pending unflushed bytes for this user so rapid-fire reads
	// near the quota boundary don't over-serve.
	bt.mu.Lock()
	used += bt.pending[userID]
	bt.mu.Unlock()

	limit := bt.EffectiveUserQuota(userOverride)
	if limit > 0 && used+bytes > limit {
		return ErrBandwidthExceeded
	}
	return nil
}

// UsagePeriod returns the current UTC period string ("YYYY-MM").
func (bt *BandwidthTracker) UsagePeriod() string { return db.CurrentPeriod() }

func (bt *BandwidthTracker) flushLoop(ctx context.Context) {
	ticker := time.NewTicker(bt.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			bt.flushNow(ctx)
			return
		case <-bt.done:
			return
		case <-ticker.C:
			bt.flushNow(ctx)
		}
	}
}

func (bt *BandwidthTracker) flushNow(ctx context.Context) {
	bt.mu.Lock()
	if len(bt.pending) == 0 {
		bt.mu.Unlock()
		return
	}
	batch := bt.pending
	bt.pending = make(map[int64]int64)
	bt.mu.Unlock()

	period := db.CurrentPeriod()
	for userID, bytes := range batch {
		if err := bt.repo.AddBandwidthUsage(ctx, userID, period, bytes); err != nil {
			// Requeue so we try again next tick.
			bt.mu.Lock()
			bt.pending[userID] += bytes
			bt.mu.Unlock()
		}
	}
}
