//go:build integration

// clone_integration_test.go — integration tests for clone orchestration with real git fixtures.

package gitclone

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/paths"
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

	// Initialize bare repo
	lyxtest.MustRun(t, bare, "git", "init", "--bare")

	// Create a working directory to seed the bare repo
	tempWork := filepath.Join(dir, "temp-work-"+name)
	if err := os.Mkdir(tempWork, 0o755); err != nil {
		t.Fatalf("mkdir temp work: %v", err)
	}

	// Initialize git repo in working directory
	lyxtest.MustRun(t, tempWork, "git", "init", "-b", "main")

	// Configure git user for this repo
	lyxtest.MustRun(t, tempWork, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, tempWork, "git", "config", "user.name", "Test")

	// Add bare as origin; use forward slashes for git compatibility on Windows
	bareURL := filepath.ToSlash(bare)
	lyxtest.MustRun(t, tempWork, "git", "remote", "add", "origin", bareURL)

	// Create and commit a README
	readmePath := filepath.Join(tempWork, "README.md")
	if err := os.WriteFile(readmePath, []byte("# "+name), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}

	lyxtest.MustRun(t, tempWork, "git", "add", "README.md")
	lyxtest.MustRun(t, tempWork, "git", "commit", "-m", "init")
	lyxtest.MustRun(t, tempWork, "git", "push", "-u", "origin", "main")

	// Clean up the temporary work directory
	if err := os.RemoveAll(tempWork); err != nil {
		t.Fatalf("remove temp work: %v", err)
	}

	return bare
}

func TestCloneHub_HappyPath(t *testing.T) {
	cwd := t.TempDir()

	// Create bare remotes for host and weft
	hostBare := makeBareRemote(t, cwd, "myrepo")
	weftBare := makeBareRemote(t, cwd, "myrepo-weft")

	// Create the derived board bare (weft.wiki)
	_ = makeBareRemote(t, cwd, "myrepo-weft.wiki")

	// Clone the hub
	hubPath, resolvedBoardURL, err := cloneHub(cwd, hostBare, weftBare, "")
	if err != nil {
		t.Fatalf("cloneHub: %v", err)
	}

	// Assert the resolved board URL matches the derived URL
	expectedBoardURL := filepath.Join(cwd, "myrepo-weft.wiki.git")
	if resolvedBoardURL != expectedBoardURL {
		t.Errorf("resolvedBoardURL = %q; want %q", resolvedBoardURL, expectedBoardURL)
	}

	// Assert hub directory structure
	hostPath := filepath.Join(hubPath, "myrepo")
	weftPath := filepath.Join(hubPath, "myrepo-weft")
	boardPath := filepath.Join(hubPath, boardDirName)

	// Check that repos exist and are git repos
	for _, path := range []string{hostPath, weftPath, boardPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("repo does not exist: %s", path)
		}
		gitDir := filepath.Join(path, ".git")
		if _, err := os.Stat(gitDir); err != nil {
			t.Fatalf(".git dir missing in %s: %v", path, err)
		}
	}

	// Assert Hub root is NOT a git repo
	hubGitDir := filepath.Join(hubPath, ".git")
	if _, err := os.Stat(hubGitDir); err == nil {
		t.Fatalf("Hub root should not be a git repo, but .git exists")
	}

	// Assert no _lyx or _codeguide were created
	for _, dirName := range []string{"_lyx", "_codeguide"} {
		dirPath := filepath.Join(hubPath, dirName)
		if _, err := os.Stat(dirPath); err == nil {
			t.Fatalf("%s should not have been created", dirName)
		}
	}
}

