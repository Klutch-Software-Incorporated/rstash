package blob

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func newTestFSStore(t *testing.T) *FSStore {
	t.Helper()
	store, err := NewFSStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFSStore: %v", err)
	}
	return store
}

func TestFSStore_PutThenGet(t *testing.T) {
	store := newTestFSStore(t)
	ctx := context.Background()

	data := []byte("hello, world")
	if err := store.Put(ctx, 1, "docs/readme.txt", bytes.NewReader(data)); err != nil {
		t.Fatalf("Put: %v", err)
	}

	rc, err := store.Get(ctx, 1, "docs/readme.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("got %q, want %q", got, data)
	}
}

func TestFSStore_PutOverwrites(t *testing.T) {
	store := newTestFSStore(t)
	ctx := context.Background()

	if err := store.Put(ctx, 1, "file.txt", bytes.NewReader([]byte("v1"))); err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	if err := store.Put(ctx, 1, "file.txt", bytes.NewReader([]byte("v2"))); err != nil {
		t.Fatalf("Put v2: %v", err)
	}

	rc, err := store.Get(ctx, 1, "file.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "v2" {
		t.Errorf("got %q, want %q", got, "v2")
	}
}

func TestFSStore_Delete(t *testing.T) {
	store := newTestFSStore(t)
	ctx := context.Background()

	if err := store.Put(ctx, 1, "file.txt", bytes.NewReader([]byte("data"))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Delete(ctx, 1, "file.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := store.Get(ctx, 1, "file.txt")
	if err == nil {
		t.Fatal("expected error getting deleted file, got nil")
	}
}

func TestFSStore_DeleteMissing(t *testing.T) {
	store := newTestFSStore(t)
	ctx := context.Background()

	// Deleting a non-existent file should not return an error.
	if err := store.Delete(ctx, 1, "nonexistent.txt"); err != nil {
		t.Fatalf("Delete nonexistent: %v", err)
	}
}

func TestFSStore_GetMissing(t *testing.T) {
	store := newTestFSStore(t)
	ctx := context.Background()

	_, err := store.Get(ctx, 1, "nonexistent.txt")
	if err == nil {
		t.Fatal("expected error getting missing file, got nil")
	}
}

func TestFSStore_DeleteTree(t *testing.T) {
	store := newTestFSStore(t)
	ctx := context.Background()

	// Create a subtree of files.
	files := []string{"photos/a.jpg", "photos/b.jpg", "photos/sub/c.jpg"}
	for _, f := range files {
		if err := store.Put(ctx, 1, f, bytes.NewReader([]byte("img"))); err != nil {
			t.Fatalf("Put %s: %v", f, err)
		}
	}

	if err := store.DeleteTree(ctx, 1, "photos/"); err != nil {
		t.Fatalf("DeleteTree: %v", err)
	}

	// All files should be gone.
	for _, f := range files {
		if _, err := store.Get(ctx, 1, f); err == nil {
			t.Errorf("expected file %s to be deleted", f)
		}
	}

	// The directory itself should be gone.
	dirPath := filepath.Join(store.basePath, "1", "photos")
	if _, err := os.Stat(dirPath); !os.IsNotExist(err) {
		t.Errorf("expected photos directory to be removed, got err: %v", err)
	}
}

func TestFSStore_PathTraversal(t *testing.T) {
	store := newTestFSStore(t)
	ctx := context.Background()

	// Attempt to escape the base path.
	_, err := store.Get(ctx, 1, "../../etc/passwd")
	if err == nil {
		t.Fatal("expected path traversal to be rejected, got nil")
	}

	err = store.Put(ctx, 1, "../../etc/evil", bytes.NewReader([]byte("bad")))
	if err == nil {
		t.Fatal("expected path traversal to be rejected, got nil")
	}
}

func TestFSStore_UserIsolation(t *testing.T) {
	store := newTestFSStore(t)
	ctx := context.Background()

	if err := store.Put(ctx, 1, "file.txt", bytes.NewReader([]byte("user1"))); err != nil {
		t.Fatalf("Put user 1: %v", err)
	}
	if err := store.Put(ctx, 2, "file.txt", bytes.NewReader([]byte("user2"))); err != nil {
		t.Fatalf("Put user 2: %v", err)
	}

	rc1, err := store.Get(ctx, 1, "file.txt")
	if err != nil {
		t.Fatalf("Get user 1: %v", err)
	}
	defer rc1.Close()
	got1, _ := io.ReadAll(rc1)

	rc2, err := store.Get(ctx, 2, "file.txt")
	if err != nil {
		t.Fatalf("Get user 2: %v", err)
	}
	defer rc2.Close()
	got2, _ := io.ReadAll(rc2)

	if string(got1) != "user1" {
		t.Errorf("user 1: got %q, want %q", got1, "user1")
	}
	if string(got2) != "user2" {
		t.Errorf("user 2: got %q, want %q", got2, "user2")
	}
}
