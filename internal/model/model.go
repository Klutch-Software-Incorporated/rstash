package model

import "time"

type User struct {
	ID           int64      `gorm:"primaryKey;autoIncrement"`
	Username     string     `gorm:"size:255;uniqueIndex;not null"`
	PasswordHash string     `gorm:"size:255;not null"`
	IsAdmin      bool       `gorm:"not null;default:false"`
	StorageQuota int64      `gorm:"not null;default:0"` // 0 = use server default
	Disabled     bool       `gorm:"not null;default:false"`
	Approved     bool       `gorm:"not null;default:true"`
	CreatedAt    time.Time  `gorm:"not null;autoCreateTime"`
	LastLoginAt  *time.Time // NULL until first login
	LastLoginIP  *string    `gorm:"size:45"` // NULL until first login
	TOSAcceptedAt     *time.Time // NULL until TOS accepted
	PrivacyAcceptedAt *time.Time // NULL until Privacy Policy accepted
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
	IsFolder      bool      `gorm:"not null;default:false"`
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
