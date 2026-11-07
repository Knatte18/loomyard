// webstergeom.go implements WebsterGeometry, the told-mode builder for a standalone session's websterengine.Geometry.
// It takes the session's target directory and state directory.

package standalonegeom

import (
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// WebsterGeometry builds a websterengine.Geometry for a standalone session.
// target is the already-absolute target directory, and stateDir is the state directory the caller derived from it.
//
// No value here is hash-derived, so the builder takes no hash8.
//
// WebsterDir, ReportsDir, ScratchDir and PromptsDir come from the websterengine accessors applied to stateDir.
// They sit at the same mirrored subpaths a hub uses.
//
// PlanDir and StencilsDir are defaults: the CLI may override either with a flag after this builder returns.
// SpecsDir has no such flag, so it is never overridden.
//
// VerifyDir is the verified-tree directory under stateDir, beside webster's other untracked state.
//
// GateConfig loads the gate config from the target worktree's own directory on each call.
// That file is the one the gate verb reads outside any hub.
// A worktree without one is refused with an error naming `lym config gate`.
//
// WorktreeRoot and PaneCwd are both target, never stateDir:
// target is the git repository an implementer works in and the directory its sessions run in.
func WebsterGeometry(target, stateDir string) websterengine.Geometry {
	return websterengine.Geometry{
		AnchorRoot:   stateDir,
		WorktreeRoot: target,
		PaneCwd:      target,
		WebsterDir:   websterengine.Dir(stateDir),
		ReportsDir:   websterengine.ReportsDir(stateDir),
		ScratchDir:   websterengine.ScratchDir(stateDir),
		PromptsDir:   websterengine.PromptsDir(stateDir),
		PlanDir:      planparser.PlanDir(stateDir),
		StencilsDir:  StencilsDir(stateDir),
		SpecsDir:     SpecsDir(stateDir),
		VerifyDir:    verifytree.Dir(stateDir),
		GateConfig: func() (gateslot.Config, error) {
			return gateslot.LoadConfig(configengine.Dirs{Anchor: target})
		},
	}
}
