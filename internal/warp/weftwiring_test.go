//go:build integration

// weftwiring_test.go covers paired weft worktree spawn, prechecks, and rollback behavior.
// These are white-box tests that exercise the weft helpers in weftwiring.go directly.

package warp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestWeftSpawnCreatesWeftDirectory verifies that paired Add creates the weft _lyx directory.
// Add is dormant: it does not create the host junction. The junction is wired by lyx init
// via WireJunctions. This test verifies the weft-side structure is correct.
func TestWeftSpawnCreatesWeftDirectory(t *testing.T) {
	t.Parallel()

	const slug = "weft-junction-test"

	f := lyxtest.CopyPairedLocal(t)

	w := New(Config{})
	_, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// Verify host _lyx junction does NOT exist (Add is dormant).
	hostLink := f.Layout.HostLyxLink(slug)
	isLink, err := fslink.IsLink(hostLink)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("fslink.IsLink(%s): %v", hostLink, err)
	}
	if isLink {
		t.Errorf("Add created host junction; want no junction (Add is dormant)")
	}

	// Verify the weft _lyx directory exists (the junction target).
	// This verifies the directory structure on the weft side is correct.
	weftLyxTarget := f.Layout.WeftLyxDirFor(slug)
	if _, err := os.Stat(weftLyxTarget); os.IsNotExist(err) {
		t.Errorf("weft _lyx target missing at %s", weftLyxTarget)
	}
}

// TestWeftSpawnNoExcludeEntry verifies that Add does not seed the exclude file.
// Add is dormant: it does not create the host junction or exclude entry. These are
// wired by lyx init via WireJunctions.
func TestWeftSpawnNoExcludeEntry(t *testing.T) {
	t.Parallel()

	const slug = "weft-exclude-test"

	f := lyxtest.CopyPairedLocal(t)

	w := New(Config{})
	_, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// Get the exclude file path.
	worktreePath := f.Layout.WorktreePath(slug)
	stdout, _, exitCode, _ := gitexec.RunGit([]string{"rev-parse", "--git-path", "info/exclude"}, worktreePath)
	if exitCode != 0 {
		t.Fatalf("git rev-parse --git-path info/exclude failed")
	}

	excludePath := strings.TrimSpace(stdout)
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(worktreePath, excludePath)
	}

	// Read the exclude file (if it exists).
	content, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read exclude file: %v", err)
	}

	// Verify _lyx is NOT present (Add is dormant).
	if strings.Contains(string(content), "_lyx") {
		t.Errorf("Add seeded exclude file with _lyx; want no entry (Add is dormant)")
	}
}

// TestWireJunctionsIdempotent verifies that WireJunctions creates the host junction
// and seeds the exclude entry, and that re-wiring is idempotent.
func TestWireJunctionsIdempotent(t *testing.T) {
	t.Parallel()

	const slug = "wire-junctions-test"

	f := lyxtest.CopyPairedLocal(t)

	// First, create a worktree via Add (dormant, no junctions).
	w := New(Config{})
	_, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// Now wire junctions via the primitive.
	if err := WireJunctions(f.Layout, slug); err != nil {
		t.Fatalf("WireJunctions(%q): %v", slug, err)
	}

	// Verify host _lyx junction exists and points to the weft target.
	hostLink := f.Layout.HostLyxLink(slug)
	isLink, err := fslink.IsLink(hostLink)
	if err != nil {
		t.Fatalf("fslink.IsLink(%s): %v", hostLink, err)
	}
	if !isLink {
		t.Errorf("WireJunctions did not create host junction at %s", hostLink)
	}

	// Verify the weft _lyx directory exists (the junction target).
	weftLyxTarget := f.Layout.WeftLyxDirFor(slug)
	if _, err := os.Stat(weftLyxTarget); os.IsNotExist(err) {
		t.Errorf("weft _lyx target missing at %s", weftLyxTarget)
	}

	// Verify exclude file contains _lyx.
	worktreePath := f.Layout.WorktreePath(slug)
	stdout, _, exitCode, _ := gitexec.RunGit([]string{"rev-parse", "--git-path", "info/exclude"}, worktreePath)
	if exitCode != 0 {
		t.Fatalf("git rev-parse --git-path info/exclude failed")
	}

	excludePath := strings.TrimSpace(stdout)
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(worktreePath, excludePath)
	}

	content, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude file: %v", err)
	}

	if !strings.Contains(string(content), "_lyx") {
		t.Errorf("exclude file does not contain _lyx entry")
	}

	// Verify re-wiring is idempotent.
	if err := WireJunctions(f.Layout, slug); err != nil {
		t.Fatalf("WireJunctions (idempotent): %v", err)
	}

	// Read exclude again and verify content unchanged.
	content2, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude file (2nd time): %v", err)
	}

	if string(content) != string(content2) {
		t.Errorf("re-wiring changed exclude file content")
	}
}

