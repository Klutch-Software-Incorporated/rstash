package db_test

import (
	"context"
	"strconv"
	"testing"

	"rstash/internal/db"
	"rstash/internal/model"
)

// countUsers returns the count of non-system users (id > 0).
func countUsers(t *testing.T, repo *db.Repository) int64 {
	t.Helper()
	var n int64
	if err := repo.GormDB().Model(&model.User{}).Where("id > 0").Count(&n).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	return n
}

// getSetting returns the raw setting value for key, or "" if not set.
func getSetting(t *testing.T, repo *db.Repository, key string) string {
	t.Helper()
	v, err := repo.GetSetting(context.Background(), key)
	if err != nil {
		t.Fatalf("get setting %s: %v", key, err)
	}
	return v
}

// getUserQuotas returns a user's StorageQuota and EgressQuota.
func getUserQuotas(t *testing.T, repo *db.Repository, userID int64) (int64, int64) {
	t.Helper()
	u, err := repo.GetUserByID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if u == nil {
		t.Fatalf("user %d not found", userID)
	}
	return u.StorageQuota, u.EgressQuota
}

// TestMigrate_FreshInstall verifies that a fresh DB with no users and no
// legacy settings gets the new self-hoster-friendly defaults: no legacy keys
// stamped, no global cap set, users-table empty.
func TestMigrate_FreshInstall(t *testing.T) {
	repo := testDB(t)

	// No legacy keys anywhere, no users.
	if got := getSetting(t, repo, "total_storage_limit"); got != "" {
		t.Errorf("fresh install should leave total_storage_limit unset, got %q", got)
	}
	if got := getSetting(t, repo, "quota_mode"); got != "" {
		t.Errorf("fresh install should have no quota_mode row, got %q", got)
	}
	if c := countUsers(t, repo); c != 0 {
		t.Errorf("fresh install should have 0 users, got %d", c)
	}
}

// TestMigrate_UpgradeOnHardcodedDefaults simulates production rstash.cloud
// before this branch: users exist but no quota_mode / egress_mode rows in
// the settings table, because the old binary ran on hardcoded defaults.
// The migration must reconstruct the legacy behavior: total_storage_limit
// set to 50GB, every user stamped with 500GB egress_quota.
func TestMigrate_UpgradeOnHardcodedDefaults(t *testing.T) {
	repo := testDB(t)
	ctx := context.Background()

	// Create users as if they existed before the upgrade. Note: OpenRepository
	// already ran Migrate, which saw zero users and did nothing (fresh install
	// path). Creating users now simulates users existing prior to a future
	// re-run of the migration. Then force a second migrate.
	alice, err := repo.CreateUser(ctx, "alice", "hash", "", false, true)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := repo.CreateUser(ctx, "bob", "hash", "", false, true)
	if err != nil {
		t.Fatal(err)
	}

	// Re-run the migration explicitly. The first run during OpenRepository
	// treated it as fresh install; now that users exist, calling Migrate
	// again exercises the "upgrade on hardcoded defaults" branch.
	if err := db.Migrate(repo.GormDB()); err != nil {
		t.Fatalf("re-run migrate: %v", err)
	}

	// total_storage_limit should now be stamped with the legacy 50GB.
	wantTotal := int64(50) << 30
	gotTotal := getSetting(t, repo, "total_storage_limit")
	if gotTotal != strconv.FormatInt(wantTotal, 10) {
		t.Errorf("total_storage_limit: got %q, want %q", gotTotal, strconv.FormatInt(wantTotal, 10))
	}

	// egress_quota stamped onto both users at the legacy 500GB.
	wantEgress := int64(500) << 30
	_, aliceEgress := getUserQuotas(t, repo, alice.ID)
	if aliceEgress != wantEgress {
		t.Errorf("alice egress_quota: got %d, want %d", aliceEgress, wantEgress)
	}
	_, bobEgress := getUserQuotas(t, repo, bob.ID)
	if bobEgress != wantEgress {
		t.Errorf("bob egress_quota: got %d, want %d", bobEgress, wantEgress)
	}

	// storage_quota stays at 0 (unlimited per user) since legacy mode
	// was "total" — individual users had no personal cap, just the
	// shared global pool.
	aliceStorage, _ := getUserQuotas(t, repo, alice.ID)
	if aliceStorage != 0 {
		t.Errorf("alice storage_quota: got %d, want 0 (total-mode preserves per-user 0)", aliceStorage)
	}

	// Legacy keys should be cleaned up (they were synthesized in-memory,
	// not written to DB; this just verifies nothing got left behind).
	if got := getSetting(t, repo, "quota_mode"); got != "" {
		t.Errorf("quota_mode should be absent after migration, got %q", got)
	}
	if got := getSetting(t, repo, "egress_mode"); got != "" {
		t.Errorf("egress_mode should be absent after migration, got %q", got)
	}
}

