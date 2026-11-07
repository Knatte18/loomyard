// burlergeom.go implements BurlerGeometry, the told-mode builder of a burlerengine.Geometry.
// It takes the standalone session's target directory and its derived state directory.

package standalonegeom

import (
	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/burlerengine"
)

// BurlerGeometry builds a burlerengine.Geometry for a standalone session anchored at target.
// target is an already-absolute standalone target directory.
// stateDir is the state directory the caller derived from it.
//
// WorktreeRoot is target: the profile's paths resolve against it, so it must be the directory burler reviews.
// AnchorPath is stateDir: the .lym/burler instruction directory joins onto it.
// Pointing AnchorPath at target would push a hidden .lym tree into the reviewed folder.
//
// Shortname is derived from the session's hash8 and Slug stays empty:
// the agent names burler forms match the names reed gives the strands.
func BurlerGeometry(target, stateDir, hash8 string) burlerengine.Geometry {
	return burlerengine.Geometry{
		WorktreeRoot: target,
		AnchorPath:   stateDir,
		RepoRoot:     target,
		Shortname:    agentname.StandaloneShortname(hash8),
		SpecsDir:     SpecsDir(stateDir),
	}
}
