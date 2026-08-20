// perchgeom.go implements PerchGeometry, the told-mode builder that constructs a
// perchengine.Geometry from a standalone session's target directory and derived state directory.

package standalonegeom

import (
	"github.com/Knatte18/loomyard/internal/perchengine"
)

// PerchGeometry builds a perchengine.Geometry for a standalone session anchored at target, an
// already-absolute standalone target directory, given the stateDir the caller already derived via
// standalonestate.Derive(target).
//
// GateDir and AnchorPath deliberately diverge: GateDir is target because it is the gate command's
// working directory, the value Engine.Run stamps onto treadleengine.Profile.GateDir, and perch's
// field is named GateDir rather than WorktreeRoot precisely because that is its only use of that
// root; AnchorPath is stateDir because it is the base perchengine.RunsDir and
// perchengine.ScratchDir join onto, so the durable/scratch pair relocates under <state>
// automatically rather than as two separate things a caller must remember.
//
// Like WebsterGeometry, PerchGeometry takes no hash8: no value in the struct is hash-derived.
func PerchGeometry(target, stateDir string) perchengine.Geometry {
	return perchengine.Geometry{
		GateDir:    target,
		AnchorPath: stateDir,
	}
}
