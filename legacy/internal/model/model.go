package model

import "time"

type User struct {
	ID           int64      `gorm:"primaryKey;autoIncrement"`
	Username     string     `gorm:"size:255;uniqueIndex;not null"`
	PasswordHash string     `gorm:"size:255;not null"`
	IsAdmin      bool       `gorm:"not null;default:false"`
	StorageQuota int64      `gorm:"not null;default:0"` // 0 = unlimited; stamped at creation from default_user_storage_limit
	Disabled     bool       `gorm:"not null;default:false"`
	Approved     bool       `gorm:"not null;default:true"`
	CreatedAt    time.Time  `gorm:"not null;autoCreateTime"`
	LastLoginAt  *time.Time // NULL until first login
	LastLoginIP  *string    `gorm:"size:45"` // NULL until first login
	TOSAcceptedAt     *time.Time // NULL until TOS accepted
	PrivacyAcceptedAt *time.Time // NULL until Privacy Policy accepted

	// Email fields (Phase 1: Email Foundation)
	Email              *string    `gorm:"size:254;uniqueIndex"`
	EmailVerified      bool       `gorm:"not null;default:false"`
	EmailVerifyToken   *string    `gorm:"size:255"`
	EmailVerifyExpiry  *time.Time
	PasswordResetToken  *string    `gorm:"size:255"`
	PasswordResetExpiry *time.Time

	// ExternallyManaged is true for accounts provisioned via the admin API
	// with provision=true (typically by a companion signup/billing app).
	// When true, the rstash web UI redirects email/delete flows to an
	// external management URL instead of handling them locally.
	ExternallyManaged bool `gorm:"not null;default:false"`

	// EgressQuota is the per-user egress limit in bytes per calendar month.
	// 0 = unlimited (no per-user cap). Stamped at user creation from the
	// default_user_egress_limit setting; afterwards editable per-user via
	// admin UI or admin API. Uploads are bounded separately by storage
	// quota and max_upload_size, not by this value.
	EgressQuota int64 `gorm:"not null;default:0"`
}

// IsUnclaimed returns true when a user was provisioned via the admin API
// but has not yet completed the /claim flow to set their password and log in.
// Detected by the combination of an active password-reset token and a
// never-logged-in state.
func (u *User) IsUnclaimed() bool {
	return u.LastLoginAt == nil && u.PasswordResetToken != nil && *u.PasswordResetToken != ""
}

type OAuthClient struct {
	ID          string    `gorm:"primaryKey;size:255"`
	RedirectURI string    `gorm:"size:2048;not null"`
	CreatedAt   time.Time `gorm:"not null;autoCreateTime"`
}

type OAuthToken struct {
	Token     string    `gorm:"primaryKey;size:255"`
	UserID    int64     `gorm:"not null;index"`
	ClientID  string    `gorm:"size:255;not null"`
	Scopes    string    `gorm:"size:1024;not null"` // space-separated
	CreatedAt time.Time `gorm:"not null;autoCreateTime"`
	ExpiresAt *time.Time
}

type Node struct {
	ID            int64     `gorm:"primaryKey;autoIncrement"`
	UserID        int64     `gorm:"not null;uniqueIndex:idx_nodes_user_path"`
	Path          string    `gorm:"size:512;not null;uniqueIndex:idx_nodes_user_path"`
	ContentType   string    `gorm:"size:255;default:''"`
	ContentLength int64     `gorm:"default:0"`
	ETag          string    `gorm:"size:255;not null"`
	CreatedAt     time.Time `gorm:"not null;autoCreateTime"`
	UpdatedAt     time.Time `gorm:"not null;autoUpdateTime"`
}

type Session struct {
	Token     string    `gorm:"primaryKey;size:255"`
	UserID    int64     `gorm:"not null;index"`
	CSRFToken string    `gorm:"size:255;not null"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime"`
	ExpiresAt time.Time `gorm:"not null"`
	IP        *string    `gorm:"size:45"`
}

type AuditEntry struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	ActorID    int64     `gorm:"not null"`
	Action     string    `gorm:"size:255;not null"`
	TargetType string    `gorm:"size:255;not null"`
	TargetID   string    `gorm:"size:255;not null"`
	Details    string    `gorm:"size:4096"`
	CreatedAt  time.Time `gorm:"not null;autoCreateTime;index"`
}

func (AuditEntry) TableName() string { return "audit_log" }

type RefreshToken struct {
	Token       string    `gorm:"primaryKey;size:255"`
	UserID      int64     `gorm:"not null;index"`
	ClientID    string    `gorm:"size:255;not null"`
	Scopes      string    `gorm:"size:1024;not null"` // space-separated
	AccessToken string    `gorm:"size:255;not null;index"`
	CreatedAt   time.Time `gorm:"not null;autoCreateTime"`
	ExpiresAt   *time.Time
}

