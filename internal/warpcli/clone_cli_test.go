//go:build integration

// clone_cli_test.go covers the warpcli handler half of the clone subcommand,
// including the warpengine.RemoveAll seam swap that verifies teardown-failure
// error messages are surfaced through the CLI output.

package warpcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/warpengine"
)

// makeBareRemote creates a bare git repository with a single commit on the main branch.
//
// It initializes a bare repo at <dir>/<name>.git, then seeds it by initializing a working
// repository, creating and committing a README, and pushing back to the bare repo.
// This ensures the bare repo has a main branch with at least one commit, so a later clone
// will check out a branch (not detached HEAD).
//
// Returns the path to the bare repository.
func makeBareRemote(t *testing.T, dir, name string) string {
	t.Helper()

	bare := filepath.Join(dir, name+".git")
	if err := os.Mkdir(bare, 0o755); err != nil {
		t.Fatalf("mkdir bare: %v", err)
	}

	// Initialize bare repo.
	lyxtest.MustRun(t, bare, "git", "init", "--bare")

	// Create a working directory to seed the bare repo.
	tempWork := filepath.Join(dir, "temp-work-"+name)
	if err := os.Mkdir(tempWork, 0o755); err != nil {
		t.Fatalf("mkdir temp work: %v", err)
	}

	lyxtest.MustRun(t, tempWork, "git", "init", "-b", "main")
	lyxtest.MustRun(t, tempWork, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, tempWork, "git", "config", "user.name", "Test")

	// Add bare as origin; use forward slashes for git compatibility on Windows.
	bareURL := filepath.ToSlash(bare)
	lyxtest.MustRun(t, tempWork, "git", "remote", "add", "origin", bareURL)

	readmePath := filepath.Join(tempWork, "README.md")
	if err := os.WriteFile(readmePath, []byte("# "+name), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}

	lyxtest.MustRun(t, tempWork, "git", "add", "README.md")
	lyxtest.MustRun(t, tempWork, "git", "commit", "-m", "init")
	lyxtest.MustRun(t, tempWork, "git", "push", "-u", "origin", "main")

	// Clean up the temporary working directory.
	if err := os.RemoveAll(tempWork); err != nil {
		t.Fatalf("remove temp work: %v", err)
	}

	return bare
}

// TestCloneHub_TeardownFailure verifies that when a clone step fails and the subsequent
// teardown (via warpengine.RemoveAll) also fails, the combined error is surfaced through
// the CLI output with the "residual hub" message.
//
// It swaps warpengine.RemoveAll cross-package to inject a teardown error, then calls
// runCloneWithReset with a non-existent board URL so the board clone triggers teardown.
// The test stays serial (no t.Parallel) because it changes the global RemoveAll seam and
// relies on t.Chdir for the cwd that runCloneWithReset reads via paths.Getwd().
func TestCloneHub_TeardownFailure(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd) // runCloneWithReset reads paths.Getwd() so cwd must be set.

	// Swap RemoveAll to inject a teardown failure; restore after test.
	orig := warpengine.RemoveAll
	t.Cleanup(func() { warpengine.RemoveAll = orig })
	warpengine.RemoveAll = func(string) error {
		return fmt.Errorf("injected RemoveAll failure")
	}

	// Create valid host and weft bare remotes; leave the board URL non-existent so
	// the board clone fails and teardownHub is triggered with the swapped RemoveAll.
	hostBare := makeBareRemote(t, cwd, "myrepo")
	weftBare := makeBareRemote(t, cwd, "myrepo-weft")
	nonExistentBoard := filepath.Join(cwd, "nonexistent-board.git")

	var buf bytes.Buffer
	code := runCloneWithReset(&buf, []string{hostBare, weftBare, nonExistentBoard}, false)

	if code == 0 {
		t.Fatalf("runCloneWithReset should have failed; output: %s", buf.String())
	}

	// Parse the JSON output envelope and verify the error contains the teardown-failure message.
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON output: %v\noutput: %s", err, buf.String())
	}
	errMsg, _ := result["error"].(string)
	if !strings.Contains(errMsg, "residual hub") {
		t.Errorf("error %q does not contain \"residual hub\"; want teardown-failure message", errMsg)
	}
}
