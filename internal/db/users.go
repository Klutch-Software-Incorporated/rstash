package db

import (
	"context"
	"database/sql"
	"fmt"

	"gosilo/internal/model"

	"golang.org/x/crypto/bcrypt"
)

// CreateUser inserts a new user with a bcrypt-hashed password.
func CreateUser(ctx context.Context, q Querier, username, password string, isAdmin bool) (*model.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	var u model.User
	err = q.QueryRowContext(ctx,
		`INSERT INTO users (username, password_hash, is_admin) VALUES (?, ?, ?)
		 RETURNING id, username, password_hash, is_admin, storage_quota, disabled, created_at, last_login_at, last_login_ip`,
		username, string(hash), isAdmin,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.StorageQuota, &u.Disabled, &u.CreatedAt, &u.LastLoginAt, &u.LastLoginIP)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &u, nil
}

// GetUserByUsername returns the user with the given username, or nil if not found.
func GetUserByUsername(ctx context.Context, q Querier, username string) (*model.User, error) {
	var u model.User
	err := q.QueryRowContext(ctx,
		"SELECT id, username, password_hash, is_admin, storage_quota, disabled, created_at, last_login_at, last_login_ip FROM users WHERE username = ?",
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.StorageQuota, &u.Disabled, &u.CreatedAt, &u.LastLoginAt, &u.LastLoginIP)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	return &u, nil
}

// GetUserByID returns the user with the given ID, or nil if not found.
func GetUserByID(ctx context.Context, q Querier, id int64) (*model.User, error) {
	var u model.User
	err := q.QueryRowContext(ctx,
		"SELECT id, username, password_hash, is_admin, storage_quota, disabled, created_at, last_login_at, last_login_ip FROM users WHERE id = ?",
		id,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.StorageQuota, &u.Disabled, &u.CreatedAt, &u.LastLoginAt, &u.LastLoginIP)
	if err == sql.ErrNoRows {
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
func ListUsers(ctx context.Context, q Querier) ([]*model.User, error) {
	rows, err := q.QueryContext(ctx,
		"SELECT id, username, password_hash, is_admin, storage_quota, disabled, created_at, last_login_at, last_login_ip FROM users WHERE id > 0 ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.StorageQuota, &u.Disabled, &u.CreatedAt, &u.LastLoginAt, &u.LastLoginIP); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, &u)
	}
	return users, rows.Err()
}

// DeleteUser deletes a user by id.
func DeleteUser(ctx context.Context, q Querier, id int64) error {
	_, err := q.ExecContext(ctx, "DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

// UpdateUserPassword hashes a new password with bcrypt and updates the user's row.
func UpdateUserPassword(ctx context.Context, q Querier, userID int64, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	_, err = q.ExecContext(ctx, "UPDATE users SET password_hash = ? WHERE id = ?", string(hash), userID)
	if err != nil {
		return fmt.Errorf("update user password: %w", err)
	}
	return nil
}

// UserCount returns the total number of users, excluding the _system sentinel.
func UserCount(ctx context.Context, q Querier) (int64, error) {
	var count int64
	err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE id > 0").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

// UpdateUserQuota sets the per-user storage quota override.
// A value of 0 means "use server default".
func UpdateUserQuota(ctx context.Context, q Querier, userID int64, quotaBytes int64) error {
	_, err := q.ExecContext(ctx, "UPDATE users SET storage_quota = ? WHERE id = ?", quotaBytes, userID)
	if err != nil {
		return fmt.Errorf("update user quota: %w", err)
	}
	return nil
}

// UpdateUserAdmin toggles the is_admin flag for a user.
func UpdateUserAdmin(ctx context.Context, q Querier, userID int64, isAdmin bool) error {
	_, err := q.ExecContext(ctx, "UPDATE users SET is_admin = ? WHERE id = ?", isAdmin, userID)
	if err != nil {
		return fmt.Errorf("update user admin: %w", err)
	}
	return nil
}

// UpdateUserDisabled toggles the disabled flag for a user.
func UpdateUserDisabled(ctx context.Context, q Querier, userID int64, disabled bool) error {
	_, err := q.ExecContext(ctx, "UPDATE users SET disabled = ? WHERE id = ?", disabled, userID)
	if err != nil {
		return fmt.Errorf("update user disabled: %w", err)
	}
	return nil
}

// UpdateUserLastLogin sets the last_login_at and last_login_ip columns for a user.
func UpdateUserLastLogin(ctx context.Context, q Querier, userID int64, ip string) error {
	_, err := q.ExecContext(ctx,
		"UPDATE users SET last_login_at = datetime('now'), last_login_ip = ? WHERE id = ?",
		ip, userID,
	)
	if err != nil {
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
func TopUsersByStorage(ctx context.Context, q Querier, limit int) ([]*TopUserRow, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT u.username, COALESCE(SUM(n.content_length), 0) AS used
		 FROM users u
		 LEFT JOIN nodes n ON n.user_id = u.id AND n.is_folder = 0
		 WHERE u.id > 0
		 GROUP BY u.id
		 ORDER BY used DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("top users by storage: %w", err)
	}
	defer rows.Close()

	var result []*TopUserRow
	for rows.Next() {
		var r TopUserRow
		if err := rows.Scan(&r.Username, &r.StorageUsed); err != nil {
			return nil, fmt.Errorf("scan top user: %w", err)
		}
		result = append(result, &r)
	}
	return result, rows.Err()
}

// GetTotalStorageUsed returns the total content_length across all users' non-folder nodes.
func GetTotalStorageUsed(ctx context.Context, q Querier) (int64, error) {
	var total int64
	err := q.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(content_length), 0) FROM nodes WHERE is_folder = 0",
	).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("get total storage used: %w", err)
	}
	return total, nil
}
