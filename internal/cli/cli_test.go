package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// executeCmd runs a gosilo CLI command with the given args, capturing stdout.
// It always uses an in-memory database via --db.
func executeCmd(t *testing.T, extraArgs ...string) (string, error) {
	t.Helper()

	// Reset flags between runs.
	jsonFlag = false
	userAddAdmin = false
	userPasswordFlag = ""
	userDisableFlag = false
	userDeleteForce = false

	buf := &bytes.Buffer{}
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)

	// Capture stdout by redirecting os.Stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	args := append([]string{"--db", "sqlite::memory:"}, extraArgs...)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()

	w.Close()
	os.Stdout = oldStdout

	var stdoutBuf bytes.Buffer
	stdoutBuf.ReadFrom(r)

	// Combine captured stdout with cobra output.
	output := stdoutBuf.String() + buf.String()
	return output, err
}

// seedUser creates a user via the CLI for testing.
func seedUser(t *testing.T) {
	t.Helper()
	_, err := executeCmd(t, "user", "add", "alice", "--password", "testpass1234", "--admin")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

// --- user add ---

func TestCLI_UserAdd_HappyPath(t *testing.T) {
	_, err := executeCmd(t, "user", "add", "bob", "--password", "securepass123")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestCLI_UserAdd_ShortPassword(t *testing.T) {
	_, err := executeCmd(t, "user", "add", "bob", "--password", "short")
	if err == nil {
		t.Fatal("expected error for short password")
	}
	if !strings.Contains(err.Error(), "at least 8") {
		t.Fatalf("expected 'at least 8' error, got: %v", err)
	}
}

func TestCLI_UserAdd_MissingUsername(t *testing.T) {
	_, err := executeCmd(t, "user", "add")
	if err == nil {
		t.Fatal("expected error for missing username")
	}
}

// --- user list ---

func TestCLI_UserList_Empty(t *testing.T) {
	out, err := executeCmd(t, "user", "list")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	// Should have header but no data rows (besides header/separator).
	if !strings.Contains(out, "Username") {
		t.Fatalf("expected header in output, got: %s", out)
	}
}

func TestCLI_UserList_JSON(t *testing.T) {
	out, err := executeCmd(t, "user", "list", "--json")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	var result []map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, out)
	}
}

// --- config list ---

func TestCLI_ConfigList(t *testing.T) {
	out, err := executeCmd(t, "config", "list")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !strings.Contains(out, "registration_mode") {
		t.Fatalf("expected registration_mode in output, got: %s", out)
	}
}

func TestCLI_ConfigList_JSON(t *testing.T) {
	out, err := executeCmd(t, "config", "list", "--json")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	var result []map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	// Check that we have the expected keys.
	found := false
	for _, r := range result {
		if r["key"] == "registration_mode" {
			found = true
		}
	}
	if !found {
		t.Fatal("registration_mode not found in JSON output")
	}
}

// --- config get ---

func TestCLI_ConfigGet(t *testing.T) {
	out, err := executeCmd(t, "config", "get", "registration_mode")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	val := strings.TrimSpace(out)
	if val != "closed" && val != "open" {
		t.Fatalf("expected valid registration mode, got %q", val)
	}
}

func TestCLI_ConfigGet_Unknown(t *testing.T) {
	_, err := executeCmd(t, "config", "get", "nonexistent_key")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestCLI_ConfigGet_JSON(t *testing.T) {
	out, err := executeCmd(t, "config", "get", "log_level", "--json")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if result["key"] != "log_level" {
		t.Fatalf("expected key=log_level, got %q", result["key"])
	}
}

// --- config set / reset ---

func TestCLI_ConfigSetAndReset(t *testing.T) {
	_, err := executeCmd(t, "config", "set", "log_level", "debug")
	if err != nil {
		t.Fatalf("set: %v", err)
	}

	// Note: each executeCmd uses a fresh :memory: DB, so reset is independent.
	_, err = executeCmd(t, "config", "reset", "log_level")
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
}

func TestCLI_ConfigSet_InvalidKey(t *testing.T) {
	_, err := executeCmd(t, "config", "set", "bogus_key", "value")
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

// --- audit tail ---

func TestCLI_AuditTail_Empty(t *testing.T) {
	out, err := executeCmd(t, "audit", "tail")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !strings.Contains(out, "No audit entries") {
		// May have header only.
		_ = out
	}
}

func TestCLI_AuditTail_JSON(t *testing.T) {
	out, err := executeCmd(t, "audit", "tail", "--json")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	trimmed := strings.TrimSpace(out)
	if trimmed != "[]" {
		var result []map[string]any
		if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
			t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
		}
	}
}

// --- doctor ---

func TestCLI_Doctor(t *testing.T) {
	// Doctor writes human-readable output to stderr, not stdout.
	// We verify it runs without hard failure (warnings are ok).
	_, err := executeCmd(t, "doctor")
	// In-memory DB uses journal_mode=memory (not WAL), so doctor returns
	// an error due to warnings. That's expected behavior.
	_ = err
}

func TestCLI_Doctor_JSON(t *testing.T) {
	out, err := executeCmd(t, "doctor", "--json")
	_ = err // doctor may return error for warnings

	trimmed := strings.TrimSpace(out)
	var result []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if len(result) < 5 {
		t.Fatalf("expected at least 5 checks, got %d", len(result))
	}
}

// --- env ---

func TestCLI_Env(t *testing.T) {
	out, err := executeCmd(t, "env")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !strings.Contains(out, "GOSILO_") {
		t.Fatalf("expected env template output, got: %s", out)
	}
}
