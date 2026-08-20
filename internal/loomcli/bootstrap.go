// bootstrap.go implements the session bootstrap's pure decisions: whether a driver must be spawned,
// the handshake that waits for the spawned driver to take the run lock, the status strand's lookup
// by its fixed name, and the two command-string builders the bootstrap composes over. Every piece
// here is a pure function or takes injected seams, so the verb body in run.go is assembly over
// judgment that is already under test -- no real lock, no real process, and no real clock is needed
// to exercise any of it.

package loomcli

import (
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shell"
)

// statusStrandDisplayName is the status strand's stable identity: the reed engine's add operation
// has no upsert semantics, so a second add with the same display name would append a second pane
// rather than replace the first. Every add and every lookup must use this exact constant so the
// bootstrap's re-entrant call finds the pane it created on an earlier invocation instead of
// duplicating it.
const statusStrandDisplayName = "loom-status"

// mustSpawnDriver reports whether the bootstrap must spawn a new detached driver, from whether the
// run lock is currently held.
//
// This is the whole re-entrancy decision: the run lock being held means a driver is already alive,
// so the bootstrap only needs to ensure the reed substrate and attach, never spawn a second driver.
// The lock is only ever probed non-blockingly and released immediately by the caller -- never held
// by the probe itself -- because holding it here would make this predicate indistinguishable from
// the very driver it is checking for.
func mustSpawnDriver(runLockHeld bool) bool {
	return !runLockHeld
}

// awaitRunLockResult is the three-way outcome of awaitRunLock.
type awaitRunLockResult int

const (
	// awaitRunLockReady means the run lock was observed held: a driver has taken it and is running.
	awaitRunLockReady awaitRunLockResult = iota
	// awaitRunLockChildDied means the spawned child process is no longer alive, before the lock was
	// ever observed held.
	awaitRunLockChildDied
	// awaitRunLockDeadline means neither of the above happened within the attempt budget.
	awaitRunLockDeadline
)

// awaitRunLock polls at most attempts times for the just-spawned driver to take the run lock.
//
// Each iteration first consults lockHeld, returning ready the moment it reports held; only then does
// it consult alive, returning child-died when the child is already gone; only then does it call wait
// and loop again. This order is load-bearing: a child that took the lock and is about to exit must
// still be reported ready, not child-died, because the lock being held is the actual thing the
// handshake is waiting for, and checking liveness first would race a child that takes the lock and
// exits within the same poll window.
//
// lockHeld, alive, and wait are all injected seams so a test can drive this whole poll with no real
// process, no real lock, and no wall-clock sleep, which the Test Tier Purity Invariant's long-sleep
// guard would otherwise flag.
func awaitRunLock(lockHeld func() (bool, error), alive func() bool, wait func(), attempts int) (awaitRunLockResult, error) {
	for i := 0; i < attempts; i++ {
		held, err := lockHeld()
		if err != nil {
			return awaitRunLockDeadline, err
		}
		if held {
			return awaitRunLockReady, nil
		}
		if !alive() {
			return awaitRunLockChildDied, nil
		}
		wait()
	}
	return awaitRunLockDeadline, nil
}

// findStatusStrand returns the first strand in strands whose Name exactly matches name.
func findStatusStrand(strands []reedengine.StrandStatus, name string) (reedengine.StrandStatus, bool) {
	for _, s := range strands {
		if s.Name == name {
			return s, true
		}
	}
	return reedengine.StrandStatus{}, false
}

// statusStrandCmd composes the status strand's pane command line through the shell seam, exactly as
// the reed header pane's own builder (headerLaunchCmd, headerpane.go) composes its command line: exe
// invoked with the two-word status verb and the watch flag.
func statusStrandCmd(sh shell.Shell, exe string) string {
	return sh.Invoke(exe) + " " + sh.Quote("loom") + " " + sh.Quote("status") + " " + sh.Quote("--watch")
}

// attachArgv returns the tmux argv for an in-place attach into socket's session, in the same shape
// internal/reedcli's own attachArgv builds.
func attachArgv(socket, session string) []string {
	return []string{"-L", socket, "attach-session", "-t", "=" + session}
}
