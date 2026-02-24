package auth

import (
	"context"
	"errors"

	"gosilo/internal/model"
)

// Sentinel errors returned by Service implementations.
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")
)

// Service defines the authentication and user management operations
// used by UI handlers. Abstracting behind this interface allows the
// local (database-backed) implementation to be swapped for an external
// provider (e.g. OIDC) later.
type Service interface {
	// Authentication
	Authenticate(ctx context.Context, username, password string) (*model.User, error)
	CheckPassword(user *model.User, password string) bool

	// Sessions
	CreateSession(ctx context.Context, userID int64) (*model.Session, error)
	GetSession(ctx context.Context, token string) (*model.Session, error)
	DestroySession(ctx context.Context, token string) error
	InvalidateOtherSessions(ctx context.Context, userID int64, keepToken string) error
	CleanupExpiredSessions(ctx context.Context) error

	// User CRUD
	CreateUser(ctx context.Context, username, password string, isAdmin bool) (*model.User, error)
	GetUser(ctx context.Context, id int64) (*model.User, error)
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
	ListUsers(ctx context.Context) ([]*model.User, error)
	DeleteUser(ctx context.Context, id int64) error
	UpdatePassword(ctx context.Context, userID int64, newPassword string) error
	UserCount(ctx context.Context) (int64, error)

	// Invites
	CreateInvite(ctx context.Context, createdBy int64) (*model.InviteCode, error)
	GetInvite(ctx context.Context, code string) (*model.InviteCode, error)
	RedeemInvite(ctx context.Context, code string, usedBy int64) error
	ListInvites(ctx context.Context) ([]*model.InviteCode, error)
	DeleteInvite(ctx context.Context, code string) error
}
