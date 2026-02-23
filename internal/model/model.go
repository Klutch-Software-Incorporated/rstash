package model

import "time"

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	IsAdmin      bool
	CreatedAt    time.Time
}

type OAuthClient struct {
	ID          string
	RedirectURI string
	CreatedAt   time.Time
}

type OAuthToken struct {
	Token     string
	UserID    int64
	ClientID  string
	Scopes    []string
	CreatedAt time.Time
	ExpiresAt *time.Time
}

type Node struct {
	ID            int64
	UserID        int64
	Path          string
	IsFolder      bool
	ContentType   string
	ContentLength int64
	ETag          string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
