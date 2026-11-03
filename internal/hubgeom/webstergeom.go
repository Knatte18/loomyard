// webstergeom.go implements WebsterGeometry, the hub-mode teller that converts a resolved *lymcwd.Location into a websterengine.Geometry.

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

// WebsterGeometry builds a websterengine.Geometry for l: the resolved Location's paths, read off
// its accessors and passed through untouched.
// It performs no cwd resolution of its own — internal/lymcwd stays the sole owner of cwd resolution
// (the Cwd Resolution Invariant), and WebsterGeometry only reads what l's caller already resolved.
// WorktreeRoot is l.WorktreePath(), the git worktree every git call opens and every card path resolves against;
// PaneCwd is l.AnchorPath(), where webster's sessions run, as ReedGeometry's PaneCwd is.
// The two differ in a hub whose anchor sits in a subdirectory.
// ParentName is what ResolveParent returns; an unresolvable parent logs a warning and leaves it empty, as reedGeometry does.
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
