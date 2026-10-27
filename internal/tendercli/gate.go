// gate.go installs the process-wide network gate for a lym process that runs in a hub.

package tendercli

import (
	"context"
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/proc"
)

// DetachedChild describes the hidden flags a detached child process was spawned with, which decide how its network gate behaves.
// The zero value is an ordinary process.
type DetachedChild struct {
	// Deferrable marks a detached push or board-sync child whose gate defers under a pause instead of waiting.
	Deferrable bool
	// ParentLocked marks a push child spawned while a parent lock record named its slug.
	ParentLocked bool
	// HubScope marks a board-sync child, held only by a hub entry whichever worktree spawned it.
	HubScope bool
}

// InstallNetworkGate installs the pause gate that holds this process's network git and GitHub calls while a pause covers it.
// The hub is the one the context's working directory resolves to; outside a hub nothing is installed.
// A git worktree whose parent holds no board-level _lym, such as an agent worktree nested in a repository, is outside a hub,
// so no in-flight marker is written under a hub path guessed from its parent.
// The gate is told the process's strand name, pid and start time, so its in-flight markers tell a live process from a dead one, and reads whether a live parent lock record names the slug it is asked about.
// A nil pauseSlug takes the slug from the working directory's worktree.
// A non-nil pauseSlug is the slug the process is held under, empty for the prime's, and the working directory is resolved as a worktree root:
// a detached push child starts in the hub's board worktree, which says nothing of the pair it serves and which the gated resolution refuses on a hub with a subpath anchor.
// With child.HubScope set and a nil pauseSlug the process is held under the empty slug and resolved the same way, so only a hub entry holds it.
// A child that is Deferrable and not ParentLocked defers under a pause instead of waiting;
// a ParentLocked child lets its own slug's owner entry pass, since the parent lock it was spawned under makes it the process that must finish the run's push.
func InstallNetworkGate(ctx context.Context, pauseSlug *string, child DetachedChild) {
	if pauseSlug == nil && child.HubScope {
		hubSlug := ""
		pauseSlug = &hubSlug
	}
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return
	}
	var location *lymcwd.Location
	if pauseSlug == nil {
		location, err = lymcwd.Resolve(cwd)
	} else {
		location, err = lymcwd.ResolveWorktree(cwd)
	}
	if err != nil || !preflight.BoardLymPresent(location) {
		return
	}
	geometry, err := hubgeom.PauseGeometry(location)
	if err != nil {
		return
	}
	if pauseSlug != nil {
		geometry.Slug = *pauseSlug
	}
	pid := os.Getpid()
	startTime, _ := proc.StartTime(pid)
	locks := &parentlock.Store{Dir: parentlock.Dir(fabricengine.BoardDir(location.HubPath))}
	pauserecord.InstallGate(pauserecord.Gate{
		Geometry:         geometry,
		Strand:           os.Getenv(agentname.StrandNameEnv),
		PID:              pid,
		StartTime:        startTime,
		Defer:            child.Deferrable && !child.ParentLocked,
		OwnerEntryExempt: child.ParentLocked,
		HoldsParentLock: func(slug string) bool {
			held, err := locks.HeldBy(slug)
			if err != nil {
				logger.Warn("tendercli: the parent locks cannot be listed, so the gate reads none held", "error", err)
				return false
			}
			return held
		},
		Notify: func(line string) {
			fmt.Fprintln(os.Stderr, line)
			logger.Info("tendercli: a network call is held by a pause", "line", line)
		},
	})
}
