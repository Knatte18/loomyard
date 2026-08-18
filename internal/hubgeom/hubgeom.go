// hubgeom.go implements the hub-mode tellers that convert a resolved *lyxcwd.Location into each
// engine's own geometry struct: ReedGeometry, BurlerGeometry, and PerchGeometry are its members.

package hubgeom

import (
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/perchengine"
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

// PerchGeometry builds a perchengine.Geometry for l: the resolved Location's paths, read off its
// accessors and passed through untouched.
// It performs no os.Getwd, no git discovery, and no path resolution of its own — internal/lyxcwd
// stays the sole owner of cwd resolution (the Cwd Resolution Invariant), and PerchGeometry only reads
// what l's caller already resolved.
// Perch's GateDir is filled from l.WorktreePath(), the value perchcli passed to perchengine.New
// before this task, so the gate command's working directory resolves byte-identically.
func PerchGeometry(l *lyxcwd.Location) perchengine.Geometry {
	return perchengine.Geometry{
		GateDir:    l.WorktreePath(),
		AnchorPath: l.AnchorPath(),
	}
}
