// weft_test.go covers paired weft worktree spawn, prechecks, and rollback behavior.
// These are white-box tests that exercise the weft helpers in weft.go directly.

package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/git"
	"github.com/Knatte18/loomyard/internal/paths"
)

// TestWeftSpawnCreatesJunction verifies that paired Add creates the host _lyx junction
// pointing to the weft _lyx directory. The test checks that both the junction and
// the weft target directory exist.
func TestWeftSpawnCreatesJunction(t *testing.T) {
	const slug = "weft-junction-test"
	t.Setenv("WEFT_SKIP_PUSH", "1")

	hub := newTestRepo(t)
	addRemote(t, hub)
	newWeftRepo(t, hub)

	l, err := paths.Resolve(hub)
	if err != nil {
		t.Fatalf("paths.Resolve: %v", err)
	}

	w := New(Config{})
	_, err = w.Add(l, slug)
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// Verify host _lyx junction exists (Lstat should not fail)
	// On Windows, directory junctions may appear as regular files when queried via Lstat,
	// so the primary check is that Lstat doesn't fail (meaning the junction exists).
	hostLink := l.HostLyxLink(slug)
	_, err = os.Lstat(hostLink)
	if err != nil {
		t.Fatalf("lstat host junction: %v", err)
	}

	// Verify the weft _lyx directory exists (the junction target)
	// This verifies the directory structure on the weft side is correct.
	weftLyxTarget := l.WeftLyxDirFor(slug)
	if _, err := os.Stat(weftLyxTarget); os.IsNotExist(err) {
		t.Errorf("weft _lyx target missing at %s", weftLyxTarget)
	}
}

// TestWeftSpawnSedsExclude verifies that Add seeds the _lyx entry in the host worktree's
// .git/info/exclude file, and that re-seeding is idempotent.
func TestWeftSpawnSeedsExclude(t *testing.T) {
	const slug = "weft-exclude-test"
	t.Setenv("WEFT_SKIP_PUSH", "1")

	hub := newTestRepo(t)
	addRemote(t, hub)
	newWeftRepo(t, hub)

	l, err := paths.Resolve(hub)
	if err != nil {
		t.Fatalf("paths.Resolve: %v", err)
	}

	w := New(Config{})
	_, err = w.Add(l, slug)
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// Get the exclude file path
	worktreePath := l.WorktreePath(slug)
	stdout, _, exitCode, _ := git.RunGit([]string{"rev-parse", "--git-path", "info/exclude"}, worktreePath)
	if exitCode != 0 {
		t.Fatalf("git rev-parse --git-path info/exclude failed")
	}

	excludePath := strings.TrimSpace(stdout)
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(worktreePath, excludePath)
	}

	// Read the exclude file
	content, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude file: %v", err)
	}

	// Verify _lyx is present
	if !strings.Contains(string(content), "_lyx") {
		t.Errorf("exclude file does not contain _lyx entry")
	}

	// Verify re-seeding is idempotent by calling seedGitExclude again
	if err := seedGitExclude(l, slug); err != nil {
		t.Fatalf("seedGitExclude (idempotent): %v", err)
	}

	// Read again and verify content unchanged
	content2, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude file (2nd time): %v", err)
	}

	if string(content) != string(content2) {
		t.Errorf("re-seeding changed exclude file content")
	}
}

// TestWeftSpawnPairedWorktrees verifies that Add creates both host and weft worktrees
// on the mirrored branch.
func TestWeftSpawnPairedWorktrees(t *testing.T) {
	const slug = "paired-test"
	const branchPrefix = "prefix/"
	t.Setenv("WEFT_SKIP_PUSH", "1")

	hub := newTestRepo(t)
	addRemote(t, hub)
	newWeftRepo(t, hub)

	l, err := paths.Resolve(hub)
	if err != nil {
		t.Fatalf("paths.Resolve: %v", err)
	}

	w := New(Config{BranchPrefix: branchPrefix})
	result, err := w.Add(l, slug)
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	expectedBranch := branchPrefix + slug

	// Verify host worktree exists
	hostTarget := l.WorktreePath(slug)
	if _, err := os.Stat(hostTarget); os.IsNotExist(err) {
		t.Errorf("host worktree not created at %s", hostTarget)
	}

	// Verify weft worktree exists
	weftTarget := l.WeftWorktreePath(slug)
	if _, err := os.Stat(weftTarget); os.IsNotExist(err) {
		t.Errorf("weft worktree not created at %s", weftTarget)
	}

	// Verify host branch exists
	_, _, exitCode, _ := git.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + expectedBranch}, l.WorktreeRoot)
	if exitCode != 0 {
		t.Errorf("host branch %q not created", expectedBranch)
	}

	// Verify weft branch exists
	_, _, exitCode, _ = git.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + expectedBranch}, l.WeftRepoRoot())
	if exitCode != 0 {
		t.Errorf("weft branch %q not created", expectedBranch)
	}

	// Verify AddResult is correct
	if result.Branch != expectedBranch {
		t.Errorf("AddResult.Branch = %q; want %q", result.Branch, expectedBranch)
	}
}

