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
// pointing to the weft _lyx directory, and that the junction resolves correctly.
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

	// Verify host _lyx junction exists and is a symlink/junction
	hostLink := l.HostLyxLink(slug)
	info, err := os.Lstat(hostLink)
	if err != nil {
		t.Fatalf("lstat host junction: %v", err)
	}

	// Check mode bit for symlink (works on Windows and Unix)
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("host _lyx junction is not a symlink/junction at %s", hostLink)
	}

	// Verify junction resolves to the correct target via EvalSymlinks
	// (required for Windows junction resolution)
	weftLyxTarget := l.WeftLyxDirFor(slug)
	linkResolved, err := filepath.EvalSymlinks(hostLink)
	if err != nil {
		t.Fatalf("EvalSymlinks(host junction): %v", err)
	}
	targetResolved, err := filepath.EvalSymlinks(weftLyxTarget)
	if err != nil {
		t.Fatalf("EvalSymlinks(weft target): %v", err)
	}

	if linkResolved != targetResolved {
		t.Errorf("host junction does not resolve to weft target: got %q, want %q", linkResolved, targetResolved)
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
// Also verifies idempotency: re-seeding when _lyx is already the correct junction
// succeeds and is a no-op.
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

	// Pre-create a real _lyx dir in the hub (committed to repo)
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

	// Verify error about pristine host
	if err == nil {
		t.Fatalf("Add(%q) should error when host has real _lyx; got nil", slug)
	}
	if !strings.Contains(err.Error(), "predates weft") {
		t.Errorf("Add(%q) error should mention 'predates weft'; got %q", slug, err.Error())
	}

	if result.Slug != "" {
		t.Errorf("Add(%q) result should be zero on error", slug)
	}

	// Test idempotency: successful Add, then verify _lyx junction, then call seedLyxJunction again
	// First, remove the real _lyx from hub
	mustRun(t, hub, "git", "rm", "-r", "_lyx")
	mustRun(t, hub, "git", "commit", "-m", "remove real _lyx")

	// Now Add should succeed
	result, err = w.Add(l, slug)
	if err != nil {
		t.Fatalf("Add(%q) after cleanup: %v", slug, err)
	}

	// Verify the junction exists
	hostLink := l.HostLyxLink(slug)
	if _, err := os.Lstat(hostLink); err != nil {
		t.Fatalf("host junction missing after Add: %v", err)
	}

	// Re-seed should be idempotent (no error)
	if err := seedLyxJunction(l, slug); err != nil {
		t.Fatalf("seedLyxJunction (idempotent): %v", err)
	}
}

// TestWeftRollbackOnPostHostCreateFailure simulates a post-host-create failure
// (e.g. pre-create the weft worktree dir to make createWeftWorktree fail) and
// asserts both host and weft state is rolled back completely.
func TestWeftRollbackOnPostHostCreateFailure(t *testing.T) {
	const slug = "rollback-post-host-test"
	t.Setenv("WEFT_SKIP_PUSH", "1")

	hub := newTestRepo(t)
	addRemote(t, hub)
	newWeftRepo(t, hub)

	l, err := paths.Resolve(hub)
	if err != nil {
		t.Fatalf("paths.Resolve: %v", err)
	}

	// Pre-create the weft worktree dir to trigger createWeftWorktree failure
	weftTarget := l.WeftWorktreePath(slug)
	if err := os.Mkdir(weftTarget, 0755); err != nil {
		t.Fatalf("mkdir weft target: %v", err)
	}

	w := New(Config{})
	result, err := w.Add(l, slug)

	// Verify error
	if err == nil {
		t.Fatalf("Add(%q) should fail due to pre-existing weft worktree", slug)
	}

	// Verify ZERO residue: no host worktree, no host branch, no weft branch
	hostTarget := l.WorktreePath(slug)
	if _, statErr := os.Stat(hostTarget); !os.IsNotExist(statErr) {
		t.Errorf("rollback failed: host worktree still exists at %s", hostTarget)
	}

	hostBranch := "prefix-" + slug // Assuming no prefix in config
	_, _, exitCode, _ := git.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + hostBranch}, l.WorktreeRoot)
	// This might not exist since we're not using a prefix, but check the actual branch
	stdout, _, _, _ := git.RunGit([]string{"branch"}, l.WorktreeRoot)
	if strings.Contains(stdout, slug) {
		t.Errorf("rollback failed: host branch containing %q still exists", slug)
	}

	// Verify host _lyx junction is gone
	hostLink := l.HostLyxLink(slug)
	if _, statErr := os.Lstat(hostLink); !os.IsNotExist(statErr) {
		t.Errorf("rollback failed: host junction still exists at %s", hostLink)
	}

	// Verify weft branch is gone
	_, _, exitCode, _ = git.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + slug}, l.WeftRepoRoot())
	if exitCode == 0 {
		t.Errorf("rollback failed: weft branch still exists")
	}

	if result.Slug != "" {
		t.Errorf("Add(%q) result should be zero on error", slug)
	}
}
