//go:build integration

// add_test.go covers Add's happy-path side effects (portal, launchers, pushed
// branch) and the zero-residue rollback on a post-creation failure. The paired
// Add creates both host and weft worktrees on the mirrored branch and requires
// a weft Prime repo; tests build this via lyxtest.CopyPairedLocal and pass
// AddOptions{SkipPush:true}.

package warpengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestAdd covers the paired worktree creation flow: the happy path, branch-prefix
// application, and each precondition failure (dirty source, existing branch,
// existing target dir, missing remote, missing weft repo).
func TestAdd(t *testing.T) {
	t.Parallel()

	const slug = "my-task"

	tests := []struct {
		name         string
		branchPrefix string
		// setup performs scenario-specific prep on top of the fresh CopyPairedLocal fixture.
		setup func(t *testing.T, f lyxtest.PairedFixture)
		// opts to pass to Add.
		opts            AddOptions
		wantBranch      string
		wantErrContains string
		// wantNoTargetDir asserts the sibling worktree dir was NOT created,
		// proving the precondition tripped before `git worktree add`.
		wantNoTargetDir bool
		// wantResultZero asserts result.Slug is empty when error occurs.
		wantResultZero bool
		// extraAssert is an optional per-case assertion hook (called only on success path).
		extraAssert func(t *testing.T, f lyxtest.PairedFixture, res AddResult)
	}{
		{
			name:       "HappyPath",
			setup:      func(t *testing.T, f lyxtest.PairedFixture) {},
			opts:       AddOptions{SkipPush: true},
			wantBranch: "my-task",
			// Add is dormant: no junctions wired by Add.
			extraAssert: func(t *testing.T, f lyxtest.PairedFixture, res AddResult) {
				// (a) from TestAddDormant: host _lyx is NOT a link.
				hostLink := f.Layout.HostLyxLink(slug)
				isLink, err := fslink.IsLink(hostLink)
				if err != nil && !os.IsNotExist(err) {
					t.Fatalf("fslink.IsLink(%s): %v", hostLink, err)
				}
				if isLink {
					t.Errorf("Add created host junction at %s; want no junction (Add is dormant)", hostLink)
				}

				// (b) from TestWeftSpawnCreatesWeftDirectory: weft _lyx dir exists.
				weftLyxTarget := f.Layout.WeftLyxDirFor(slug)
				if _, err := os.Stat(weftLyxTarget); os.IsNotExist(err) {
					t.Errorf("weft _lyx target missing at %s", weftLyxTarget)
				}

				// (c) from TestWeftSpawnNoExcludeEntry: exclude file does NOT contain _lyx.
				worktreePath := res.Path
				stdout, _, exitCode, _ := gitexec.RunGit([]string{"rev-parse", "--git-path", "info/exclude"}, worktreePath)
				if exitCode != 0 {
					t.Fatalf("git rev-parse --git-path info/exclude failed")
				}
				excludePath := strings.TrimSpace(stdout)
				if !filepath.IsAbs(excludePath) {
					excludePath = filepath.Join(worktreePath, excludePath)
				}
				content, err := os.ReadFile(excludePath)
				if err != nil && !os.IsNotExist(err) {
					t.Fatalf("read exclude file: %v", err)
				}
				if strings.Contains(string(content), "_lyx") {
					t.Errorf("Add seeded exclude file with _lyx; want no entry (Add is dormant)")
				}

				// (d) from TestWeftSpawnPairedWorktrees: weft worktree and branch exist.
				weftTarget := f.Layout.WeftWorktreePath(slug)
				if _, err := os.Stat(weftTarget); os.IsNotExist(err) {
					t.Errorf("weft worktree not created at %s", weftTarget)
				}
				weftRepoRoot := f.Layout.WeftRepoRoot()
				_, _, exitCode, _ = gitexec.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + slug}, weftRepoRoot)
				if exitCode != 0 {
					t.Errorf("weft branch %q not created", slug)
				}

				// (e) from TestWeftForkPointMirrorsHost: merge-base equals weft main tip.
				mainTipStdout, _, exitCode, _ := gitexec.RunGit([]string{"rev-parse", "refs/heads/main"}, weftRepoRoot)
				if exitCode != 0 {
					t.Fatalf("git rev-parse refs/heads/main failed")
				}
				mainTip := strings.TrimSpace(mainTipStdout)
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
			},
		},
		{
			name:         "BranchPrefix",
			branchPrefix: "hanf/",
			setup:        func(t *testing.T, f lyxtest.PairedFixture) {},
			opts:         AddOptions{SkipPush: true},
			wantBranch:   "hanf/my-task",
			extraAssert:  nil,
		},
		{
			name: "DirtySource",
			setup: func(t *testing.T, f lyxtest.PairedFixture) {
				// Modify a tracked file without committing so the clean check fails.
				if err := os.WriteFile(filepath.Join(f.Hub, "README"), []byte("modified"), 0644); err != nil {
					t.Fatalf("modify README: %v", err)
				}
			},
			opts:            AddOptions{SkipPush: true},
			wantErrContains: "source worktree has uncommitted changes",
			wantNoTargetDir: true,
			wantResultZero:  true,
			extraAssert:     nil,
		},
		{
			name: "BranchExists",
			setup: func(t *testing.T, f lyxtest.PairedFixture) {
				lyxtest.MustRun(t, f.Hub, "git", "branch", slug)
			},
			opts:            AddOptions{SkipPush: true},
			wantErrContains: `branch "my-task" already exists`,
			wantResultZero:  true,
			extraAssert:     nil,
		},
		{
			name: "NoRemote",
			setup: func(t *testing.T, f lyxtest.PairedFixture) {
				// Remove the origin remote from the hub.
				lyxtest.MustRun(t, f.Hub, "git", "remote", "remove", "origin")
			},
			opts:            AddOptions{SkipPush: true},
			wantErrContains: "no remote configured",
			wantNoTargetDir: true,
			wantResultZero:  true,
			extraAssert:     nil,
		},
		{
			name: "NoWeftRepo",
			setup: func(t *testing.T, f lyxtest.PairedFixture) {
				// Rename the weft prime dir so WeftRepoRoot() does not resolve.
				if err := os.Rename(f.WeftPrime, f.WeftPrime+"-disabled"); err != nil {
					t.Fatalf("rename weft prime: %v", err)
				}
			},
			opts:            AddOptions{SkipPush: true},
			wantErrContains: "no weft repo",
			wantNoTargetDir: true,
			wantResultZero:  true,
			extraAssert:     nil,
		}, // Migrated from TestWeftPrechecksHardRequireWeftRepo: result.Slug == ""
		{
			name: "DetachedHEAD",
			setup: func(t *testing.T, f lyxtest.PairedFixture) {
				// Detach HEAD by checking out a specific commit SHA.
				lyxtest.MustRun(t, f.Hub, "git", "checkout", "--detach")
			},
			opts:            AddOptions{SkipPush: true},
			wantErrContains: "detached HEAD",
			wantNoTargetDir: true,
			wantResultZero:  true,
			extraAssert:     nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := lyxtest.CopyPairedLocal(t)
			tt.setup(t, f)

			w := New(Config{BranchPrefix: tt.branchPrefix})
			result, err := w.Add(f.Layout, slug, tt.opts)

			target := f.Layout.WorktreePath(slug)

			if tt.wantErrContains != "" {
				if err == nil {
					t.Fatalf("Add(%q) error = nil; want error containing %q", slug, tt.wantErrContains)
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("Add(%q) error = %q; want substring %q", slug, err.Error(), tt.wantErrContains)
				}
				if tt.wantNoTargetDir {
					if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
						t.Errorf("Add(%q) created %q; want no directory", slug, target)
					}
				}
				if tt.wantResultZero {
					if result.Slug != "" {
						t.Errorf("Add(%q) result.Slug = %q; want empty on error", slug, result.Slug)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("Add(%q) error = %v; want nil", slug, err)
			}
			if result.Branch != tt.wantBranch {
				t.Errorf("Add(%q).Branch = %q; want %q", slug, result.Branch, tt.wantBranch)
			}
			if result.Path != target {
				t.Errorf("Add(%q).Path = %q; want %q", slug, result.Path, target)
			}
			// SkipPush:true is set in all happy-path cases, so Pushed must be false.
			// This assertion is load-bearing: it verifies that Pushed reflects the
			// actual push semantics rather than always returning true.
			wantPushed := !tt.opts.SkipPush && !tt.opts.SkipGit
			if result.Pushed != wantPushed {
				t.Errorf("Add(%q).Pushed = %v; want %v", slug, result.Pushed, wantPushed)
			}
			if _, statErr := os.Stat(result.Path); statErr != nil {
				t.Errorf("Add(%q) worktree dir missing: %v", slug, statErr)
			}
			// Run per-case extraAssert hook if defined.
			if tt.extraAssert != nil {
				tt.extraAssert(t, f, result)
			}
		})
	}
}

