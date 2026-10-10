//go:build integration

// mutation_record_integration_test.go asserts the mutation record at the engine boundary: a Remove refused for a dirty worktree records nothing,
// and an Add that fails partway and runs rollbackAdd is the mutated-then-errored case.
// Both are read through res.Mutated().Entries(), exercising the exported surface a CLI or this
// package's own live-state harness consumer actually has.
//
// Package fabricengine_test to reuse the
// broken-origin-remote failure injection gitkit.MustRun/TestAdd_GitFailureCarriesGitsOwnReason
// already established (add_rollback_adopt_test.go); shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestMutationRecord_RemoveDirtyWarpRefusalRecordsNothing covers the ordering this slice polices: remove.go runs its dirty pre-flight before its archive and every teardown, so a correctly-refusing Remove has mutated nothing and its record is empty.
// The pre-flight's own error is a bare fmt.Errorf, never a *destructiveRefusal — RefusalOf(err) must report false, asserted as an absence, not a set of contents.
func TestMutationRecord_RemoveDirtyWarpRefusalRecordsNothing(t *testing.T) {
	t.Parallel()

	const slug = "dirty-warp-remove"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	// An untracked file is enough: Remove's pre-flight dirty check reads scopeAll (tracked and
	// untracked alike), so this alone trips the "worktree has uncommitted changes; use --force"
	// refusal without needing a staged or committed change.
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

// TestMutationRecord_AddRollbackOrdersCreationBeforeItsOwnDestruction forces Add to fail after its
// worktree-creation steps have already landed, and asserts the returned record carries the
// creations and the rollback's own destructions IN EXECUTION ORDER — a worktree_created entry before
// the worktree_removed that undoes it, not merely both present. Array order is the only thing
// carrying ordering in this vocabulary, so a set-membership assertion would prove only half the
// contract.
func TestMutationRecord_AddRollbackOrdersCreationBeforeItsOwnDestruction(t *testing.T) {
	t.Parallel()

	const slug = "add-rollback-order"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location

	// Break the warp origin remote so the push at the very end of Add fails after every earlier
	// step — warp and weft worktree creation, junction wiring — has already landed, forcing
	// rollbackAdd to run and undo them. The same injection add_rollback_adopt_test.go's
	// TestAdd_GitFailureCarriesGitsOwnReason and TestAddRollback_UnwiresJunctionsOnPostWiringFailure
	// already establish.
	gitkit.MustRun(t, l.WorktreePath(), "git", "remote", "set-url", "origin",
		filepath.Join(t.TempDir(), "no-such-remote.git"))

	topology := h.Topology
	// SkipPush skips Add's pre-flight probes of origin, which would otherwise refuse on the broken URL before any mutation;
	// step 11's warp push ignores SkipPush, so it still fails after creation.
	res, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true})
	if err == nil {
		t.Fatalf("Add(%q) = nil error; want the broken-origin push failure", slug)
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
