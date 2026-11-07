// cleanup_primary_integration_test.go pins the carve-out that keeps `lym fabric cleanup` from
// deleting the repo's primary weft branch.
// Branch-space liveness does not protect that branch once the prime warp worktree is off the default branch.
// `--apply --force` would then delete it along with any weft commit only it carries.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCleanup_ProtectsPrimaryWeftBranchAfterCheckout moves the prime pair off the default branch.
// The test starts from a real Bolt checkout worktree on the warp's default branch.
// It asserts cleanup reports the primary weft branch as protected and leaves it on disk, even under --apply --force.
func TestCleanup_ProtectsPrimaryWeftBranchAfterCheckout(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	primaryWeftBranch := fabricengine.RecordsBranchName("main")

	// Move the prime pair off the default branch, exactly as `lym fabric checkout` does.
	// The Bolt checkout worktree stays on "main", which records the repo's primary warp branch.
	gitkit.MustRun(t, l.WorktreePath(), "git", "checkout", "-b", "alt")
	gitkit.MustRun(t, h.PrimeRecords(), "git", "checkout", "-b", fabricengine.RecordsBranchName("alt"))

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
}

// TestCleanup_RefusesWhenPrimaryWeftBranchIsUndeterminable asserts that a hub gets no orphan sweep at all.
// The case covered is a hub whose board worktree is not a readable git checkout.
// Cleanup's deletions are irreversible, so an undeterminable primary branch must fail closed.
func TestCleanup_RefusesWhenPrimaryWeftBranchIsUndeterminable(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	if err := os.RemoveAll(fabricengine.BoardDir(h.Location.HubPath)); err != nil {
		t.Fatalf("remove board worktree: %v", err)
	}

	if _, err := h.Topology.Cleanup(h.Location, true, true, false); err == nil {
		t.Fatal("Cleanup() = nil error; want a refusal when the primary weft branch cannot be determined")
	}
}
