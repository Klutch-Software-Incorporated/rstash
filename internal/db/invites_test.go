package db_test

import (
	"context"
	"testing"

	"gosilo/internal/db"
)

func TestCreateAndGetInviteCode(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	admin, err := db.CreateUser(ctx, database, "invadmin", "password123", true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	inv, err := db.CreateInviteCode(ctx, database, admin.ID)
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	if inv.Code == "" {
		t.Fatal("invite code is empty")
	}
	if inv.UsedBy != nil {
		t.Fatal("new invite should not be used")
	}

	got, err := db.GetInviteCode(ctx, database, inv.Code)
	if err != nil {
		t.Fatalf("get invite: %v", err)
	}
	if got == nil {
		t.Fatal("expected invite, got nil")
	}
	if got.Code != inv.Code {
		t.Fatalf("expected code %s, got %s", inv.Code, got.Code)
	}
}

func TestRedeemInviteCode(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	admin, _ := db.CreateUser(ctx, database, "redeemadmin", "password123", true)
	inv, _ := db.CreateInviteCode(ctx, database, admin.ID)

	user, _ := db.CreateUser(ctx, database, "redeemuser", "password123", false)

	if err := db.RedeemInviteCode(ctx, database, inv.Code, user.ID); err != nil {
		t.Fatalf("redeem invite: %v", err)
	}

	// Verify it's now used.
	got, _ := db.GetInviteCode(ctx, database, inv.Code)
	if got.UsedBy == nil {
		t.Fatal("invite should be used")
	}
	if *got.UsedBy != user.ID {
		t.Fatalf("expected used_by %d, got %d", user.ID, *got.UsedBy)
	}
}

func TestRedeemInviteCode_AlreadyUsed(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	admin, _ := db.CreateUser(ctx, database, "dblradmin", "password123", true)
	inv, _ := db.CreateInviteCode(ctx, database, admin.ID)

	user1, _ := db.CreateUser(ctx, database, "dblruser1", "password123", false)
	user2, _ := db.CreateUser(ctx, database, "dblruser2", "password123", false)

	if err := db.RedeemInviteCode(ctx, database, inv.Code, user1.ID); err != nil {
		t.Fatalf("first redeem: %v", err)
	}

	err := db.RedeemInviteCode(ctx, database, inv.Code, user2.ID)
	if err == nil {
		t.Fatal("expected error on double redeem")
	}
}

func TestListInviteCodes(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	admin, _ := db.CreateUser(ctx, database, "listadmin", "password123", true)
	db.CreateInviteCode(ctx, database, admin.ID)
	db.CreateInviteCode(ctx, database, admin.ID)

	invites, err := db.ListInviteCodes(ctx, database)
	if err != nil {
		t.Fatalf("list invites: %v", err)
	}
	if len(invites) != 2 {
		t.Fatalf("expected 2 invites, got %d", len(invites))
	}
}

func TestDeleteInviteCode(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	admin, _ := db.CreateUser(ctx, database, "deladmin", "password123", true)
	inv, _ := db.CreateInviteCode(ctx, database, admin.ID)

	if err := db.DeleteInviteCode(ctx, database, inv.Code); err != nil {
		t.Fatalf("delete invite: %v", err)
	}

	got, _ := db.GetInviteCode(ctx, database, inv.Code)
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}
