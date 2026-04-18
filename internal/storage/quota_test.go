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

func TestQuota_NoLimits_AlwaysAllows(t *testing.T) {
	repo := testRepo(t)
	userID := createTestUser(t, repo, "alice", 0)

	qc := NewQuotaChecker(QuotaConfig{}, repo)
	err := qc.Check(context.Background(), repo, userID, 999999999)
	if err != nil {
		t.Fatalf("no limits should always allow, got: %v", err)
	}
}

func TestQuota_GlobalCap_AllowsWithinLimit(t *testing.T) {
	repo := testRepo(t)
	userID := createTestUser(t, repo, "alice", 0)
	addTestNode(t, repo, userID, "/file1.txt", 500)

	qc := NewQuotaChecker(QuotaConfig{TotalLimit: 1000}, repo)
	err := qc.Check(context.Background(), repo, userID, 400)
	if err != nil {
		t.Fatalf("should allow within limit, got: %v", err)
	}
}

func TestQuota_GlobalCap_BlocksWhenExceeded(t *testing.T) {
	repo := testRepo(t)
	userID := createTestUser(t, repo, "alice", 0)
	addTestNode(t, repo, userID, "/file1.txt", 800)

	qc := NewQuotaChecker(QuotaConfig{TotalLimit: 1000}, repo)
	err := qc.Check(context.Background(), repo, userID, 300)
	if err != ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded, got: %v", err)
	}
}

func TestQuota_GlobalCap_AccountsForAllUsers(t *testing.T) {
	repo := testRepo(t)
	alice := createTestUser(t, repo, "alice", 0)
	bob := createTestUser(t, repo, "bob", 0)
	addTestNode(t, repo, alice, "/file1.txt", 600)
	addTestNode(t, repo, bob, "/file1.txt", 300)

	qc := NewQuotaChecker(QuotaConfig{TotalLimit: 1000}, repo)
	// Total is 900, adding 200 would be 1100 > 1000.
	err := qc.Check(context.Background(), repo, alice, 200)
	if err != ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded, got: %v", err)
	}
}

func TestQuota_PerUser_AllowsWithinLimit(t *testing.T) {
	repo := testRepo(t)
	// User has an explicit 1000-byte limit on their row.
	userID := createTestUser(t, repo, "alice", 1000)
	addTestNode(t, repo, userID, "/file1.txt", 300)

	qc := NewQuotaChecker(QuotaConfig{}, repo)
	err := qc.Check(context.Background(), repo, userID, 500)
	if err != nil {
		t.Fatalf("should allow within limit, got: %v", err)
	}
}

func TestQuota_PerUser_BlocksWhenExceeded(t *testing.T) {
	repo := testRepo(t)
	userID := createTestUser(t, repo, "alice", 1000)
	addTestNode(t, repo, userID, "/file1.txt", 800)

	qc := NewQuotaChecker(QuotaConfig{}, repo)
	err := qc.Check(context.Background(), repo, userID, 300)
	if err != ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded, got: %v", err)
	}
}

func TestQuota_PerUser_ZeroMeansUnlimited(t *testing.T) {
	repo := testRepo(t)
	// StorageQuota=0 on the user row means unlimited. Prior semantics
	// (0 = "use server default") no longer apply.
	userID := createTestUser(t, repo, "alice", 0)
	addTestNode(t, repo, userID, "/file1.txt", 500)

	qc := NewQuotaChecker(QuotaConfig{}, repo)
	err := qc.Check(context.Background(), repo, userID, 999999999)
	if err != nil {
		t.Fatalf("StorageQuota=0 should be unlimited, got: %v", err)
	}
}

func TestQuota_PerUserAndGlobal_BothEnforced(t *testing.T) {
	repo := testRepo(t)
	alice := createTestUser(t, repo, "alice", 500) // per-user: 500
	addTestNode(t, repo, alice, "/file1.txt", 200)

	qc := NewQuotaChecker(QuotaConfig{TotalLimit: 1000}, repo)

	// 200 + 200 = 400, within per-user 500 and global 1000.
	if err := qc.Check(context.Background(), repo, alice, 200); err != nil {
		t.Fatalf("should allow within both limits, got: %v", err)
	}

	// 200 + 400 = 600 > per-user 500.
	if err := qc.Check(context.Background(), repo, alice, 400); err != ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded from per-user cap, got: %v", err)
	}
}

func TestQuota_OverwriteAccountsForDelta(t *testing.T) {
	repo := testRepo(t)
	userID := createTestUser(t, repo, "alice", 1000)
	addTestNode(t, repo, userID, "/file1.txt", 800)

	qc := NewQuotaChecker(QuotaConfig{}, repo)

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

	alice := createTestUser(t, repo, "alice", 0)    // unlimited
	bob := createTestUser(t, repo, "bob", 5000)     // explicit 5000

	qc := NewQuotaChecker(QuotaConfig{}, repo)

	if got := qc.GetUserQuota(ctx, alice); got != 0 {
		t.Errorf("alice quota: got %d, want 0 (unlimited)", got)
	}
	if got := qc.GetUserQuota(ctx, bob); got != 5000 {
		t.Errorf("bob quota: got %d, want 5000", got)
	}
	if got := qc.GetUserQuota(ctx, 9999); got != 0 {
		t.Errorf("non-existent user quota: got %d, want 0", got)
	}
}

func TestQuota_GlobalCap_Transaction(t *testing.T) {
	repo := testRepo(t)
	userID := createTestUser(t, repo, "alice", 0)
	addTestNode(t, repo, userID, "/file1.txt", 500)

	qc := NewQuotaChecker(QuotaConfig{TotalLimit: 1000}, repo)

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