type AbuseReport struct {
	ID            int64      `gorm:"primaryKey;autoIncrement"`
	ReporterEmail string     `gorm:"size:255;not null"`
	ReportedPath  string     `gorm:"size:1024;not null"`
	Reason        string     `gorm:"size:255;not null"`
	Description   *string    `gorm:"size:4096"`
	Status        string     `gorm:"size:64;not null;default:'open'"` // "open", "reviewed", "dismissed", "actioned"
	ReporterIP    *string    `gorm:"size:45"`
	ReviewerID    *int64
	ReviewNote    *string    `gorm:"size:4096"`
	CreatedAt     time.Time  `gorm:"not null;autoCreateTime"`
	ReviewedAt    *time.Time
}

type AuthorizationCode struct {
	Code                string    `gorm:"primaryKey;size:255"`
	UserID              int64     `gorm:"not null"`
	ClientID            string    `gorm:"size:255;not null"`
	RedirectURI         string    `gorm:"size:2048;not null"`
	Scopes              string    `gorm:"size:1024;not null"`
	CodeChallenge       string    `gorm:"size:255;not null"`
	CodeChallengeMethod string    `gorm:"size:32;not null"`
	CreatedAt           time.Time `gorm:"not null;autoCreateTime"`
	ExpiresAt           time.Time `gorm:"not null"`
	Used                bool      `gorm:"not null;default:false"`
}

type Setting struct {
	Key       string    `gorm:"primaryKey;size:255"`
	Value     string    `gorm:"size:4096;not null"`
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime"`
}

type EmailSend struct {
	ID        uint      `gorm:"primaryKey"`
	To        string    `gorm:"size:254;not null;index"`
	Type      string    `gorm:"size:32;not null;index"` // "test", "verification", "reset", "announcement"
	Subject   string    `gorm:"size:512;not null"`
	Status    string    `gorm:"size:16;not null"` // "sent", "failed"
	Error     string    `gorm:"size:1024"`
	CreatedAt time.Time `gorm:"not null;index"`
}

// APIKey is an admin API credential issued and managed via the admin UI.
// The raw key is shown once at creation; only the bcrypt hash is stored.
// KeyPrefix (first 8 hex chars) is indexed for fast lookup before bcrypt compare.
type APIKey struct {
	ID           int64     `gorm:"primaryKey;autoIncrement"`
	Name         string    `gorm:"size:255;not null"`
	Description  string    `gorm:"size:1024;default:''"`
	KeyHash      string    `gorm:"size:255;not null"`
	KeyPrefix    string    `gorm:"size:16;index;not null"`
	RateLimitRPM int       `gorm:"not null;default:60"`
	LastUsedAt   *time.Time
	CreatedAt    time.Time `gorm:"not null;autoCreateTime"`
}

// WebhookSubscription is an admin-registered HTTP endpoint that receives
// state-change events from rstash. Deliveries are HMAC-signed with Secret
// (shown once at creation) and retried with exponential backoff via the
// WebhookDelivery outbox.
type WebhookSubscription struct {
	ID            int64     `gorm:"primaryKey;autoIncrement"`
	Name          string    `gorm:"size:255;not null"`
	URL           string    `gorm:"size:2048;not null"`
	Secret        string    `gorm:"size:255;not null"`
	Events        string    `gorm:"size:1024;not null"` // space-separated, "*" = all
	Active        bool      `gorm:"not null;default:true"`
	LastSuccessAt *time.Time
	LastErrorAt   *time.Time
	LastError     string    `gorm:"size:1024;default:''"`
	FailureCount  int       `gorm:"not null;default:0"`
	CreatedAt     time.Time `gorm:"not null;autoCreateTime"`
}

// EgressUsage tracks per-user egress bytes by calendar-month period
// ("YYYY-MM"). Uploads are not counted. Enforcement happens in the storage
// service via GetDocument before the response body is written.
type EgressUsage struct {
	UserID    int64     `gorm:"primaryKey;autoIncrement:false"`
	Period    string    `gorm:"primaryKey;size:7"`
	BytesOut  int64     `gorm:"not null;default:0"`
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime"`
}

// WebhookDelivery is a queued delivery attempt in the outbox. The worker
// picks rows where DeliveredAt IS NULL and NextAttemptAt <= now, POSTs
// the payload, and either marks DeliveredAt or schedules retry via
// exponential backoff.
type WebhookDelivery struct {
	ID             int64     `gorm:"primaryKey;autoIncrement"`
	SubscriptionID int64     `gorm:"not null;index"`
	Event          string    `gorm:"size:64;not null"`
	Payload        []byte    `gorm:"not null"`
	Attempts       int       `gorm:"not null;default:0"`
	LastError      string    `gorm:"size:1024;default:''"`
	NextAttemptAt  time.Time `gorm:"not null;index"`
	DeliveredAt    *time.Time
	CreatedAt      time.Time `gorm:"not null;autoCreateTime"`
}
