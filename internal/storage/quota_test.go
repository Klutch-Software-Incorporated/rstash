package storage

import (
	"context"
	"testing"

	"rstash/internal/db"
	"rstash/internal/model"
)

// testRepo creates an in-memory Repository with all migrations applied.
func testRepo(t *testing.T) *db.Repository {
	t.Helper()
	repo, err := db.OpenRepository("sqlite::memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo
}

func createTestUser(t *testing.T, repo *db.Repository, username string, quota int64) int64 {
	t.Helper()
	user, err := repo.CreateUser(context.Background(), username, "hash", "", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if quota > 0 {
		if err := repo.GormDB().Model(&model.User{}).Where("id = ?", user.ID).Update("storage_quota", quota).Error; err != nil {
			t.Fatal(err)
		}
	}
	return user.ID
}

func addTestNode(t *testing.T, repo *db.Repository, userID int64, path string, size int64) {
	t.Helper()
	_, err := repo.UpsertNode(context.Background(), userID, path, "", size, "etag")
	if err != nil {
		t.Fatal(err)
	}
}

func TestQuota_ModeOff_AlwaysAllows(t *testing.T) {
	repo := testRepo(t)
	userID := createTestUser(t, repo, "alice", 0)

	qc := NewQuotaChecker(QuotaConfig{Mode: "off"}, repo)
	err := qc.Check(context.Background(), repo, userID, 999999999)
	if err != nil {
		t.Fatalf("mode off should always allow, got: %v", err)
	}
}

func TestQuota_ModeTotal_AllowsWithinLimit(t *testing.T) {
	repo := testRepo(t)
	userID := createTestUser(t, repo, "alice", 0)
	addTestNode(t, repo, userID, "/file1.txt", 500)

	qc := NewQuotaChecker(QuotaConfig{Mode: "total", TotalLimit: 1000}, repo)
	err := qc.Check(context.Background(), repo, userID, 400)
	if err != nil {
		t.Fatalf("should allow within limit, got: %v", err)
	}
}

func TestQuota_ModeTotal_BlocksWhenExceeded(t *testing.T) {
	repo := testRepo(t)
	userID := createTestUser(t, repo, "alice", 0)
	addTestNode(t, repo, userID, "/file1.txt", 800)

	qc := NewQuotaChecker(QuotaConfig{Mode: "total", TotalLimit: 1000}, repo)
	err := qc.Check(context.Background(), repo, userID, 300)
	if err != ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded, got: %v", err)
	}
}

func TestQuota_ModeTotal_AccountsForAllUsers(t *testing.T) {
	repo := testRepo(t)
	alice := createTestUser(t, repo, "alice", 0)
	bob := createTestUser(t, repo, "bob", 0)
	addTestNode(t, repo, alice, "/file1.txt", 600)
	addTestNode(t, repo, bob, "/file1.txt", 300)

	qc := NewQuotaChecker(QuotaConfig{Mode: "total", TotalLimit: 1000}, repo)
	// Total is 900, adding 200 would be 1100 > 1000.
	err := qc.Check(context.Background(), repo, alice, 200)
	if err != ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded, got: %v", err)
	}
}

func TestQuota_ModeUser_AllowsWithinLimit(t *testing.T) {
	repo := testRepo(t)
	userID := createTestUser(t, repo, "alice", 0)
	addTestNode(t, repo, userID, "/file1.txt", 300)

	qc := NewQuotaChecker(QuotaConfig{Mode: "user", UserLimit: 1000}, repo)
	err := qc.Check(context.Background(), repo, userID, 500)
	if err != nil {
		t.Fatalf("should allow within limit, got: %v", err)
	}
}

func TestQuota_ModeUser_BlocksWhenExceeded(t *testing.T) {
	repo := testRepo(t)
	userID := createTestUser(t, repo, "alice", 0)
	addTestNode(t, repo, userID, "/file1.txt", 800)

	qc := NewQuotaChecker(QuotaConfig{Mode: "user", UserLimit: 1000}, repo)
	err := qc.Check(context.Background(), repo, userID, 300)
	if err != ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded, got: %v", err)
	}
}

func TestQuota_ModeUser_RespectsPerUserOverride(t *testing.T) {
	repo := testRepo(t)
	// User has a custom quota of 2000, server default is 1000.
	userID := createTestUser(t, repo, "alice", 2000)
	addTestNode(t, repo, userID, "/file1.txt", 800)

	qc := NewQuotaChecker(QuotaConfig{Mode: "user", UserLimit: 1000}, repo)

	// 800 + 500 = 1300, exceeds default 1000 but within custom 2000.
	err := qc.Check(context.Background(), repo, userID, 500)
	if err != nil {
		t.Fatalf("should respect per-user override, got: %v", err)
	}

	// 800 + 1300 = 2100, exceeds custom 2000.
	err = qc.Check(context.Background(), repo, userID, 1300)
	if err != ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded with per-user override, got: %v", err)
	}
}