func TestCloneHub_GeometryRoundTrip(t *testing.T) {
	cwd := t.TempDir()

	// Create bare remotes
	hostBare := makeBareRemote(t, cwd, "myrepo")
	weftBare := makeBareRemote(t, cwd, "myrepo-weft")
	_ = makeBareRemote(t, cwd, "myrepo-weft.wiki")

	// Clone the hub
	hubPath, _, err := cloneHub(cwd, hostBare, weftBare, "")
	if err != nil {
		t.Fatalf("cloneHub: %v", err)
	}

	// Resolve geometry from the cloned host Prime
	hostPath := filepath.Join(hubPath, "myrepo")
	layout, err := paths.Resolve(hostPath)
	if err != nil {
		t.Fatalf("paths.Resolve: %v", err)
	}

	// Assert geometry
	if layout.Hub != hubPath {
		t.Errorf("layout.Hub = %q; want %q", layout.Hub, hubPath)
	}
	if layout.PrimeName() != "myrepo" {
		t.Errorf("layout.PrimeName() = %q; want myrepo", layout.PrimeName())
	}

	expectedWeftRoot := filepath.Join(hubPath, "myrepo-weft")
	if layout.WeftRepoRoot() != expectedWeftRoot {
		t.Errorf("layout.WeftRepoRoot() = %q; want %q", layout.WeftRepoRoot(), expectedWeftRoot)
	}
}

func TestCloneHub_ExplicitBoardURL(t *testing.T) {
	cwd := t.TempDir()

	// Create bare remotes
	hostBare := makeBareRemote(t, cwd, "myrepo")
	weftBare := makeBareRemote(t, cwd, "myrepo-weft")

	// Create an explicit board bare (different name to verify it's used)
	_ = makeBareRemote(t, cwd, "myrepo-weft.wiki") // Create the default board URL so derivation would work
	explicitBoardBare := makeBareRemote(t, cwd, "myboard")

	// Clone with explicit board URL
	hubPath, resolvedBoardURL, err := cloneHub(cwd, hostBare, weftBare, explicitBoardBare)
	if err != nil {
		t.Fatalf("cloneHub: %v", err)
	}

	// Assert the resolved board URL matches the explicit URL
	if resolvedBoardURL != explicitBoardBare {
		t.Errorf("resolvedBoardURL = %q; want %q", resolvedBoardURL, explicitBoardBare)
	}

	// Assert board repo exists
	boardPath := filepath.Join(hubPath, boardDirName)
	if _, err := os.Stat(boardPath); err != nil {
		t.Fatalf("board does not exist: %s", boardPath)
	}

	gitDir := filepath.Join(boardPath, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		t.Fatalf(".git dir missing in board: %v", err)
	}
}

