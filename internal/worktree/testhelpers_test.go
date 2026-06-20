// testhelpers_test.go provides shared helpers for the worktree package's
// internal (white-box) tests.

package worktree

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

// addRemote creates a bare git repository and configures it as the origin
// remote for the hub repository.
//
// It creates:
//   - bare := t.TempDir() (a temporary bare repository)
//   - runs git init --bare in bare
//   - runs git remote add origin <bare> in hub
//
// Returns the bare repository path.
//
// Note: addRemote deliberately does NOT push the base branch. The Add method
// creates a new branch and pushes it with -u, which populates the bare repo.
func addRemote(t *testing.T, hub string) string {
	t.Helper()

	bare := t.TempDir()

	// Initialize bare repository
	mustRun(t, bare, "git", "init", "--bare")

	// Add remote to hub
	mustRun(t, hub, "git", "remote", "add", "origin", bare)

	return bare
}

// newWeftRepo creates a sibling weft Prime worktree at <container>/<base(hub)>-weft
// as an initialized git repository with a placeholder _lyx structure.
//
// Given the hub (whose Prime host worktree is at hub), creates:
//   - weftPrime := filepath.Join(filepath.Dir(hub), filepath.Base(hub)+"-weft")
//   - git init -b main
//   - git config user.email test@test.com
//   - git config user.name Test
//   - creates _lyx/config/ subdirectory with a placeholder file
//   - git add .
//   - git commit -m init
//
// This ensures WeftRepoRoot() resolves correctly (which is Join(Hub, PrimeName()+"-weft")
// where PrimeName() is the base of the hub).
//
// Returns the weft Prime worktree path.
//
// Note: Tests should set WEFT_SKIP_PUSH=1 via t.Setenv unless they wire addWeftRemote.
func newWeftRepo(t *testing.T, hub string) string {
	t.Helper()

	container := filepath.Dir(hub)
	base := filepath.Base(hub)
	weftPrime := filepath.Join(container, base+"-weft")

	// Create the weft directory
	if err := os.Mkdir(weftPrime, 0755); err != nil {
		t.Fatalf("failed to create weft directory: %v", err)
	}

	// Initialize git repository
	mustRun(t, weftPrime, "git", "init", "-b", "main")

	// Configure git user
	mustRun(t, weftPrime, "git", "config", "user.email", "test@test.com")
	mustRun(t, weftPrime, "git", "config", "user.name", "Test")

	// Create _lyx/config/ structure with a placeholder file
	lyxConfigDir := filepath.Join(weftPrime, "_lyx", "config")
	if err := os.MkdirAll(lyxConfigDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	placeholderFile := filepath.Join(lyxConfigDir, "placeholder")
	if err := os.WriteFile(placeholderFile, []byte("weft config"), 0644); err != nil {
		t.Fatalf("failed to write placeholder: %v", err)
	}

	// Stage and commit
	mustRun(t, weftPrime, "git", "add", ".")
	mustRun(t, weftPrime, "git", "commit", "-m", "init")

	return weftPrime
}

// addWeftRemote creates a bare git repository and configures it as the origin
// remote for the weft Prime repository.
//
// It creates:
//   - weftBare := t.TempDir() (a temporary bare repository)
//   - runs git init --bare in weftBare
//   - runs git remote add origin <weftBare> in weftPrime
//
// Returns the bare repository path.
//
// Note: addWeftRemote deliberately does NOT push the base branch. The paired Add
// creates a new branch and pushes it with -u, which populates the bare repo.
func addWeftRemote(t *testing.T, weftPrime string) string {
	t.Helper()

	weftBare := t.TempDir()

	// Initialize bare repository
	mustRun(t, weftBare, "git", "init", "--bare")

	// Add remote to weft prime
	mustRun(t, weftPrime, "git", "remote", "add", "origin", weftBare)

	return weftBare
}
