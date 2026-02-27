package db_test

import (
	"context"
	"testing"

	"gosilo/internal/db"
)

func TestCreateAndGetSession(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	user, err := db.CreateUser(ctx, database, "sessuser", "password123", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	sess, err := db.CreateSession(ctx, database, user.ID, "")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if sess.Token == "" {
		t.Fatal("session token is empty")
	}
	if sess.CSRFToken == "" {
		t.Fatal("csrf token is empty")
	}
	if sess.UserID != user.ID {
		t.Fatalf("expected user_id %d, got %d", user.ID, sess.UserID)
	}

	// Get it back.
	got, err := db.GetSessionByToken(ctx, database, sess.Token)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got == nil {
		t.Fatal("expected session, got nil")
	}
	if got.Token != sess.Token {
		t.Fatalf("expected token %s, got %s", sess.Token, got.Token)
	}
}

func TestGetSessionByToken_NotFound(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	got, err := db.GetSessionByToken(ctx, database, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil session")
	}
}

func TestDeleteSession(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	user, _ := db.CreateUser(ctx, database, "delsessuser", "password123", false, true)
	sess, _ := db.CreateSession(ctx, database, user.ID, "")

	if err := db.DeleteSession(ctx, database, sess.Token); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	got, err := db.GetSessionByToken(ctx, database, sess.Token)
	if err != nil {
		t.Fatalf("get session after delete: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestDeleteUserSessions(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	user, _ := db.CreateUser(ctx, database, "multisessuser", "password123", false, true)
	db.CreateSession(ctx, database, user.ID, "")
	db.CreateSession(ctx, database, user.ID, "")

	if err := db.DeleteUserSessions(ctx, database, user.ID); err != nil {
		t.Fatalf("delete user sessions: %v", err)
	}
}

func TestExpiredSessionReturnsNil(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	user, _ := db.CreateUser(ctx, database, "expuser", "password123", false, true)
	sess, _ := db.CreateSession(ctx, database, user.ID, "")

	// Manually expire the session.
	database.ExecContext(ctx, "UPDATE sessions SET expires_at = datetime('now', '-1 day') WHERE token = ?", sess.Token)

	got, err := db.GetSessionByToken(ctx, database, sess.Token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for expired session")
	}
}