// TestWeftSpawnPairedWorktrees verifies that Add creates weft worktrees
// on the mirrored branch, and that the weft-side assertions are correct.
// Covered by TestAdd are: host worktree creation, branch naming, and AddResult.
func TestWeftSpawnPairedWorktrees(t *testing.T) {
	t.Parallel()

	const slug = "paired-test"

	f := lyxtest.CopyPairedLocal(t)

	w := New(Config{})
	_, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// Verify weft worktree exists at the expected location.
	weftTarget := f.Layout.WeftWorktreePath(slug)
	if _, err := os.Stat(weftTarget); os.IsNotExist(err) {
		t.Errorf("weft worktree not created at %s", weftTarget)
	}

	// Verify weft branch exists via WeftRepoRoot().
	weftRepoRoot := f.Layout.WeftRepoRoot()
	_, _, exitCode, _ := gitexec.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + slug}, weftRepoRoot)
	if exitCode != 0 {
		t.Errorf("weft branch %q not created", slug)
	}
}

// TestWeftPrechecks verifies that Add enforces preconditions on weft state:
// weft repo must exist, weft worktree must not exist, weft branch must not exist,
// and the host must be pristine (no real _lyx, only junctions allowed).
func TestWeftPrechecks(t *testing.T) {
	tests := []struct {
		name            string
		setup           func(t *testing.T, f lyxtest.PairedFixture)
		wantErrContains string
		wantNoTargetDir bool
		wantResultZero  bool
	}{
		{
			name: "TestWeftPrechecksHardRequireWeftRepo",
			setup: func(t *testing.T, f lyxtest.PairedFixture) {
				// Rename the weft prime dir so WeftRepoRoot() does not resolve.
				if err := os.Rename(f.WeftPrime, f.WeftPrime+"-disabled"); err != nil {
					t.Fatalf("rename weft prime: %v", err)
				}
			},
			wantErrContains: "no weft repo",
			wantNoTargetDir: true,
			wantResultZero:  true,
		},
		{
			name: "TestWeftPrechecksRejectExistingWeftWorktree",
			setup: func(t *testing.T, f lyxtest.PairedFixture) {
				// Pre-create the weft worktree dir.
				slug := "weft-prechecks-test"
				weftTarget := f.Layout.WeftWorktreePath(slug)
				if err := os.Mkdir(weftTarget, 0755); err != nil {
					t.Fatalf("mkdir weft target: %v", err)
				}
			},
			wantErrContains: "weft worktree directory already exists",
			wantNoTargetDir: true,
			wantResultZero:  true,
		},
	}

	const slug = "weft-prechecks-test"

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := lyxtest.CopyPairedLocal(t)
			tt.setup(t, f)

			w := New(Config{})
			result, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})

			// Verify error.
			if err == nil {
				t.Fatalf("Add(%q) should error; got nil", slug)
			}
			if !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Errorf("Add(%q) error = %q; want substring %q", slug, err.Error(), tt.wantErrContains)
			}

			// Verify zero residue: no host worktree created.
			if tt.wantNoTargetDir {
				hostTarget := f.Layout.WorktreePath(slug)
				if _, statErr := os.Stat(hostTarget); !os.IsNotExist(statErr) {
					t.Errorf("Add(%q) created host worktree despite error", slug)
				}
			}

			if tt.wantResultZero {
				if result.Slug != "" {
					t.Errorf("Add(%q) result should be zero on error; got non-empty result", slug)
				}
			}
		})
	}
}

