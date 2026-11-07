// junction_repoint_test.go tests WireJunctions repairs a warp junction that dangles or points at the wrong target.
// Such a junction is repaired through seedLymJunction, not refused as a real pre-existing directory.
// Each case runs against both the _lym junction and the non-_lym junction.
// Warp's seedLymJunction does not repair these drift shapes, so no differential harness covers them.
//
// The file is in package fabricengine_test,
// and it uses the external-test-package fixture idiom of lifecycle_differential_test.go.
// It shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWireJunctions_RepairsCorruptedJunctions points a warp junction at an unrelated real directory.
// The test also points a junction at a target that does not exist, once for each of the two junctions.
// It asserts WireJunctions removes and recreates each at the correct weft target.
// The junction must not be refused as pre-existing user content.
// The steps share one hub and run serially in table order.
// Each step's WireJunctions call leaves every junction healthy, so the next step starts from a repaired hub.
func TestWireJunctions_RepairsCorruptedJunctions(t *testing.T) {
	tiergate.Require(t, 1)
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
			name:          "lym_wrong_target",
			link:          fabricengine.CodeLymLink(l, slug),
			correctTarget: fabricengine.WeftLymDirFor(l, slug),
			corruptTarget: func(t *testing.T) string { return realDirectory(t, "not-the-weft-lym-dir") },
		},
		{
			name:          "extra_wrong_target",
			link:          extraLink,
			correctTarget: extraTarget,
			corruptTarget: func(t *testing.T) string { return realDirectory(t, "not-the-weft-extra-dir") },
		},
		{
			name:          "lym_dangling",
			link:          fabricengine.CodeLymLink(l, slug),
			correctTarget: fabricengine.WeftLymDirFor(l, slug),
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

			if err := fabricengine.WireJunctions(l, slug, []string{"_lym", "_extra"}); err != nil {
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
