package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"rstash/internal/model"

	"gorm.io/gorm"
)

// ListWebhookSubscriptions returns all subscriptions, most-recently-created first.
func (r *Repository) ListWebhookSubscriptions(ctx context.Context) ([]*model.WebhookSubscription, error) {
	var subs []*model.WebhookSubscription
	if err := r.db.WithContext(ctx).Order("created_at DESC").Find(&subs).Error; err != nil {
		return nil, fmt.Errorf("list webhook subscriptions: %w", err)
	}
	return subs, nil
}

// GetWebhookSubscription returns a subscription by ID, or nil if not found.
func (r *Repository) GetWebhookSubscription(ctx context.Context, id int64) (*model.WebhookSubscription, error) {
	var s model.WebhookSubscription
	err := r.db.WithContext(ctx).First(&s, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get webhook subscription: %w", err)
	}
	return &s, nil
}

// CreateWebhookSubscription inserts a new subscription.
func (r *Repository) CreateWebhookSubscription(ctx context.Context, s *model.WebhookSubscription) error {
	if err := r.db.WithContext(ctx).Create(s).Error; err != nil {
		return fmt.Errorf("create webhook subscription: %w", err)
	}
	return nil
}

// UpdateWebhookSubscription saves changes to a subscription's mutable fields.
func (r *Repository) UpdateWebhookSubscription(ctx context.Context, s *model.WebhookSubscription) error {
	err := r.db.WithContext(ctx).Model(&model.WebhookSubscription{}).
		Where("id = ?", s.ID).
		Updates(map[string]any{
			"name":   s.Name,
			"url":    s.URL,
			"events": s.Events,
			"active": s.Active,
		}).Error
	if err != nil {
		return fmt.Errorf("update webhook subscription: %w", err)
	}
	return nil
}

// DeleteWebhookSubscription removes a subscription and any pending deliveries.
func (r *Repository) DeleteWebhookSubscription(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("subscription_id = ?", id).Delete(&model.WebhookDelivery{}).Error; err != nil {
			return fmt.Errorf("delete deliveries: %w", err)
		}
		if err := tx.Delete(&model.WebhookSubscription{}, id).Error; err != nil {
			return fmt.Errorf("delete subscription: %w", err)
		}
		return nil
	})
}

// ListActiveWebhookSubscriptionsForEvent returns subscriptions whose Events
// field includes the given event name (or "*").
func (r *Repository) ListActiveWebhookSubscriptionsForEvent(ctx context.Context, event string) ([]*model.WebhookSubscription, error) {
	var subs []*model.WebhookSubscription
	if err := r.db.WithContext(ctx).Where("active = ?", true).Find(&subs).Error; err != nil {
		return nil, fmt.Errorf("list subscriptions for event: %w", err)
	}
	// Filter in Go so we don't need dialect-specific LIKE/space handling.
	filtered := subs[:0]
	for _, s := range subs {
		if subscriptionMatchesEvent(s.Events, event) {
			filtered = append(filtered, s)
		}
	}
	return filtered, nil
}

func subscriptionMatchesEvent(events, event string) bool {
	for _, e := range splitFields(events) {
		if e == "*" || e == event {
			return true
		}
	}
	return false
}

func splitFields(s string) []string {
	out := []string{}
	start := -1
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ' ' || s[i] == '\t' || s[i] == '\n' {
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	return out
}

// CreateWebhookDelivery inserts a pending delivery row.
func (r *Repository) CreateWebhookDelivery(ctx context.Context, d *model.WebhookDelivery) error {
	if err := r.db.WithContext(ctx).Create(d).Error; err != nil {
		return fmt.Errorf("create webhook delivery: %w", err)
	}
	return nil
}

// ClaimPendingWebhookDeliveries returns up to n deliveries due for attempt.
// Callers must update Attempts/NextAttemptAt or DeliveredAt after processing.
func (r *Repository) ClaimPendingWebhookDeliveries(ctx context.Context, n int) ([]*model.WebhookDelivery, error) {
	var deliveries []*model.WebhookDelivery
	err := r.db.WithContext(ctx).
		Where("delivered_at IS NULL AND next_attempt_at <= ?", time.Now().UTC()).
		Order("next_attempt_at ASC").
		Limit(n).
		Find(&deliveries).Error
	if err != nil {
		return nil, fmt.Errorf("claim webhook deliveries: %w", err)
	}
	return deliveries, nil
}

// MarkWebhookDeliverySuccess records a 2xx result.
func (r *Repository) MarkWebhookDeliverySuccess(ctx context.Context, deliveryID, subID int64) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.WebhookDelivery{}).Where("id = ?", deliveryID).
			Updates(map[string]any{"delivered_at": now, "last_error": ""}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.WebhookSubscription{}).Where("id = ?", subID).
			Updates(map[string]any{"last_success_at": now, "last_error": "", "failure_count": 0}).Error; err != nil {
			return err
		}
		return nil
	})
}

// MarkWebhookDeliveryFailure records a non-2xx/network-error result and
// schedules the next attempt using exponential backoff. When attempts >= maxAttempts,
// no further retry is scheduled (next_attempt_at remains unchanged) and the
// failure_count on the subscription is incremented.
func (r *Repository) MarkWebhookDeliveryFailure(ctx context.Context, deliveryID, subID int64, attempts int, nextAttempt time.Time, errMsg string, permanent bool) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"attempts":   attempts,
			"last_error": errMsg,
		}
		if !permanent {
			updates["next_attempt_at"] = nextAttempt
		} else {
			// Park the delivery far in the future so it is not retried.
			updates["next_attempt_at"] = time.Now().UTC().Add(100 * 365 * 24 * time.Hour)
		}
		if err := tx.Model(&model.WebhookDelivery{}).Where("id = ?", deliveryID).Updates(updates).Error; err != nil {
			return err
		}
		subUpdates := map[string]any{
			"last_error_at": now,
			"last_error":    errMsg,
		}
		if permanent {
			if err := tx.Model(&model.WebhookSubscription{}).Where("id = ?", subID).
				UpdateColumn("failure_count", gorm.Expr("failure_count + 1")).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&model.WebhookSubscription{}).Where("id = ?", subID).
			Updates(subUpdates).Error; err != nil {
			return err
		}
		return nil
	})
}
