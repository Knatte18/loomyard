// paths.go declares the prime-anchored lifecycle path constructors:
// `BattenDir`, `StatusFile`, `RunLock`, `StatusLock`, `PrimeRunLock`, and `LandLock`.
// Each forwards to the matching shedrun constructor: the run-scoped ones take a run ID,
// and the hub-scoped ones take only the location.
// battencli derives no path itself: shedrun is the sole declarer of the "shed" path segment.
//
// `StatusFile` is durable, fabric-synced state in the `_lym`-rooted run directory.
// Every other constructor here is ephemeral and sits at the mirrored `.lym` subpath.
// None of them calls `os.Getwd` or any git command.
// Each path is built by shedrun with `filepath.Join` onto the given `*lymcwd.Location`'s `AnchorPath()`.
package battencli

import (
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// BattenDir returns the path to a run's ephemeral scratch directory, under which its run lock and status lock live.
func BattenDir(l *lymcwd.Location, runID string) string {
	return shedrun.ScratchDir(l, runID)
}

// StatusFile returns the path to a run's durable, fabric-synced status.json.
func StatusFile(l *lymcwd.Location, runID string) string {
	return shedrun.StatusFile(l, runID)
}

// RunLock returns the path to a run's ephemeral run lock.
// It must never equal the status lock's path: shedengine rejects that configuration outright,
// and a shared file would hang on the first persist rather than fail.
func RunLock(l *lymcwd.Location, runID string) string {
	return shedrun.RunLock(l, runID)
}

// StatusLock returns the path to the advisory lock guarding concurrent access to a run's status file.
func StatusLock(l *lymcwd.Location, runID string) string {
	return shedrun.StatusLock(l, runID)
}

// PrimeRunLock returns the path to the hub-scoped advisory lock that serialises every slug's create and teardown.
// The lock is per-hub rather than per-run, so the function takes no run ID.
func PrimeRunLock(l *lymcwd.Location) string {
	return shedrun.PrimeRunLock(l)
}

// LandLock returns the path to the hub-scoped advisory lock that one landing holds for its whole body.
func LandLock(l *lymcwd.Location) string {
	return shedrun.LandLock(l)
}