// TestWeftSpawnPushesWeftBranch verifies that Add with SkipPush:false pushes the weft
// branch to the weft-bare remote.
func TestWeftSpawnPushesWeftBranch(t *testing.T) {
	t.Parallel()

	const slug = "weft-push-test"

	f := lyxtest.CopyPaired(t)

	w := New(Config{})
	_, err := w.Add(f.Layout, slug, AddOptions{})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// Verify the weft branch was pushed to the weft-bare remote.
	// Use git ls-remote to check for the mirrored branch ref in the weft-bare.
	stdout, _, exitCode, _ := gitexec.RunGit([]string{"ls-remote", f.WeftBare}, f.Layout.WeftRepoRoot())
	if exitCode != 0 {
		t.Fatalf("git ls-remote weft-bare failed")
	}

	// The branch should appear as refs/heads/<slug> in the remote.
	if !strings.Contains(stdout, "refs/heads/"+slug) {
		t.Errorf("weft branch %q not found in weft-bare after push; output: %s", slug, stdout)
	}
}

// TestWeftRollbackOnPostHostCreateFailure simulates a post-host-create failure
// and asserts both host and weft state is rolled back completely.
// Note: since Add is dormant (does not create junctions), rollback does not need
// to remove the host junction.
func TestWeftRollbackOnPostHostCreateFailure(t *testing.T) {
	t.Parallel()

	const slug = "rollback-post-host-test"
	const branch = "lyx/" + slug // matches the default BranchPrefix

	f := lyxtest.CopyPairedLocal(t)

	// Manually create the host and weft worktrees and branches to simulate the state
	// after Add steps 7-8 complete. This allows us to test rollbackAdd without having
	// to trigger an Add failure partway through (which is difficult due to prechecks).
	hostTarget := f.Layout.WorktreePath(slug)
	weftTarget := f.Layout.WeftWorktreePath(slug)

	// Create host worktree and branch.
	lyxtest.MustRun(t, f.Layout.WorktreeRoot, "git", "worktree", "add", "-b", branch, hostTarget)

	// Create weft worktree and branch.
	lyxtest.MustRun(t, f.Layout.WeftRepoRoot(), "git", "worktree", "add", "-b", branch, weftTarget)

	// Now call rollbackAdd to verify both are cleaned up.
	w := New(Config{})
	rollbackErr := w.rollbackAdd(f.Layout, slug, branch, hostTarget)
	if rollbackErr != nil {
		t.Logf("rollbackAdd returned error (may be expected): %v", rollbackErr)
	}

	// Verify ZERO residue: no host worktree, no host branch, no weft branch, no weft worktree.
	if _, statErr := os.Stat(hostTarget); !os.IsNotExist(statErr) {
		t.Errorf("rollback failed: host worktree still exists at %s", hostTarget)
	}

	// Verify host branch is gone.
	stdout, _, _, _ := gitexec.RunGit([]string{"branch"}, f.Layout.WorktreeRoot)
	if strings.Contains(stdout, slug) {
		t.Errorf("rollback failed: host branch containing %q still exists", slug)
	}

	// Verify weft worktree is gone.
	if _, statErr := os.Stat(weftTarget); !os.IsNotExist(statErr) {
		t.Errorf("rollback failed: weft worktree still exists at %s", weftTarget)
	}

	// Verify weft branch is gone.
	_, _, exitCode, _ := gitexec.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + branch}, f.Layout.WeftRepoRoot())
	if exitCode == 0 {
		t.Errorf("rollback failed: weft branch still exists")
	}
}

// TestWireJunctionsPreservesBehavior verifies that WireJunctions correctly creates
// the host _lyx junction and exclude entry. The junction resolves to the correct
// weft target, and the exclude file is properly seeded.
func TestWireJunctionsPreservesBehavior(t *testing.T) {
	t.Parallel()

	const slug = "seeder-parity-test"

	f := lyxtest.CopyPairedLocal(t)

	// Create worktree via Add (dormant).
	w := New(Config{})
	_, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// Wire junctions via the primitive.
	if err := WireJunctions(f.Layout, slug); err != nil {
		t.Fatalf("WireJunctions(%q): %v", slug, err)
	}

	// Verify host _lyx junction exists and points to the weft target.
	hostLink := f.Layout.HostLyxLink(slug)
	isLink, err := fslink.IsLink(hostLink)
	if err != nil {
		t.Fatalf("fslink.IsLink(%s): %v", hostLink, err)
	}
	if !isLink {
		t.Errorf("WireJunctions did not create host junction at %s", hostLink)
	}

	// Verify the junction resolves to the correct weft target.
	weftTarget := f.Layout.WeftLyxDirFor(slug)
	_, err = os.Stat(weftTarget)
	if os.IsNotExist(err) {
		t.Errorf("weft _lyx target missing at %s", weftTarget)
	}

	// Verify .git/info/exclude contains the _lyx entry.
	worktreePath := f.Layout.WorktreePath(slug)
	stdout, _, exitCode, _ := gitexec.RunGit([]string{"rev-parse", "--git-path", "info/exclude"}, worktreePath)
	if exitCode != 0 {
		t.Fatalf("git rev-parse --git-path info/exclude failed")
	}

	excludePath := strings.TrimSpace(stdout)
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(worktreePath, excludePath)
	}

	content, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "_lyx") {
		t.Errorf("exclude file does not contain _lyx entry")
	}

	// Verify the entry is a line-exact match (not just a substring).
	found := false
	for _, line := range strings.Split(contentStr, "\n") {
		if strings.TrimSpace(line) == "_lyx" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("exclude file does not contain _lyx as a complete line")
	}
}

