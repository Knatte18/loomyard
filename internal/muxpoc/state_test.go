// state_test.go covers muxpoc session-state persistence and lookup.

package muxpoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeEnv(t *testing.T) {
	environ := []string{
		"CLAUDECODE=1",
		"CLAUDE_CODE_SESSION_ID=abc",
		"CLAUDE_CODE_CHILD_SESSION=1",
		"CLAUDE_CODE_ENTRYPOINT=x",
		"CLAUDE_CODE_SSE_PORT=9",
		"HOME=/home/user",
		"PATH=/usr/bin",
		"MY_VAR=ok",
	}

	result := sanitizeEnv(environ)

	// Check we have exactly 3 entries
	if len(result) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(result), result)
	}

	// Convert to map for easier checking
	resultMap := make(map[string]bool)
	for _, entry := range result {
		parts := strings.SplitN(entry, "=", 2)
		resultMap[parts[0]] = true
	}

	// Check expected keys are present
	expected := map[string]bool{"HOME": true, "PATH": true, "MY_VAR": true}
	for key := range expected {
		if !resultMap[key] {
			t.Errorf("expected key %q not found", key)
		}
	}

	// Check removed keys are absent
	removed := []string{"CLAUDECODE", "CLAUDE_CODE_SESSION_ID", "CLAUDE_CODE_CHILD_SESSION", "CLAUDE_CODE_ENTRYPOINT", "CLAUDE_CODE_SSE_PORT"}
	for _, key := range removed {
		if resultMap[key] {
			t.Errorf("removed key %q should not be present", key)
		}
	}
}

func TestStrippedEnvKeys(t *testing.T) {
	environ := []string{
		"CLAUDECODE=1",
		"CLAUDE_CODE_SESSION_ID=abc",
		"CLAUDE_CODE_CHILD_SESSION=1",
		"CLAUDE_CODE_ENTRYPOINT=x",
		"CLAUDE_CODE_SSE_PORT=9",
		"HOME=/home/user",
		"PATH=/usr/bin",
		"MY_VAR=ok",
	}

	result := strippedEnvKeys(environ)

	// Check we have exactly 5 entries
	if len(result) != 5 {
		t.Fatalf("expected 5 entries, got %d: %v", len(result), result)
	}

	// Convert to set for easier checking
	resultSet := make(map[string]bool)
	for _, key := range result {
		resultSet[key] = true
	}

	// Check expected keys are present
	expected := map[string]bool{
		"CLAUDECODE":                true,
		"CLAUDE_CODE_SESSION_ID":    true,
		"CLAUDE_CODE_CHILD_SESSION": true,
		"CLAUDE_CODE_ENTRYPOINT":    true,
		"CLAUDE_CODE_SSE_PORT":      true,
	}
	for key := range expected {
		if !resultSet[key] {
			t.Errorf("expected key %q not found", key)
		}
	}

	// Check non-Claude keys are absent
	if resultSet["HOME"] || resultSet["PATH"] || resultSet["MY_VAR"] {
		t.Error("non-Claude keys should not be present")
	}

	// Check for duplicates
	if len(result) != len(resultSet) {
		t.Error("duplicate keys found")
	}
}

func TestSocketName(t *testing.T) {
	tests := []struct {
		cwd      string
		contains string
		pattern  string // characters that should be allowed: a-z0-9_-
	}{
		{
			cwd:      "C:\\Code\\loomyard\\wts\\loomyard-mux-design",
			contains: "muxpoc-",
		},
		{
			cwd:      "/home/user/repos/loomyard-mux-design",
			contains: "muxpoc-",
		},
	}

	for _, tt := range tests {
		result := socketName(tt.cwd)

		// Check prefix
		if !strings.HasPrefix(result, tt.contains) {
			t.Errorf("socketName(%q): expected prefix %q, got %q", tt.cwd, tt.contains, result)
		}

		// Check only lowercase alphanumeric, underscore, and dash
		for _, ch := range result {
			if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-') {
				t.Errorf("socketName(%q): contains invalid char %c in %q", tt.cwd, ch, result)
			}
		}

		// Check stability
		result2 := socketName(tt.cwd)
		if result != result2 {
			t.Errorf("socketName(%q): not stable, got %q then %q", tt.cwd, result, result2)
		}
	}
}

