// paths.go declares the five prime-anchored lifecycle path constructors: BattenDir, StatusFile,
// RunLock, StatusLock, and PrimeRunLock. Each is a thin forwarding layer over the corresponding
// shedrun constructor, keyed on the given run-id -- battencli owns no path-derivation logic of its
// own, per the Shed Run-Directory Invariant's sole-declarer claim over the "shed" segment.
//
// StatusFile is durable, fabric-synced state, unlike RunLock, StatusLock and PrimeRunLock, which
// stay ephemeral: per the Durable-vs-Ephemeral State Invariant, StatusFile lives under prime's own
// anchor at shedrun's _lyx-rooted run directory, while the three locks live at the mirrored .lyx
// subpath. None of the five calls os.Getwd or any git command -- each is a plain filepath.Join
// resolved through shedrun onto the given *lyxcwd.Location's AnchorPath(), per the Cwd Resolution
// Invariant.
package battencli

import (
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// BattenDir returns the path to a run's ephemeral scratch directory, under which its run lock and
// status lock live: shedrun.ScratchDir(l, runID).
func BattenDir(l *lyxcwd.Location, runID string) string {
	return shedrun.ScratchDir(l, runID)
}

// StatusFile returns the path to a run's durable, fabric-synced status.json: shedrun.StatusFile(l,
// runID).
func StatusFile(l *lyxcwd.Location, runID string) string {
	return shedrun.StatusFile(l, runID)
}

// RunLock returns the path to a run's ephemeral run lock: shedrun.RunLock(l, runID).
// It must never equal StatusLock(l, runID): shedengine.Shed's own validation rejects LockPath ==
// StatusLockPath outright, and a shared file would hang on the first persist rather than fail.
func RunLock(l *lyxcwd.Location, runID string) string {
	return shedrun.RunLock(l, runID)
}

// StatusLock returns the path to the advisory lock guarding concurrent access to StatusFile(l,
// runID): shedrun.StatusLock(l, runID).
func StatusLock(l *lyxcwd.Location, runID string) string {
	return shedrun.StatusLock(l, runID)
}

// PrimeRunLock returns the path to the hub-scoped advisory lock that serialises every slug's create
// and teardown against one another: shedrun.PrimeRunLock(l).
// The lock it names is per-hub rather than per-run, so it takes no run-id; battencli stays its only
// caller while shedrun declares it.
func PrimeRunLock(l *lyxcwd.Location) string {
	return shedrun.PrimeRunLock(l)
}