// TestWeftForkPointMirrorsHost verifies that a new weft branch forks from the
// parent weft branch (whose name matches the host worktree's branch name at spawn time),
// preserving the merge-base for future squash-merge-back operations.
func TestWeftForkPointMirrorsHost(t *testing.T) {
	t.Parallel()

	const slug = "fork-point-test"

	f := lyxtest.CopyPairedLocal(t)

	// Capture weft main's tip SHA before spawning.
	weftRepoRoot := f.Layout.WeftRepoRoot()
	mainTipStdout, _, exitCode, _ := gitexec.RunGit([]string{"rev-parse", "refs/heads/main"}, weftRepoRoot)
	if exitCode != 0 {
		t.Fatalf("git rev-parse refs/heads/main failed")
	}
	mainTip := strings.TrimSpace(mainTipStdout)

	// Spawn a new weft worktree on main.
	w := New(Config{})
	_, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// Assert git merge-base <new-weft-branch> main equals mainTip.
	mergeBaseSHA, _, exitCode, _ := gitexec.RunGit(
		[]string{"merge-base", slug, "main"},
		weftRepoRoot,
	)
	if exitCode != 0 {
		t.Fatalf("git merge-base %s main failed", slug)
	}
	mergeBase := strings.TrimSpace(mergeBaseSHA)

	if mergeBase != mainTip {
		t.Errorf("fork point: merge-base(%s, main) = %s; want %s (main's tip)", slug, mergeBase, mainTip)
	}
}

