package db_test

import (
	"context"
	"testing"

	"rstash/internal/db"
	"rstash/internal/model"
)

func TestCreateAndGetAPIKey(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	raw, err := db.GenerateAPIKey()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	hash, err := db.HashAPIKey(raw)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	key := &model.APIKey{
		Name:         "test",
		KeyHash:      hash,
		KeyPrefix:    db.APIKeyPrefix(raw),
		RateLimitRPM: 60,
	}
	if err := database.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("create: %v", err)
	}
	if key.ID == 0 {
		t.Fatal("expected ID to be populated after create")
	}

	got, err := database.GetAPIKey(ctx, key.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.Name != "test" {
		t.Fatalf("expected to find key with name 'test', got %+v", got)
	}
}

func TestFindAPIKeyByRaw(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	raw, _ := db.GenerateAPIKey()
	hash, _ := db.HashAPIKey(raw)
	key := &model.APIKey{
		Name:         "findme",
		KeyHash:      hash,
		KeyPrefix:    db.APIKeyPrefix(raw),
		RateLimitRPM: 60,
	}
	if err := database.CreateAPIKey(ctx, key); err != nil {
		t.Fatal(err)
	}

	got, err := database.FindAPIKeyByRaw(ctx, raw)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil {
		t.Fatal("expected to find key, got nil")
	}
	if got.ID != key.ID {
		t.Errorf("wrong key: want ID %d, got %d", key.ID, got.ID)
	}
}

func TestFindAPIKeyByRaw_WrongKey(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	raw, _ := db.GenerateAPIKey()
	hash, _ := db.HashAPIKey(raw)
	key := &model.APIKey{
		Name:         "findme",
		KeyHash:      hash,
		KeyPrefix:    db.APIKeyPrefix(raw),
		RateLimitRPM: 60,
	}
	if err := database.CreateAPIKey(ctx, key); err != nil {
		t.Fatal(err)
	}

	// Wrong key with same prefix shouldn't match.
	wrong := raw[:8] + "0000000000000000000000000000000000000000000000000000000000"
	got, err := database.FindAPIKeyByRaw(ctx, wrong)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for wrong key, got %+v", got)
	}

	// Completely unrelated key.
	got, err = database.FindAPIKeyByRaw(ctx, "0000000000000000000000000000000000000000000000000000000000000000")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for unrelated key, got %+v", got)
	}
}

func TestListAPIKeys(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	for _, name := range []string{"a", "b", "c"} {
		raw, _ := db.GenerateAPIKey()
		hash, _ := db.HashAPIKey(raw)
		key := &model.APIKey{
			Name:         name,
			KeyHash:      hash,
			KeyPrefix:    db.APIKeyPrefix(raw),
			RateLimitRPM: 60,
		}
		if err := database.CreateAPIKey(ctx, key); err != nil {
			t.Fatal(err)
		}
	}

	keys, err := database.ListAPIKeys(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
}

func TestDeleteAPIKey(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	raw, _ := db.GenerateAPIKey()
	hash, _ := db.HashAPIKey(raw)
	key := &model.APIKey{
		Name:         "bye",
		KeyHash:      hash,
		KeyPrefix:    db.APIKeyPrefix(raw),
		RateLimitRPM: 60,
	}
	if err := database.CreateAPIKey(ctx, key); err != nil {
		t.Fatal(err)
	}

	if err := database.DeleteAPIKey(ctx, key.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	got, err := database.GetAPIKey(ctx, key.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}
