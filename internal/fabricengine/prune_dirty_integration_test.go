// prune_dirty_integration_test.go pins Prune's dirty-weft gate.
// Prune removes a stale pair's weft worktree with `git worktree remove --force`.
// That discards uncommitted tracked changes without a trace, so the gate refuses the worktree unless --force is given.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestPrune_ProtectsDirtyWeftWorktreeUntilForced builds a stale pair.
// Its weft worktree holds an uncommitted tracked change.
// It asserts the dry run and the apply run both protect it, the content survives --apply, and --force removes it.
//
//testtiming:keep uncommitted tracked weft work surviving prune --apply until --force; coverage of its blocks by other tests does not show an assertion of this
func TestPrune_ProtectsDirtyWeftWorktreeUntilForced(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "prune-dirty"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	weftPath := fabricengine.RecordsWorktreePath(l, slug)
	tracked := filepath.Join(weftPath, "tracked.md")
	gitkit.CommitFile(t, weftPath, "tracked.md", "committed\n", "seed tracked file")

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
