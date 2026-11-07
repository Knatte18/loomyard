// buttons.go builds the status-bar button context that reed asks for, lazily, for a session.
// The context holds the session's place, its placeholder values and the machine's buttons.
// The {beam} directory is resolved only when the render needs it.

package hubgeom

import (
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/desktophook"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// buttonContext returns the function reed calls, at boot and attach only, to learn l's button context.
// The place is the prime's when prime is set.
// Otherwise it is the beam's when the pair's origin record exists and carries the beam mark, and the task's when not.
// A failed desktop load is logged and leaves the machine's buttons empty, so the session's own buttons still render.
func buttonContext(l *lymcwd.Location, prime bool) func() (reedengine.ButtonContext, error) {
	return func() (reedengine.ButtonContext, error) {
		place := reedengine.ButtonPlacePrime
		if !prime {
			origin, found, err := fabricengine.ReadOriginFor(l, l.WorktreeName)
			if err != nil {
				return reedengine.ButtonContext{}, fmt.Errorf("hubgeom: read origin record for %q: %w", l.WorktreeName, err)
			}
			place = reedengine.ButtonPlaceTask
			if found && origin.Beam {
				place = reedengine.ButtonPlaceBeam
			}
		}

		buttons := reedengine.ButtonContext{
			Place:    place,
			Slug:     l.WorktreeName,
			Worktree: l.AnchorPath(),
			Session:  reedengine.SessionName(l.WorktreePath()),
			Beam:     buttonBeam(l, place),
		}
		desktop, err := desktophook.LoadConfig(fabricengine.ConfigDirs(l))
		if err != nil {
			logger.Warn("hubgeom: could not load the desktop config, so the machine adds no status-bar buttons", "worktree", l.WorktreeName, "err", err)
		} else {
			buttons.MachineButtons = desktop.Buttons
		}
		return buttons, nil
	}
}

// buttonBeam returns the function reed calls to resolve the beam directory for a session of the given place.
// It is called only when a button's text names {beam}.
// The prime and a beam session are their own beam.
// A task's beam is its parent pair, read through ParentOf over the prime's location.
// When that answers "", the beam is the prime itself.
// A parent pair absent on this machine leaves the Anchor empty, since the parent's path is built without being checked.
func buttonBeam(l *lymcwd.Location, place reedengine.ButtonPlace) func() (reedengine.ButtonBeam, error) {
	return func() (reedengine.ButtonBeam, error) {
		if place != reedengine.ButtonPlaceTask {
			return reedengine.ButtonBeam{Anchor: l.AnchorPath(), Name: l.WorktreeName, PrimeAnchor: l.AnchorPath()}, nil
		}

		primeName, err := fabricengine.PrimeName(l)
		if err != nil {
			return reedengine.ButtonBeam{}, fmt.Errorf("hubgeom: resolve the prime for the beam of %q: %w", l.WorktreeName, err)
		}
		prime, err := lymcwd.NamedWorktree(l.HubPath, primeName)
		if err != nil {
			return reedengine.ButtonBeam{}, fmt.Errorf("hubgeom: locate the prime %q for the beam of %q: %w", primeName, l.WorktreeName, err)
		}
		parent, err := ParentOf(prime, l.WorktreeName)
		if err != nil {
			return reedengine.ButtonBeam{}, err
		}
		if parent == "" {
			return reedengine.ButtonBeam{Anchor: prime.AnchorPath(), Name: primeName, PrimeAnchor: prime.AnchorPath()}, nil
		}

		beam := reedengine.ButtonBeam{Name: parent, PrimeAnchor: prime.AnchorPath()}
		parentLocation, err := lymcwd.NamedWorktree(l.HubPath, parent)
		if err != nil {
			return reedengine.ButtonBeam{}, fmt.Errorf("hubgeom: locate the parent %q of %q: %w", parent, l.WorktreeName, err)
		}
		if info, err := os.Stat(parentLocation.AnchorPath()); err == nil && info.IsDir() {
			beam.Anchor = parentLocation.AnchorPath()
		}
		return beam, nil
	}
}
