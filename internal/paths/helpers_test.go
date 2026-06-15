// helpers_test.go provides shared test scaffolding (temporary git repos and
// worktrees) for the paths package tests.

package paths_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// mustRun is a test helper that runs a command with the given arguments in the
// specified directory. It fails the test (via t.Fatalf) if the command returns
// a non-zero exit code.
func mustRun(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("command failed: %v; output: %s", err, output)
	}
}

// newTestRepo creates a test repository in a temporary directory structure.
//
// It creates:
//   - container := t.TempDir() (the parent directory)
//   - hub := filepath.Join(container, "hub") (the git repository)
//
// Then initializes the hub as a git repository with:
//   - git init -b main
//   - git config user.email test@test.com
//   - git config user.name Test
//   - writes hub/README with content "test"
//   - git add .
//   - git commit -m init
//
// Returns the hub directory path. The container is available via filepath.Dir(hub).
func newTestRepo(t *testing.T) string {
	t.Helper()

	container := t.TempDir()
	hub := filepath.Join(container, "hub")

	// Create the hub directory
	if err := os.Mkdir(hub, 0755); err != nil {
		t.Fatalf("failed to create hub directory: %v", err)
	}

	// Initialize git repository
	mustRun(t, hub, "git", "init", "-b", "main")

	// Configure git user
	mustRun(t, hub, "git", "config", "user.email", "test@test.com")
	mustRun(t, hub, "git", "config", "user.name", "Test")

	// Create and commit README
	readmeFile := filepath.Join(hub, "README")
	if err := os.WriteFile(readmeFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}

	// Stage and commit
	mustRun(t, hub, "git", "add", ".")
	mustRun(t, hub, "git", "commit", "-m", "init")

	return hub
}
