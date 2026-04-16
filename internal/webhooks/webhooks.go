// Package webhooks implements the rstash webhook outbox: admin-registered
// subscribers receive state-change events via HMAC-signed HTTP POSTs, with
// exponential-backoff retries handled by a background worker.
package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"rstash/internal/db"
	"rstash/internal/model"
)

// Emitter publishes state-change events to registered webhook subscribers.
// Emit inserts a delivery row per matching subscription; the background
// worker performs the actual POSTs.
type Emitter struct {
	repo *db.Repository
}

// NewEmitter returns an Emitter that uses the given repository for storage.
func NewEmitter(repo *db.Repository) *Emitter {
	return &Emitter{repo: repo}
}

// Emit fanouts an event to all active subscriptions that match the event name.
// Errors from subscription lookup are logged but not returned — webhook delivery
// should never block the calling flow.
func (e *Emitter) Emit(ctx context.Context, event Event, data any) {
	if e == nil || e.repo == nil {
		return
	}
	name := string(event)
	subs, err := e.repo.ListActiveWebhookSubscriptionsForEvent(ctx, name)
	if err != nil {
		slog.Error("webhook: list subscriptions", "event", name, "error", err)
		return
	}
	if len(subs) == 0 {
		return
	}

	payload, err := json.Marshal(map[string]any{
		"event":     name,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"data":      data,
	})
	if err != nil {
		slog.Error("webhook: marshal payload", "event", name, "error", err)
		return
	}

	for _, s := range subs {
		d := &model.WebhookDelivery{
			SubscriptionID: s.ID,
			Event:          name,
			Payload:        payload,
			NextAttemptAt:  time.Now().UTC(),
		}
		if err := e.repo.CreateWebhookDelivery(ctx, d); err != nil {
			slog.Error("webhook: create delivery", "sub", s.ID, "event", name, "error", err)
		}
	}
}

// sign returns the hex-encoded HMAC-SHA256 of body using secret.
func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// backoffSchedule returns the delay to the next attempt given how many attempts
// have already been made. After maxAttempts, the caller should mark permanent.
var backoffSchedule = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	1 * time.Hour,
	6 * time.Hour,
	24 * time.Hour,
}

// MaxAttempts is the failure count at which a delivery is considered permanently
// failed and no further retry is scheduled.
const MaxAttempts = 8

func nextAttempt(attempts int) time.Duration {
	idx := attempts - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(backoffSchedule) {
		idx = len(backoffSchedule) - 1
	}
	return backoffSchedule[idx]
}

// Worker is the background goroutine that processes pending deliveries.
type Worker struct {
	repo     *db.Repository
	client   *http.Client
	interval time.Duration
	batch    int
	done     chan struct{}
}

// NewWorker constructs a delivery worker. Call Start to run it.
func NewWorker(repo *db.Repository) *Worker {
	return &Worker{
		repo:     repo,
		client:   &http.Client{Timeout: 30 * time.Second},
		interval: 15 * time.Second,
		batch:    50,
		done:     make(chan struct{}),
	}
}

// Start runs the worker until ctx is cancelled or Stop is called.
func (w *Worker) Start(ctx context.Context) {
	go w.loop(ctx)
}

// Stop signals the worker to exit.
func (w *Worker) Stop() {
	close(w.done)
}

func (w *Worker) loop(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.done:
			return
		case <-ticker.C:
			w.drain(ctx)
		}
	}
}

func (w *Worker) drain(ctx context.Context) {
	deliveries, err := w.repo.ClaimPendingWebhookDeliveries(ctx, w.batch)
	if err != nil {
		slog.Error("webhook: claim deliveries", "error", err)
		return
	}
	for _, d := range deliveries {
		w.deliver(ctx, d)
	}
}

func (w *Worker) deliver(ctx context.Context, d *model.WebhookDelivery) {
	sub, err := w.repo.GetWebhookSubscription(ctx, d.SubscriptionID)
	if err != nil || sub == nil {
		// Subscription gone — park delivery permanently.
		_ = w.repo.MarkWebhookDeliveryFailure(ctx, d.ID, d.SubscriptionID, d.Attempts+1, time.Time{}, "subscription missing", true)
		return
	}

	req, err := http.NewRequestWithContext(ctx, "POST", sub.URL, strings.NewReader(string(d.Payload)))
	if err != nil {
		w.scheduleRetry(ctx, d, fmt.Sprintf("build request: %v", err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "rstash-webhooks/1.0")
	req.Header.Set("X-Rstash-Event", d.Event)
	req.Header.Set("X-Rstash-Delivery-Id", strconv.FormatInt(d.ID, 10))
	req.Header.Set("X-Rstash-Signature", "sha256="+sign(sub.Secret, d.Payload))

	resp, err := w.client.Do(req)
	if err != nil {
		w.scheduleRetry(ctx, d, fmt.Sprintf("http: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := w.repo.MarkWebhookDeliverySuccess(ctx, d.ID, d.SubscriptionID); err != nil {
			slog.Error("webhook: mark success", "delivery", d.ID, "error", err)
		}
		return
	}

	w.scheduleRetry(ctx, d, fmt.Sprintf("http %d", resp.StatusCode))
}

func (w *Worker) scheduleRetry(ctx context.Context, d *model.WebhookDelivery, reason string) {
	attempts := d.Attempts + 1
	if attempts >= MaxAttempts {
		slog.Warn("webhook: delivery permanently failed", "delivery", d.ID, "attempts", attempts, "reason", reason)
		_ = w.repo.MarkWebhookDeliveryFailure(ctx, d.ID, d.SubscriptionID, attempts, time.Time{}, reason, true)
		return
	}
	next := time.Now().UTC().Add(nextAttempt(attempts))
	_ = w.repo.MarkWebhookDeliveryFailure(ctx, d.ID, d.SubscriptionID, attempts, next, reason, false)
}

// GenerateSecret returns a cryptographically random hex secret for a new subscription.
func GenerateSecret() (string, error) {
	return db.GenerateAPIKey() // 32 random bytes, hex-encoded
}
