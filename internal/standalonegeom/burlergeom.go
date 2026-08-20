// burlergeom.go implements BurlerGeometry, the told-mode builder that constructs a
// burlerengine.Geometry from a standalone session's target directory and derived state directory.

package standalonegeom

import (
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
// Like WebsterGeometry, BurlerGeometry takes no hash8: no value in the struct is hash-derived.
func BurlerGeometry(target, stateDir string) burlerengine.Geometry {
	return burlerengine.Geometry{
		WorktreeRoot: target,
		AnchorPath:   stateDir,
	}
}
