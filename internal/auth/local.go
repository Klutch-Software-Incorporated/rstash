package auth

import (
	"context"

	"rstash/internal/db"
	"rstash/internal/model"
)

// LocalService implements Service using the local database via Repository.
type LocalService struct {
	repo *db.Repository
}

// NewLocalService returns a Service backed by the given repository.
func NewLocalService(repo *db.Repository) *LocalService {
	return &LocalService{repo: repo}
}

// --- Authentication ---

func (s *LocalService) Authenticate(ctx context.Context, username, password string) (*model.User, error) {
	user, err := s.repo.GetUserByUsername(ctx, username)
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

func (s *LocalService) CreateSession(ctx context.Context, userID int64, ip string) (*model.Session, error) {
	return s.repo.CreateSession(ctx, userID, ip)
}

func (s *LocalService) GetSession(ctx context.Context, token string) (*model.Session, error) {
	return s.repo.GetSessionByToken(ctx, token)
}

func (s *LocalService) DestroySession(ctx context.Context, token string) error {
	return s.repo.DeleteSession(ctx, token)
}

func (s *LocalService) InvalidateOtherSessions(ctx context.Context, userID int64, keepToken string) error {
	return s.repo.DeleteUserSessionsExcept(ctx, userID, keepToken)
}

func (s *LocalService) CleanupExpiredSessions(ctx context.Context) error {
	return s.repo.DeleteExpiredSessions(ctx)
}

// --- User CRUD ---

func (s *LocalService) CreateUser(ctx context.Context, username, password string, isAdmin, approved bool) (*model.User, error) {
	return s.repo.CreateUser(ctx, username, password, isAdmin, approved)
}

func (s *LocalService) GetUser(ctx context.Context, id int64) (*model.User, error) {
	return s.repo.GetUserByID(ctx, id)
}

func (s *LocalService) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	return s.repo.GetUserByUsername(ctx, username)
}

func (s *LocalService) ListUsers(ctx context.Context) ([]*model.User, error) {
	return s.repo.ListUsers(ctx)
}

func (s *LocalService) DeleteUser(ctx context.Context, id int64) error {
	// Delete OAuth tokens and refresh tokens first (no ON DELETE CASCADE), then sessions, then user.
	if err := s.repo.DeleteOAuthTokensByUserID(ctx, id); err != nil {
		return err
	}
	if err := s.repo.DeleteRefreshTokensByUserID(ctx, id); err != nil {
		return err
	}
	if err := s.repo.DeleteUserSessions(ctx, id); err != nil {
		return err
	}
	return s.repo.DeleteUser(ctx, id)
}

func (s *LocalService) UpdatePassword(ctx context.Context, userID int64, newPassword string) error {
	return s.repo.UpdateUserPassword(ctx, userID, newPassword)
}

func (s *LocalService) UserCount(ctx context.Context) (int64, error) {
	return s.repo.UserCount(ctx)
}

// --- Admin user management ---

func (s *LocalService) ToggleAdmin(ctx context.Context, userID int64, isAdmin bool) error {
	return s.repo.UpdateUserAdmin(ctx, userID, isAdmin)
}

func (s *LocalService) SetDisabled(ctx context.Context, userID int64, disabled bool) error {
	return s.repo.UpdateUserDisabled(ctx, userID, disabled)
}

func (s *LocalService) SetApproved(ctx context.Context, userID int64, approved bool) error {
	return s.repo.UpdateUserApproved(ctx, userID, approved)
}

func (s *LocalService) ListUserSessions(ctx context.Context, userID int64) ([]*model.Session, error) {
	return s.repo.ListUserSessions(ctx, userID)
}

func (s *LocalService) CountUserSessions(ctx context.Context, userID int64) (int64, error) {
	return s.repo.CountUserSessions(ctx, userID)
}

func (s *LocalService) TerminateSession(ctx context.Context, token string) error {
	return s.repo.DeleteSession(ctx, token)
}

func (s *LocalService) TerminateAllSessions(ctx context.Context, userID int64) error {
	return s.repo.DeleteUserSessions(ctx, userID)
}
