package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestAllow_WithinBurst(t *testing.T) {
	rl := NewRateLimiter(10, 5)
	defer rl.Stop()

	for i := 0; i < 5; i++ {
		ok, _ := rl.Allow("a")
		if !ok {
			t.Fatalf("request %d should be allowed within burst", i+1)
		}
	}
}

func TestAllow_DenyAfterBurstExhausted(t *testing.T) {
	rl := NewRateLimiter(10, 2)
	defer rl.Stop()

	rl.Allow("a")
	rl.Allow("a")

	ok, retryAfter := rl.Allow("a")
	if ok {
		t.Fatal("3rd request should be denied after burst exhausted")
	}
	if retryAfter <= 0 {
		t.Fatal("retryAfter should be positive")
	}
}

func TestAllow_RefillOverTime(t *testing.T) {
	rl := NewRateLimiter(100, 1)
	defer rl.Stop()

	ok, _ := rl.Allow("a")
	if !ok {
		t.Fatal("first request should be allowed")
	}

	ok, _ = rl.Allow("a")
	if ok {
		t.Fatal("second request should be denied immediately")
	}

	time.Sleep(15 * time.Millisecond)

	ok, _ = rl.Allow("a")
	if !ok {
		t.Fatal("request should be allowed after refill")
	}
}

func TestAllow_RetryAfterReasonable(t *testing.T) {
	rl := NewRateLimiter(1, 1)
	defer rl.Stop()

	rl.Allow("a") // exhaust

	_, retryAfter := rl.Allow("a")
	if retryAfter < 500*time.Millisecond || retryAfter > 1100*time.Millisecond {
		t.Fatalf("retryAfter should be ~1s, got %v", retryAfter)
	}
}

func TestAllow_IndependentKeys(t *testing.T) {
	rl := NewRateLimiter(10, 1)
	defer rl.Stop()

	ok, _ := rl.Allow("a")
	if !ok {
		t.Fatal("key a first request should be allowed")
	}

	ok, _ = rl.Allow("b")
	if !ok {
		t.Fatal("key b first request should be allowed (independent)")
	}

	ok, _ = rl.Allow("a")
	if ok {
		t.Fatal("key a second request should be denied")
	}
}

func TestStop_SafeToCallTwice(t *testing.T) {
	rl := NewRateLimiter(10, 5)
	rl.Stop()
	rl.Stop() // should not panic
}

func TestSweep_RemovesFullBuckets(t *testing.T) {
	rl := NewRateLimiter(1000, 1)
	defer rl.Stop()

	rl.Allow("a")
	rl.Allow("a") // exhaust

	// Manually advance the bucket's last time to simulate staleness.
	rl.mu.Lock()
	rl.buckets["a"].last = time.Now().Add(-10 * time.Second)
	rl.mu.Unlock()

	rl.sweep()

	rl.mu.Lock()
	_, exists := rl.buckets["a"]
	rl.mu.Unlock()
	if exists {
		t.Fatal("bucket should have been swept")
	}
}

func TestMiddleware_AllowedReturns200(t *testing.T) {
	rl := NewRateLimiter(10, 5)
	defer rl.Stop()

	handler := RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestMiddleware_DeniedReturns429WithRetryAfter(t *testing.T) {
	rl := NewRateLimiter(1, 1)
	defer rl.Stop()

	handler := RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req) // first — allowed

	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req) // second — denied

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}

	ra := rec.Header().Get("Retry-After")
	if ra == "" {
		t.Fatal("expected Retry-After header")
	}
	secs, err := strconv.Atoi(ra)
	if err != nil {
		t.Fatalf("Retry-After should be an integer, got %q", ra)
	}
	if secs < 1 {
		t.Fatalf("Retry-After should be >= 1, got %d", secs)
	}
}

func TestUserRateLimit_KeysByUsername(t *testing.T) {
	rl := NewRateLimiter(1, 2) // rate=1/sec, burst=2 so each user gets 2 free requests
	defer rl.Stop()

	handler := UserRateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	send := func(user string) int {
		req := httptest.NewRequest("GET", "/storage/"+user+"/notes/foo", nil)
		req.SetPathValue("user", user)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	// Alice uses her whole burst.
	for i := 0; i < 2; i++ {
		if code := send("alice"); code != http.StatusOK {
			t.Fatalf("alice req %d: expected 200, got %d", i+1, code)
		}
	}

	// Alice's next request hits the cap.
	if code := send("alice"); code != http.StatusTooManyRequests {
		t.Fatalf("alice over-cap req: expected 429, got %d", code)
	}

	// Bob has his own bucket and is unaffected by alice.
	for i := 0; i < 2; i++ {
		if code := send("bob"); code != http.StatusOK {
			t.Fatalf("bob req %d: expected 200 (not affected by alice), got %d", i+1, code)
		}
	}
}

func TestUserRateLimit_DisabledWhenRateZero(t *testing.T) {
	rl := NewRateLimiter(0, 0) // disabled
	defer rl.Stop()

	handler := UserRateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 100; i++ {
		req := httptest.NewRequest("GET", "/storage/alice/notes/foo", nil)
		req.SetPathValue("user", "alice")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("req %d denied when limiter disabled", i)
		}
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{"ipv4 with port", "192.168.1.1:12345", "192.168.1.1"},
		{"ipv6 with port", "[::1]:12345", "::1"},
		{"bare address", "192.168.1.1", "192.168.1.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{RemoteAddr: tt.remoteAddr}
			got := clientIP(r)
			if got != tt.want {
				t.Fatalf("clientIP(%q) = %q, want %q", tt.remoteAddr, got, tt.want)
			}
		})
	}
}
