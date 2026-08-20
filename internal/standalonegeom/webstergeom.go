// webstergeom.go implements WebsterGeometry, the told-mode builder that constructs a
// websterengine.Geometry from a standalone session's target directory and derived state
// directory.

package standalonegeom

import (
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// WebsterGeometry builds a websterengine.Geometry for a standalone session, given the
// already-absolute target directory and the stateDir the caller already derived via
// standalonestate.Derive(target).
//
// Unlike ReedGeometry, WebsterGeometry takes no hash8: none of webster's eight values is
// hash-derived, so no unused parameter is added for symmetry with the reed builder.
//
// WebsterDir, ReportsDir, ScratchDir and PromptsDir come from the four websterengine accessors
// applied to stateDir, so standalone's _lyx/webster and .lyx/webster pair are ordinary directory
// siblings under <state>, at the same mirrored subpaths a hub uses.
//
// PlanDir and StencilsDir are the defaults only: the CLI argument boundary may override either
// from a flag after calling this builder, and this package knows nothing about flags. StencilsDir's
// default comes from StencilsDir(stateDir), the sole construction site for the standalone stencils
// directory.
//
// WorktreeRoot is target, never stateDir: it is the fork-audit workdir and the {{.worktree_root}}
// token's value in standalone, since target is the git repository an implementer's work happens
// in.
func WebsterGeometry(target, stateDir string) websterengine.Geometry {
	return websterengine.Geometry{
		AnchorRoot:   stateDir,
		WorktreeRoot: target,
		WebsterDir:   websterengine.Dir(stateDir),
		ReportsDir:   websterengine.ReportsDir(stateDir),
		ScratchDir:   websterengine.ScratchDir(stateDir),
		PromptsDir:   websterengine.PromptsDir(stateDir),
		PlanDir:      planparser.PlanDir(stateDir),
		StencilsDir:  StencilsDir(stateDir),
	}
}
