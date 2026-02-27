package auth

import (
	"context"
	"database/sql"

	"gosilo/internal/db"
	"gosilo/internal/model"
)

// LocalService implements Service using the local SQLite database.
type LocalService struct {
	db *sql.DB
}

// NewLocalService returns a Service backed by the given database.
func NewLocalService(database *sql.DB) *LocalService {
	return &LocalService{db: database}
}

// --- Authentication ---

func (s *LocalService) Authenticate(ctx context.Context, username, password string) (*model.User, error) {
	user, err := db.GetUserByUsername(ctx, s.db, username)
	if err != nil {
		return nil, err
	}
	if user == nil || !db.CheckPassword(user, password) {
		return nil, ErrInvalidCredentials
	}
	if user.Disabled {
		return nil, ErrAccountDisabled
	}
	if !user.Approved {
		return nil, ErrAccountPendingApproval
	}
	return user, nil
}

func (s *LocalService) CheckPassword(user *model.User, password string) bool {
	return db.CheckPassword(user, password)
}

// --- Sessions ---

func (s *LocalService) CreateSession(ctx context.Context, userID int64) (*model.Session, error) {
	return db.CreateSession(ctx, s.db, userID)
}

func (s *LocalService) GetSession(ctx context.Context, token string) (*model.Session, error) {
	return db.GetSessionByToken(ctx, s.db, token)
}

func (s *LocalService) DestroySession(ctx context.Context, token string) error {
	return db.DeleteSession(ctx, s.db, token)
}

func (s *LocalService) InvalidateOtherSessions(ctx context.Context, userID int64, keepToken string) error {
	return db.DeleteUserSessionsExcept(ctx, s.db, userID, keepToken)
}

func (s *LocalService) CleanupExpiredSessions(ctx context.Context) error {
	return db.DeleteExpiredSessions(ctx, s.db)
}

// --- User CRUD ---

func (s *LocalService) CreateUser(ctx context.Context, username, password string, isAdmin, approved bool) (*model.User, error) {
	return db.CreateUser(ctx, s.db, username, password, isAdmin, approved)
}

func (s *LocalService) GetUser(ctx context.Context, id int64) (*model.User, error) {
	return db.GetUserByID(ctx, s.db, id)
}

func (s *LocalService) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	return db.GetUserByUsername(ctx, s.db, username)
}

func (s *LocalService) ListUsers(ctx context.Context) ([]*model.User, error) {
	return db.ListUsers(ctx, s.db)
}

func (s *LocalService) DeleteUser(ctx context.Context, id int64) error {
	// Delete OAuth tokens and refresh tokens first (no ON DELETE CASCADE), then sessions, then user.
	if err := db.DeleteOAuthTokensByUserID(ctx, s.db, id); err != nil {
		return err
	}
	if err := db.DeleteRefreshTokensByUserID(ctx, s.db, id); err != nil {
		return err
	}
	if err := db.DeleteUserSessions(ctx, s.db, id); err != nil {
		return err
	}
	return db.DeleteUser(ctx, s.db, id)
}

func (s *LocalService) UpdatePassword(ctx context.Context, userID int64, newPassword string) error {
	return db.UpdateUserPassword(ctx, s.db, userID, newPassword)
}

func (s *LocalService) UserCount(ctx context.Context) (int64, error) {
	return db.UserCount(ctx, s.db)
}

// --- Admin user management ---

func (s *LocalService) ToggleAdmin(ctx context.Context, userID int64, isAdmin bool) error {
	return db.UpdateUserAdmin(ctx, s.db, userID, isAdmin)
}

func (s *LocalService) SetDisabled(ctx context.Context, userID int64, disabled bool) error {
	return db.UpdateUserDisabled(ctx, s.db, userID, disabled)
}

func (s *LocalService) SetApproved(ctx context.Context, userID int64, approved bool) error {
	return db.UpdateUserApproved(ctx, s.db, userID, approved)
}

func (s *LocalService) ListUserSessions(ctx context.Context, userID int64) ([]*model.Session, error) {
	return db.ListUserSessions(ctx, s.db, userID)
}

func (s *LocalService) CountUserSessions(ctx context.Context, userID int64) (int64, error) {
	return db.CountUserSessions(ctx, s.db, userID)
}

func (s *LocalService) TerminateSession(ctx context.Context, token string) error {
	return db.DeleteSession(ctx, s.db, token)
}

func (s *LocalService) TerminateAllSessions(ctx context.Context, userID int64) error {
	return db.DeleteUserSessions(ctx, s.db, userID)
}

