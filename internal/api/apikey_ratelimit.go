package api

import (
	"math"
	"sync"
	"time"
)

// apiKeyBucket is a per-APIKey token bucket. Unlike the per-IP RateLimiter,
// each APIKey may have a different refill rate (RateLimitRPM on the model).
type apiKeyBucket struct {
	tokens float64
	last   time.Time
}

// APIKeyRateLimiter enforces per-API-key token-bucket rate limits. The rate
// (requests per minute) is pulled from the APIKey record on each Allow call,
// so admin UI changes take effect immediately without restarts.
type APIKeyRateLimiter struct {
	mu      sync.Mutex
	buckets map[int64]*apiKeyBucket
	done    chan struct{}
	stop    sync.Once
}

// NewAPIKeyRateLimiter constructs a rate limiter and starts its sweep loop.
func NewAPIKeyRateLimiter() *APIKeyRateLimiter {
	rl := &APIKeyRateLimiter{
		buckets: make(map[int64]*apiKeyBucket),
		done:    make(chan struct{}),
	}
	go rl.sweepLoop()
	return rl
}

// Allow checks whether a request for the given key ID is allowed. rpm <= 0 disables
// rate limiting for this key. Burst equals rpm (fills to one minute's worth).
func (rl *APIKeyRateLimiter) Allow(keyID int64, rpm int) (bool, time.Duration) {
	if rpm <= 0 {
		return true, 0
	}
	rate := float64(rpm) / 60.0 // requests per second
	burst := float64(rpm)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[keyID]
	if !ok {
		b = &apiKeyBucket{tokens: burst, last: now}
		rl.buckets[keyID] = b
	}

	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * rate
	if b.tokens > burst {
		b.tokens = burst
	}
	b.last = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}

	deficit := 1 - b.tokens
	retryAfter := time.Duration(math.Ceil(deficit/rate*1000)) * time.Millisecond
	return false, retryAfter
}

// Stop shuts down the sweep goroutine.
func (rl *APIKeyRateLimiter) Stop() {
	rl.stop.Do(func() { close(rl.done) })
}

func (rl *APIKeyRateLimiter) sweepLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.sweep()
		case <-rl.done:
			return
		}
	}
}

// sweep removes buckets that have been idle long enough to have refilled.
// Since per-key rate is not fixed, we use a conservative timeout: any bucket
// not touched for 10 minutes is dropped.
func (rl *APIKeyRateLimiter) sweep() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-10 * time.Minute)
	for id, b := range rl.buckets {
		if b.last.Before(cutoff) {
			delete(rl.buckets, id)
		}
	}
}
