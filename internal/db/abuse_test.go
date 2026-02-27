package db_test

import (
	"context"
	"testing"

	"gosilo/internal/db"
)

func TestCreateAbuseReport(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	report, err := db.CreateAbuseReport(ctx, database, "test@example.com", "/public/bad.txt", "illegal", "bad content", "1.2.3.4")
	if err != nil {
		t.Fatalf("create abuse report: %v", err)
	}
	if report.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if report.Status != "open" {
		t.Fatalf("expected status 'open', got %q", report.Status)
	}
	if report.ReporterEmail != "test@example.com" {
		t.Fatalf("expected email 'test@example.com', got %q", report.ReporterEmail)
	}
	if report.Reason != "illegal" {
		t.Fatalf("expected reason 'illegal', got %q", report.Reason)
	}
}

func TestGetAbuseReport(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	created, err := db.CreateAbuseReport(ctx, database, "a@b.com", "/file.txt", "spam", "", "10.0.0.1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := db.GetAbuseReport(ctx, database, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil report")
	}
	if got.ReportedPath != "/file.txt" {
		t.Fatalf("expected path '/file.txt', got %q", got.ReportedPath)
	}

	// Non-existent ID returns nil.
	got, err = db.GetAbuseReport(ctx, database, 9999)
	if err != nil {
		t.Fatalf("get non-existent: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for non-existent report")
	}
}

func TestListAbuseReports(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	// Create several reports.
	_, _ = db.CreateAbuseReport(ctx, database, "a@b.com", "/f1", "spam", "", "1.1.1.1")
	_, _ = db.CreateAbuseReport(ctx, database, "c@d.com", "/f2", "illegal", "", "2.2.2.2")

	// List all.
	all, err := db.ListAbuseReports(ctx, database, "", 50, 0)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(all))
	}

	// List by status.
	open, err := db.ListAbuseReports(ctx, database, "open", 50, 0)
	if err != nil {
		t.Fatalf("list open: %v", err)
	}
	if len(open) != 2 {
		t.Fatalf("expected 2 open reports, got %d", len(open))
	}

	reviewed, err := db.ListAbuseReports(ctx, database, "reviewed", 50, 0)
	if err != nil {
		t.Fatalf("list reviewed: %v", err)
	}
	if len(reviewed) != 0 {
		t.Fatalf("expected 0 reviewed reports, got %d", len(reviewed))
	}
}

func TestUpdateAbuseReportStatus(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	admin, err := db.CreateUser(ctx, database, "admin", "password", true, true)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	report, err := db.CreateAbuseReport(ctx, database, "a@b.com", "/bad", "dmca", "copyright", "1.1.1.1")
	if err != nil {
		t.Fatalf("create report: %v", err)
	}

	// Review the report.
	err = db.UpdateAbuseReportStatus(ctx, database, report.ID, "reviewed", admin.ID, "Looks fine, no action needed.")
	if err != nil {
		t.Fatalf("update status: %v", err)
	}

	// Verify.
	got, err := db.GetAbuseReport(ctx, database, report.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != "reviewed" {
		t.Fatalf("expected status 'reviewed', got %q", got.Status)
	}
	if got.ReviewerID == nil || *got.ReviewerID != admin.ID {
		t.Fatal("expected reviewer ID to match admin")
	}
	if got.ReviewNote == nil || *got.ReviewNote != "Looks fine, no action needed." {
		t.Fatalf("expected review note, got %v", got.ReviewNote)
	}
	if got.ReviewedAt == nil {
		t.Fatal("expected reviewed_at to be set")
	}
}

func TestCountOpenAbuseReports(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	// Initially zero.
	count, err := db.CountOpenAbuseReports(ctx, database)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}

	// Create two reports.
	r1, _ := db.CreateAbuseReport(ctx, database, "a@b.com", "/f1", "spam", "", "1.1.1.1")
	_, _ = db.CreateAbuseReport(ctx, database, "c@d.com", "/f2", "illegal", "", "2.2.2.2")

	count, err = db.CountOpenAbuseReports(ctx, database)
	if err != nil {
		t.Fatalf("count after create: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2, got %d", count)
	}

	// Review one — count should drop.
	admin, _ := db.CreateUser(ctx, database, "admin", "password", true, true)
	_ = db.UpdateAbuseReportStatus(ctx, database, r1.ID, "reviewed", admin.ID, "ok")

	count, err = db.CountOpenAbuseReports(ctx, database)
	if err != nil {
		t.Fatalf("count after review: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}
}