// TestAddRollback covers the transactional rollback on post-creation failure.
// It pre-creates a regular file at the portal location to trigger createPortal's
// "already exists" error, then asserts ZERO residue: no worktree dir, no local branch,
// no _launchers/<slug>/, and no weft worktree/branch left behind.
func TestAddRollback(t *testing.T) {
	t.Parallel()

	const slug = "rollback-test"
	f := lyxtest.CopyPairedLocal(t)

	// Pre-create a regular file at the portal location to trip createPortal's refuse-to-clobber.
	portalLink := filepath.Join(f.Layout.PortalsDir(), slug)
	if err := os.MkdirAll(filepath.Dir(portalLink), 0755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(portalLink, []byte("blocker"), 0644); err != nil {
		t.Fatalf("create blocker file: %v", err)
	}

	w := New(Config{})
	result, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})

	if err == nil {
		t.Fatalf("Add(%q) should have failed; got nil error", slug)
	}
	if !strings.Contains(err.Error(), "already exists") && !strings.Contains(err.Error(), "already exists — remove it first") {
		t.Errorf("Add(%q) error should mention 'already exists'; got %q", slug, err.Error())
	}

	// Assert ZERO residue.

	// 1. No host worktree dir.
	target := f.Layout.WorktreePath(slug)
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		if statErr == nil {
			t.Errorf("Add(%q) rollback failed: host worktree dir still exists at %q", slug, target)
		}
	}

	// 2. No host local branch.
	_, _, exitCode, _ := gitexec.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + slug}, f.Layout.WorktreeRoot)
	if exitCode == 0 {
		t.Errorf("Add(%q) rollback failed: host branch %q still exists", slug, slug)
	}

	// 3. No weft worktree dir.
	weftTarget := f.Layout.WeftWorktreePath(slug)
	if _, statErr := os.Stat(weftTarget); !os.IsNotExist(statErr) {
		if statErr == nil {
			t.Errorf("Add(%q) rollback failed: weft worktree dir still exists at %q", slug, weftTarget)
		}
	}

	// 4. No weft branch.
	_, _, exitCode, _ = gitexec.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + slug}, f.Layout.WeftRepoRoot())
	if exitCode == 0 {
		t.Errorf("Add(%q) rollback failed: weft branch %q still exists", slug, slug)
	}

	// 5. No _launchers/<slug>/.
	launcherDir := f.Layout.LauncherDir(slug)
	if _, statErr := os.Stat(launcherDir); !os.IsNotExist(statErr) {
		t.Errorf("Add(%q) rollback failed: launcher dir still exists at %q", slug, launcherDir)
	}

	// 6. No new branch on bare remote.
	stdout, _, _, _ := gitexec.RunGit([]string{"ls-remote", "origin"}, f.Layout.WorktreeRoot)
	if strings.Contains(stdout, slug) {
		t.Errorf("Add(%q) rollback failed: host branch pushed to remote", slug)
	}

	if result.Slug != "" {
		t.Errorf("Add(%q) result should be zero on error; got non-empty result", slug)
	}
}

