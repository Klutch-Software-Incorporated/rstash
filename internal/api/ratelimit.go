package api

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	last   time.Time
}

// RateLimiter implements a per-key token bucket rate limiter.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64 // tokens per second
	burst   float64 // max tokens (bucket capacity)
	done    chan struct{}
	stop    sync.Once
}

// NewRateLimiter creates a rate limiter that allows rate requests/sec with the
// given burst size. It starts a background goroutine that sweeps stale entries
// every 5 minutes. Call Stop to release the goroutine.
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		burst:   float64(burst),
		done:    make(chan struct{}),
	}
	go rl.sweepLoop()
	return rl
}

// Allow reports whether one request for the given key is allowed. If denied, it
// returns the duration the caller should wait before retrying.
// When rate is 0, all requests are allowed (rate limiting disabled).
func (rl *RateLimiter) Allow(key string) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.rate == 0 {
		return true, 0
	}

	now := time.Now()
	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{tokens: rl.burst, last: now}
		rl.buckets[key] = b
	}

	// Refill tokens based on elapsed time.
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	b.last = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}

	// Calculate retry-after: time until tokens reach 1.
	deficit := 1 - b.tokens
	retryAfter := time.Duration(math.Ceil(deficit/rl.rate*1000)) * time.Millisecond
	return false, retryAfter
}

// UpdateConfig changes the rate and burst for new requests. Existing buckets
// continue with the updated parameters on their next Allow() call.
func (rl *RateLimiter) UpdateConfig(rate float64, burst int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.rate = rate
	rl.burst = float64(burst)
}

// Stop shuts down the background sweep goroutine. Safe to call multiple times.
func (rl *RateLimiter) Stop() {
	rl.stop.Do(func() { close(rl.done) })
}

func (rl *RateLimiter) sweepLoop() {
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

// sweep removes buckets that have fully refilled (idle clients).
func (rl *RateLimiter) sweep() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for key, b := range rl.buckets {
		elapsed := now.Sub(b.last).Seconds()
		refilled := b.tokens + elapsed*rl.rate
		if refilled >= rl.burst {
			delete(rl.buckets, key)
		}
	}
}

// RateLimit returns middleware that enforces per-IP rate limiting. When a
// request is denied, it responds with 429 Too Many Requests and a Retry-After
// header.
func RateLimit(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			allowed, retryAfter := limiter.Allow(ip)
			if !allowed {
				secs := int(math.Ceil(retryAfter.Seconds()))
				if secs < 1 {
					secs = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// UserRateLimit returns middleware that enforces per-user rate limiting on
// storage routes. The {user} path value must be set by the mux before this
// middleware runs; if it's empty, the middleware passes through.
//
// Per-user limits complement the per-IP limiter: one user rotating through
// many IPs (CGNAT, mobile, VPN) can evade per-IP caps. The per-user bucket
// ties abuse budget to the account, not the network path.
//
// When the limiter's rate is 0, Allow returns immediately — effectively
// zero cost, so it's safe to always wrap.
func UserRateLimit(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := r.PathValue("user")
			if user == "" {
				next.ServeHTTP(w, r)
				return
			}
			allowed, retryAfter := limiter.Allow("user:" + user)
			if !allowed {
				secs := int(math.Ceil(retryAfter.Seconds()))
				if secs < 1 {
					secs = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP extracts the IP address from the request's RemoteAddr.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
