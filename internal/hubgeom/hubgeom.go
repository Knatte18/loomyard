// hubgeom.go implements the hub-mode tellers that convert a resolved *lyxcwd.Location into each
// engine's own geometry struct: ReedGeometry and BurlerGeometry are its members.

package hubgeom

import (
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// ReedGeometry builds a reedengine.Geometry for l: the resolved Location's paths, read off its
// accessors and passed through untouched, plus the name prefix and parent reed forms strand names from.
// It performs no os.Getwd, no git discovery, and no path resolution of its own — internal/lyxcwd
// stays the sole owner of cwd resolution (the Cwd Resolution Invariant).
//
// NameCode is the hub's recorded code; an absent record leaves it empty and is no error here,
// since reed refuses the spawn that needs a code while `reed status`, `down` and the watchdog keep working.
// The prime leaves NameSlug and ParentName empty;
// a task worktree sets NameSlug to its raw worktree name and ParentName to the parent recorded in its default run's seed.
// An unreadable seed logs a warning and leaves ParentName empty, since the parent is an optional escalation channel.
// The prime's name failing to resolve is no error either: the wiring functions' Tier 1 tests hand
// ReedGeometry fictional, git-less locations, and a failure here only costs the slug and parent,
// so it logs a warning and tells the geometry of a prime.
// The error return stays so a later check that must stop a wiring can use it without a signature change.
func ReedGeometry(l *lyxcwd.Location) (reedengine.Geometry, error) {
	prime, err := fabricengine.PrimeName(l)
	if err != nil {
		logger.Warn("hubgeom: main worktree name unresolved; telling a prime-shaped name geometry", "worktree", l.WorktreeName, "error", err)
		prime = l.WorktreeName
	}
	return reedGeometry(l, prime), nil
}

// reedGeometry is ReedGeometry once the prime's name is resolved, split out so the unit test can
// tell a fixture prime without spawning git.
func reedGeometry(l *lyxcwd.Location, prime string) reedengine.Geometry {
	code, _ := fabricengine.ReadCode(fabricengine.BoardDir(l.HubPath))
	var slug, parent string
	if l.WorktreeName != prime {
		slug = l.WorktreeName
		seed, found, seedErr := shedrun.ReadSeed(l, shedrun.SelfRunID)
		switch {
		case seedErr != nil:
			logger.Warn("hubgeom: default run seed unreadable; strands get no parent", "worktree", l.WorktreeName, "error", seedErr)
		case found:
			parent = seed.Parent
		}
	}
	return reedengine.Geometry{
		SocketKey:    reedengine.ServerName(l.HubPath),
		SessionName:  reedengine.SessionName(l.WorktreePath()),
		AnchorPath:   l.AnchorPath(),
		PaneCwd:      l.AnchorPath(),
		WorktreeRoot: l.WorktreePath(),
		LogsDir:      fabricengine.HubLogsDir(l.HubPath),
		RepoName:     l.RepoName,
		WorktreeName: l.WorktreeName,
		HubPath:      l.HubPath,
		NameCode:     code,
		NameSlug:     slug,
		ParentName:   parent,
	}
}

// BurlerGeometry builds a burlerengine.Geometry for l: the resolved Location's paths, read off its
// accessors and passed through untouched.
// It performs no os.Getwd, no git discovery, and no path resolution of its own — internal/lyxcwd
// stays the sole owner of cwd resolution (the Cwd Resolution Invariant), and BurlerGeometry only
// reads what l's caller already resolved.
// WorktreeRoot is l.AnchorPath(), NOT l.WorktreePath(): a review segment's _lyx content is
// anchor-anchored, matching the commit seam that commits it, so telling burler the anchor path as
// its profile root is what keeps the two aligned. Converging or reverting the two would silently
// change behaviour in a subpath-anchored hub, where the anchor path and the worktree path diverge.
// standalonegeom.BurlerGeometry is the mode where WorktreeRoot and AnchorPath still legitimately
// diverge: standalone fills WorktreeRoot with the reviewed target directory, not the anchor path.
func BurlerGeometry(l *lyxcwd.Location) burlerengine.Geometry {
	return burlerengine.Geometry{
		WorktreeRoot: l.AnchorPath(),
		AnchorPath:   l.AnchorPath(),
	}
}