// TestAddAdoptWeftBranchLockedFails asserts that when Add attempts to adopt an
// existing weft branch that is already checked out in another weft worktree, the
// resulting error is composed from local context (the branch name and git exit
// code) rather than git's own stderr text.
func TestAddAdoptWeftBranchLockedFails(t *testing.T) {
	t.Parallel()

	const slug = "adopt-lock-test"
	f := lyxtest.CopyPairedLocal(t)

	// Create a weft branch ahead of time (outside Add) so Add routes to the adopt path.
	weftBranch := slug
	parentBranch := "main"
	lyxtest.MustRun(t, f.Layout.WeftRepoRoot(), "git", "branch", weftBranch, parentBranch)

	// Lock the weft branch by checking it out in a separate weft worktree. This causes
	// the adopt-path `git worktree add <path> <branch>` inside Add to fail with
	// "already checked out".
	lockPath := filepath.Join(f.Layout.Hub, "lock-weft-adopt")
	lyxtest.MustRun(t, f.Layout.WeftRepoRoot(), "git", "worktree", "add", lockPath, weftBranch)
	t.Cleanup(func() {
		_, _, _, _ = gitexec.RunGit([]string{"worktree", "remove", "--force", lockPath}, f.Layout.WeftRepoRoot())
	})

	w := New(Config{})
	result, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})

	if err == nil {
		t.Fatalf("Add(%q) with locked weft branch error = nil; want adopt failure", slug)
	}
	if !strings.Contains(err.Error(), weftBranch) {
		t.Errorf("Add(%q) error = %q; want substring %q (branch name)", slug, err.Error(), weftBranch)
	}
	if strings.Contains(err.Error(), "fatal:") {
		t.Errorf("Add(%q) error = %q; want no %q substring (raw git stderr leak)", slug, err.Error(), "fatal:")
	}
	if strings.Contains(err.Error(), "already checked out") {
		t.Errorf("Add(%q) error = %q; want no %q substring (raw git stderr leak)", slug, err.Error(), "already checked out")
	}
	if result.Slug != "" {
		t.Errorf("Add(%q) result should be zero on error; got non-empty result", slug)
	}
}

