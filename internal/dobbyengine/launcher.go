// launcher.go keeps the hub's desktop launcher on this machine.
// A dobby start adds it for a worktree this machine owns.
// A catch-up removes it once this machine owns nothing in the hub.

package dobbyengine

import (
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/desktoplauncher"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/selfexec"
)

// Launcher names a hub's desktop launcher.
// Its zero value keeps no launcher.
type Launcher struct {
	// BoardDir is the hub's board directory, whose owners tell whether this machine owns anything in the hub.
	BoardDir string
	// Shortname is the hub's shortname, which names its launcher; empty keeps no launcher.
	Shortname string
}

// OwnsAnything reports whether this machine, here, owns anything in the hub whose owners are owners.
// That is the prime, or a beam not done whose recorded owner reads as here through the board's machine registry.
func OwnsAnything(owners boardengine.Owners, here machinename.Identity) bool {
	if owners.Registry.ReadsAsHere(owners.Prime, here) {
		return true
	}
	for _, entry := range owners.Entries {
		if !entry.Done && owners.Registry.ReadsAsHere(entry.Owner, here) {
			return true
		}
	}
	return false
}

// addLauncher adds the hub's desktop launcher, running the dobby's lym binary.
// It adds one only when this dobby's worktree reads owned by this machine.
// A worktree with no recorded owner or one another machine owns adds none, and so does a Launcher with no shortname.
// A machine with no desktop adds none; any other failure is logged at Warn, and the dobby goes on.
func (d *daemon) addLauncher() {
	shortname := d.deps.CatchUp.Launcher.Shortname
	if shortname == "" || d.deps.Owner == nil {
		return
	}
	owner, elsewhere, err := d.deps.Owner()
	if err != nil {
		logger.Warn("dobbyengine: the owner read for the desktop launcher failed, so no launcher was added", "worktree", d.geom.WorktreeName, "error", err)
		return
	}
	if owner == "" || elsewhere {
		return
	}
	lymPath, err := selfexec.ResolveLymBinary(func() (string, error) { return d.geom.LymExecutable, nil })
	if err != nil {
		logger.Warn("dobbyengine: the lym binary for the desktop launcher did not resolve, so no launcher was added", "worktree", d.geom.WorktreeName, "error", err)
		return
	}
	desktop, found := locateDesktop()
	if !found {
		return
	}
	if err := desktop.Add(lymPath, shortname); err != nil {
		logger.Warn("dobbyengine: the hub's desktop launcher was not added", "shortname", shortname, "error", err)
	}
}

// removeLauncherIfOwnsNothing removes the launcher of the hub launcher names once this machine owns nothing there.
// The owners are read from the hub's local board.
// A Launcher with no shortname and a machine with no desktop remove nothing; a failure is logged at Warn.
func removeLauncherIfOwnsNothing(launcher Launcher) {
	if launcher.Shortname == "" {
		return
	}
	owners, err := boardengine.ReadOwners(launcher.BoardDir)
	if err != nil {
		logger.Warn("dobbyengine: the owners did not read, so the hub's desktop launcher was left as it stands", "shortname", launcher.Shortname, "error", err)
		return
	}
	here, err := machinename.ReadIdentity()
	if err != nil {
		logger.Warn("dobbyengine: this machine's name did not read, so the hub's desktop launcher was left as it stands", "shortname", launcher.Shortname, "error", err)
		return
	}
	if OwnsAnything(owners, here) {
		return
	}
	desktop, found := locateDesktop()
	if !found {
		return
	}
	if err := desktop.Remove(launcher.Shortname); err != nil {
		logger.Warn("dobbyengine: the hub's desktop launcher was not removed", "shortname", launcher.Shortname, "error", err)
	}
}

// locateDesktop returns this machine's desktop, and false when it has none.
func locateDesktop() (desktoplauncher.Desktop, bool) {
	desktop, err := desktoplauncher.Locate()
	if err != nil {
		logger.Debug("dobbyengine: no desktop on this machine, so no desktop launcher is kept", "cause", err)
		return desktoplauncher.Desktop{}, false
	}
	return desktop, true
}
