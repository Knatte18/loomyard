// reedgeom.go implements ReedGeometry, the told-mode builder for a reedengine.Geometry of a standalone session.
// It takes the session's target directory, state directory, and hash8 identifier.

package standalonegeom

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/standalonestate"
)

// ReedGeometry builds a reedengine.Geometry for a standalone session anchored at target.
// target is the absolute standalone target directory.
// stateDir and hash8 are the state directory and identifier the caller derived from target.
//
// AnchorPath and PaneCwd diverge on purpose.
// Reed's state files (reed.json, reed.lock) live under stateDir.
// Every pane starts in target instead, the git repository an implementer builds, tests, and commits in.
//
// SessionName's readable half is the basename of the normalized target.
// reedengine.SanitizeSessionName runs over it before the "-<hash8>" suffix is appended.
// A standalone target is a plain checkout named by its operator.
// A dot in such a name is routine, and tmux refuses a session name that carries one.
// The hash8 suffix keeps the name unique, so sanitizing costs nothing.
//
// The readable half comes from the normalized spelling of target, not the told one.
// That keeps both halves of the name pointing at the same directory.
// hash8 is also computed from the normalized spelling.
// A symlinked and a real spelling of one repository therefore share one socket key and one state directory.
// Normalizing again is idempotent, so a caller that already normalized target loses nothing.
//
// PaneCwd and WorktreeRoot stay as told: both name a directory,
// and the told spelling is the one the operator recognizes in an error.
//
// WorktreeName is the raw basename of target.
// It is a display value in error messages and never reaches a tmux target.
func ReedGeometry(target, stateDir, hash8 string) reedengine.Geometry {
	readableName := filepath.Base(standalonestate.Normalize(target))
	return reedengine.Geometry{
		SocketKey:    "lym-" + hash8,
		SessionName:  reedengine.SanitizeSessionName(readableName) + "-" + hash8,
		AnchorPath:   stateDir,
		PaneCwd:      target,
		WorktreeRoot: target,
		// LogsDir is stateDir joined with "logs", told directly.
		// Standalone mode has no board, so no board-shaped logs path exists to derive it from.
		// Deriving it from a hub layout would name a directory that standalone mode never creates.
		LogsDir:      filepath.Join(stateDir, "logs"),
		WorktreeName: filepath.Base(target),
		HubPath:      stateDir,
		// A standalone run has no board to record a shortname on and no seed,
		// so the shortname derives from hash8, and the slug and parent stay empty.
		NameShortname: agentname.StandaloneShortname(hash8),
	}
}
