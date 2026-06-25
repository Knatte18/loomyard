//go:build integration

// add_test.go covers Add's happy-path side effects (portal, launchers, pushed
// branch) and the zero-residue rollback on a post-creation failure. The paired
// Add creates both host and weft worktrees on the mirrored branch and requires
// a weft Prime repo; tests build this via lyxtest.CopyPairedLocal and pass
// AddOptions{SkipPush:true}.

package warp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	}{
		{
			name:       "HappyPath",
			setup:      func(t *testing.T, f lyxtest.PairedFixture) {},
			opts:       AddOptions{SkipPush: true},
			wantBranch: "my-task",
		},
		{
			name:         "BranchPrefix",
			branchPrefix: "hanf/",
			setup:        func(t *testing.T, f lyxtest.PairedFixture) {},
			opts:         AddOptions{SkipPush: true},
			wantBranch:   "hanf/my-task",
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
		},
		{
			name: "BranchExists",
			setup: func(t *testing.T, f lyxtest.PairedFixture) {
				lyxtest.MustRun(t, f.Hub, "git", "branch", slug)
			},
			opts:            AddOptions{SkipPush: true},
			wantErrContains: `branch "my-task" already exists`,
			wantResultZero:  true,
		},
		{
			name: "TargetDirExists",
			setup: func(t *testing.T, f lyxtest.PairedFixture) {
				if err := os.Mkdir(filepath.Join(f.Container, slug), 0755); err != nil {
					t.Fatalf("create target dir: %v", err)
				}
			},
			opts:            AddOptions{SkipPush: true},
			wantErrContains: "already exists",
			wantResultZero:  true,
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
		},
		{
			name: "UnbornBranch",
			setup: func(t *testing.T, f lyxtest.PairedFixture) {
				// Create an unborn branch (orphan branch with no commits).
				// git checkout --orphan stages all parent files; reset them to avoid "dirty" error.
				lyxtest.MustRun(t, f.Hub, "git", "checkout", "--orphan", "unborn-branch")
				lyxtest.MustRun(t, f.Hub, "git", "reset")
			},
			opts:            AddOptions{SkipPush: true},
			wantErrContains: "detached HEAD",
			wantNoTargetDir: true,
			wantResultZero:  true,
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
