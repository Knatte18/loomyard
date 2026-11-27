// weavergeom_test.go pins WeaverGeometry against the weaverengine accessors and the other packages' joins.
// The fixture keeps the hub, the worktree root and the anchor path in three distinct directories,
// so a field mix-up inside WeaverGeometry surfaces instead of passing silently.

package hubgeom

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

func TestWeaverGeometry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name      string
		anchorRel string
	}{
		{"subpath-anchored fixture", filepath.Join("sub", "dir")},
		{"unanchored fixture", "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hub := filepath.Join(t.TempDir(), "some-hub-LYMHUB")
			worktreeName := "some-worktree"
			worktreeRoot := filepath.Join(hub, worktreeName)
			anchorPath := filepath.Join(worktreeRoot, tt.anchorRel)
			l := &lymcwd.Location{
				RepoName:     "distinct-repo-name",
				HubPath:      hub,
				WorktreeName: worktreeName,
				AnchorRel:    tt.anchorRel,
			}

			got := WeaverGeometry(l)

			checks := []struct {
				field     string
				got, want string
			}{
				{"AnchorRoot", got.AnchorRoot, anchorPath},
				{"WorktreeRoot", got.WorktreeRoot, worktreeRoot},
				{"PaneCwd", got.PaneCwd, anchorPath},
				{"RunDir", got.RunDir, weaverengine.Dir(anchorPath)},
				{"ReportsDir", got.ReportsDir, weaverengine.ReportsDir(anchorPath)},
				{"ScratchDir", got.ScratchDir, weaverengine.ScratchDir(anchorPath)},
				{"PromptsDir", got.PromptsDir, weaverengine.PromptsDir(anchorPath)},
				{"StencilsDir", got.StencilsDir, fabriclayout.StencilsDir(hub)},
				{"SpecsDir", got.SpecsDir, fabriclayout.SpecsDir(hub)},
				{"PlanDir", got.PlanDir, planparser.PlanDir(anchorPath)},
				{"VerifyDir", got.VerifyDir, verifytree.Dir(anchorPath)},
				{"GateWaitDir", got.GateWaitDir, gateslot.WaitDir(anchorPath)},
				{"DecisionRecordPath", got.DecisionRecordPath, loomengine.DiscussionDecisionRecord(l)},
			}
			for _, c := range checks {
				if c.got != c.want {
					t.Errorf("WeaverGeometry(l).%s = %q; want %q", c.field, c.got, c.want)
				}
			}
			if got.GateSlots == nil {
				t.Fatal("WeaverGeometry(l).GateSlots = nil; want the hub's pool")
			}
			if want := gateslot.Dir(fabriclayout.BoardDir(hub)); got.GateSlots.Dir != want {
				t.Errorf("WeaverGeometry(l).GateSlots.Dir = %q; want %q", got.GateSlots.Dir, want)
			}
			if got.GateConfig == nil {
				t.Error("WeaverGeometry(l).GateConfig = nil; want the hub's gate config reader")
			}
		})
	}
}