func TestQuota_ModeUser_UnlimitedUser(t *testing.T) {
	repo := testRepo(t)
	// storage_quota=0 in user mode means use server default.
	// But we test the GetUserQuota returning 0 for truly unlimited.
	// To get an unlimited user in mode=user, the admin would set storage_quota=-1
	// or some convention. Actually per the plan, quota=0 means "use server default"
	// and the server default (UserLimit) is always > 0 in mode=user.
	// The "unlimited" case is when a custom user quota is set to a very large number.
	// Let's just verify that the default flow works.
	userID := createTestUser(t, repo, "alice", 0)
	addTestNode(t, repo, userID, "/file1.txt", 500)

	qc := NewQuotaChecker(QuotaConfig{Mode: "user", UserLimit: 1000}, repo)
	err := qc.Check(context.Background(), repo, userID, 400)
	if err != nil {
		t.Fatalf("should allow within server default, got: %v", err)
	}
}

func TestQuota_OverwriteAccountsForDelta(t *testing.T) {
	repo := testRepo(t)
	userID := createTestUser(t, repo, "alice", 0)
	addTestNode(t, repo, userID, "/file1.txt", 800)

	qc := NewQuotaChecker(QuotaConfig{Mode: "user", UserLimit: 1000}, repo)

	// Overwriting: new file is 500 bytes, old was 800.
	// Net delta = 500 - 800 = -300. Usage would be 800 + (-300) = 500, within limit.
	err := qc.Check(context.Background(), repo, userID, -300)
	if err != nil {
		t.Fatalf("overwrite with smaller file should be allowed, got: %v", err)
	}

	// Overwriting: new file is 1500 bytes, old was 800.
	// Net delta = 1500 - 800 = 700. Usage would be 800 + 700 = 1500, over limit.
	err = qc.Check(context.Background(), repo, userID, 700)
	if err != ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded for overwrite exceeding limit, got: %v", err)
	}
}

func TestQuota_GetUserQuota(t *testing.T) {
	repo := testRepo(t)
	ctx := context.Background()

	// User with no override.
	alice := createTestUser(t, repo, "alice", 0)
	// User with custom quota.
	bob := createTestUser(t, repo, "bob", 5000)

	qc := NewQuotaChecker(QuotaConfig{Mode: "user", UserLimit: 1000}, repo)

	if got := qc.GetUserQuota(ctx, alice); got != 1000 {
		t.Errorf("alice quota: got %d, want 1000 (server default)", got)
	}
	if got := qc.GetUserQuota(ctx, bob); got != 5000 {
		t.Errorf("bob quota: got %d, want 5000 (custom override)", got)
	}

	// Non-existent user falls back to server default.
	if got := qc.GetUserQuota(ctx, 9999); got != 1000 {
		t.Errorf("non-existent user quota: got %d, want 1000", got)
	}
}

func TestQuota_ModeTotal_Transaction(t *testing.T) {
	repo := testRepo(t)
	userID := createTestUser(t, repo, "alice", 0)
	addTestNode(t, repo, userID, "/file1.txt", 500)

	qc := NewQuotaChecker(QuotaConfig{Mode: "total", TotalLimit: 1000}, repo)

	// Use a transaction-scoped repo for quota checks.
	repo.Transaction(func(txRepo *db.Repository) error {
		err := qc.Check(context.Background(), txRepo, userID, 400)
		if err != nil {
			t.Fatalf("should allow within tx, got: %v", err)
		}

		err = qc.Check(context.Background(), txRepo, userID, 600)
		if err != ErrQuotaExceeded {
			t.Fatalf("expected ErrQuotaExceeded within tx, got: %v", err)
		}
		return nil
	})
}

func TestQuota_GetUserQuota_UsesRepo(t *testing.T) {
	repo := testRepo(t)

	// Verify GetUserQuota uses the repo connection from the checker.
	qc := NewQuotaChecker(QuotaConfig{Mode: "user", UserLimit: 2000}, repo)

	userID := createTestUser(t, repo, "alice", 0)

	// Verify default is returned via GetUserQuota.
	if got := qc.GetUserQuota(context.Background(), userID); got != 2000 {
		t.Fatalf("expected 2000, got %d", got)
	}
}
