// config_driven_junctions_integration_test.go proves two junction-wiring behaviors.
// First, WireJunctions and UnwireJunctions wire and unwire exactly the names a caller passes to them,
// so a future module can add its own junction name to the pathspec without a fabric or lymcwd change.
// Second, a worktree with a narrower pathspec than the default ("_lym" only) is still reported healthy by Healthy().
// A narrow pathspec is a legitimate, unenforced state, not a drift shape.
//
// It mirrors the imports and the hubforge.CopyHub fixture pattern of junction_pattern_integration_test.go.
// It shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWireJunctions_WiresEveryPassedName checks that wiring and unwiring cover exactly the names passed in.
// The wired names are a set including `_extra` and `_other`, neither in the default pathspec nor hub-reserved.
// No config is seeded, since WireJunctions reads none.
// It shows that a pathspec extended by one token is wired with no fabric/lymcwd code change:
// a caller that sources an extended pathspec passes its names exactly this way.
//
//testtiming:keep WireJunctions and UnwireJunctions wiring exactly the name set they are given, including names outside the default pathspec; coverage of its blocks by other tests does not show an assertion of this
func TestWireJunctions_WiresEveryPassedName(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	l := h.Location
	slug := l.WorktreeName
	names := []string{"_lym", "_other", "_extra"}

	if err := fabricengine.WireJunctions(l, slug, names); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	junctions := fabricengine.WarpJunctions(l, slug, names)
	for _, j := range junctions {
		isLink, err := fslink.IsLink(j.Link)
		if err != nil || !isLink {
			t.Errorf("junction %s (%s) is not a link: isLink=%v err=%v", j.Name, j.Link, isLink, err)
		}
		if info, err := os.Stat(j.Target); err != nil || !info.IsDir() {
			t.Errorf("weft target for %s (%s) not materialised: stat err=%v", j.Name, j.Target, err)
		}
	}

	lines := gitkit.ExcludeLines(t, fabricengine.WorktreePath(l, slug))
	for _, name := range names {
		pattern := fabricengine.ExcludePatternForTest(l.AnchorRel, name)
		if !containsLine(lines, pattern) {
			t.Errorf(".git/info/exclude does not contain %q after WireJunctions: %v", pattern, lines)
		}
	}

	result, err := fabricengine.UnwireJunctions(l, slug, names)
	if err != nil {
		t.Fatalf("UnwireJunctions: %v", err)
	}
	if !slices.Equal(result.JunctionsRemoved, names) {
		t.Errorf("JunctionsRemoved = %v; want %v", result.JunctionsRemoved, names)
	}

	for _, j := range junctions {
		if _, statErr := os.Lstat(j.Link); !os.IsNotExist(statErr) {
			t.Errorf("junction %s (%s) still exists after UnwireJunctions", j.Name, j.Link)
		}
	}
}

// TestHealthy_NarrowPathspecIsHealthy checks that a pathspec naming neither structural directory reads as healthy.
// Healthy loads its junction names from the repo-wide fabric.yaml and unions them with both structural directory sets,
// so a worktree with only `_lym` and `.lym` wired is still reported in sync.
// Such a pathspec is a legitimate, unenforced reality, not a drift shape Healthy should flag.
//
// Healthy checks weft-branch correspondence before the junction loop.
// The hub from `hubforge.CopyHub` already has the weft primary checked out on the suffixed branch,
// so this test needs no explicit checkout.
//
//testtiming:keep a redundant narrow pathspec reading healthy; coverage of its blocks by other tests does not show an assertion of this
func TestHealthy_NarrowPathspecIsHealthy(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	// Healthy reads the repo-wide pathspec, so the test overrides it with one naming neither structural directory;
	// `_lym` is then expected only through the structural directories.
	hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: _other\n")

	l := h.Location
	slug := l.WorktreeName

	// The wired names are both structural directories, `_lym` and `.lym`.
	// They also include the pathspec names that survive hub-reserved filtering, here `_other`.
	// The config names neither structural directory.
	if err := fabricengine.WireJunctions(l, slug, []string{"_lym", ".lym", "_other"}); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	ok, reason, err := fabricengine.Healthy(l)
	if err != nil {
		t.Fatalf("Healthy: %v", err)
	}
	if !ok {
		t.Errorf("Healthy ok = false (reason %+v); want true with _lym wired structurally even though the config names neither structural directory", reason)
	}
	if reason.Cause == fabricengine.CauseConfigLoadFailed {
		t.Errorf("Healthy reason = %+v; want a real verdict, not CauseConfigLoadFailed", reason)
	}
}
