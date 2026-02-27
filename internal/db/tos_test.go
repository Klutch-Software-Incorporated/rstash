package db_test

import (
	"context"
	"testing"

	"gosilo/internal/db"
)

func TestAcceptTOS(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	user, err := db.CreateUser(ctx, database, "alice", "password123", false)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Initially nil.
	if user.TOSAcceptedAt != nil {
		t.Fatal("expected TOSAcceptedAt to be nil initially")
	}

	// Accept TOS.
	if err := db.AcceptTOS(ctx, database, user.ID); err != nil {
		t.Fatalf("accept TOS: %v", err)
	}

	// Verify.
	updated, err := db.GetUserByID(ctx, database, user.ID)
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

	user, err := db.CreateUser(ctx, database, "bob", "password123", false)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Initially nil.
	if user.PrivacyAcceptedAt != nil {
		t.Fatal("expected PrivacyAcceptedAt to be nil initially")
	}

	// Accept Privacy.
	if err := db.AcceptPrivacy(ctx, database, user.ID); err != nil {
		t.Fatalf("accept Privacy: %v", err)
	}

	// Verify.
	updated, err := db.GetUserByID(ctx, database, user.ID)
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

	user, err := db.CreateUser(ctx, database, "carol", "password123", false)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_ = db.AcceptTOS(ctx, database, user.ID)
	_ = db.AcceptPrivacy(ctx, database, user.ID)

	updated, err := db.GetUserByID(ctx, database, user.ID)
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
