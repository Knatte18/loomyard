// webstergeom.go implements WebsterGeometry, which builds a websterengine.Geometry for a hub-mode *lymcwd.Location.

package hubgeom

import (
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// WebsterGeometry builds a websterengine.Geometry for l from the paths l already resolved.
// It reads l's accessors and passes them through unchanged.
// It performs no cwd resolution of its own: internal/lymcwd owns cwd resolution.
// This function only reads the paths its caller resolved.
// WorktreeRoot is l.WorktreePath(), the git worktree that git calls open and card paths resolve against.
// PaneCwd is l.AnchorPath(), the directory webster's sessions run in.
// The two differ when the hub's anchor sits in a subdirectory of the worktree.
// ParentName is the parent's name; when the parent cannot be resolved, a warning is logged and it stays empty.
// DecisionRecordPath is the location's decision record, where the task's declared PATTERN changes are read from.
// GateConfig loads the hub's gate config over the hub's config dirs at each call.
func WebsterGeometry(l *lymcwd.Location) websterengine.Geometry {
	anchorPath := l.AnchorPath()
	return websterengine.Geometry{
		AnchorRoot:         anchorPath,
		WorktreeRoot:       l.WorktreePath(),
		PaneCwd:            anchorPath,
		WebsterDir:         websterengine.Dir(anchorPath),
		ReportsDir:         websterengine.ReportsDir(anchorPath),
		ScratchDir:         websterengine.ScratchDir(anchorPath),
		PromptsDir:         websterengine.PromptsDir(anchorPath),
		StencilsDir:        fabricengine.StencilsDir(l.HubPath),
		SpecsDir:           fabricengine.SpecsDir(l.HubPath),
		PlanDir:            planparser.PlanDir(anchorPath),
		VerifyDir:          verifytree.Dir(anchorPath),
		GateSlots:          GateSlots(l),
		GateWaitDir:        gateslot.WaitDir(anchorPath),
		GateConfig:         func() (gateslot.Config, error) { return gateslot.LoadConfig(fabricengine.ConfigDirs(l)) },
		ParentName:         parentNameOrEmpty(l, "webster prompts"),
		DecisionRecordPath: loomengine.DiscussionDecisionRecord(l),
	}
}
