//go:build integration

// worktreelist_test.go covers the porcelain worktree-list parser, including
// the bare-repo rejection and Main-on-first-entry behavior.

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
)

// TestList covers the porcelain parser: a fresh repo yields exactly the main worktree,
// and additional worktrees appear after it with Main=false.
func TestList(t *testing.T) {
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
				// git may emit forward slashes on all platforms; normalize
				// before comparing the main entry against the hub path.
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
				// This test is not meant to be called; it's handled in the
				// outer loop with a special case.
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Special handling for BareRepoRejection test case
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

			h := hubforge.NewHub(t, ".")
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

// TestList_ParsesPrunable covers parseWorktreePorcelain's Prunable branch through the public List
// entry point: a worktree deleted without `git worktree prune` reports Prunable == true, while the
// hub's own prime entry — never deleted — reports Prunable == false.
func TestList_ParsesPrunable(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
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

// TestList_NotAGitRepo asserts that calling List against a directory that is not inside any git
// repository fails with an error that carries BOTH local context (the source directory and git's
// exit code) AND git's own explanation.
//
// The earlier rule here was that git's stderr must never appear. That rule left the operator with a
// bare exit number as the sole account of a git failure — a live round watched two simultaneous
// `lyx fabric add` calls report "failed (git exit 255)" with git's actual reason discarded — while
// nineteen other RunGit sites in this same package already printed stderr. Local context alone is
// not a diagnosis;
// the context is what tells the operator WHERE, and git's stderr is what tells them WHY.
func TestList_NotAGitRepo(t *testing.T) {
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
