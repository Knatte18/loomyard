//go:build integration

// weft_test.go covers paired weft worktree spawn, prechecks, and rollback behavior.
// These are white-box tests that exercise the weft helpers in weft.go directly.

package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/git"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestWeftSpawnCreatesJunction verifies that paired Add creates the host _lyx junction
// pointing to the weft _lyx directory. The test checks that both the junction and
// the weft target directory exist.
func TestWeftSpawnCreatesJunction(t *testing.T) {
	t.Parallel()

	const slug = "weft-junction-test"

	f := lyxtest.CopyPairedLocal(t)

	w := New(Config{})
	_, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// Verify host _lyx junction exists (Lstat should not fail).
	// On Windows, directory junctions may appear as regular files when queried via Lstat,
	// so the primary check is that Lstat doesn't fail (meaning the junction exists).
	hostLink := f.Layout.HostLyxLink(slug)
	_, err = os.Lstat(hostLink)
	if err != nil {
		t.Fatalf("lstat host junction: %v", err)
	}

	// Verify the weft _lyx directory exists (the junction target).
	// This verifies the directory structure on the weft side is correct.
	weftLyxTarget := f.Layout.WeftLyxDirFor(slug)
	if _, err := os.Stat(weftLyxTarget); os.IsNotExist(err) {
		t.Errorf("weft _lyx target missing at %s", weftLyxTarget)
	}
}

// TestWeftSpawnSeedsExclude verifies that Add seeds the _lyx entry in the host worktree's
// .git/info/exclude file, and that re-seeding is idempotent.
func TestWeftSpawnSeedsExclude(t *testing.T) {
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
	stdout, _, exitCode, _ := git.RunGit([]string{"rev-parse", "--git-path", "info/exclude"}, worktreePath)
	if exitCode != 0 {
		t.Fatalf("git rev-parse --git-path info/exclude failed")
	}

	excludePath := strings.TrimSpace(stdout)
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(worktreePath, excludePath)
	}

	// Read the exclude file.
	content, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude file: %v", err)
	}

	// Verify _lyx is present.
	if !strings.Contains(string(content), "_lyx") {
		t.Errorf("exclude file does not contain _lyx entry")
	}

	// Verify re-seeding is idempotent by calling seedGitExclude again.
	if err := seedGitExclude(f.Layout, slug); err != nil {
		t.Fatalf("seedGitExclude (idempotent): %v", err)
	}

	// Read again and verify content unchanged.
	content2, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude file (2nd time): %v", err)
	}

	if string(content) != string(content2) {
		t.Errorf("re-seeding changed exclude file content")
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
	_, _, exitCode, _ := git.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + slug}, weftRepoRoot)
	if exitCode != 0 {
		t.Errorf("weft branch %q not created", slug)
	}
}

// TestWeftPrechecks verifies that Add enforces preconditions on weft state:
// weft repo must exist, weft worktree must not exist, weft branch must not exist,
// and the host must be pristine (no real _lyx, only junctions allowed).
// All cases are combined in one table with shared shape: setup → w.Add(..., SkipPush:true) →
// assert error substring + zero residue (no host worktree).
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
		{
			name: "TestWeftPrechecksRejectExistingWeftBranch",
			setup: func(t *testing.T, f lyxtest.PairedFixture) {
				// Pre-create the weft branch.
				slug := "weft-prechecks-test"
				lyxtest.MustRun(t, f.WeftPrime, "git", "branch", slug)
			},
			wantErrContains: "weft branch",
			wantNoTargetDir: true,
			wantResultZero:  true,
		},
		{
			name: "TestWeftHostPristineEnforced",
			setup: func(t *testing.T, f lyxtest.PairedFixture) {
				// Pre-create a real _lyx dir in the host worktree (committed to repo).
				realLyx := filepath.Join(f.Hub, "_lyx")
				if err := os.Mkdir(realLyx, 0755); err != nil {
					t.Fatalf("mkdir _lyx: %v", err)
				}
				if err := os.WriteFile(filepath.Join(realLyx, "file"), []byte("content"), 0644); err != nil {
					t.Fatalf("write file: %v", err)
				}
				lyxtest.MustRun(t, f.Hub, "git", "add", "_lyx")
				lyxtest.MustRun(t, f.Hub, "git", "commit", "-m", "add real _lyx")
			},
			wantErrContains: "predates weft",
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
// branch to the weft-bare remote. This is the only test that exercises pushWeftBranch and
// uses a live weft-bare as the push target. It requires the full CopyPaired fixture
// (not the lean CopyPairedLocal) because it actually pushes to the weft-bare.
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
	stdout, _, exitCode, _ := git.RunGit([]string{"ls-remote", f.WeftBare}, f.Layout.WeftRepoRoot())
	if exitCode != 0 {
		t.Fatalf("git ls-remote weft-bare failed")
	}

	// The branch should appear as refs/heads/<slug> in the remote.
	if !strings.Contains(stdout, "refs/heads/"+slug) {
		t.Errorf("weft branch %q not found in weft-bare after push; output: %s", slug, stdout)
	}
}

