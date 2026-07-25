//go:build integration

// reconcile_stale_registration_test.go proves Reconcile repairs a weft
// worktree that was deleted by hand (plain rm, not `git worktree remove`),
// the drift shape that leaves a stale git worktree registration still
// claiming the weft branch. Pre-fix, `git worktree add` refused with
// "missing but already registered worktree" on every reconcile run, so the
// drift was permanently unrepairable by any fabric verb (a live review round
// confirmed this); Reconcile now prunes stale registrations before adopting.
//
// Package fabricengine_test to reuse the external-test-package fixture idiom
// of lifecycle_differential_test.go; shares the single TestMain in
// testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestReconcile_RecreatesHandDeletedWeftWorktree deletes a pair's weft
// worktree directory with plain os.RemoveAll (leaving the stale registration
// behind) and asserts one Reconcile run recreates it from the surviving
// branch with no per-pair error.
func TestReconcile_RecreatesHandDeletedWeftWorktree(t *testing.T) {
	t.Parallel()

	const slug = "stale-reg-recreate"
	fixture := lyxtest.CopyPairedLocal(t)
	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
		"fabric": fabricengine.ConfigTemplate(),
	})
	// Mirror CloneHub's post-clone state so Add's fork-from-parent logic has
	// the suffixed primary branch it expects.
	lyxtest.MustRun(t, fixture.WeftPrime, "git", "checkout", "-b", fabricengine.WeftBranchName("main"))

	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})
	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	// The drift injection: delete the weft worktree directory out from under
	// git, exactly as a stray rm would — the registration and branch survive.
	weftPath := l.WeftWorktreePath(slug)
	if err := os.RemoveAll(weftPath); err != nil {
		t.Fatalf("hand-delete weft worktree: %v", err)
	}

	result, err := topology.Reconcile(l)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// Find the pair for our slug and assert a clean recreate. Result paths are
	// emitted forward-slashed, so normalize the expectation the same way.
	var found bool
	for _, pair := range result.Pairs {
		if pair.WeftWorktree != filepath.ToSlash(weftPath) {
			continue
		}
		found = true
		if pair.Action != fabricengine.ReconcileActionWeftRecreated {
			t.Errorf("Action = %q; want %q", pair.Action, fabricengine.ReconcileActionWeftRecreated)
		}
		if pair.Error != "" {
			t.Errorf("Error = %q; want empty (stale registration must be pruned before adopt)", pair.Error)
		}
	}
	if !found {
		t.Fatalf("Reconcile result has no pair for slug %q: %+v", slug, result.Pairs)
	}

	// The weft worktree is back on disk, checked out on its suffixed branch.
	if info, err := os.Stat(weftPath); err != nil || !info.IsDir() {
		t.Fatalf("weft worktree not recreated at %s: %v", weftPath, err)
	}
	if got, want := currentBranchOf(t, weftPath), fabricengine.WeftBranchName(slug); got != want {
		t.Errorf("recreated weft worktree branch = %q; want %q", got, want)
	}
}
