// mutation_record_integration_test.go asserts the mutation record at the engine boundary.
// A Remove refused for a dirty worktree records nothing.
// An Add that fails partway and runs rollbackAdd is the mutated-then-errored case.
// Both read the record through res.Mutated().Entries(), the surface a CLI or this package's live-state harness uses.
//
// The file is in package fabricengine_test and reuses the refuseRecordsCommits failure injection.
// The injection itself lives in add_rollback_adopt_test.go.
// It shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestMutationRecord_RemoveDirtyWarpRefusalRecordsNothing covers the ordering that Remove keeps:
// its dirty pre-flight in remove.go runs before the archive and every teardown,
// so a Remove refused for a dirty worktree has mutated nothing and its record is empty.
// The pre-flight error is a bare fmt.Errorf, never a *destructiveRefusal, so RefusalOf(err) reports false.
func TestMutationRecord_RemoveDirtyWarpRefusalRecordsNothing(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "dirty-warp-remove"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	// An untracked file is enough: Remove's pre-flight dirty check reads scopeAll (tracked and untracked alike),
	// so this alone trips the "worktree has uncommitted changes; use --force" refusal.
	// A staged or committed change is not needed.
	target := fabricengine.WorktreePath(l, slug)
	if err := os.WriteFile(filepath.Join(target, "uncommitted.txt"), []byte("dirt\n"), 0o644); err != nil {
		t.Fatalf("dirty the warp worktree at %s: %v", target, err)
	}

	res, err := topology.Remove(l, slug, false, false)
	if err == nil {
		t.Fatalf("Remove(%q, force=false) = nil error; want the dirty-warp pre-flight refusal", slug)
	}
	if _, isRefusal := fabricengine.RefusalOf(err); isRefusal {
		t.Errorf("RefusalOf(err) reported true; want false — the dirty pre-flight returns a bare error, never a *destructiveRefusal")
	}

	if n := res.Mutated().Len(); n != 0 {
		t.Errorf("refused Remove recorded %d mutations (%+v); want 0", n, res.Mutated().Entries())
	}
}

// TestMutationRecord_AddRollbackOrdersCreationBeforeItsOwnDestruction forces Add to fail after worktree creation.
// It asserts that the returned record carries the creations and the rollback's own destructions in execution order:
// a worktree_created entry before the worktree_removed that undoes it, not merely both present.
// Array order is the only thing carrying ordering in this vocabulary,
// so a set-membership assertion would prove only half the contract.
func TestMutationRecord_AddRollbackOrdersCreationBeforeItsOwnDestruction(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "add-rollback-order"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	// Refuse every records commit, so the origin-record commit fails after every earlier step has landed:
	// warp and weft worktree creation and junction wiring.
	// That forces rollbackAdd to run and undo them.
	// The same injection is used by TestAddRollback_UnwiresJunctionsOnPostWiringFailure in add_rollback_adopt_test.go.
	refuseRecordsCommits(t, l)

	topology := h.Topology
	res, err := topology.Add(l, slug, fabricengine.AddOptions{})
	if err == nil {
		t.Fatalf("Add(%q) = nil error; want the refused records commit", slug)
	}

	entries := res.Mutated().Entries()
	createdIdx, removedIdx := -1, -1
	for i, entry := range entries {
		if entry.Kind == fabricengine.KindWorktreeCreated && createdIdx == -1 {
			createdIdx = i
		}
		if entry.Kind == fabricengine.KindWorktreeRemoved && removedIdx == -1 {
			removedIdx = i
		}
	}
	if createdIdx == -1 {
		t.Fatalf("record %+v has no %s entry; want Add's own worktree creation recorded", entries, fabricengine.KindWorktreeCreated)
	}
	if removedIdx == -1 {
		t.Fatalf("record %+v has no %s entry; want rollbackAdd's own destruction recorded", entries, fabricengine.KindWorktreeRemoved)
	}
	if createdIdx > removedIdx {
		t.Errorf("record has %s at index %d before %s at index %d; want the creation to precede its own rollback destruction: %+v",
			fabricengine.KindWorktreeRemoved, removedIdx, fabricengine.KindWorktreeCreated, createdIdx, entries)
	}
}
