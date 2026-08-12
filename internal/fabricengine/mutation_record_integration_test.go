//go:build integration

// mutation_record_integration_test.go asserts the mutation record survives the two headline
// mutated-then-errored paths at the engine boundary: a Remove that destroys the portal and launchers
// before its own dirty pre-flight refuses, and an Add that fails partway and runs rollbackAdd.
// Both are read through res.Mutated().Entries(), exercising the exported surface a CLI or fabrictest
// consumer actually has.
//
// Package fabricengine_test to reuse newFabricFixture (reconcile_stale_registration_test.go) and the
// broken-origin-remote failure injection lyxtest.MustRun/TestAdd_GitFailureCarriesGitsOwnReason
// already established (add_rollback_adopt_test.go); shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestMutationRecord_RemoveDirtyWarpRefusalCarriesThePortalAndLauncherDeletions covers the pre-flight
// ordering defect this slice exists to police: remove.go runs removePortal and removeLaunchers BEFORE
// its dirty pre-flight, so a correctly-refusing Remove has already destroyed both by the time it
// returns an error. The returned record must still name both deletions, and the pre-flight's own
// error is a bare fmt.Errorf, never a *destructiveRefusal — RefusalOf(err) must report false, asserted
// as an absence, not a set of contents.
func TestMutationRecord_RemoveDirtyWarpRefusalCarriesThePortalAndLauncherDeletions(t *testing.T) {
	t.Parallel()

	const slug = "dirty-warp-remove"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})

	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add(%q): %v", slug, err)
	}

	// An untracked file is enough: Remove's pre-flight dirty check reads scopeAll (tracked and
	// untracked alike), so this alone trips the "worktree has uncommitted changes; use --force"
	// refusal without needing a staged or committed change.
	target := fabricengine.WorktreePath(l, slug)
	if err := os.WriteFile(filepath.Join(target, "uncommitted.txt"), []byte("dirt\n"), 0o644); err != nil {
		t.Fatalf("dirty the warp worktree at %s: %v", target, err)
	}

	res, err := topology.Remove(l, slug, false)
	if err == nil {
		t.Fatalf("Remove(%q, force=false) = nil error; want the dirty-warp pre-flight refusal", slug)
	}
	if _, isRefusal := fabricengine.RefusalOf(err); isRefusal {
		t.Errorf("RefusalOf(err) reported true; want false — the dirty pre-flight returns a bare error, never a *destructiveRefusal")
	}

	entries := res.Mutated().Entries()
	var sawLinkRemoved, sawPathRemoved bool
	for _, entry := range entries {
		switch entry.Kind {
		case fabricengine.KindLinkRemoved:
			sawLinkRemoved = true
		case fabricengine.KindPathRemoved:
			sawPathRemoved = true
		}
	}
	if !sawLinkRemoved {
		t.Errorf("record %+v has no %s entry; want the portal removePortal already performed", entries, fabricengine.KindLinkRemoved)
	}
	if !sawPathRemoved {
		t.Errorf("record %+v has no %s entry; want the launcher teardown removeLaunchers already performed", entries, fabricengine.KindPathRemoved)
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
	fixture := newFabricFixture(t)
	l := fixture.Layout

	// Break the warp origin remote so the push at the very end of Add fails after every earlier
	// step — warp and weft worktree creation, junction wiring — has already landed, forcing
	// rollbackAdd to run and undo them. The same injection add_rollback_adopt_test.go's
	// TestAdd_GitFailureCarriesGitsOwnReason and TestAddRollback_UnwiresJunctionsOnPostWiringFailure
	// already establish.
	lyxtest.MustRun(t, l.WorktreePath(), "git", "remote", "set-url", "origin",
		filepath.Join(t.TempDir(), "no-such-remote.git"))

	topology := fabricengine.NewTopology(fabricengine.Config{})
	res, err := topology.Add(l, slug, fabricengine.AddOptions{})
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
