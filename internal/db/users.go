package db

import (
	"context"
	"fmt"
	"time"

	"gosilo/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// CreateUser inserts a new user with a bcrypt-hashed password.
func (r *Repository) CreateUser(ctx context.Context, username, password string, isAdmin, approved bool) (*model.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u := model.User{
		Username:     username,
		PasswordHash: string(hash),
		IsAdmin:      isAdmin,
		Approved:     approved,
	}
	if err := r.db.WithContext(ctx).Create(&u).Error; err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &u, nil
}

// GetUserByUsername returns the user with the given username, or nil if not found.
func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&u).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	return &u, nil
}

// GetUserByID returns the user with the given ID, or nil if not found.
func (r *Repository) GetUserByID(ctx context.Context, id int64) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).First(&u, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}

// CheckPassword reports whether the given plaintext password matches the user's hash.
func CheckPassword(user *model.User, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) == nil
}

// ListUsers returns all users ordered by id, excluding the _system sentinel.
func (r *Repository) ListUsers(ctx context.Context) ([]*model.User, error) {
	var users []*model.User
	err := r.db.WithContext(ctx).Where("id >= 1").Order("id").Find(&users).Error
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return users, nil
}

// DeleteUser deletes a user by id.
func (r *Repository) DeleteUser(ctx context.Context, id int64) error {
	if err := r.db.WithContext(ctx).Delete(&model.User{}, id).Error; err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

// UpdateUserPassword hashes a new password with bcrypt and updates the user's row.
func (r *Repository) UpdateUserPassword(ctx context.Context, userID int64, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).Update("password_hash", string(hash)).Error; err != nil {
		return fmt.Errorf("update user password: %w", err)
	}
	return nil
}

// UserCount returns the total number of users, excluding the _system sentinel.
func (r *Repository) UserCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.User{}).Where("id >= 1").Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

// UpdateUserQuota sets the per-user storage quota override.
func (r *Repository) UpdateUserQuota(ctx context.Context, userID int64, quotaBytes int64) error {
	if err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).Update("storage_quota", quotaBytes).Error; err != nil {
		return fmt.Errorf("update user quota: %w", err)
	}
	return nil
}

// UpdateUserAdmin toggles the is_admin flag for a user.
func (r *Repository) UpdateUserAdmin(ctx context.Context, userID int64, isAdmin bool) error {
	if err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).Update("is_admin", isAdmin).Error; err != nil {
		return fmt.Errorf("update user admin: %w", err)
	}
	return nil
}

// UpdateUserApproved sets the approved flag for a user.
func (r *Repository) UpdateUserApproved(ctx context.Context, userID int64, approved bool) error {
	if err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).Update("approved", approved).Error; err != nil {
		return fmt.Errorf("update user approved: %w", err)
	}
	return nil
}

// CountPendingUsers returns the number of unapproved users.
func (r *Repository) CountPendingUsers(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.User{}).Where("id >= 1 AND approved = ?", false).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count pending users: %w", err)
	}
	return count, nil
}

// ApproveAllPending approves all pending users and returns the count affected.
func (r *Repository) ApproveAllPending(ctx context.Context) (int64, error) {
	result := r.db.WithContext(ctx).Model(&model.User{}).Where("approved = ? AND id >= 1", false).Update("approved", true)
	if result.Error != nil {
		return 0, fmt.Errorf("approve all pending: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// UpdateUserDisabled toggles the disabled flag for a user.
func (r *Repository) UpdateUserDisabled(ctx context.Context, userID int64, disabled bool) error {
	if err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).Update("disabled", disabled).Error; err != nil {
		return fmt.Errorf("update user disabled: %w", err)
	}
	return nil
}

// AcceptTOS records that the user accepted the Terms of Service.
func (r *Repository) AcceptTOS(ctx context.Context, userID int64) error {
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).Update("tos_accepted_at", now).Error; err != nil {
		return fmt.Errorf("accept tos: %w", err)
	}
	return nil
}

// AcceptPrivacy records that the user accepted the Privacy Policy.
func (r *Repository) AcceptPrivacy(ctx context.Context, userID int64) error {
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).Update("privacy_accepted_at", now).Error; err != nil {
		return fmt.Errorf("accept privacy: %w", err)
	}
	return nil
}

// UpdateUserLastLogin sets the last_login_at and last_login_ip columns for a user.
func (r *Repository) UpdateUserLastLogin(ctx context.Context, userID int64, ip string) error {
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).Updates(map[string]any{
		"last_login_at": now,
		"last_login_ip": ip,
	}).Error; err != nil {
		return fmt.Errorf("update user last login: %w", err)
	}
	return nil
}

// TopUserRow holds a user's storage usage for ranking.
type TopUserRow struct {
	Username    string
	StorageUsed int64
}

// TopUsersByStorage returns the top N users by total storage used.
func (r *Repository) TopUsersByStorage(ctx context.Context, limit int) ([]*TopUserRow, error) {
	var results []*TopUserRow
	err := r.db.WithContext(ctx).
		Table("users u").
		Select("u.username, COALESCE(SUM(n.content_length), 0) AS storage_used").
		Joins("LEFT JOIN nodes n ON n.user_id = u.id AND n.is_folder = ?", false).
		Where("u.id >= 1").
		Group("u.id").
		Order("storage_used DESC").
		Limit(limit).
		Scan(&results).Error
	if err != nil {
		return nil, fmt.Errorf("top users by storage: %w", err)
	}
	return results, nil
}

// GetTotalStorageUsed returns the total content_length across all users' non-folder nodes.
func (r *Repository) GetTotalStorageUsed(ctx context.Context) (int64, error) {
	var total *int64
	err := r.db.WithContext(ctx).Model(&model.Node{}).Where("is_folder = ?", false).Select("COALESCE(SUM(content_length), 0)").Scan(&total).Error
	if err != nil {
		return 0, fmt.Errorf("get total storage used: %w", err)
	}
	if total == nil {
		return 0, nil
	}
	return *total, nil
}
