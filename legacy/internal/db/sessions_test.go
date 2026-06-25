package db_test

import (
	"context"
	"testing"
)

func TestCreateAndGetSession(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	user, err := database.CreateUser(ctx, "sessuser", "password123", "", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	sess, err := database.CreateSession(ctx, user.ID, "")
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
	got, err := database.GetSessionByToken(ctx, sess.Token)
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

	got, err := database.GetSessionByToken(ctx, "nonexistent")
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

	user, _ := database.CreateUser(ctx, "delsessuser", "password123", "", false, true)
	sess, _ := database.CreateSession(ctx, user.ID, "")

	if err := database.DeleteSession(ctx, sess.Token); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	got, err := database.GetSessionByToken(ctx, sess.Token)
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

	user, _ := database.CreateUser(ctx, "multisessuser", "password123", "", false, true)
	database.CreateSession(ctx, user.ID, "")
	database.CreateSession(ctx, user.ID, "")

	if err := database.DeleteUserSessions(ctx, user.ID); err != nil {
		t.Fatalf("delete user sessions: %v", err)
	}
}

func TestExpiredSessionReturnsNil(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	user, _ := database.CreateUser(ctx, "expuser", "password123", "", false, true)
	sess, _ := database.CreateSession(ctx, user.ID, "")

	// Manually expire the session.
	database.GormDB().Exec("UPDATE sessions SET expires_at = datetime('now', '-1 day') WHERE token = ?", sess.Token)

	got, err := database.GetSessionByToken(ctx, sess.Token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for expired session")
	}
}