// TestAddAdoptExistingWeftBranch asserts that Add adopts an existing weft branch
// instead of aborting with an error.
func TestAddAdoptExistingWeftBranch(t *testing.T) {
	t.Parallel()

	const slug = "adopt-test"
	f := lyxtest.CopyPairedLocal(t)

	// Create a weft branch ahead of time (outside Add).
	weftBranch := "adopt-test"
	parentBranch := "main" // Common convention in test fixtures.
	lyxtest.MustRun(t, f.Layout.WeftRepoRoot(), "git", "branch", weftBranch, parentBranch)

	// Add should adopt the existing branch instead of erroring.
	w := New(Config{})
	result, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Add(%q) with existing weft branch error = %v; want nil (adopt)", slug, err)
	}

	// Verify the weft worktree was created on the existing branch.
	weftTarget := f.Layout.WeftWorktreePath(slug)
	if _, statErr := os.Stat(weftTarget); statErr != nil {
		t.Errorf("Add(%q) weft worktree dir missing: %v", slug, statErr)
	}

	// Verify the branch in the weft worktree is the adopted branch.
	currentBranch, _, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
		weftTarget,
	)
	if err != nil || exitCode != 0 {
		t.Fatalf("get weft current branch: %v (exit %d)", err, exitCode)
	}
	if strings.TrimSpace(currentBranch) != weftBranch {
		t.Errorf("weft worktree branch = %q; want %q (adopted)", strings.TrimSpace(currentBranch), weftBranch)
	}

	// Assert result is valid and no junctions were wired by Add.
	if result.Slug != slug {
		t.Errorf("result.Slug = %q; want %q", result.Slug, slug)
	}

	hostLink := f.Layout.HostLyxLink(slug)
	isLink, err := fslink.IsLink(hostLink)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("fslink.IsLink(%s): %v", hostLink, err)
	}
	if isLink {
		t.Errorf("Add(%q) with adopt created host junction; want no junction (Add is dormant)", slug)
	}
}
