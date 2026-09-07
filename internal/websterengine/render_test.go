// render_test.go covers the plan-directory display split the standalone Master fix introduced
// (crucible round fable5-high-r3, F-A4): hub geometry keeps its byte-exact relative "_lyx/plan"
// spelling, standalone geometry gets the absolute state-directory spelling, and the card pointers
// re-root onto whichever display applies.

package websterengine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
)

func TestMasterPlanDirDisplay(t *testing.T) {
	// The base is always the PANE's cwd: hub geometry's pane runs at the anchor the plan junction
	// hangs off, standalone geometry's pane runs at the target repo while the plan lives in the
	// derived state directory — outside the pane cwd by construction.
	paneCwd := filepath.Join(string(filepath.Separator), "hub", "worktree")

	t.Run("HubGeometryRendersRelative", func(t *testing.T) {
		got := masterPlanDirDisplay(paneCwd, filepath.Join(paneCwd, "_lyx", "plan"))
		if got != "_lyx/plan" {
			t.Errorf("masterPlanDirDisplay(hub) = %q; want %q — the hub prompt's bytes must not change", got, "_lyx/plan")
		}
	})

	t.Run("StandaloneGeometryRendersAbsolute", func(t *testing.T) {
		planDir := filepath.Join(string(filepath.Separator), "state", "lyx", "abcd1234", "_lyx", "plan")
		got := masterPlanDirDisplay(paneCwd, planDir)
		if got != planDir {
			t.Errorf("masterPlanDirDisplay(standalone) = %q; want the absolute plan dir %q — no relative spelling reaches it from the pane's cwd", got, planDir)
		}
	})
}

func TestRenderCardPointers_ReRootsOntoPlanDirDisplay(t *testing.T) {
	cards := []planparser.Card{{SourcePath: "_lyx/plan/01-alpha.md"}, {SourcePath: "_lyx/plan/02-beta.md"}}

	if got, want := renderCardPointers(cards, "_lyx/plan"), "- `_lyx/plan/01-alpha.md`\n- `_lyx/plan/02-beta.md`"; got != want {
		t.Errorf("renderCardPointers(hub display) = %q; want %q (byte-identical to the old verbatim form)", got, want)
	}
	abs := filepath.Join(string(filepath.Separator), "state", "plan")
	if got, want := renderCardPointers(cards, abs), "- `"+abs+"/01-alpha.md`\n- `"+abs+"/02-beta.md`"; got != want {
		t.Errorf("renderCardPointers(standalone display) = %q; want %q", got, want)
	}
}

// TestRenderProgress_NilBatchStateIsSkippedNotPanicked covers the round-4 review's R4-15. A
// state.json carrying an explicit null for a batch key parses to a present-but-nil entry; every
// other reader of this map already guards it, and this one panicked INSIDE Run's Master prompt
// render, taking the run down rather than surfacing a diagnosable error.
func TestRenderProgress_NilBatchStateIsSkippedNotPanicked(t *testing.T) {
	batches := []batcher.Batch{
		{Cards: []planparser.Card{{Number: 1, Slug: "one"}}},
		{Cards: []planparser.Card{{Number: 2, Slug: "two"}}},
	}
	st := &State{Batches: map[int]*BatchState{
		1: nil,
		2: {Slug: "two", Terminal: true, Status: DigestStatusDone},
	}}

	got := RenderProgress(batches, st)
	if strings.Contains(got, "01-one") {
		t.Errorf("RenderProgress() = %q; want the nil entry skipped rather than reported", got)
	}
	if !strings.Contains(got, "02-two") {
		t.Errorf("RenderProgress() = %q; want the well-formed terminal batch still reported", got)
	}
}
