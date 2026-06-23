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

	f := lyxtest.CopyPaired(t)

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

	f := lyxtest.CopyPaired(t)

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

	f := lyxtest.CopyPaired(t)

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

			f := lyxtest.CopyPaired(t)
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

// TestWeftRollbackOnPostHostCreateFailure simulates a post-host-create failure
// (e.g. pre-create the weft worktree dir to make createWeftWorktree fail) and
// asserts both host and weft state is rolled back completely.
// This test exercises rollbackAdd by manually creating the worktree and branch state
// that would exist after steps 7-8 of Add complete, then invoking rollback directly.
func TestWeftRollbackOnPostHostCreateFailure(t *testing.T) {
	t.Parallel()

	const slug = "rollback-post-host-test"
	const branch = "lyx/" + slug // matches the default BranchPrefix

	f := lyxtest.CopyPaired(t)

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
