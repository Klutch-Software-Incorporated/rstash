package db_test

import (
	"context"
	"testing"

	"gosilo/internal/db"
)

func TestUpdateUserLastLogin(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	user, err := db.CreateUser(ctx, database, "loginuser", "password", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Initially nil.
	if user.LastLoginAt != nil {
		t.Fatal("expected LastLoginAt to be nil initially")
	}
	if user.LastLoginIP != nil {
		t.Fatal("expected LastLoginIP to be nil initially")
	}

	// Update last login.
	if err := db.UpdateUserLastLogin(ctx, database, user.ID, "192.168.1.1"); err != nil {
		t.Fatalf("update last login: %v", err)
	}

	// Verify via GetUserByID.
	updated, err := db.GetUserByID(ctx, database, user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if updated.LastLoginAt == nil {
		t.Fatal("expected LastLoginAt to be set after update")
	}
	if updated.LastLoginIP == nil || *updated.LastLoginIP != "192.168.1.1" {
		t.Fatalf("expected LastLoginIP to be '192.168.1.1', got %v", updated.LastLoginIP)
	}
}

func TestUserLastLoginNilByDefault(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	user, err := db.CreateUser(ctx, database, "nillogin", "password", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Fetch again via GetUserByUsername.
	fetched, err := db.GetUserByUsername(ctx, database, "nillogin")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}

	if fetched.LastLoginAt != nil {
		t.Fatalf("expected nil LastLoginAt, got %v", *fetched.LastLoginAt)
	}
	if fetched.LastLoginIP != nil {
		t.Fatalf("expected nil LastLoginIP, got %v", *fetched.LastLoginIP)
	}

	// Suppress unused variable warning.
	_ = user
}
