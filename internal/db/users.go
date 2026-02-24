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
		 RETURNING id, username, password_hash, is_admin, created_at`,
		username, string(hash), isAdmin,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &u, nil
}

// GetUserByUsername returns the user with the given username, or nil if not found.
func GetUserByUsername(ctx context.Context, q Querier, username string) (*model.User, error) {
	var u model.User
	err := q.QueryRowContext(ctx,
		"SELECT id, username, password_hash, is_admin, created_at FROM users WHERE username = ?",
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt)
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
		"SELECT id, username, password_hash, is_admin, created_at FROM users WHERE id = ?",
		id,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt)
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

// ListUsers returns all users ordered by id.
func ListUsers(ctx context.Context, q Querier) ([]*model.User, error) {
	rows, err := q.QueryContext(ctx,
		"SELECT id, username, password_hash, is_admin, created_at FROM users ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt); err != nil {
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

// UserCount returns the total number of users.
func UserCount(ctx context.Context, q Querier) (int64, error) {
	var count int64
	err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}
