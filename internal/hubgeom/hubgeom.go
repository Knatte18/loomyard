// hubgeom.go implements the hub-mode tellers that convert a resolved *lyxcwd.Location into each
// engine's own geometry struct: ReedGeometry and BurlerGeometry are its members.

package hubgeom

import (
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// ReedGeometry builds a reedengine.Geometry for l: the resolved Location's paths, read off its
// accessors and passed through untouched.
// It performs no os.Getwd, no git discovery, and no path resolution of its own — internal/lyxcwd
// stays the sole owner of cwd resolution (the Cwd Resolution Invariant), and ReedGeometry only reads
// what l's caller already resolved.
func ReedGeometry(l *lyxcwd.Location) reedengine.Geometry {
	return reedengine.Geometry{
		SocketKey:    reedengine.ServerName(l.HubPath),
		SessionName:  reedengine.SessionName(l.WorktreePath()),
		AnchorPath:   l.AnchorPath(),
		PaneCwd:      l.AnchorPath(),
		WorktreeRoot: l.WorktreePath(),
		LogsDir:      fabricengine.HubLogsDir(l.HubPath),
		RepoName:     l.RepoName,
		HubPath:      l.HubPath,
	}
}

// BurlerGeometry builds a burlerengine.Geometry for l: the resolved Location's paths, read off its
// accessors and passed through untouched.
// It performs no os.Getwd, no git discovery, and no path resolution of its own — internal/lyxcwd
// stays the sole owner of cwd resolution (the Cwd Resolution Invariant), and BurlerGeometry only
// reads what l's caller already resolved.
func BurlerGeometry(l *lyxcwd.Location) burlerengine.Geometry {
	return burlerengine.Geometry{
		WorktreeRoot: l.WorktreePath(),
		AnchorPath:   l.AnchorPath(),
	}
}
