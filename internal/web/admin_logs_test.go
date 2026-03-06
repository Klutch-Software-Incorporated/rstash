package web

import (
	"os"
	"strings"
	"testing"
)

func TestTailFile_BasicRead(t *testing.T) {
	f, err := os.CreateTemp("", "rstash-test-log-*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	// Write 5 lines.
	for i := 1; i <= 5; i++ {
		f.WriteString("line " + strings.Repeat("x", i) + "\n")
	}
	f.Close()

	lines, err := tailFile(f.Name(), 200)
	if err != nil {
		t.Fatalf("tailFile: %v", err)
	}
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}
}

func TestTailFile_TailLimitsLines(t *testing.T) {
	f, err := os.CreateTemp("", "rstash-test-log-*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	// Write 500 lines.
	for i := 1; i <= 500; i++ {
		f.WriteString("line-" + strings.Repeat("0", 4) + "\n")
	}
	f.Close()

	lines, err := tailFile(f.Name(), 10)
	if err != nil {
		t.Fatalf("tailFile: %v", err)
	}
	if len(lines) != 10 {
		t.Fatalf("expected 10 lines, got %d", len(lines))
	}
}

func TestTailFile_NonExistentFile(t *testing.T) {
	_, err := tailFile("/nonexistent/path/log.txt", 10)
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestFilterByLevel_MatchesTarget(t *testing.T) {
	lines := []string{
		"time=2024-01-01T00:00:00Z level=INFO msg=hello",
		"time=2024-01-01T00:00:01Z level=ERROR msg=oops",
		"time=2024-01-01T00:00:02Z level=INFO msg=world",
		"time=2024-01-01T00:00:03Z level=ERROR msg=fail",
		"time=2024-01-01T00:00:04Z level=DEBUG msg=trace",
	}

	// Filter for ERROR.
	filtered := filterByLevel(lines, "error")
	if len(filtered) != 2 {
		t.Fatalf("expected 2 ERROR lines, got %d", len(filtered))
	}
	for _, line := range filtered {
		if !strings.Contains(line, "level=ERROR") {
			t.Fatalf("unexpected line in filtered output: %s", line)
		}
	}

	// Filter for INFO.
	filtered = filterByLevel(lines, "info")
	if len(filtered) != 2 {
		t.Fatalf("expected 2 INFO lines, got %d", len(filtered))
	}

	// Filter for DEBUG.
	filtered = filterByLevel(lines, "debug")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 DEBUG line, got %d", len(filtered))
	}
}

func TestFilterByLevel_NoMatches(t *testing.T) {
	lines := []string{
		"time=2024-01-01T00:00:00Z level=INFO msg=hello",
		"time=2024-01-01T00:00:01Z level=INFO msg=world",
	}

	filtered := filterByLevel(lines, "error")
	if len(filtered) != 0 {
		t.Fatalf("expected 0 lines, got %d", len(filtered))
	}
}

func TestFilterByLevel_EmptyInput(t *testing.T) {
	filtered := filterByLevel(nil, "error")
	if len(filtered) != 0 {
		t.Fatalf("expected 0 lines, got %d", len(filtered))
	}
}
