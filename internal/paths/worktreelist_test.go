package paths_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/mhgo/internal/paths"
)

// TestList covers the porcelain parser: a fresh repo yields exactly the main
// worktree, and additional worktrees appear after it with Main=false.
func TestList(t *testing.T) {
	tests := []struct {
		name string
		// extraWorktrees is the number of additional worktrees created
		// alongside the main checkout before listing.
		extraWorktrees int
		verify         func(t *testing.T, hub string, entries []paths.WorktreeEntry)
	}{
		{
			name:           "SingleWorktree",
			extraWorktrees: 0,
			verify: func(t *testing.T, hub string, entries []paths.WorktreeEntry) {
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
			verify: func(t *testing.T, hub string, entries []paths.WorktreeEntry) {
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
			verify: func(t *testing.T, hub string, entries []paths.WorktreeEntry) {
				// This test is not meant to be called; it's handled in the
				// outer loop with a special case.
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Special handling for BareRepoRejection test case
			if tt.name == "BareRepoRejection" {
				container := t.TempDir()
				bareRepo := filepath.Join(container, "bare.git")
				mustRun(t, container, "git", "init", "--bare", bareRepo)

				entries, err := paths.List(bareRepo)
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

			hub := newTestRepo(t)
			for i := 0; i < tt.extraWorktrees; i++ {
				wtPath := filepath.Join(filepath.Dir(hub), fmt.Sprintf("wt%d", i+1))
				mustRun(t, hub, "git", "worktree", "add", wtPath)
			}

			entries, err := paths.List(hub)
			if err != nil {
				t.Fatalf("List() error = %v; want nil", err)
			}

			tt.verify(t, hub, entries)
		})
	}
}
