# Billing Integration Design Notes

Captured March 2026 during Phase 2 planning. Not implemented — saved for future reference.

## Decision: Defer Billing

The existing per-user quota override (`StorageQuota` on User, settable by admin) is sufficient
for manual account management. Formal billing should wait until there's real demand for hosted
accounts.

## Recommended Provider: MoR (Merchant of Record)

Evaluated three MoR providers so rstash doesn't handle tax/VAT compliance:

| Provider | Fee | Pros | Cons |
|----------|-----|------|------|
| **Paddle** | 5% + 50c | Most established, proven, Go SDK, no account closure horror stories | Complex dashboard, strict approval process (may require processing history) |
| **Polar** | 4% + 40c | Developer/OSS focused, cheapest, open-source, clean APIs | Young (v1.0 Sept 2024), reports of approved accounts being reversed, card-only |
| **Dodo Payments** | Lowest | Go SDK, 220+ countries, quick integration | Very new, reports of account closures and held balances |

**Recommendation:** Paddle for stability, Polar if you want alignment with OSS values and can
accept the risk of a younger platform.

## Architecture: Provider as Quota Source

The billing provider (whichever is chosen) writes quotas directly — no intermediate "plans"
system in the app.

### Flow
1. User signs up (free tier = server default `quota_user`)
2. User clicks "Upgrade" → hosted checkout (Paddle/Polar/Dodo overlay)
3. Webhook fires → read product metadata `storage_quota` → set `user.StorageQuota`
4. User cancels → webhook → reset `StorageQuota` to 0 (back to server default)

### User Model Changes
```go
// Add to User struct:
PaddleCustomerID   string `gorm:"size:255"` // or generic BillingCustomerID
SubscriptionStatus string `gorm:"size:32"`  // "active", "past_due", "canceled", ""
Plan               string `gorm:"size:64"`  // display name from product, e.g. "Pro"
```

### Config
```
RSTASH_PADDLE_API_KEY        — API key (empty = billing disabled)
RSTASH_PADDLE_WEBHOOK_SECRET — webhook signature verification
```
Price IDs to show on upgrade page: runtime setting (so they can change without restart).

### Webhook Behavior
- `subscription.created` / `subscription.updated` (status=active): read product metadata `storage_quota`, parse as byte size, set `user.StorageQuota` + `user.Plan` (product name)
- `subscription.updated` (cancel_at_period_end=true): show "canceling" in UI, keep quota until period ends
- `subscription.deleted`: reset `StorageQuota` to 0, clear `Plan`
- `past_due`: keep full quota (don't punish during payment retry window)

### UI Changes
- Hide all billing UI when API key is empty (like `HasMailer` pattern → `HasBilling`)
- Profile: show plan badge + "Manage Billing" link (creates customer portal session)
- Upgrade page: show available prices from runtime setting
- Admin status: plan distribution counts

### Interface Option (if multi-provider needed later)
```go
type BillingProvider interface {
    CheckoutURL(ctx context.Context, customerID, priceID, successURL, cancelURL string) (string, error)
    PortalURL(ctx context.Context, customerID string) (string, error)
    ParseWebhook(r *http.Request) (*BillingEvent, error)
}

type BillingEvent struct {
    Type         string // "subscription.created", "subscription.updated", "subscription.canceled"
    CustomerID   string
    PlanName     string
    StorageQuota int64  // bytes, from product metadata
    Status       string // "active", "past_due", "canceled"
}
```

Each provider adapter is ~100-150 lines. Not needed if only supporting one provider.

### Paddle Approval Requirements
- Live website with HTTPS, product description, screenshots
- Static pricing page (doesn't need working checkout)
- Terms of Service, Privacy Policy, Refund Policy
- Identity verification (individual)
- May request 3-month processing statement (not always required)
- Timeline: auto-approve or 5-7 business days for manual review
- Sandbox available immediately (no approval needed)

### Key Design Decisions
- Webhook sets `StorageQuota` directly — no intermediate plans_config or plan definitions in the app
- Admin per-user quota override still wins (set StorageQuota > 0 manually)
- Free tier = no subscription, server default quota applies
- Downgrade at end of billing period, not immediately
- Keep access during payment retry window (past_due)
- Store billing customer ID on User for portal session creation
