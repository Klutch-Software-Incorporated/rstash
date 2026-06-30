package db_test

import (
	"context"
	"testing"
)

func TestAcceptTOS(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	user, err := database.CreateUser(ctx, "alice", "password123", "", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Initially nil.
	if user.TOSAcceptedAt != nil {
		t.Fatal("expected TOSAcceptedAt to be nil initially")
	}

	// Accept TOS.
	if err := database.AcceptTOS(ctx, user.ID); err != nil {
		t.Fatalf("accept TOS: %v", err)
	}

	// Verify.
	updated, err := database.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if updated.TOSAcceptedAt == nil {
		t.Fatal("expected TOSAcceptedAt to be set after acceptance")
	}
}

func TestAcceptPrivacy(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	user, err := database.CreateUser(ctx, "bob", "password123", "", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Initially nil.
	if user.PrivacyAcceptedAt != nil {
		t.Fatal("expected PrivacyAcceptedAt to be nil initially")
	}

	// Accept Privacy.
	if err := database.AcceptPrivacy(ctx, user.ID); err != nil {
		t.Fatalf("accept Privacy: %v", err)
	}

	// Verify.
	updated, err := database.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if updated.PrivacyAcceptedAt == nil {
		t.Fatal("expected PrivacyAcceptedAt to be set after acceptance")
	}
}

func TestAcceptTOSAndPrivacy_BothSet(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	user, err := database.CreateUser(ctx, "carol", "password123", "", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_ = database.AcceptTOS(ctx, user.ID)
	_ = database.AcceptPrivacy(ctx, user.ID)

	updated, err := database.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if updated.TOSAcceptedAt == nil {
		t.Fatal("expected TOSAcceptedAt to be set")
	}
	if updated.PrivacyAcceptedAt == nil {
		t.Fatal("expected PrivacyAcceptedAt to be set")
	}
}
