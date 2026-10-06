// render_test.go covers the plan-directory display split the standalone Master fix introduced
// (crucible round fable5-high-r3, F-A4).

package websterengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// TestPlanDirDisplay proves hub geometry keeps the plan directory's byte-exact relative "_lyx/plan"
// spelling, standalone geometry gets the absolute plan directory no relative spelling reaches from
// the pane's cwd, and the card pointers re-root onto whichever display applies.
//
//testtiming:keep pins both geometries' plan-directory display and the card pointers re-rooted onto it byte for byte; the covering begin-batch and render tests run hub geometry only
func TestPlanDirDisplay(t *testing.T) {
	t.Parallel()

	// The base is always the PANE's cwd: hub geometry's pane runs at the anchor the plan junction
	// hangs off, standalone geometry's pane runs at the target repo while the plan lives in the
	// derived state directory — outside the pane cwd by construction.
	paneCwd := filepath.Join(string(filepath.Separator), "hub", "worktree")
	standalone := filepath.Join(string(filepath.Separator), "state", "lyx", "abcd1234", "_lyx", "plan")
	cards := []planparser.Card{{SourcePath: "_lyx/plan/01-alpha.md"}, {SourcePath: "_lyx/plan/02-beta.md"}}

	tests := []struct {
		name        string
		planDir     string
		wantDisplay string
	}{
		{name: "hub geometry renders relative", planDir: filepath.Join(paneCwd, "_lyx", "plan"), wantDisplay: "_lyx/plan"},
		{name: "standalone geometry renders absolute", planDir: standalone, wantDisplay: standalone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			display := masterPlanDirDisplay(paneCwd, tt.planDir)
			if display != tt.wantDisplay {
				t.Fatalf("masterPlanDirDisplay() = %q; want %q", display, tt.wantDisplay)
			}
			want := "- `" + tt.wantDisplay + "/01-alpha.md`\n- `" + tt.wantDisplay + "/02-beta.md`"
			if got := renderCardPointers(cards, display); got != want {
				t.Errorf("renderCardPointers() = %q; want %q", got, want)
			}
		})
	}
}