func TestCloneHub_AbortIfExists(t *testing.T) {
	cwd := t.TempDir()

	// Pre-create the hub directory
	hubName := "myrepo-HUB"
	hubPath := filepath.Join(cwd, hubName)
	if err := os.Mkdir(hubPath, 0o755); err != nil {
		t.Fatalf("mkdir hub: %v", err)
	}

	// Create a marker file to verify the pre-existing dir is untouched
	markerPath := filepath.Join(hubPath, "marker.txt")
	if err := os.WriteFile(markerPath, []byte("pre-existing"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	// Create bare remotes
	hostBare := makeBareRemote(t, cwd, "myrepo")
	weftBare := makeBareRemote(t, cwd, "myrepo-weft")
	_ = makeBareRemote(t, cwd, "myrepo-weft.wiki")

	// Try to clone (should fail)
	_, _, err := cloneHub(cwd, hostBare, weftBare, "")
	if err == nil {
		t.Fatalf("cloneHub should have failed because hub already exists")
	}

	// Assert the hub directory still exists with the marker file
	if _, err := os.Stat(hubPath); err != nil {
		t.Fatalf("hub directory was removed or modified unexpectedly")
	}

	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("marker file was removed from pre-existing hub")
	}
}

func TestCloneHub_StrictAbort(t *testing.T) {
	// Test with host clone failure
	t.Run("HostCloneFailure", func(t *testing.T) {
		cwd := t.TempDir()

		// Non-existent host bare
		nonExistentHost := filepath.Join(cwd, "nonexistent-host.git")

		// Valid weft and board
		weftBare := makeBareRemote(t, cwd, "myrepo-weft")
		_ = makeBareRemote(t, cwd, "myrepo-weft.wiki")

		// Clone should fail
		hubPath, _, err := cloneHub(cwd, nonExistentHost, weftBare, "")
		if err == nil {
			t.Fatalf("cloneHub should have failed with non-existent host")
		}

		// Hub should be fully removed
		if _, err := os.Stat(hubPath); err == nil {
			t.Fatalf("hub directory should have been removed after clone failure")
		}
	})

	// Test with weft clone failure
	t.Run("WeftCloneFailure", func(t *testing.T) {
		cwd := t.TempDir()

		hostBare := makeBareRemote(t, cwd, "myrepo")

		// Non-existent weft bare
		nonExistentWeft := filepath.Join(cwd, "nonexistent-weft.git")

		_ = makeBareRemote(t, cwd, "myrepo-weft.wiki")

		// Clone should fail
		hubPath, _, err := cloneHub(cwd, hostBare, nonExistentWeft, "")
		if err == nil {
			t.Fatalf("cloneHub should have failed with non-existent weft")
		}

		// Hub should be fully removed
		if _, err := os.Stat(hubPath); err == nil {
			t.Fatalf("hub directory should have been removed after clone failure")
		}
	})

	// Test with board clone failure
	t.Run("BoardCloneFailure", func(t *testing.T) {
		cwd := t.TempDir()

		hostBare := makeBareRemote(t, cwd, "myrepo")
		weftBare := makeBareRemote(t, cwd, "myrepo-weft")

		// Non-existent board bare; do not create the default board URL so cloning fails
		nonExistentBoard := filepath.Join(cwd, "nonexistent-board.git")

		// Clone should fail
		hubPath, _, err := cloneHub(cwd, hostBare, weftBare, nonExistentBoard)
		if err == nil {
			t.Fatalf("cloneHub should have failed with non-existent board")
		}

		// Hub should be fully removed
		if _, err := os.Stat(hubPath); err == nil {
			t.Fatalf("hub directory should have been removed after clone failure")
		}
	})
}

func TestCloneHub_TeardownFailure(t *testing.T) {
	cwd := t.TempDir()

	// Create bare remotes
	hostBare := makeBareRemote(t, cwd, "myrepo")

	// Non-existent weft to trigger a clone failure
	nonExistentWeft := filepath.Join(cwd, "nonexistent-weft.git")

	// Override removeAll to return an error
	oldRemoveAll := removeAll
	failureCount := 0
	removeAll = func(path string) error {
		failureCount++
		// First call fails, subsequent calls succeed (for cleanup)
		if failureCount == 1 {
			return os.ErrPermission
		}
		return oldRemoveAll(path)
	}
	t.Cleanup(func() {
		removeAll = oldRemoveAll
	})

	// Clone should fail with both the clone error and the removal error
	_, _, err := cloneHub(cwd, hostBare, nonExistentWeft, "")
	if err == nil {
		t.Fatalf("cloneHub should have failed with clone error")
	}

	// The returned error should mention the residual hub path
	errMsg := err.Error()
	if errMsg == "" {
		t.Fatalf("error should not be empty")
	}

	// Compute the expected hub path to verify it exists
	expectedHubPath := filepath.Join(cwd, deriveHostName(hostBare)+"-HUB")
	if !strings.Contains(errMsg, expectedHubPath) {
		t.Errorf("error message should contain residual hub path: got %q, want to contain %q", errMsg, expectedHubPath)
	}

	// Check that removeAll was called
	if failureCount == 0 {
		t.Fatalf("removeAll should have been called")
	}

	// The hub directory should still exist since removal failed
	if _, statErr := os.Stat(expectedHubPath); statErr != nil {
		t.Fatalf("hub directory should still exist after failed removal")
	}

	// Clean up the residual hub (this will succeed because failureCount >= 2)
	removeAll(expectedHubPath)
}
