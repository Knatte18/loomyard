// weavergeom.go implements WeaverGeometry, which builds a weaverengine.Geometry for a hub-mode *lymcwd.Location.

package hubgeom

import (
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

// WeaverGeometry builds a weaverengine.Geometry for l from the paths l already resolved.
// It reads l's accessors and passes them through unchanged.
// Weaver's own accessors name the run, reports, scratch and prompts directories.
// It performs no cwd resolution of its own.
// ParentName is the parent's name; when the parent cannot be resolved, a warning is logged and it stays empty.
func WeaverGeometry(l *lymcwd.Location) weaverengine.Geometry {
	anchorPath := l.AnchorPath()
	return weaverengine.Geometry{
		AnchorRoot:         anchorPath,
		WorktreeRoot:       l.WorktreePath(),
		PaneCwd:            anchorPath,
		RunDir:             weaverengine.Dir(anchorPath),
		ReportsDir:         weaverengine.ReportsDir(anchorPath),
		ScratchDir:         weaverengine.ScratchDir(anchorPath),
		PromptsDir:         weaverengine.PromptsDir(anchorPath),
		StencilsDir:        fabricengine.StencilsDir(l.HubPath),
		SpecsDir:           fabricengine.SpecsDir(l.HubPath),
		PlanDir:            planparser.PlanDir(anchorPath),
		VerifyDir:          verifytree.Dir(anchorPath),
		GateSlots:          GateSlots(l),
		GateWaitDir:        gateslot.WaitDir(anchorPath),
		GateConfig:         func() (gateslot.Config, error) { return gateslot.LoadConfig(fabricengine.ConfigDirs(l)) },
		ParentName:         parentNameOrEmpty(l, "weaver prompts"),
		DecisionRecordPath: loomengine.DiscussionDecisionRecord(l),
	}
}
