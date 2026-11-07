// worktreelist_test.go covers the porcelain worktree-list parser.
// It covers the bare-repo rejection and the Main-on-first-entry behavior.

package fabricengine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestList covers the porcelain parser: a fresh repo yields exactly the main worktree,
// and additional worktrees appear after it with Main=false.
func TestList(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// extraWorktrees is the number of additional worktrees created
		// alongside the main checkout before listing.
		extraWorktrees int
		verify         func(t *testing.T, hub string, entries []fabricengine.WorktreeEntry)
	}{
		{
			name:           "SingleWorktree",
			extraWorktrees: 0,
			verify: func(t *testing.T, hub string, entries []fabricengine.WorktreeEntry) {
				if len(entries) != 1 {
					t.Fatalf("List() len = %d; want 1", len(entries))
				}
				e := entries[0]
				if !e.Main {
					t.Errorf("entries[0].Main = false; want true")
				}
				if e.Branch != "main" {
					t.Errorf("entries[0].Branch = %q; want %q", e.Branch, "main")
				}
				if e.Head == "" {
					t.Error(`entries[0].Head = ""; want non-empty`)
				}
			},
		},
		{
			name:           "TwoWorktrees",
			extraWorktrees: 1,
			verify: func(t *testing.T, hub string, entries []fabricengine.WorktreeEntry) {
				if len(entries) != 2 {
					t.Fatalf("List() len = %d; want 2", len(entries))
				}
				if !entries[0].Main {
					t.Errorf("entries[0].Main = false; want true")
				}
				// git may emit forward slashes on any platform.
				// The main entry's path is normalized before it is compared with the hub path.
				gotPath := filepath.FromSlash(entries[0].Path)
				if gotPath != hub {
					t.Errorf("entries[0].Path = %q; want %q", gotPath, hub)
				}
				if entries[1].Main {
					t.Errorf("entries[1].Main = true; want false")
				}
			},
		},
		{
			name:           "BareRepoRejection",
			extraWorktrees: 0,
			verify: func(t *testing.T, hub string, entries []fabricengine.WorktreeEntry) {
				// The outer loop handles the BareRepoRejection case itself;
				// verify is never called for it.
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// The BareRepoRejection case runs against a bare repository it creates itself.
			if tt.name == "BareRepoRejection" {
				bareRepo := filepath.Join(t.TempDir(), "bare.git")
				gitkit.MustRun(t, t.TempDir(), "git", "init", "--bare", bareRepo)

				entries, err := fabricengine.List(bareRepo)
				if err == nil {
					t.Fatalf("List() error = nil; want error containing 'bare'")
				}
				if !strings.Contains(err.Error(), "bare") {
					t.Errorf("List() error = %q; want error containing 'bare'", err.Error())
				}
				if entries != nil {
					t.Errorf("List() entries = %v; want nil", entries)
				}
				return
			}

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			hub := h.PrimeWorktree()

			for i := 0; i < tt.extraWorktrees; i++ {
				wtPath := filepath.Join(filepath.Dir(hub), fmt.Sprintf("wt%d", i+1))
				gitkit.MustRun(t, hub, "git", "worktree", "add", wtPath)
			}

			entries, err := fabricengine.List(hub)
			if err != nil {
				t.Fatalf("List() error = %v; want nil", err)
			}

			tt.verify(t, hub, entries)
		})
	}
}

// TestList_ParsesPrunable covers the Prunable field that List reports for a deleted worktree.
// A worktree deleted without `git worktree prune` reports Prunable == true.
// The hub's prime entry, which is never deleted, reports Prunable == false.
//
//testtiming:keep List reporting a deleted-but-unpruned worktree as Prunable and the prime as not; coverage of its blocks by other tests does not show an assertion of this
func TestList_ParsesPrunable(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hub := h.PrimeWorktree()

	wtPath := filepath.Join(filepath.Dir(hub), "wt-prunable")
	gitkit.MustRun(t, hub, "git", "worktree", "add", wtPath)
	if err := os.RemoveAll(wtPath); err != nil {
		t.Fatalf("RemoveAll(%q): %v", wtPath, err)
	}

	entries, err := fabricengine.List(hub)
	if err != nil {
		t.Fatalf("List() error = %v; want nil", err)
	}

	var prunableCount int
	var primePrunable bool
	for _, e := range entries {
		if e.Prunable {
			prunableCount++
		}
		if e.Main {
			primePrunable = e.Prunable
		}
	}
	if prunableCount != 1 {
		t.Errorf("List() prunable entry count = %d; want 1", prunableCount)
	}
	if primePrunable {
		t.Error("List() prime entry Prunable = true; want false")
	}
}

// TestList_NotAGitRepo asserts that List fails on a directory outside any git repository.
// The error carries local context (the source directory and git's exit code) and git's own explanation.
// The local context says where the failure happened, and git's own explanation says why.
//
//testtiming:keep List outside any git repository failing with both the local context and git's own explanation; coverage of its blocks by other tests does not show an assertion of this
func TestList_NotAGitRepo(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	notARepo := t.TempDir()

	entries, err := fabricengine.List(notARepo)
	if err == nil {
		t.Fatalf("List(%q) error = nil; want error (not a git repository)", notARepo)
	}
	wantSubstr := fmt.Sprintf("%q", notARepo)
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("List(%q) error = %q; want substring %q (source dir)", notARepo, err.Error(), wantSubstr)
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("List(%q) error = %q; want git's own explanation included, not just an exit code",
			notARepo, err.Error())
	}
	if entries != nil {
		t.Errorf("List(%q) entries = %v; want nil", notARepo, entries)
	}
}
