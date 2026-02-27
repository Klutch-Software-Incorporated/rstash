package model

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	IsAdmin      bool
	StorageQuota int64 // 0 = use server default
	Disabled     bool
	Approved     bool
	CreatedAt    string
	LastLoginAt        *string // NULL until first login
	LastLoginIP        *string // NULL until first login
	TOSAcceptedAt      *string // NULL until TOS accepted
	PrivacyAcceptedAt  *string // NULL until Privacy Policy accepted
}

type OAuthClient struct {
	ID          string
	RedirectURI string
	CreatedAt   string
}

type OAuthToken struct {
	Token     string
	UserID    int64
	ClientID  string
	Scopes    []string
	CreatedAt string
	ExpiresAt *string
}

type Node struct {
	ID            int64
	UserID        int64
	Path          string
	IsFolder      bool
	ContentType   string
	ContentLength int64
	ETag          string
	CreatedAt     string
	UpdatedAt     string
}

type Session struct {
	Token     string
	UserID    int64
	CSRFToken string
	CreatedAt string
	ExpiresAt string
}

type AuditEntry struct {
	ID         int64
	ActorID    int64
	Action     string
	TargetType string
	TargetID   string
	Details    string
	CreatedAt  string
}

type RefreshToken struct {
	Token       string
	UserID      int64
	ClientID    string
	Scopes      []string
	AccessToken string
	CreatedAt   string
	ExpiresAt   *string
}

type AbuseReport struct {
	ID            int64
	ReporterEmail string
	ReportedPath  string
	Reason        string
	Description   *string
	Status        string // "open", "reviewed", "dismissed", "actioned"
	ReporterIP    *string
	ReviewerID    *int64
	ReviewNote    *string
	CreatedAt     string
	ReviewedAt    *string
}

type AuthorizationCode struct {
	Code                string
	UserID              int64
	ClientID            string
	RedirectURI         string
	Scopes              string
	CodeChallenge       string
	CodeChallengeMethod string
	CreatedAt           string
	ExpiresAt           string
	Used                bool
}
