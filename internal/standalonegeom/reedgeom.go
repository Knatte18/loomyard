// reedgeom.go implements ReedGeometry, the told-mode builder that constructs a reedengine.Geometry
// from a standalone session's target directory, derived state directory, and hash8 identifier.

package standalonegeom

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/reedengine"
)

// ReedGeometry builds a reedengine.Geometry for a standalone session anchored at target, an
// already-absolute standalone target directory, given the stateDir and hash8 the caller already
// derived via standalonestate.Derive(target).
//
// AnchorPath and PaneCwd deliberately diverge: reed's state files (reed.json, reed.lock) belong
// under stateDir, while every pane must start in target, because target is the git repository
// holding the source an implementer builds, tests, and commits in.
func ReedGeometry(target, stateDir, hash8 string) reedengine.Geometry {
	return reedengine.Geometry{
		SocketKey:    "lyx-" + hash8,
		SessionName:  filepath.Base(target) + "-" + hash8,
		AnchorPath:   stateDir,
		PaneCwd:      target,
		WorktreeRoot: target,
		// LogsDir is stateDir joined with "logs", told directly and deliberately NOT
		// fabricengine.HubLogsDir(stateDir), which would produce a board-shaped path that does
		// not exist in standalone mode.
		LogsDir:  filepath.Join(stateDir, "logs"),
		RepoName: filepath.Base(target),
		HubPath:  stateDir,
	}
}
