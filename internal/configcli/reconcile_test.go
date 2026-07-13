// reconcile_test.go — tests for the lyx config reconcile subcommand.
//
// Migrated from internal/update/update_test.go. The two git-init-backed
// scenarios (dry-run and --apply) now live in reconcile_integration_test.go
// per the Test Tier Purity Invariant; this file keeps the spawn-free
// not-a-git-repo error-path assertion.

package configcli

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

// TestReconcile_NotAGitRepo verifies that "lyx config reconcile" run from a
// non-git temp directory surfaces hubgeometry's bare ErrNotAGitRepo sentinel
// with no "resolve layout:" prefix and no raw "fatal:" git stderr.
func TestReconcile_NotAGitRepo(t *testing.T) {
	tmpDir := t.TempDir()

	// Chdir into the non-git temp dir so hubgeometry.Getwd inside RunCLI resolves there.
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldCwd) //nolint:errcheck

	var buf bytes.Buffer
	runExitCode := RunCLI(&buf, []string{"reconcile"})

	if runExitCode != 1 {
		t.Errorf("RunCLI(reconcile) in non-git dir = %d; want 1", runExitCode)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v, output: %s", err, buf.String())
	}
	errMsg, _ := result["error"].(string)
	if errMsg != "not a git repository" {
		t.Errorf("RunCLI(reconcile) error = %q; want exactly \"not a git repository\"", errMsg)
	}
}