// TestMigrate_UpgradeWithExplicitLegacySettings simulates a prod instance
// where the admin had set quota_mode=user with an explicit quota_user value,
// plus egress_mode=user with an explicit egress_quota_user. These values
// must be stamped onto existing users and the legacy rows cleaned up.
func TestMigrate_UpgradeWithExplicitLegacySettings(t *testing.T) {
	repo := testDB(t)
	ctx := context.Background()

	// Users first (OpenRepository's initial migrate sees 0 users, no-op).
	alice, err := repo.CreateUser(ctx, "alice", "hash", "", false, true)
	if err != nil {
		t.Fatal(err)
	}
	// Bob was already given a custom per-user override before migration.
	bob, err := repo.CreateUser(ctx, "bob", "hash", "", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.GormDB().Model(&model.User{}).Where("id = ?", bob.ID).
		Update("storage_quota", int64(1)<<30).Error; err != nil {
		t.Fatal(err)
	}

	// Write legacy settings as the old binary would have.
	_ = repo.SetSetting(ctx, "quota_mode", "user")
	_ = repo.SetSetting(ctx, "quota_user", "2GB")
	_ = repo.SetSetting(ctx, "egress_mode", "user")
	_ = repo.SetSetting(ctx, "egress_quota_user", "100GB")

	if err := db.Migrate(repo.GormDB()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Alice (no prior override) gets the legacy default stamped.
	wantAlice := int64(2) << 30
	aliceStorage, aliceEgress := getUserQuotas(t, repo, alice.ID)
	if aliceStorage != wantAlice {
		t.Errorf("alice storage_quota: got %d, want %d", aliceStorage, wantAlice)
	}
	// Bob (prior override) keeps his explicit value.
	wantBob := int64(1) << 30
	bobStorage, bobEgress := getUserQuotas(t, repo, bob.ID)
	if bobStorage != wantBob {
		t.Errorf("bob storage_quota: got %d, want %d (preserve explicit override)", bobStorage, wantBob)
	}

	// Both users got egress stamped since neither had a prior value.
	wantEgress := int64(100) << 30
	if aliceEgress != wantEgress {
		t.Errorf("alice egress_quota: got %d, want %d", aliceEgress, wantEgress)
	}
	if bobEgress != wantEgress {
		t.Errorf("bob egress_quota: got %d, want %d", bobEgress, wantEgress)
	}

	// Legacy keys deleted.
	for _, k := range []string{"quota_mode", "quota_user", "egress_mode", "egress_quota_user"} {
		if got := getSetting(t, repo, k); got != "" {
			t.Errorf("legacy key %q should be deleted, got %q", k, got)
		}
	}
}

// TestMigrate_UpgradeWithModeOff verifies that quota_mode=off preserves the
// prior "no enforcement" state: no users get stamped with storage limits,
// no total_storage_limit is written, but legacy keys are cleaned up.
func TestMigrate_UpgradeWithModeOff(t *testing.T) {
	repo := testDB(t)
	ctx := context.Background()

	alice, err := repo.CreateUser(ctx, "alice", "hash", "", false, true)
	if err != nil {
		t.Fatal(err)
	}

	_ = repo.SetSetting(ctx, "quota_mode", "off")
	_ = repo.SetSetting(ctx, "egress_mode", "off")

	if err := db.Migrate(repo.GormDB()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	aliceStorage, aliceEgress := getUserQuotas(t, repo, alice.ID)
	if aliceStorage != 0 {
		t.Errorf("mode=off should leave storage_quota at 0, got %d", aliceStorage)
	}
	if aliceEgress != 0 {
		t.Errorf("mode=off should leave egress_quota at 0, got %d", aliceEgress)
	}
	if got := getSetting(t, repo, "total_storage_limit"); got != "" {
		t.Errorf("mode=off should not write total_storage_limit, got %q", got)
	}
	if got := getSetting(t, repo, "quota_mode"); got != "" {
		t.Errorf("quota_mode legacy key should be deleted, got %q", got)
	}
}

// TestMigrate_Idempotent verifies that running the migration twice produces
// the same result as running it once (no re-stamping, no double-writes).
func TestMigrate_Idempotent(t *testing.T) {
	repo := testDB(t)
	ctx := context.Background()

	alice, err := repo.CreateUser(ctx, "alice", "hash", "", false, true)
	if err != nil {
		t.Fatal(err)
	}

	_ = repo.SetSetting(ctx, "quota_mode", "user")
	_ = repo.SetSetting(ctx, "quota_user", "500MB")

	if err := db.Migrate(repo.GormDB()); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	firstStorage, _ := getUserQuotas(t, repo, alice.ID)

	// Simulate billing later downgrading alice back to the free tier's
	// 0 (unlimited). A second migration must not re-stamp her.
	if err := repo.GormDB().Model(&model.User{}).Where("id = ?", alice.ID).
		Update("storage_quota", 0).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Migrate(repo.GormDB()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	secondStorage, _ := getUserQuotas(t, repo, alice.ID)

	want := int64(500) << 20
	if firstStorage != want {
		t.Errorf("first migrate stamped wrong value: got %d, want %d", firstStorage, want)
	}
	if secondStorage != 0 {
		t.Errorf("second migrate should not re-stamp a billing-cleared 0; got %d, want 0", secondStorage)
	}
}