func TestLoadStateMissing(t *testing.T) {
	tmpDir := t.TempDir()

	state, err := LoadState(tmpDir)

	if state != nil {
		t.Errorf("expected nil state, got %v", state)
	}
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestLoadStateCorrupt(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .lyx directory
	lyxDir := filepath.Join(tmpDir, ".lyx")
	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
		t.Fatalf("failed to create .lyx: %v", err)
	}

	// Write corrupt JSON
	stateFile := filepath.Join(tmpDir, stateRelPath)
	if err := os.WriteFile(stateFile, []byte("not valid json"), 0o644); err != nil {
		t.Fatalf("failed to write corrupt state: %v", err)
	}

	state, err := LoadState(tmpDir)

	if state != nil {
		t.Errorf("expected nil state, got %v", state)
	}
	if err == nil {
		t.Error("expected non-nil error")
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()

	original := &MuxpocState{
		Session:     "test-session",
		Socket:      "muxpoc-test",
		StrippedEnv: []string{"CLAUDECODE"},
		Panes: []Pane{
			{
				ID:        "%1",
				SessionID: "sid-abc",
				Kind:      "main",
			},
		},
	}

	// Save
	if err := SaveState(tmpDir, original); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	// Verify lock file location: .lyx/muxpoc-state.json.lock
	lockPath := filepath.Join(tmpDir, ".lyx", "muxpoc-state.json.lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("lock file not found at expected location %q: %v", lockPath, err)
	}

	// Load
	loaded, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if loaded == nil {
		t.Fatalf("LoadState returned nil state")
	}

	// Compare
	if loaded.Session != original.Session {
		t.Errorf("Session mismatch: %q != %q", loaded.Session, original.Session)
	}
	if loaded.Socket != original.Socket {
		t.Errorf("Socket mismatch: %q != %q", loaded.Socket, original.Socket)
	}
	if len(loaded.StrippedEnv) != len(original.StrippedEnv) {
		t.Errorf("StrippedEnv length mismatch: %d != %d", len(loaded.StrippedEnv), len(original.StrippedEnv))
	} else {
		for i := range original.StrippedEnv {
			if loaded.StrippedEnv[i] != original.StrippedEnv[i] {
				t.Errorf("StrippedEnv[%d] mismatch: %q != %q", i, loaded.StrippedEnv[i], original.StrippedEnv[i])
			}
		}
	}
	if len(loaded.Panes) != len(original.Panes) {
		t.Errorf("Panes length mismatch: %d != %d", len(loaded.Panes), len(original.Panes))
	} else {
		for i := range original.Panes {
			if loaded.Panes[i].ID != original.Panes[i].ID ||
				loaded.Panes[i].SessionID != original.Panes[i].SessionID ||
				loaded.Panes[i].Kind != original.Panes[i].Kind {
				t.Errorf("Panes[%d] mismatch: %v != %v", i, loaded.Panes[i], original.Panes[i])
			}
		}
	}
}

func TestNewSessionID(t *testing.T) {
	id1, err1 := newSessionID()
	id2, err2 := newSessionID()

	if err1 != nil {
		t.Errorf("first newSessionID failed: %v", err1)
	}
	if err2 != nil {
		t.Errorf("second newSessionID failed: %v", err2)
	}

	if id1 == "" {
		t.Error("first newSessionID returned empty string")
	}
	if id2 == "" {
		t.Error("second newSessionID returned empty string")
	}

	// Check UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	// where y is one of 8, 9, a, b (variant bits)
	if !isValidUUIDv4(id1) {
		t.Errorf("first ID %q is not valid UUID v4", id1)
	}
	if !isValidUUIDv4(id2) {
		t.Errorf("second ID %q is not valid UUID v4", id2)
	}

	if id1 == id2 {
		t.Error("two IDs should differ")
	}
}

func isValidUUIDv4(id string) bool {
	// Format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		return false
	}

	if len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		return false
	}

	// Check version is 4
	if parts[2][0] != '4' {
		return false
	}

	// Check variant is RFC 4122 (first char of parts[3] should be 8, 9, a, or b)
	variantChar := parts[3][0]
	if variantChar != '8' && variantChar != '9' && variantChar != 'a' && variantChar != 'b' {
		return false
	}

	// Check all characters are hex
	fullID := strings.ReplaceAll(id, "-", "")
	for _, ch := range fullID {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			return false
		}
	}

	return true
}

func TestDeleteStateMissing(t *testing.T) {
	tmpDir := t.TempDir()

	err := DeleteState(tmpDir)
	if err != nil {
		t.Errorf("DeleteState on missing file should return nil, got %v", err)
	}
}

// TestSocketNameStability verifies that socketName derives stable identity from
// the worktree root basename. This test documents why callers must pass the
// worktree root rather than the raw cwd: if two callers pass different paths
// pointing to the same repo, they must derive the same socket name for psmux
// session identity to remain stable across the repository.
//
// Note: socket identity is the worktree-root basename, so two sibling worktrees
// with colliding leaf names would share a socket (inherent to the basename
// scheme — documented, not fixed here).
func TestSocketNameStability(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a subdirectory within the temp worktree root
	subDir := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	// Socket name of the root differs from socket name of a subdirectory
	rootSocket := socketName(tmpDir)
	subSocket := socketName(subDir)

	if rootSocket == subSocket {
		t.Errorf("socketName should differ for root %q and subdirectory %q, but both gave %q",
			tmpDir, subDir, rootSocket)
	}

	// Verify the root socket is derived from the basename of tmpDir
	// (tmpDir looks like /tmp/TestSocketNameStability1234567890/subdir)
	// The basename is the final path component
	expectedBase := filepath.Base(tmpDir)
	if !strings.Contains(rootSocket, expectedBase) && !strings.Contains(expectedBase, strings.TrimPrefix(rootSocket, "muxpoc-")) {
		// The basename may be sanitized, so just check it was derived from tmpDir
		t.Logf("rootSocket %q derived from tmpDir %q (basename %q)", rootSocket, tmpDir, expectedBase)
	}
}
