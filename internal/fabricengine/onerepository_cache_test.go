// onerepository_cache_test.go covers the per-worktree layout answers oneRepositoryAt keeps for the process.
// It is an internal-package test because oneRepositoryAt and commitAnchorMarker are unexported.

package fabricengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestOneRepositoryAt_KeepsAnswerPerWorktree pins what is kept, what drops it and what is never kept.
// A kept answer survives a change made behind fabric's back, a marker commit through fabric drops it,
// and a failed read is read again on the next call.
// It reads the process-wide answer map that every marker commit clears, so it does not run in parallel.
func TestOneRepositoryAt_KeepsAnswerPerWorktree(t *testing.T) {
	tiergate.Require(t, 1)

	tests := []struct {
		name string
		// run makes its repository under hub and checks the answers it reads.
		run func(t *testing.T, hub string)
	}{
		{"KeptAcrossAnOutsideMarkerRemoval", func(t *testing.T, hub string) {
			prime := geometryRepository(t, hub, "prime")
			gitkit.CommitFile(t, prime, lymcwd.AnchorFileName, ".\n", "record the anchor")
			gitkit.Git(t, prime, "branch", BoltBranchName)
			if !oneRepositoryAt(prime) {
				t.Fatalf("oneRepositoryAt(%s) = false; want true", prime)
			}

			gitkit.Git(t, prime, "rm", "-q", lymcwd.AnchorFileName)
			gitkit.Git(t, prime, "commit", "-q", "-m", "remove the anchor")
			gitkit.Git(t, prime, "branch", "-q", "-D", BoltBranchName)

			if !oneRepositoryAt(prime) {
				t.Errorf("oneRepositoryAt(%s) after an outside marker removal = false; want the kept true", prime)
			}
		}},
		{"DroppedByAFabricMarkerCommit", func(t *testing.T, hub string) {
			prime := geometryRepository(t, hub, "prime")
			if oneRepositoryAt(prime) {
				t.Fatalf("oneRepositoryAt(%s) = true; want false", prime)
			}

			if _, err := commitAnchorMarker(NewMutations(hub), prime, ".", "record the anchor"); err != nil {
				t.Fatalf("commitAnchorMarker() error = %v; want nil", err)
			}

			if !oneRepositoryAt(prime) {
				t.Errorf("oneRepositoryAt(%s) after a fabric marker commit = false; want true", prime)
			}
		}},
		{"FailedReadNotKept", func(t *testing.T, hub string) {
			path := filepath.Join(hub, "repo")
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", path, err)
			}
			gitFile := filepath.Join(path, ".git")
			if err := os.WriteFile(gitFile, nil, 0o644); err != nil {
				t.Fatalf("write %s: %v", gitFile, err)
			}
			if oneRepositoryAt(path) {
				t.Fatalf("oneRepositoryAt(%s) over an empty .git file = true; want false", path)
			}

			if err := os.Remove(gitFile); err != nil {
				t.Fatalf("remove %s: %v", gitFile, err)
			}
			geometryRepository(t, hub, "repo")
			gitkit.CommitFile(t, path, lymcwd.AnchorFileName, ".\n", "record the anchor")

			if !oneRepositoryAt(path) {
				t.Errorf("oneRepositoryAt(%s) after the failed read was repaired = false; want true", path)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t, t.TempDir())
		})
	}
}
