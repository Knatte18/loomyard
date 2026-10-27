//go:build tier1

// reconcile_empty_anchor_integration_test.go pins Reconcile's refusal to wire a pair whose own lym-anchor marker is present but empty.
// lymcwd treats an empty marker as absent and answers the "." anchor.
// Wiring such a pair on a subpath-anchored hub laid a full SECOND junction set at the repo root beside the live set at the real anchor.
// The refusal is per pair: the other pairs are still reconciled.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.
// The hub is anchored at "backend" so the warp root, where a second set would land, holds no junctions of its own.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// pairResultFor returns the reconcile outcome of the warp worktree called slug, failing the test when the pass reported none.
func pairResultFor(t *testing.T, h *hubforge.Hub, res fabricengine.ReconcileResult, slug string) fabricengine.ReconcilePairResult {
	t.Helper()

	want := filepath.ToSlash(fabricengine.WorktreePath(h.Location, slug))
	for _, pair := range res.Pairs {
		if pair.CodeWorktree == want {
			return pair
		}
	}
	t.Fatalf("Reconcile reported no pair for %s; pairs = %+v", want, res.Pairs)
	return fabricengine.ReconcilePairResult{}
}

// TestReconcile_RefusesEmptyAnchorMarkerInsteadOfWiringAtTheRoot truncates one worktree's own marker and asserts that pair alone fails, naming the file and the way forward.
func TestReconcile_RefusesEmptyAnchorMarkerInsteadOfWiringAtTheRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// failing and healthy name the truncated worktree and the one that must still reconcile, the prime's name being the hub's own.
		failing func(h *hubforge.Hub) string
		healthy func(h *hubforge.Hub) string
		// moveRecord stands a move record on the failing weft instead of truncating its marker.
		moveRecord bool
	}{
		{"PrimeWeft", func(h *hubforge.Hub) string { return h.Location.WorktreeName }, func(*hubforge.Hub) string { return hubforge.TemplatePairSlug }, false},
		{"PairWeft", func(*hubforge.Hub) string { return hubforge.TemplatePairSlug }, func(h *hubforge.Hub) string { return h.Location.WorktreeName }, false},
		{"PairWeftWithMoveRecord", func(*hubforge.Hub) string { return hubforge.TemplatePairSlug }, func(h *hubforge.Hub) string { return h.Location.WorktreeName }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "backend", Pairs: []string{hubforge.TemplatePairSlug}})
			failing, healthy := tt.failing(h), tt.healthy(h)

			wantInError := []string{lymcwd.AnchorFileName, "git -C"}
			if tt.moveRecord {
				writeMoveRecord(t, fabricengine.RecordsWorktreePath(h.Location, failing), fabricengine.MoveRecord{A: "backend", B: "x", Kind: fabricengine.MoveKindMove, PID: 4242})
				wantInError = []string{"interrupted", "lym fabric anchor " + failing + " --set x"}
			} else {
				markerPath := filepath.Join(fabricengine.RecordsWorktreePath(h.Location, failing), lymcwd.AnchorFileName)
				if err := os.WriteFile(markerPath, []byte("   \n"), 0o644); err != nil {
					t.Fatalf("truncate %s: %v", markerPath, err)
				}
				truncatedAt, err := lymcwd.NamedWorktree(h.Path, failing)
				if err != nil {
					t.Fatalf("NamedWorktree(%s) after truncation: %v", failing, err)
				}
				if truncatedAt.AnchorRel != "." {
					t.Fatalf("AnchorRel after truncation = %q; want %q (the empty-is-absent fallback must stay)", truncatedAt.AnchorRel, ".")
				}
			}

			res, err := h.Topology.Reconcile(h.Location)
			if err != nil {
				t.Fatalf("Reconcile() = %v; want the pass to go on and report the pair", err)
			}

			failed := pairResultFor(t, h, res, failing)
			for _, want := range wantInError {
				if !strings.Contains(failed.Error, want) {
					t.Errorf("pair %s Error = %q; want it to contain %q", failing, failed.Error, want)
				}
			}
			if ok := pairResultFor(t, h, res, healthy); ok.Error != "" {
				t.Errorf("pair %s Error = %q; want the other pair reconciled", healthy, ok.Error)
			}

			warpRoot := fabricengine.WorktreePath(h.Location, failing)
			for _, name := range []string{lymdirs.LymDirName, lymdirs.DotLymDirName, fabricengine.BoardDirName} {
				rootLink := filepath.Join(warpRoot, name)
				if _, statErr := os.Lstat(rootLink); statErr == nil {
					t.Errorf("a second junction %s was wired at the warp worktree root; want none", rootLink)
				}
			}
			anchoredLink := filepath.Join(warpRoot, "backend", lymdirs.LymDirName)
			if _, statErr := os.Lstat(anchoredLink); statErr != nil {
				t.Errorf("anchored junction %s went missing: %v", anchoredLink, statErr)
			}
		})
	}
}
