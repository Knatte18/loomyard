// cleanup_primary_integration_test.go pins the carve-out that keeps `lym fabric cleanup` from
// deleting the repo's primary weft branch.
// Branch-space liveness does not protect that branch once the prime warp worktree is off the default branch.
// `--apply --force` would then delete it along with any weft commit only it carries.
// Each test runs over a two-repository hub and a one-repository hub, whose primary comes from different sources.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

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

// TestCleanup_ProtectsPrimaryWeftBranchAfterCheckout moves the prime pair off the default branch.
// The test starts from a real hub whose primary weft branch is the default branch plus "-weft".
// It asserts cleanup reports the primary weft branch as protected and leaves it on disk, even under --apply --force.
//
//testtiming:keep the primary weft branch and its commits surviving cleanup --apply --force after a checkout moved the prime pair; coverage of its blocks by other tests does not show an assertion of this
func TestCleanup_ProtectsPrimaryWeftBranchAfterCheckout(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name          string
		build         func(t *testing.T) *hubforge.Hub
		defaultBranch string
		// moveOffDefault moves the prime pair off defaultBranch.
		moveOffDefault func(t *testing.T, h *hubforge.Hub)
	}{
		{
			name:          "TwoRepository",
			build:         func(t *testing.T) *hubforge.Hub { return hubforge.CopyHub(t, hubforge.Shape{Anchor: "."}) },
			defaultBranch: "main",
			moveOffDefault: func(t *testing.T, h *hubforge.Hub) {
				// Exactly as `lym fabric checkout` does.
				// The Bolt checkout worktree stays on "main", which records the repo's primary warp branch.
				gitkit.MustRun(t, h.Location.WorktreePath(), "git", "checkout", "-b", "alt")
				gitkit.MustRun(t, h.PrimeRecords(), "git", "checkout", "-b", fabricengine.RecordsBranchName("alt"))
			},
		},
		{
			name: "OneRepository",
			build: func(t *testing.T) *hubforge.Hub {
				return hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{DefaultBranch: "master"})
			},
			defaultBranch: "master",
			moveOffDefault: func(t *testing.T, h *hubforge.Hub) {
				gitkit.MustRun(t, h.Location.WorktreePath(), "git", "branch", "alt")
				if _, err := h.Topology.Checkout(h.Location, "alt"); err != nil {
					t.Fatalf("Checkout(alt) error = %v", err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := tt.build(t)
			l := h.Location
			primaryWeftBranch := fabricengine.RecordsBranchName(tt.defaultBranch)

			tt.moveOffDefault(t, h)

			result, err := h.Topology.Cleanup(l, true, true, false)
			if err != nil {
				t.Fatalf("Cleanup(apply=true, force=true) error = %v", err)
			}

			var seen bool
			for _, entry := range result.Entries {
				if entry.Branch != primaryWeftBranch {
					continue
				}
				seen = true
				if entry.Deleted {
					t.Errorf("Cleanup deleted the primary weft branch %q", primaryWeftBranch)
				}
				if !entry.Protected {
					t.Errorf("Cleanup entry for %q has Protected = false; want the primary weft branch protected", primaryWeftBranch)
				}
			}
			if !seen {
				t.Fatalf("Cleanup reported no entry for %q; want it reported and protected", primaryWeftBranch)
			}

			if !gitkit.BranchExists(t, mustRecordsRepoRoot(t, l), primaryWeftBranch) {
				t.Fatalf("primary weft branch %q no longer exists after cleanup --apply --force", primaryWeftBranch)
			}
		})
	}
}

// TestCleanup_RefusesWhenPrimaryWeftBranchIsUndeterminable asserts that a hub gets no orphan sweep at all.
// The cases covered are a two-repository hub whose board worktree is not a readable git checkout,
// and a one-repository hub whose records store holds no prime key or cannot be read.
// Cleanup's deletions are irreversible, so an undeterminable primary branch must fail closed.
func TestCleanup_RefusesWhenPrimaryWeftBranchIsUndeterminable(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name  string
		build func(t *testing.T) *hubforge.Hub
		// spoil makes the primary undeterminable, given the hub and its records store's path.
		spoil func(t *testing.T, h *hubforge.Hub, recordsDir string)
		// wantErrorFormats are substrings of the refusal, each a format with the records store's path as its one operand.
		wantErrorFormats []string
	}{
		{
			name:  "TwoRepositoryBoardGone",
			build: func(t *testing.T) *hubforge.Hub { return hubforge.CopyHub(t, hubforge.Shape{Anchor: "."}) },
			spoil: func(t *testing.T, h *hubforge.Hub, recordsDir string) {
				if err := os.RemoveAll(fabricengine.BoardDir(h.Location.HubPath)); err != nil {
					t.Fatalf("remove board worktree: %v", err)
				}
			},
		},
		{
			name:  "OneRepositoryKeyUnset",
			build: func(t *testing.T) *hubforge.Hub { return hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{}) },
			spoil: func(t *testing.T, h *hubforge.Hub, recordsDir string) {
				gitkit.Git(t, recordsDir, "config", "--unset", fabricengine.PrimeRecordsBranchKey)
			},
			wantErrorFormats: []string{"git -C %s config " + fabricengine.PrimeRecordsBranchKey},
		},
		{
			name:  "OneRepositoryConfigUnparsable",
			build: func(t *testing.T) *hubforge.Hub { return hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{}) },
			spoil: func(t *testing.T, h *hubforge.Hub, recordsDir string) {
				if err := os.WriteFile(filepath.Join(recordsDir, ".git", "config"), []byte("[core\nthis is not a config file\n"), 0o644); err != nil {
					t.Fatalf("overwrite the records store's config: %v", err)
				}
			},
			wantErrorFormats: []string{"cannot read the records store at %s"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := tt.build(t)
			recordsDir := mustRecordsRepoRoot(t, h.Location)
			tt.spoil(t, h, recordsDir)

			_, err := h.Topology.Cleanup(h.Location, true, true, false)
			if err == nil {
				t.Fatal("Cleanup() = nil error; want a refusal when the primary weft branch cannot be determined")
			}
			for _, format := range tt.wantErrorFormats {
				want := fmt.Sprintf(format, recordsDir)
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Cleanup() error = %q; want it to contain %q", err.Error(), want)
				}
			}
		})
	}
}