// TestWeftForkPointSubtaskIsolation verifies that weft branch fork points are isolated
// per parent branch, not tied to weft main.
func TestWeftForkPointSubtaskIsolation(t *testing.T) {
	t.Parallel()

	const slug = "subtask-isolation-test"

	f := lyxtest.CopyPairedLocal(t)

	// Create non-main host branch Y with a commit.
	hostRoot := f.Layout.WorktreeRoot
	lyxtest.MustRun(t, hostRoot, "git", "checkout", "-b", "Y")
	if err := os.WriteFile(filepath.Join(hostRoot, "Y-file"), []byte("Y content"), 0o644); err != nil {
		t.Fatalf("write Y-file: %v", err)
	}
	lyxtest.MustRun(t, hostRoot, "git", "add", "Y-file")
	lyxtest.MustRun(t, hostRoot, "git", "commit", "-m", "Y commit")

	// Create matching weft branch Y and advance it one commit beyond weft main.
	weftRepoRoot := f.Layout.WeftRepoRoot()

	// First, capture the current main tip for verification later.
	mainTipStdout, _, _, _ := gitexec.RunGit([]string{"rev-parse", "refs/heads/main"}, weftRepoRoot)
	mainTip := strings.TrimSpace(mainTipStdout)

	// Create weft branch Y from main.
	lyxtest.MustRun(t, weftRepoRoot, "git", "branch", "Y", "main")

	// Create a temporary weft worktree on branch Y to add an extra commit.
	tempWeftPath := filepath.Join(weftRepoRoot, "temp-weft-Y")
	lyxtest.MustRun(t, weftRepoRoot, "git", "worktree", "add", "-b", "temp-Y", tempWeftPath, "Y")
	if err := os.WriteFile(filepath.Join(tempWeftPath, "Y-extra"), []byte("Y extra"), 0o644); err != nil {
		t.Fatalf("write Y-extra: %v", err)
	}
	lyxtest.MustRun(t, tempWeftPath, "git", "add", "Y-extra")
	lyxtest.MustRun(t, tempWeftPath, "git", "commit", "-m", "Y extra commit")

	// Advance weft branch Y to the temp commit's SHA.
	currentYTipStdout, _, _, _ := gitexec.RunGit([]string{"rev-parse", "refs/heads/temp-Y"}, weftRepoRoot)
	currentYTip := strings.TrimSpace(currentYTipStdout)
	lyxtest.MustRun(t, weftRepoRoot, "git", "branch", "-f", "Y", "temp-Y")

	// Clean up the temp worktree.
	lyxtest.MustRun(t, weftRepoRoot, "git", "worktree", "remove", tempWeftPath)
	lyxtest.MustRun(t, weftRepoRoot, "git", "branch", "-D", "temp-Y")

	// Now spawn a new weft worktree while on host branch Y.
	w := New(Config{})
	_, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// Assert git merge-base <new-weft-branch> Y equals currentYTip (not mainTip).
	mergeBaseSHA, _, exitCode, _ := gitexec.RunGit(
		[]string{"merge-base", slug, "Y"},
		weftRepoRoot,
	)
	if exitCode != 0 {
		t.Fatalf("git merge-base %s Y failed", slug)
	}
	mergeBase := strings.TrimSpace(mergeBaseSHA)

	if mergeBase != currentYTip {
		t.Errorf("subtask isolation: merge-base(%s, Y) = %s; want %s (Y's tip)", slug, mergeBase, currentYTip)
	}

	// Verify that the fork point is NOT weft main's tip (anti-regression).
	if mergeBase == mainTip {
		t.Errorf("subtask isolation: fork point equals main's tip; should be isolated to branch Y")
	}
}

// TestWeftMissingParentBranch verifies that Add returns an error and performs full
// paired rollback when the parent host branch has no matching weft branch.
func TestWeftMissingParentBranch(t *testing.T) {
	t.Parallel()

	const slug = "missing-parent-test"

	f := lyxtest.CopyPairedLocal(t)

	// Create host branch Z with a commit but no matching weft branch.
	hostRoot := f.Layout.WorktreeRoot
	lyxtest.MustRun(t, hostRoot, "git", "checkout", "-b", "Z")
	if err := os.WriteFile(filepath.Join(hostRoot, "Z-file"), []byte("Z content"), 0o644); err != nil {
		t.Fatalf("write Z-file: %v", err)
	}
	lyxtest.MustRun(t, hostRoot, "git", "add", "Z-file")
	lyxtest.MustRun(t, hostRoot, "git", "commit", "-m", "Z commit")

	// DO NOT create matching weft branch Z; this is the missing-parent scenario.

	// Run Add and expect an error (git worktree add will fail because Z doesn't exist in weft).
	w := New(Config{})
	result, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})

	if err == nil {
		t.Fatalf("Add(%q) should have failed; got nil error", slug)
	}

	// Assert ZERO residue: no host worktree, no host branch, no weft worktree, no weft branch.

	// 1. No host worktree dir.
	target := f.Layout.WorktreePath(slug)
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		if statErr == nil {
			t.Errorf("missing parent: host worktree dir still exists at %q", target)
		}
	}

	// 2. No host local branch.
	_, _, exitCode, _ := gitexec.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + slug}, f.Layout.WorktreeRoot)
	if exitCode == 0 {
		t.Errorf("missing parent: host branch %q still exists", slug)
	}

	// 3. No weft worktree dir.
	weftTarget := f.Layout.WeftWorktreePath(slug)
	if _, statErr := os.Stat(weftTarget); !os.IsNotExist(statErr) {
		if statErr == nil {
			t.Errorf("missing parent: weft worktree dir still exists at %q", weftTarget)
		}
	}

	// 4. No weft branch.
	_, _, exitCode, _ = gitexec.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + slug}, f.Layout.WeftRepoRoot())
	if exitCode == 0 {
		t.Errorf("missing parent: weft branch %q still exists", slug)
	}

	// 5. Result should be zero.
	if result.Slug != "" {
		t.Errorf("missing parent: result.Slug = %q; want empty on error", result.Slug)
	}
}
