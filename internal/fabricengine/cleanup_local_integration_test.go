//go:build integration

// cleanup_local_integration_test.go covers Cleanup over a local pair's orphan weft branch, which is neither archived nor deleted on origin.
// The hub's origin URLs point at a path that does not exist, so any origin step fails the call.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestCleanup_LocalOrphanPushesNothing removes a local pair's worktrees by hand and asserts a dry run reports its weft branch as Local, and apply deletes it locally with no archive tag and no origin call.
func TestCleanup_LocalOrphanPushesNothing(t *testing.T) {
	t.Parallel()

	const slug = "localorphan"
	weftBranch := fabricengine.RecordsBranchName(slug)
	h := addLocalPairOnDeadOrigin(t, slug)
	removePairWorktreesByHand(t, h, slug)

	findEntry := func(t *testing.T, res fabricengine.CleanupResult) fabricengine.CleanupBranchEntry {
		t.Helper()

		for _, entry := range res.Entries {
			if entry.Branch == weftBranch {
				return entry
			}
		}
		t.Fatalf("Cleanup entries = %+v; want one for %q", res.Entries, weftBranch)
		return fabricengine.CleanupBranchEntry{}
	}

	dry, err := h.Topology.Cleanup(h.Location, false, false, true)
	if err != nil {
		t.Fatalf("Cleanup dry run: %v", err)
	}
	if entry := findEntry(t, dry); !entry.Local || entry.Deleted || entry.Protected {
		t.Errorf("dry-run entry = %+v; want Local, not Deleted, not Protected", entry)
	}
	if !gitkit.BranchExists(t, h.PrimeRecords(), weftBranch) {
		t.Fatalf("weft branch %q gone after a dry run", weftBranch)
	}

	applied, err := h.Topology.Cleanup(h.Location, true, false, true)
	if err != nil {
		t.Fatalf("Cleanup apply: %v", err)
	}
	entry := findEntry(t, applied)
	if !entry.Local || !entry.Deleted || entry.ArchiveTag != "" || entry.RemoteError != "" || entry.RemoteDeleted || entry.Error != "" {
		t.Errorf("applied entry = %+v; want Local and Deleted with no archive tag, remote outcome or error", entry)
	}
	if tags := strings.TrimSpace(gitkit.Git(t, h.PrimeRecords(), "tag", "-l", "archive/*")); tags != "" {
		t.Errorf("archive tags in the weft repo = %q; want none", tags)
	}
	if gitkit.BranchExists(t, h.PrimeRecords(), weftBranch) {
		t.Errorf("weft branch %q still exists after apply", weftBranch)
	}
}
