//go:build tier1

// sourcedirty_integration_test.go covers SourceDirtyPaths and the typed refusal Add returns for a dirty source worktree.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestSourceDirtyPaths pins that a clean source lists nothing, a modified tracked file is listed and refuses Add as *ErrSourceDirty, and an untracked file alone neither lists nor refuses.
func TestSourceDirtyPaths(t *testing.T) {
	t.Parallel()
	const tracked = "tracked.txt"

	tests := []struct {
		name      string
		dirty     func(t *testing.T, prime string)
		wantPaths []string
	}{
		{"CleanSource", func(t *testing.T, prime string) {}, nil},
		{"ModifiedTrackedFile", func(t *testing.T, prime string) {
			if err := os.WriteFile(filepath.Join(prime, tracked), []byte("changed\n"), 0o644); err != nil {
				t.Fatalf("modify %s: %v", tracked, err)
			}
		}, []string{tracked}},
		{"UntrackedFileAlone", func(t *testing.T, prime string) {
			if err := os.WriteFile(filepath.Join(prime, "untracked.txt"), []byte("new\n"), 0o644); err != nil {
				t.Fatalf("write untracked file: %v", err)
			}
		}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			gitkit.CommitFile(t, h.PrimeWorktree(), tracked, "original\n", "add tracked file")
			tt.dirty(t, h.PrimeWorktree())

			paths, err := fabricengine.SourceDirtyPaths(h.Location)
			if err != nil {
				t.Fatalf("SourceDirtyPaths: %v", err)
			}
			if !slices.Equal(paths, tt.wantPaths) {
				t.Errorf("SourceDirtyPaths() = %v; want %v", paths, tt.wantPaths)
			}

			_, addErr := h.Topology.Add(h.Location, "dirtyprobe", fabricengine.AddOptions{SkipPush: true})
			var refusal *fabricengine.ErrSourceDirty
			if len(tt.wantPaths) == 0 {
				if errors.As(addErr, &refusal) {
					t.Fatalf("Add refused a source with no tracked changes: %v", addErr)
				}
				return
			}
			if !errors.As(addErr, &refusal) {
				t.Fatalf("Add error = %v; want *ErrSourceDirty", addErr)
			}
			if !slices.Equal(refusal.Paths, tt.wantPaths) {
				t.Errorf("ErrSourceDirty.Paths = %v; want %v", refusal.Paths, tt.wantPaths)
			}
		})
	}
}
