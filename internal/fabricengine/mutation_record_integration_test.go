// mutation_record_integration_test.go asserts the mutation record at the engine boundary.
// An Add that fails partway and runs rollbackAdd is the mutated-then-errored case.
// It reads the record through res.Mutated().Entries(), the surface a CLI or this package's live-state harness uses.
//
// The file is in package fabricengine_test and shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// refuseRecordsCommits points l's records repository at a hooks directory whose pre-commit hook refuses every commit;
// so Add fails at its origin-record commit, the last step that can fail after the pair is wired.
func refuseRecordsCommits(t *testing.T, l *lymcwd.Location) {
	t.Helper()

	hooks := t.TempDir()
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\necho refused by test >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write pre-commit hook: %v", err)
	}
	gitkit.MustRun(t, mustRecordsRepoRoot(t, l), "git", "config", "core.hooksPath", hooks)
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
