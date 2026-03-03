package db_test

import (
	"context"
	"testing"
	"time"
)

func TestCreateAndGetRefreshToken(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	user, err := database.CreateUser(ctx, "refreshuser", "password", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, err = database.UpsertOAuthClient(ctx, "https://app.example.com", "https://app.example.com/callback")
	if err != nil {
		t.Fatalf("upsert client: %v", err)
	}

	// Create an access token first.
	accessToken, err := database.CreateOAuthToken(ctx, user.ID, "https://app.example.com", []string{"*:rw"}, 24*time.Hour)
	if err != nil {
		t.Fatalf("create access token: %v", err)
	}

	// Create refresh token.
	rt, err := database.CreateRefreshToken(ctx, user.ID, "https://app.example.com", []string{"*:rw"}, accessToken.Token, 90*24*time.Hour)
	if err != nil {
		t.Fatalf("create refresh token: %v", err)
	}

	if rt.Token == "" {
		t.Fatal("expected non-empty token")
	}
	if rt.UserID != user.ID {
		t.Fatalf("expected user_id %d, got %d", user.ID, rt.UserID)
	}
	if rt.AccessToken != accessToken.Token {
		t.Fatal("expected access_token to match")
	}

	// Get it back.
	got, err := database.GetRefreshToken(ctx, rt.Token)
	if err != nil {
		t.Fatalf("get refresh token: %v", err)
	}
	if got == nil {
		t.Fatal("expected to find refresh token")
	}
	if got.Token != rt.Token {
		t.Fatal("token mismatch")
	}
}

func TestDeleteRefreshTokenByAccessToken(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	user, err := database.CreateUser(ctx, "refreshuser2", "password", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, err = database.UpsertOAuthClient(ctx, "https://app.example.com", "https://app.example.com/callback")
	if err != nil {
		t.Fatalf("upsert client: %v", err)
	}

	accessToken, err := database.CreateOAuthToken(ctx, user.ID, "https://app.example.com", []string{"*:rw"}, 24*time.Hour)
	if err != nil {
		t.Fatalf("create access token: %v", err)
	}

	rt, err := database.CreateRefreshToken(ctx, user.ID, "https://app.example.com", []string{"*:rw"}, accessToken.Token, 90*24*time.Hour)
	if err != nil {
		t.Fatalf("create refresh token: %v", err)
	}

	// Delete by access token.
	if err := database.DeleteRefreshTokenByAccessToken(ctx, accessToken.Token); err != nil {
		t.Fatalf("delete by access token: %v", err)
	}

	// Should be gone.
	got, err := database.GetRefreshToken(ctx, rt.Token)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Fatal("expected refresh token to be deleted")
	}
}

func TestDeleteExpiredRefreshTokens(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	user, err := database.CreateUser(ctx, "refreshuser3", "password", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, err = database.UpsertOAuthClient(ctx, "https://app.example.com", "https://app.example.com/callback")
	if err != nil {
		t.Fatalf("upsert client: %v", err)
	}

	accessToken, err := database.CreateOAuthToken(ctx, user.ID, "https://app.example.com", []string{"*:rw"}, 24*time.Hour)
	if err != nil {
		t.Fatalf("create access token: %v", err)
	}

	// Create a non-expiring refresh token.
	rt, err := database.CreateRefreshToken(ctx, user.ID, "https://app.example.com", []string{"*:rw"}, accessToken.Token, 0)
	if err != nil {
		t.Fatalf("create refresh token: %v", err)
	}

	// Cleanup should not delete it.
	if err := database.DeleteExpiredRefreshTokens(ctx); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	got, _ := database.GetRefreshToken(ctx, rt.Token)
	if got == nil {
		t.Fatal("expected non-expiring refresh token to survive cleanup")
	}
}
