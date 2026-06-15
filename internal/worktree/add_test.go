// add_test.go covers Add's happy-path side effects (portal, launchers, pushed
// branch) and the zero-residue rollback on a post-creation failure.

package worktree_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/mhgo/internal/git"
	"github.com/Knatte18/mhgo/internal/paths"
	"github.com/Knatte18/mhgo/internal/worktree"
)

// TestAdd covers the worktree creation flow: the happy path, branch-prefix
// application, and each precondition failure (dirty source, existing branch,
// existing target dir, missing remote).
func TestAdd(t *testing.T) {
	const slug = "my-task"

	tests := []struct {
		name         string
		branchPrefix string
		// setup performs scenario-specific prep on top of the fresh repo
		// returned by newTestRepo (e.g. adding a remote or dirtying the tree).
		setup           func(t *testing.T, hub string)
		wantBranch      string
		wantErrContains string
		// wantNoTargetDir asserts the sibling worktree dir was NOT created,
		// proving the precondition tripped before `git worktree add`.
		wantNoTargetDir bool
	}{
		{
			name:       "HappyPath",
			setup:      func(t *testing.T, hub string) { addRemote(t, hub) },
			wantBranch: "my-task",
		},
		{
			name:         "BranchPrefix",
			branchPrefix: "hanf/",
			setup:        func(t *testing.T, hub string) { addRemote(t, hub) },
			wantBranch:   "hanf/my-task",
		},
		{
			name: "DirtySource",
			setup: func(t *testing.T, hub string) {
				addRemote(t, hub)
				// Modify a tracked file without committing so the clean check fails.
				if err := os.WriteFile(filepath.Join(hub, "README"), []byte("modified"), 0644); err != nil {
					t.Fatalf("modify README: %v", err)
				}
			},
			wantErrContains: "source worktree has uncommitted changes",
			wantNoTargetDir: true,
		},
		{
			name: "BranchExists",
			setup: func(t *testing.T, hub string) {
				addRemote(t, hub)
				mustRun(t, hub, "git", "branch", slug)
			},
			wantErrContains: `branch "my-task" already exists`,
		},
		{
			name: "TargetDirExists",
			setup: func(t *testing.T, hub string) {
				addRemote(t, hub)
				if err := os.Mkdir(filepath.Join(filepath.Dir(hub), slug), 0755); err != nil {
					t.Fatalf("create target dir: %v", err)
				}
			},
			wantErrContains: "already exists",
		},
		{
			name:            "NoRemote",
			setup:           func(t *testing.T, hub string) {}, // intentionally no remote
			wantErrContains: "no remote configured",
			wantNoTargetDir: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := newTestRepo(t)
			tt.setup(t, hub)

			// Resolve Layout from the hub
			l, err := paths.Resolve(hub)
			if err != nil {
				t.Fatalf("paths.Resolve(%q): %v", hub, err)
			}

			w := worktree.New(worktree.Config{BranchPrefix: tt.branchPrefix})
			result, err := w.Add(l, slug)

			target := l.WorktreePath(slug)

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
			if !result.Pushed {
				t.Errorf("Add(%q).Pushed = false; want true", slug)
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
// no _launchers/<slug>/, and the bare remote receives no new branch.
func TestAddRollback(t *testing.T) {
	const slug = "rollback-test"
	hub := newTestRepo(t)
	addRemote(t, hub)

	// Resolve Layout
	l, err := paths.Resolve(hub)
	if err != nil {
		t.Fatalf("paths.Resolve(%q): %v", hub, err)
	}

	// Pre-create a regular file at the portal location to trip createPortal's refuse-to-clobber
	portalLink := filepath.Join(l.PortalsDir(), slug)
	if err := os.MkdirAll(filepath.Dir(portalLink), 0755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(portalLink, []byte("blocker"), 0644); err != nil {
		t.Fatalf("create blocker file: %v", err)
	}

	w := worktree.New(worktree.Config{})
	result, err := w.Add(l, slug)

	if err == nil {
		t.Fatalf("Add(%q) should have failed; got nil error", slug)
	}
	if !strings.Contains(err.Error(), "already exists") && !strings.Contains(err.Error(), "already exists — remove it first") {
		t.Errorf("Add(%q) error should mention 'already exists'; got %q", slug, err.Error())
	}

	// Assert ZERO residue

	// 1. No worktree dir
	target := l.WorktreePath(slug)
	// Note: the worktree dir may have been created but should be removed by rollback
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		// On Windows, the junction creation failure may happen before or after git worktree add
		// We expect it to be cleaned up by rollback. If the dir exists, the rollback failed.
		if statErr == nil {
			t.Errorf("Add(%q) rollback failed: worktree dir still exists at %q", slug, target)
		}
	}

	// 2. No local branch
	_, _, exitCode, _ := git.RunGit([]string{"rev-parse", "--verify", "refs/heads/" + slug}, l.WorktreeRoot)
	if exitCode == 0 {
		t.Errorf("Add(%q) rollback failed: local branch %q still exists", slug, slug)
	}

	// 3. No _launchers/<slug>/
	launcherDir := l.LauncherDir(slug)
	if _, statErr := os.Stat(launcherDir); !os.IsNotExist(statErr) {
		t.Errorf("Add(%q) rollback failed: launcher dir still exists at %q", slug, launcherDir)
	}

	// 4. No new branch on bare remote
	// Check that the remote doesn't have the branch
	_, _, exitCode, _ = git.RunGit([]string{"ls-remote", "--heads", "origin", slug}, l.WorktreeRoot)
	// ls-remote returns 0 even if branch doesn't exist (it just returns empty output)
	// Instead, check if we can see the branch in ls-remote output
	stdout, _, _, _ := git.RunGit([]string{"ls-remote", "origin"}, l.WorktreeRoot)
	if strings.Contains(stdout, slug) {
		t.Errorf("Add(%q) rollback failed: branch pushed to remote", slug)
	}

	if result.Slug != "" {
		t.Errorf("Add(%q) result should be zero on error; got non-empty result", slug)
	}
}
