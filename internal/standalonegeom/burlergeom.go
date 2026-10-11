// burlergeom.go implements BurlerGeometry, the told-mode builder that constructs a
// burlerengine.Geometry from a standalone session's target directory and derived state directory.

package standalonegeom

import (
	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/burlerengine"
)

// BurlerGeometry builds a burlerengine.Geometry for a standalone session anchored at target, an
// already-absolute standalone target directory, given the stateDir the caller already derived via
// standalonestate.Derive(target).
//
// WorktreeRoot and AnchorPath deliberately diverge: WorktreeRoot is target because
// burlerengine's (*Profile).validate resolves Target.Paths, Fasit.Paths, ReviewPath,
// FixerReportPath and both prior-report lists against it, so it must be the directory the operator
// asked burler to review; AnchorPath is stateDir because it is the base burler's .lyx/burler
// instruction directory joins onto, and pointing it at target is exactly what would push a hidden
// .lyx tree into the reviewed folder.
//
// Shortname is derived from the session's hash8 and Slug stays empty, matching ReedGeometry, so the agent names burler forms match the names reed gives the strands.
func BurlerGeometry(target, stateDir, hash8 string) burlerengine.Geometry {
	return burlerengine.Geometry{
		WorktreeRoot: target,
		AnchorPath:   stateDir,
		RepoRoot:     target,
		Shortname:    agentname.StandaloneShortname(hash8),
	}
}
