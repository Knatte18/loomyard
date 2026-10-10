//go:build integration

// junction_repoint_test.go proves WireJunctions (via seedLyxJunction) repairs
// a corrupted warp _lyx junction — one that dangles, or resolves to the wrong
// target — instead of refusing it as a real pre-existing directory. A live
// review round found the pre-fix code fell through the "predates weft"
// refusal for both drift shapes, and Reconcile's documented contract
// ("handles both the missing-junction and the wrong-target cases") was false
// as a result; only the missing-junction case had test coverage
// (TestReconcile_DifferentialEquivalence/JunctionRepointed). This file adds
// the two drift shapes that were unrepairable pre-fix. Fabric-only: warp's
// seedLyxJunction has the same unfixed gap, so a differential harness would
// diverge here.
//
// From card 15 onward, seedLyxJunction's per-junction repair loop runs over
// two junctions (_lyx and a second, non-_lyx junction), so this file also
// carries the non-_lyx counterparts of both repoint cases: the per-junction
// refusal/repair behaviour must hold identically for the second, non-_lyx
// junction, which has no _lyx-shaped shortcut to fall back on.
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
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestWireJunctions_RepairsCorruptedJunctions points a warp junction at an unrelated (but real)
// directory, and at a target that does not exist, once for the _lyx junction and once for the
// second, non-_lyx junction, then asserts WireJunctions removes and recreates each at the correct
// weft target instead of refusing it as pre-existing user content.
// The steps share one hub and run serially in table order: each step's WireJunctions call leaves
// every junction healthy again, so the next step starts from a repaired hub.
func TestWireJunctions_RepairsCorruptedJunctions(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	slug := l.WorktreeName

	extraLink := filepath.Join(fabricengine.WorktreePath(l, slug), l.AnchorRel, "_extra")
	extraTarget := filepath.Join(fabricengine.RecordsWorktreePath(l, slug), l.AnchorRel, "_extra")

	realDirectory := func(t *testing.T, name string) string {
		t.Helper()
		dir := filepath.Join(t.TempDir(), name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir wrong target: %v", err)
		}
		return dir
	}
	missingDirectory := func(t *testing.T, name string) string {
		t.Helper()
		return filepath.Join(t.TempDir(), name)
	}

	tests := []struct {
		name          string
		link          string
		correctTarget string
		corruptTarget func(t *testing.T) string
	}{
		{
			name:          "lyx_wrong_target",
			link:          fabricengine.CodeLyxLink(l, slug),
			correctTarget: fabricengine.WeftLyxDirFor(l, slug),
			corruptTarget: func(t *testing.T) string { return realDirectory(t, "not-the-weft-lyx-dir") },
		},
		{
			name:          "extra_wrong_target",
			link:          extraLink,
			correctTarget: extraTarget,
			corruptTarget: func(t *testing.T) string { return realDirectory(t, "not-the-weft-extra-dir") },
		},
		{
			name:          "lyx_dangling",
			link:          fabricengine.CodeLyxLink(l, slug),
			correctTarget: fabricengine.WeftLyxDirFor(l, slug),
			corruptTarget: func(t *testing.T) string { return missingDirectory(t, "does-not-exist") },
		},
		{
			name:          "extra_dangling",
			link:          extraLink,
			correctTarget: extraTarget,
			corruptTarget: func(t *testing.T) string { return missingDirectory(t, "does-not-exist-extra") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.RemoveAll(tt.link); err != nil {
				t.Fatalf("remove existing junction: %v", err)
			}
			if err := fslink.CreateDirLink(tt.link, tt.corruptTarget(t)); err != nil {
				t.Fatalf("seed corrupted junction: %v", err)
			}

			if err := fabricengine.WireJunctions(l, slug, []string{"_lyx", "_extra"}); err != nil {
				t.Fatalf("WireJunctions: %v", err)
			}

			isLink, err := fslink.IsLink(tt.link)
			if err != nil || !isLink {
				t.Fatalf("junction at %s is not a link after WireJunctions: isLink=%v err=%v", tt.link, isLink, err)
			}
			resolved, err := fslink.PointsTo(tt.link)
			if err != nil {
				t.Fatalf("PointsTo(%s): %v", tt.link, err)
			}
			wantResolved, err := filepath.EvalSymlinks(tt.correctTarget)
			if err != nil {
				t.Fatalf("EvalSymlinks(%s): %v", tt.correctTarget, err)
			}
			if resolved != wantResolved {
				t.Errorf("junction resolves to %s; want %s", resolved, wantResolved)
			}
		})
	}
}
