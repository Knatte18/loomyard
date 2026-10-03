//go:build integration

// cli_test.go covers the ide CLI cobra surface: spawn dispatch with a stubbed
// launcher, the non-git refusal and the missing-slug error.

package idecli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/ideengine"
)

// TestRunCLISpawnDispatch tests that spawn subcommand dispatches correctly with stubbed launcher.
// Stays serial (no t.Parallel): it swaps the package-level ideengine.CodeLauncher below and restores
// it in a defer, which under t.Parallel() is both a data race on a production package-level variable
// and a restore firing while sibling tests still run.
func TestRunCLISpawnDispatch(t *testing.T) {
	// Create a real hub so lyxcwd.Resolve succeeds inside the PersistentPreRunE.
	h := hubforge.NewHub(t, ".")

	// Stub ideengine.CodeLauncher so the test does not open VS Code.
	originalLauncher := ideengine.CodeLauncher
	defer func() { ideengine.CodeLauncher = originalLauncher }()
	ideengine.CodeLauncher = func(dir string) error { return nil }

	var out bytes.Buffer
	code := RunCLIIn(h.PrimeWorktree(), &out, []string{"spawn", "child"})

	// spawn should succeed or fail for a handler reason, not layout resolution.
	if code != 0 && !strings.Contains(out.String(), "spawn failed") {
		t.Fatalf("unexpected error during dispatch; output: %s", out.String())
	}
}

// TestRunCLI_NotAGitRepo verifies that "lyx ide menu" run from a non-git temp directory surfaces
// lyxcwd's bare ErrNotAGitRepo sentinel with no "failed to resolve layout:" prefix and no raw
// "fatal:" git stderr — the PersistentPreRunE aborts before menu's body runs, so the interactive
// picker is never reached.
func TestRunCLI_NotAGitRepo(t *testing.T) {
	tmpDir := t.TempDir()

	var out bytes.Buffer
	code := RunCLIIn(tmpDir, &out, []string{"menu"})

	if code != 1 {
		t.Errorf("RunCLI(menu) in non-git dir = %d; want 1", code)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI(menu) output is not valid JSON: %v; got: %q", err, out.String())
	}
	errMsg, _ := env["error"].(string)
	if errMsg != "not a git repository" {
		t.Errorf("RunCLI(menu) error = %q; want exactly \"not a git repository\"", errMsg)
	}
}

// TestRunCLI_MissingSlug verifies that "lyx ide spawn" with no slug errors appropriately.
func TestRunCLI_MissingSlug(t *testing.T) {
	// Requires a real hub so the PersistentPreRunE can resolve layout.
	h := hubforge.NewHub(t, ".")

	var out bytes.Buffer
	code := RunCLIIn(h.PrimeWorktree(), &out, []string{"spawn"})

	if code != 1 {
		t.Errorf("RunCLI(spawn) with no slug = %d; want 1", code)
	}
	if !strings.Contains(out.String(), "spawn") {
		t.Errorf("RunCLI(spawn) output missing \"spawn\"; got: %q", out.String())
	}
}
