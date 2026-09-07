// render_test.go covers the plan-directory display split the standalone Master fix introduced
// (crucible round fable5-high-r3, F-A4): hub geometry keeps its byte-exact relative "_lyx/plan"
// spelling, standalone geometry gets the absolute state-directory spelling, and the card pointers
// re-root onto whichever display applies.

package websterengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

func TestMasterPlanDirDisplay(t *testing.T) {
	anchor := filepath.Join(string(filepath.Separator), "hub", "worktree")

	t.Run("HubGeometryRendersRelative", func(t *testing.T) {
		got := masterPlanDirDisplay(anchor, filepath.Join(anchor, "_lyx", "plan"))
		if got != "_lyx/plan" {
			t.Errorf("masterPlanDirDisplay(hub) = %q; want %q — the hub prompt's bytes must not change", got, "_lyx/plan")
		}
	})

	t.Run("StandaloneGeometryRendersAbsolute", func(t *testing.T) {
		planDir := filepath.Join(string(filepath.Separator), "state", "lyx", "abcd1234", "_lyx", "plan")
		got := masterPlanDirDisplay(anchor, planDir)
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
