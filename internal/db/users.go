package db

import (
	"context"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"rstash/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var usernameRe = regexp.MustCompile(`^[a-z][a-z0-9._-]*$`)

// ValidateUsername normalizes and validates a username per RFC 7613 UsernameCaseMapped.
// It lowercases, trims whitespace, and checks format constraints.
// Returns the normalized username or an error.
func ValidateUsername(input string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(input))
	if name == "" {
		return "", fmt.Errorf("username is required")
	}
	if len(name) > 63 {
		return "", fmt.Errorf("username must be 63 characters or fewer")
	}
	if !usernameRe.MatchString(name) {
		return "", fmt.Errorf("username must start with a letter and contain only lowercase letters, digits, dots, hyphens, or underscores")
	}
	return name, nil
}

// ValidateEmail normalizes and validates an email address.
// It lowercases, trims whitespace, and validates format via net/mail.ParseAddress.
// Returns the normalized email or an error.
func ValidateEmail(input string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(input))
	if email == "" {
		return "", fmt.Errorf("email is required")
	}
	if len(email) > 254 {
		return "", fmt.Errorf("email must be 254 characters or fewer")
	}
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return "", fmt.Errorf("invalid email address")
	}
	return addr.Address, nil
}

// CreateUser inserts a new user with a bcrypt-hashed password.
// The username is normalized to lowercase as a safety net.
// If email is non-empty, it is normalized and stored on the user.
func (r *Repository) CreateUser(ctx context.Context, username, password, email string, isAdmin, approved bool) (*model.User, error) {
	username = strings.ToLower(strings.TrimSpace(username))
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
	if email != "" {
		normalized := strings.ToLower(strings.TrimSpace(email))
		u.Email = &normalized
	}
	if err := r.db.WithContext(ctx).Create(&u).Error; err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &u, nil
}

// GetUserByUsername returns the user with the given username, or nil if not found.
// The username is normalized to lowercase for case-insensitive lookup.
func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	username = strings.ToLower(strings.TrimSpace(username))
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

// UpdateUserEmail sets the email column for a user (normalized to lowercase).
func (r *Repository) UpdateUserEmail(ctx context.Context, userID int64, email string) error {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).Update("email", normalized).Error; err != nil {
		return fmt.Errorf("update user email: %w", err)
	}
	return nil
}

// GetUserByEmail returns the user with the given email, or nil if not found.
func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	var u model.User
	err := r.db.WithContext(ctx).Where("email = ?", normalized).First(&u).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return &u, nil
}

// SetEmailVerifyToken stores a verification token and expiry for a user.
func (r *Repository) SetEmailVerifyToken(ctx context.Context, userID int64, token string, expiry time.Time) error {
	if err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).Updates(map[string]any{
		"email_verify_token":  token,
		"email_verify_expiry": expiry,
		"email_verified":      false,
	}).Error; err != nil {
		return fmt.Errorf("set email verify token: %w", err)
	}
	return nil
}

// VerifyUserEmail sets email_verified=true and clears the verify token.
func (r *Repository) VerifyUserEmail(ctx context.Context, userID int64) error {
	if err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).Updates(map[string]any{
		"email_verified":      true,
		"email_verify_token":  nil,
		"email_verify_expiry": nil,
	}).Error; err != nil {
		return fmt.Errorf("verify user email: %w", err)
	}
	return nil
}

// GetUserByEmailVerifyToken returns the user with the given verify token, or nil if not found or expired.
func (r *Repository) GetUserByEmailVerifyToken(ctx context.Context, token string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("email_verify_token = ? AND email_verify_expiry > ?", token, time.Now().UTC()).First(&u).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by email verify token: %w", err)
	}
	return &u, nil
}

// SetPasswordResetToken stores a password reset token and expiry for a user.
func (r *Repository) SetPasswordResetToken(ctx context.Context, userID int64, token string, expiry time.Time) error {
	if err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).Updates(map[string]any{
		"password_reset_token":  token,
		"password_reset_expiry": expiry,
	}).Error; err != nil {
		return fmt.Errorf("set password reset token: %w", err)
	}
	return nil
}

// ClearPasswordResetToken removes the password reset token from a user.
func (r *Repository) ClearPasswordResetToken(ctx context.Context, userID int64) error {
	if err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).Updates(map[string]any{
		"password_reset_token":  nil,
		"password_reset_expiry": nil,
	}).Error; err != nil {
		return fmt.Errorf("clear password reset token: %w", err)
	}
	return nil
}

// GetUserByPasswordResetToken returns the user with the given reset token, or nil if not found or expired.
func (r *Repository) GetUserByPasswordResetToken(ctx context.Context, token string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("password_reset_token = ? AND password_reset_expiry > ?", token, time.Now().UTC()).First(&u).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by password reset token: %w", err)
	}
	return &u, nil
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
		Joins("LEFT JOIN nodes n ON n.user_id = u.id").
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

// GetTotalStorageUsed returns the total content_length across all users' document nodes.
func (r *Repository) GetTotalStorageUsed(ctx context.Context) (int64, error) {
	var total *int64
	err := r.db.WithContext(ctx).Model(&model.Node{}).Select("COALESCE(SUM(content_length), 0)").Scan(&total).Error
	if err != nil {
		return 0, fmt.Errorf("get total storage used: %w", err)
	}
	if total == nil {
		return 0, nil
	}
	return *total, nil
}
