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
