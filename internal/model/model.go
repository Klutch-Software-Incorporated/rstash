package model

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	IsAdmin      bool
	CreatedAt    string
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
