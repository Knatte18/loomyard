package worktree_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

			w := worktree.New(worktree.Config{BranchPrefix: tt.branchPrefix})
			result, err := w.Add(hub, slug)

			target := filepath.Join(filepath.Dir(hub), slug)

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
