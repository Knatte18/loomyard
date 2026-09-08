// reedgeom.go implements ReedGeometry, the told-mode builder that constructs a reedengine.Geometry
// from a standalone session's target directory, derived state directory, and hash8 identifier.

package standalonegeom

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/standalonestate"
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
//
// That readable half is taken from the NORMALIZED spelling of target, not the told one, because the
// two halves of the session name must agree about which directory they name: hash8 comes from
// standalonestate.Derive, which resolves symlinks before hashing, so a symlinked and a real spelling
// of one repository land on ONE socket key and ONE state directory -- but used to produce two
// different session names sharing that one reed.json (R4 review finding R4-24). reed's
// foreign-session guard caught the collision loudly, with advice that did not fit the situation.
// standalonestate.Normalize is called rather than re-derived here so the rule stays declared once, in
// the package that owns the identity; it is idempotent, so a caller that has already normalized its
// target at the CLI boundary loses nothing by this call.
//
// PaneCwd and WorktreeRoot stay exactly as TOLD: both name a directory rather than an identity, both
// spellings reach the same one, and the told spelling is the one the operator typed and will
// recognise in a header or an error. RepoName is likewise left RAW -- it is the header pane's display
// token, never a tmux target.
func ReedGeometry(target, stateDir, hash8 string) reedengine.Geometry {
	readableName := filepath.Base(standalonestate.Normalize(target))
	return reedengine.Geometry{
		SocketKey:    "lyx-" + hash8,
		SessionName:  reedengine.SanitizeSessionName(readableName) + "-" + hash8,
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
