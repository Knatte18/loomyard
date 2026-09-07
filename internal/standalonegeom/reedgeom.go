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
//
// SessionName's readable half runs through reedengine.SanitizeSessionName before the "-<hash8>"
// suffix is appended, because standalone's whole premise is that target is a plain checkout the
// operator already has, named whatever they named it. A dot in a repository directory name is
// routine ("foo.js", "site.com", "app.git"), and reedengine's validateToldTmuxIdentity refuses a
// session name carrying one -- so the raw basename used to kill an otherwise healthy standalone run
// with advice to rename the operator's own repository (R4 review finding R4-10). Hub mode never hit
// this because hub worktree names are lyx-created and slug-shaped.
// Sanitizing costs no uniqueness here: hash8 already carries the identity, exactly as the hash half
// of reedengine.ServerName carries it for the socket key.
// RepoName below is deliberately left RAW -- it is the header pane's display token, never a tmux
// target, so the operator should see the directory name they actually have.
func ReedGeometry(target, stateDir, hash8 string) reedengine.Geometry {
	return reedengine.Geometry{
		SocketKey:    "lyx-" + hash8,
		SessionName:  reedengine.SanitizeSessionName(filepath.Base(target)) + "-" + hash8,
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
