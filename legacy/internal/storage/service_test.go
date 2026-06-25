package storage_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"rstash/internal/blob"
	"rstash/internal/db"
	"rstash/internal/storage"
)

func setup(t *testing.T) (*storage.Service, int64) {
	t.Helper()

	repo, err := db.OpenRepository("sqlite::memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })

	blobs, err := blob.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { blobs.Close() })

	svc := storage.NewService(repo, blobs, nil)

	user, err := repo.CreateUser(context.Background(), "alice", "secret", "", false, true)
	if err != nil {
		t.Fatal(err)
	}

	return svc, user.ID
}

func TestPutAndGetDocument(t *testing.T) {
	svc, userID := setup(t)
	ctx := context.Background()

	// PUT a new document.
	result, err := svc.PutDocument(ctx, userID, "/greeting.txt", strings.NewReader("hello"), "text/plain", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsNew {
		t.Fatal("expected IsNew=true for first PUT")
	}
	if result.ETag == "" {
		t.Fatal("expected non-empty ETag")
	}

	// GET the document back.
	getResult, err := svc.GetDocument(ctx, userID, "/greeting.txt", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}
	defer getResult.Content.Close()

	body, _ := io.ReadAll(getResult.Content)
	if string(body) != "hello" {
		t.Fatalf("expected %q, got %q", "hello", string(body))
	}
	if getResult.ContentType != "text/plain" {
		t.Fatalf("expected text/plain, got %q", getResult.ContentType)
	}
	if getResult.ETag != result.ETag {
		t.Fatalf("ETag mismatch: PUT=%q GET=%q", result.ETag, getResult.ETag)
	}

	// PUT again (update).
	result2, err := svc.PutDocument(ctx, userID, "/greeting.txt", strings.NewReader("world"), "text/plain", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}
	if result2.IsNew {
		t.Fatal("expected IsNew=false for update")
	}
	if result2.ETag == result.ETag {
		t.Fatal("expected different ETag after update")
	}
}

func TestGetDocumentNotFound(t *testing.T) {
	svc, userID := setup(t)
	ctx := context.Background()

	_, err := svc.GetDocument(ctx, userID, "/nonexistent.txt", storage.Conditions{})
	if err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteDocument(t *testing.T) {
	svc, userID := setup(t)
	ctx := context.Background()

	// PUT then DELETE.
	putResult, err := svc.PutDocument(ctx, userID, "/file.txt", strings.NewReader("data"), "text/plain", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}

	delResult, err := svc.DeleteDocument(ctx, userID, "/file.txt", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}
	if delResult.ETag != putResult.ETag {
		t.Fatalf("DELETE ETag should match last PUT ETag: %q vs %q", delResult.ETag, putResult.ETag)
	}

	// GET should now 404.
	_, err = svc.GetDocument(ctx, userID, "/file.txt", storage.Conditions{})
	if err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestFolderListing(t *testing.T) {
	svc, userID := setup(t)
	ctx := context.Background()

	// PUT two documents in the same folder.
	_, err := svc.PutDocument(ctx, userID, "/docs/a.txt", strings.NewReader("aaa"), "text/plain", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.PutDocument(ctx, userID, "/docs/b.txt", strings.NewReader("bbb"), "text/plain", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}

	// GET folder listing.
	desc, etag, err := svc.GetFolder(ctx, userID, "/docs/", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}

	if desc.Context != "http://remotestorage.io/spec/folder-description" {
		t.Fatalf("unexpected @context: %q", desc.Context)
	}
	if len(desc.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(desc.Items))
	}
	if _, ok := desc.Items["a.txt"]; !ok {
		t.Fatal("missing a.txt in folder listing")
	}
	if _, ok := desc.Items["b.txt"]; !ok {
		t.Fatal("missing b.txt in folder listing")
	}
	if etag == "" {
		t.Fatal("expected non-empty folder ETag")
	}

	// Root folder should show /docs/ subfolder.
	rootDesc, _, err := svc.GetFolder(ctx, userID, "/", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rootDesc.Items["docs/"]; !ok {
		t.Fatalf("expected docs/ in root listing, got %v", rootDesc.Items)
	}
}

func TestConditionalIfNoneMatch(t *testing.T) {
	svc, userID := setup(t)
	ctx := context.Background()

	putResult, err := svc.PutDocument(ctx, userID, "/test.txt", strings.NewReader("data"), "text/plain", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}

	// GET with matching If-None-Match → 304.
	_, err = svc.GetDocument(ctx, userID, "/test.txt", storage.Conditions{IfNoneMatch: []string{putResult.ETag}})
	if err != storage.ErrNotModified {
		t.Fatalf("expected ErrNotModified, got %v", err)
	}

	// GET with non-matching If-None-Match → 200.
	result, err := svc.GetDocument(ctx, userID, "/test.txt", storage.Conditions{IfNoneMatch: []string{"bogus"}})
	if err != nil {
		t.Fatal(err)
	}
	result.Content.Close()
}

func TestConditionalIfMatch(t *testing.T) {
	svc, userID := setup(t)
	ctx := context.Background()

	putResult, err := svc.PutDocument(ctx, userID, "/test.txt", strings.NewReader("v1"), "text/plain", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}

	// PUT with matching If-Match → OK.
	_, err = svc.PutDocument(ctx, userID, "/test.txt", strings.NewReader("v2"), "text/plain", storage.Conditions{IfMatch: putResult.ETag})
	if err != nil {
		t.Fatal(err)
	}

	// PUT with stale If-Match → 412.
	_, err = svc.PutDocument(ctx, userID, "/test.txt", strings.NewReader("v3"), "text/plain", storage.Conditions{IfMatch: putResult.ETag})
	if err != storage.ErrPreconditionFailed {
		t.Fatalf("expected ErrPreconditionFailed, got %v", err)
	}
}

func TestConditionalIfNoneMatchStar(t *testing.T) {
	svc, userID := setup(t)
	ctx := context.Background()

	// PUT with If-None-Match: * on non-existing → OK.
	_, err := svc.PutDocument(ctx, userID, "/new.txt", strings.NewReader("data"), "text/plain", storage.Conditions{IfNoneMatch: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}

	// PUT with If-None-Match: * on existing → 412.
	_, err = svc.PutDocument(ctx, userID, "/new.txt", strings.NewReader("other"), "text/plain", storage.Conditions{IfNoneMatch: []string{"*"}})
	if err != storage.ErrPreconditionFailed {
		t.Fatalf("expected ErrPreconditionFailed, got %v", err)
	}
}

func TestDeleteCleansUpEmptyFolders(t *testing.T) {
	svc, userID := setup(t)
	ctx := context.Background()

	// Create a nested document.
	_, err := svc.PutDocument(ctx, userID, "/a/b/c.txt", strings.NewReader("data"), "text/plain", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}

	// Verify folder /a/b/ exists.
	desc, _, err := svc.GetFolder(ctx, userID, "/a/b/", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(desc.Items) != 1 {
		t.Fatalf("expected 1 item in /a/b/, got %d", len(desc.Items))
	}

	// Delete the document.
	_, err = svc.DeleteDocument(ctx, userID, "/a/b/c.txt", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}

	// After delete, /a/b/ and /a/ should be cleaned up.
	// Root should have no children.
	rootDesc, _, err := svc.GetFolder(ctx, userID, "/", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rootDesc.Items) != 0 {
		t.Fatalf("expected empty root after cleanup, got %v", rootDesc.Items)
	}
}

func TestHeadDocument(t *testing.T) {
	svc, userID := setup(t)
	ctx := context.Background()

	putResult, err := svc.PutDocument(ctx, userID, "/head.txt", strings.NewReader("content"), "text/plain", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}

	headResult, err := svc.HeadDocument(ctx, userID, "/head.txt", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}
	if headResult.ETag != putResult.ETag {
		t.Fatalf("HEAD ETag mismatch: %q vs %q", headResult.ETag, putResult.ETag)
	}
	if headResult.ContentType != "text/plain" {
		t.Fatalf("expected text/plain, got %q", headResult.ContentType)
	}
	if headResult.ContentLength != 7 { // "content" is 7 bytes
		t.Fatalf("expected content-length 7, got %d", headResult.ContentLength)
	}
}

func TestFolderETagPropagation(t *testing.T) {
	svc, userID := setup(t)
	ctx := context.Background()

	// PUT first document.
	_, err := svc.PutDocument(ctx, userID, "/folder/a.txt", strings.NewReader("aaa"), "text/plain", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}

	_, etag1, err := svc.GetFolder(ctx, userID, "/folder/", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}

	// PUT second document — folder ETag should change.
	_, err = svc.PutDocument(ctx, userID, "/folder/b.txt", strings.NewReader("bbb"), "text/plain", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}

	_, etag2, err := svc.GetFolder(ctx, userID, "/folder/", storage.Conditions{})
	if err != nil {
		t.Fatal(err)
	}

	if etag1 == etag2 {
		t.Fatal("folder ETag should change when a child is added")
	}
}