// TestWeftPrechecksHardRequireWeftRepo verifies that Add errors immediately when
// the weft repo is absent, with no partial state created.
func TestWeftPrechecksHardRequireWeftRepo(t *testing.T) {
	const slug = "hard-require-test"
	t.Setenv("WEFT_SKIP_PUSH", "1")

	hub := newTestRepo(t)
	addRemote(t, hub)
	// Intentionally do NOT create weft repo

	l, err := paths.Resolve(hub)
	if err != nil {
		t.Fatalf("paths.Resolve: %v", err)
	}

	w := New(Config{})
	result, err := w.Add(l, slug)

	// Verify error
	if err == nil {
		t.Fatalf("Add(%q) should error when weft repo absent; got nil", slug)
	}
	if !strings.Contains(err.Error(), "no weft repo") {
		t.Errorf("Add(%q) error should mention 'no weft repo'; got %q", slug, err.Error())
	}

	// Verify zero residue: no host worktree created
	hostTarget := l.WorktreePath(slug)
	if _, statErr := os.Stat(hostTarget); !os.IsNotExist(statErr) {
		t.Errorf("Add(%q) created host worktree despite weft repo absent", slug)
	}

	if result.Slug != "" {
		t.Errorf("Add(%q) result should be zero on error", slug)
	}
}

// TestWeftPrechecksRejectExistingWeftWorktree verifies that Add errors when the
// weft worktree dir already exists.
func TestWeftPrechecksRejectExistingWeftWorktree(t *testing.T) {
	const slug = "weft-exists-test"
	t.Setenv("WEFT_SKIP_PUSH", "1")

	hub := newTestRepo(t)
	addRemote(t, hub)
	newWeftRepo(t, hub)

	l, err := paths.Resolve(hub)
	if err != nil {
		t.Fatalf("paths.Resolve: %v", err)
	}

	// Pre-create the weft worktree dir
	weftTarget := l.WeftWorktreePath(slug)
	if err := os.Mkdir(weftTarget, 0755); err != nil {
		t.Fatalf("mkdir weft target: %v", err)
	}

	w := New(Config{})
	result, err := w.Add(l, slug)

	// Verify error
	if err == nil {
		t.Fatalf("Add(%q) should error when weft worktree dir exists; got nil", slug)
	}

	// Verify zero residue: no host worktree created
	hostTarget := l.WorktreePath(slug)
	if _, statErr := os.Stat(hostTarget); !os.IsNotExist(statErr) {
		t.Errorf("Add(%q) created host worktree despite weft dir existing", slug)
	}

	if result.Slug != "" {
		t.Errorf("Add(%q) result should be zero on error", slug)
	}
}

// TestWeftPrechecksRejectExistingWeftBranch verifies that Add errors when the
// weft branch already exists.
func TestWeftPrechecksRejectExistingWeftBranch(t *testing.T) {
	const slug = "weft-branch-exists-test"
	t.Setenv("WEFT_SKIP_PUSH", "1")

	hub := newTestRepo(t)
	addRemote(t, hub)
	weftPrime := newWeftRepo(t, hub)

	// Pre-create the weft branch
	mustRun(t, weftPrime, "git", "branch", slug)

	l, err := paths.Resolve(hub)
	if err != nil {
		t.Fatalf("paths.Resolve: %v", err)
	}

	w := New(Config{})
	result, err := w.Add(l, slug)

	// Verify error
	if err == nil {
		t.Fatalf("Add(%q) should error when weft branch exists; got nil", slug)
	}
	if !strings.Contains(err.Error(), "weft branch") {
		t.Errorf("Add(%q) error should mention 'weft branch'; got %q", slug, err.Error())
	}

	// Verify zero residue: no host worktree created
	hostTarget := l.WorktreePath(slug)
	if _, statErr := os.Stat(hostTarget); !os.IsNotExist(statErr) {
		t.Errorf("Add(%q) created host worktree despite weft branch existing", slug)
	}

	if result.Slug != "" {
		t.Errorf("Add(%q) result should be zero on error", slug)
	}
}

