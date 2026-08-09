//go:build integration

// prune_dirty_integration_test.go pins Prune's dirty-weft gate.
// `prune --apply` removes a stale pair's weft worktree with `git worktree remove --force`, which
// discards uncommitted tracked changes with no trace; before the gate, it did so with no dirty
// check and no --force flag of its own, while `remove` refused the identical state.
//
// Package fabricengine_test to reuse newFabricFixture from
// reconcile_stale_registration_test.go; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestPrune_ProtectsDirtyWeftWorktreeUntilForced builds a stale pair whose weft worktree carries an
// uncommitted tracked change, then asserts the dry run and the apply run agree that it is protected,
// the content survives --apply, and --force removes it.
func TestPrune_ProtectsDirtyWeftWorktreeUntilForced(t *testing.T) {
	t.Parallel()

	const slug = "prune-dirty"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})

	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	weftPath := fabricengine.WeftWorktreePath(l, slug)
	tracked := filepath.Join(weftPath, "tracked.md")
	if err := os.WriteFile(tracked, []byte("committed\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", tracked, err)
	}
	lyxtest.MustRun(t, weftPath, "git", "add", "tracked.md")
	lyxtest.MustRun(t, weftPath, "git", "commit", "-m", "seed tracked file")

	// The uncommitted work a forced removal would discard.
	const sentinel = "PRUNE-SENTINEL"
	if err := os.WriteFile(tracked, []byte("committed\n"+sentinel+"\n"), 0o644); err != nil {
		t.Fatalf("dirty %s: %v", tracked, err)
	}

	// Make the pair stale: the warp worktree directory disappears, the registration stays.
	if err := os.RemoveAll(fabricengine.WorktreePath(l, slug)); err != nil {
		t.Fatalf("hand-delete warp worktree: %v", err)
	}

	dry, err := topology.Prune(l, false, false)
	if err != nil {
		t.Fatalf("Prune(dry) error = %v", err)
	}
	dryEntry := findPruneEntryByWeftPath(t, dry.Entries, weftPath)
	if !dryEntry.Protected {
		t.Errorf("dry run Protected = false for a dirty weft worktree; want true")
	}

	applied, err := topology.Prune(l, true, false)
	if err != nil {
		t.Fatalf("Prune(apply) error = %v", err)
	}
	appliedEntry := findPruneEntryByWeftPath(t, applied.Entries, weftPath)
	if !appliedEntry.Protected {
		t.Errorf("apply Protected = false; want the dry-run verdict to match")
	}
	if appliedEntry.Removed {
		t.Errorf("apply Removed = true for a protected entry; want false")
	}
	if !strings.Contains(appliedEntry.Error, "--force") {
		t.Errorf("apply Error = %q; want it to name --force as the way through", appliedEntry.Error)
	}

	content, readErr := os.ReadFile(tracked)
	if readErr != nil {
		t.Fatalf("uncommitted weft work destroyed by prune --apply: %v", readErr)
	}
	if !strings.Contains(string(content), sentinel) {
		t.Fatalf("uncommitted weft work lost: %q no longer contains %q", content, sentinel)
	}

	forced, err := topology.Prune(l, true, true)
	if err != nil {
		t.Fatalf("Prune(apply, force) error = %v", err)
	}
	forcedEntry := findPruneEntryByWeftPath(t, forced.Entries, weftPath)
	if !forcedEntry.Removed {
		t.Errorf("apply+force Removed = false; want the dirty weft worktree removed on an explicit --force")
	}
}