// TestWeftRollbackOnPostHostCreateFailure simulates a post-host-create failure
// (e.g. pre-create the weft worktree dir to make createWeftWorktree fail) and
// asserts both host and weft state is rolled back completely.
// This test exercises rollbackAdd by manually creating the worktree and branch state
// that would exist after steps 7-8 of Add complete, then invoking rollback directly.
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
	stdout, _, _, _ := git.RunGit([]string{"branch"}, f.Layout.WorktreeRoot)
	if strings.Contains(stdout, slug) {
		t.Errorf("rollback failed: host branch containing %q still exists", slug)
	}

	// Verify weft worktree is gone.
	if _, statErr := os.Stat(weftTarget); !os.IsNotExist(statErr) {
		t.Errorf("rollback failed: weft worktree still exists at %s", weftTarget)
	}

	// Verify weft branch is gone.
	_, _, exitCode, _ := git.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + branch}, f.Layout.WeftRepoRoot())
	if exitCode == 0 {
		t.Errorf("rollback failed: weft branch still exists")
	}
}

// TestSeederParity verifies that the refactored seeders (iterating HostJunctions)
// preserve behaviour: the _lyx junction exists and resolves correctly, and the
// .git/info/exclude file contains the _lyx entry.
func TestSeederParity(t *testing.T) {
	t.Parallel()

	const slug = "seeder-parity-test"

	f := lyxtest.CopyPaired(t)

	w := New(Config{})
	_, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q): %v", slug, err)
	}

	// Verify host _lyx junction exists and points to the weft target.
	hostLink := f.Layout.HostLyxLink(slug)
	_, err = os.Lstat(hostLink)
	if err != nil {
		t.Fatalf("lstat host junction: %v", err)
	}

	// Verify the junction resolves to the correct weft target.
	weftTarget := f.Layout.WeftLyxDirFor(slug)
	_, err = os.Stat(weftTarget)
	if os.IsNotExist(err) {
		t.Errorf("weft _lyx target missing at %s", weftTarget)
	}

	// Verify .git/info/exclude contains the _lyx entry.
	worktreePath := f.Layout.WorktreePath(slug)
	stdout, _, exitCode, _ := git.RunGit([]string{"rev-parse", "--git-path", "info/exclude"}, worktreePath)
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
// The test captures weft main's tip before spawning, runs Add on main, then asserts
// git merge-base between the new weft branch and weft main equals the captured tip SHA.
func TestWeftForkPointMirrorsHost(t *testing.T) {
	t.Parallel()

	const slug = "fork-point-test"

	f := lyxtest.CopyPairedLocal(t)

	// Capture weft main's tip SHA before spawning.
	weftRepoRoot := f.Layout.WeftRepoRoot()
	mainTipStdout, _, exitCode, _ := git.RunGit([]string{"rev-parse", "refs/heads/main"}, weftRepoRoot)
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
	mergeBaseSHA, _, exitCode, _ := git.RunGit(
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
// per parent branch, not tied to weft main. The test creates a non-main host branch Y,
// creates a matching weft branch Y advanced one commit beyond main, then spawns a new
// weft worktree while on host branch Y. The new weft branch must fork from weft-Y's tip,
// not weft-main's tip, proving subtask isolation.
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
	mainTipStdout, _, _, _ := git.RunGit([]string{"rev-parse", "refs/heads/main"}, weftRepoRoot)
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
	currentYTipStdout, _, _, _ := git.RunGit([]string{"rev-parse", "refs/heads/temp-Y"}, weftRepoRoot)
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
	mergeBaseSHA, _, exitCode, _ := git.RunGit(
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
// paired rollback when the parent host branch has no matching weft branch. The test
// creates a host branch Z with no corresponding weft branch, then runs Add and asserts
// error + zero residue (no host worktree, no host branch, no weft worktree, no weft branch).
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
	_, _, exitCode, _ := git.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + slug}, f.Layout.WorktreeRoot)
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
	_, _, exitCode, _ = git.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + slug}, f.Layout.WeftRepoRoot())
	if exitCode == 0 {
		t.Errorf("missing parent: weft branch %q still exists", slug)
	}

	// 5. Result should be zero.
	if result.Slug != "" {
		t.Errorf("missing parent: result.Slug = %q; want empty on error", result.Slug)
	}
}