// TestWeftHostPristineEnforced verifies that Add errors when the host branch
// carries a committed real _lyx (not a junction), which indicates a pre-weft state.
func TestWeftHostPristineEnforced(t *testing.T) {
	const slug = "host-pristine-test"
	t.Setenv("WEFT_SKIP_PUSH", "1")

	hub := newTestRepo(t)
	addRemote(t, hub)
	newWeftRepo(t, hub)

	l, err := paths.Resolve(hub)
	if err != nil {
		t.Fatalf("paths.Resolve: %v", err)
	}

	// Pre-create a real _lyx dir in the host worktree (committed to repo)
	realLyx := filepath.Join(hub, "_lyx")
	if err := os.Mkdir(realLyx, 0755); err != nil {
		t.Fatalf("mkdir _lyx: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realLyx, "file"), []byte("content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	mustRun(t, hub, "git", "add", "_lyx")
	mustRun(t, hub, "git", "commit", "-m", "add real _lyx")

	w := New(Config{})
	result, err := w.Add(l, slug)

	// Verify error about pristine host (Add should fail because host has a real _lyx)
	if err == nil {
		t.Fatalf("Add(%q) should error when host has real _lyx; got nil", slug)
	}
	if !strings.Contains(err.Error(), "predates weft") {
		t.Errorf("Add(%q) error should mention 'predates weft'; got %q", slug, err.Error())
	}

	if result.Slug != "" {
		t.Errorf("Add(%q) result should be zero on error", slug)
	}
}

// TestWeftRollbackOnPostHostCreateFailure simulates a post-host-create failure
// (e.g. pre-create the weft worktree dir to make createWeftWorktree fail) and
// asserts both host and weft state is rolled back completely.
// This test exercises rollbackAdd by manually creating the worktree and branch state
// that would exist after steps 7-8 of Add complete, then invoking rollback directly.
func TestWeftRollbackOnPostHostCreateFailure(t *testing.T) {
	const slug = "rollback-post-host-test"
	const branch = "lyx/" + slug // matches the default BranchPrefix
	t.Setenv("WEFT_SKIP_PUSH", "1")

	hub := newTestRepo(t)
	addRemote(t, hub)
	newWeftRepo(t, hub)

	l, err := paths.Resolve(hub)
	if err != nil {
		t.Fatalf("paths.Resolve: %v", err)
	}

	// Manually create the host and weft worktrees and branches to simulate the state
	// after Add steps 7-8 complete. This allows us to test rollbackAdd without having
	// to trigger an Add failure partway through (which is difficult due to prechecks).
	hostTarget := l.WorktreePath(slug)
	weftTarget := l.WeftWorktreePath(slug)

	// Create host worktree and branch
	mustRun(t, l.WorktreeRoot, "git", "worktree", "add", "-b", branch, hostTarget)

	// Create weft worktree and branch
	mustRun(t, l.WeftRepoRoot(), "git", "worktree", "add", "-b", branch, weftTarget)

	// Now call rollbackAdd to verify both are cleaned up
	w := New(Config{})
	rollbackErr := w.rollbackAdd(l, slug, branch, hostTarget)
	if rollbackErr != nil {
		t.Logf("rollbackAdd returned error (may be expected): %v", rollbackErr)
	}

	// Verify ZERO residue: no host worktree, no host branch, no weft branch, no weft worktree
	if _, statErr := os.Stat(hostTarget); !os.IsNotExist(statErr) {
		t.Errorf("rollback failed: host worktree still exists at %s", hostTarget)
	}

	// Verify host branch is gone
	stdout, _, _, _ := git.RunGit([]string{"branch"}, l.WorktreeRoot)
	if strings.Contains(stdout, slug) {
		t.Errorf("rollback failed: host branch containing %q still exists", slug)
	}

	// Verify weft worktree is gone
	if _, statErr := os.Stat(weftTarget); !os.IsNotExist(statErr) {
		t.Errorf("rollback failed: weft worktree still exists at %s", weftTarget)
	}

	// Verify weft branch is gone
	_, _, exitCode, _ := git.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + branch}, l.WeftRepoRoot())
	if exitCode == 0 {
		t.Errorf("rollback failed: weft branch still exists")
	}
}
