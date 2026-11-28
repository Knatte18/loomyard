// bootstrap.go holds the session bootstrap's pure decisions about spawning a driver and waiting for it.
// It also holds the strand lookups and actions, the status strand's command and spec, and the envelope fields.
// Each decision is pure or takes injected seams, so its tests need no real lock, process, or clock.

package loomcli

import (
	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/statuspane"
)

// statusStrandDisplayName is the status strand's stable identity, an explicit role.
// The reed engine's add has no upsert semantics: it refuses an add whose explicit role another strand already holds.
// Every add and every lookup must use this exact constant,
// so a re-entrant bootstrap call finds the pane an earlier invocation created instead of having its add refused.
const statusStrandDisplayName = loomengine.LoomStatusStrandName

// statusStrandOrder is the status strand's overview order key, one place after the runs strand's,
// so the checklist window sits right after the beam overview.
const statusStrandOrder = orchcli.RunsStrandOrder + 1

// statusStrandAddSpec returns the add spec for the status strand's pane, given its command and window title.
// It places the strand in an overview window titled with the given title.
// The window is ordered after the runs strand's window when the session has one.
// It has no size and no segment, so its window button takes the plain style.
// `IfAbsent` stays false because the status strand's ensure step already decides between keep, replace, and add.
func statusStrandAddSpec(cmd, title string) reedengine.AddSpec {
	return reedengine.AddSpec{
		NameOverride: statusStrandDisplayName,
		Cmd:          cmd,
		Display:      render.Display{Anchor: render.AnchorOverview, Order: statusStrandOrder, Title: title},
	}
}

// driverStrandDisplayName is the stable identity of the loom driver session's own strand.
// Its value must never vary.
const driverStrandDisplayName = loomengine.LoomDriverStrandName

// mustSpawnDriver reports whether the bootstrap must spawn a new driver.
// It decides from the run lock's held state and whether a live driver strand already exists.
//
// The conjunction is required on both paths: each signal is blind to exactly the driver kind the other sees.
// An operator may run `lym loom run` by hand against an `llm`-seeded run.
// That spawns no driver strand at all, so a bootstrap consulting the strand table alone would find none,
// and it would launch a Claude driver alongside the live Go one.
// A live loom driver session between two `lym shed step` invocations holds no run lock:
// it takes the lock only inside each step and releases it between them.
// So a bootstrap consulting the lock alone would spawn a second driver into a worktree that already has one.
//
// The driver lock is the Go driver's liveness between steps:
// the driver holds it for its whole life, while the run lock is free whenever the driver is not inside a step.
// So the driver lock alone tells a settling Go driver from a dead one.
//
// The run lock and the driver lock are only ever probed non-blockingly, and the caller releases them immediately.
// The probe itself never holds either:
// holding one here would make this predicate indistinguishable from the very driver it is checking for.
func mustSpawnDriver(runLockHeld, driverLockHeld, driverStrandLive bool) bool {
	return !runLockHeld && !driverLockHeld && !driverStrandLive
}

// startEnvelopeFields builds the success envelope `lym loom start` prints:
// the run's driver, slug, resolved run id, and status file.
// It is a pure function, so a Tier 1 test can pin the key set.
func startEnvelopeFields(driver, slug, runID, statusFile string) map[string]any {
	return map[string]any{
		"driver":      driver,
		"slug":        slug,
		"run_id":      runID,
		"status_file": statusFile,
	}
}

// awaitRunLockResult is the four-way outcome of polling for a just-spawned driver to take its lock.
type awaitRunLockResult int

const (
	// awaitRunLockReady means the lock the handshake waits on was observed held: a driver has taken it and is running.
	awaitRunLockReady awaitRunLockResult = iota
	// awaitRunLockChildDied means the spawned child process exited:
	// it did so before the lock was ever observed held.
	awaitRunLockChildDied
	// awaitRunLockHalted means the child is still alive and has never been observed holding the lock,
	// but the machine's own persisted state has already left running: the driver's phase-machine pass is over,
	// and the process is alive only for post-run bookkeeping.
	awaitRunLockHalted
	// awaitRunLockDeadline means none of the above happened within the attempt budget.
	awaitRunLockDeadline
)

// awaitRunLock polls at most attempts times for the just-spawned driver to take the lock lockHeld reads.
// The go arm's handshake waits for the driver lock, which `lym loom drive` takes first and holds for life,
// and the post-run work is that verb's.
//
// Each iteration consults the three seams in a fixed order: lockHeld, then alive, then halted.
// Only then does it call wait and loop again.
//
// lockHeld comes first because the lock being held is the actual thing the handshake waits for:
// a child that took the lock and is about to exit must still be reported ready, not child-died.
// Checking liveness first would race a child that takes the lock and exits within one poll window.
//
// halted comes last because it is the weakest of the three signals:
// it says only that the machine is not running, which is also true of a status file a wedged driver never touched.
// It exists because the run lock alone cannot tell a wedged spawn from a driver that finished and is still working:
// the driver releases the lock when its run returns,
// and the process can stay alive briefly for its post-run work with the lock free.
// Without this signal, a fast halt on a resume of a halted run is reported as a wedged spawn,
// and the bootstrap refuses.
//
// lockHeld, alive, halted, and wait are injected seams:
// a test can drive this whole poll with no real process, no real lock, no real status file, and no wall-clock sleep.
// A wall-clock sleep would otherwise trip the Test Tier Purity Invariant's long-sleep guard.
func awaitRunLock(lockHeld func() (bool, error), alive func() bool, halted func() bool, wait func(), attempts int) (awaitRunLockResult, error) {
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
		if halted() {
			return awaitRunLockHalted, nil
		}
		wait()
	}
	return awaitRunLockDeadline, nil
}

// handshakeDisposition is whether the session bootstrap proceeds or refuses after the handshake.
type handshakeDisposition int

const (
	// handshakeProceed means the bootstrap proceeds to its success envelope.
	handshakeProceed handshakeDisposition = iota
	// handshakeRefuse means the bootstrap reports a failure on the envelope.
	handshakeRefuse
)

// dispositionForHandshake returns whether the bootstrap proceeds or refuses for a handshake outcome.
// The handshake is the go arm's wait for the spawned `lym loom drive` to take its driver lock;
// the post-run bookkeeping is that verb's.
//
// awaitRunLockChildDied proceeds.
// A child already gone before the handshake's first poll is a driver that ran and finished.
// That is the common case: a run that halts fast exits within milliseconds, well inside the first poll interval.
// Examples of a fast halt are a blocked Preflight and an exhausted bounce budget.
// Refusing there would tell the operator the bootstrap broke when their task had in fact halted,
// and it would withhold the success envelope that is the bootstrap's entire job.
//
// awaitRunLockHalted proceeds for the same reason: the driver halted just as fast, but has not exited yet,
// so the child is alive, the lock is free, and the machine is not running.
// This result is also reported on the first poll of every resume of a halted run.
// Until the new driver rewrites it, that run's status file still reads halted.
// Refusing there would report a fast halt as a bootstrap failure.
//
// Only awaitRunLockDeadline is a genuine refusal.
// The child is still alive after the whole attempt budget, has never taken the lock, and the machine never left running.
// That is a wedged spawn and nothing else.
func dispositionForHandshake(result awaitRunLockResult) handshakeDisposition {
	switch result {
	case awaitRunLockReady, awaitRunLockChildDied, awaitRunLockHalted:
		return handshakeProceed
	default:
		return handshakeRefuse
	}
}

// findStatusStrand returns the first strand in strands that the given name addresses:
// the strand's full name, its role segment, or a legacy exact name.
func findStatusStrand(strands []reedengine.StrandStatus, name string) (reedengine.StrandStatus, bool) {
	for _, s := range strands {
		if agentname.Matches(s.Name, name) {
			return s, true
		}
	}
	return reedengine.StrandStatus{}, false
}

// findDriverStrand returns the first loom driver strand, including one recorded under the legacy literal name.
func findDriverStrand(strands []reedengine.StrandStatus) (reedengine.StrandStatus, bool) {
	for _, s := range strands {
		if loomengine.IsDriverStrand(s.Name) {
			return s, true
		}
	}
	return reedengine.StrandStatus{}, false
}

// statusStrandAction is what the bootstrap must do about the status strand.
type statusStrandAction int

const (
	// statusStrandKeep means a live status strand is already present; add nothing.
	statusStrandKeep statusStrandAction = iota
	// statusStrandAdd means no strand carries the status strand's name; add one.
	statusStrandAdd
	// statusStrandReplace means a strand carries the name but is not live;
	// remove that stale entry first, then add.
	statusStrandReplace
)

// resolveStatusStrandAction decides what the bootstrap owes the status strand, from reed's tracked strands.
//
// Presence alone does not settle the answer: reed keeps tracking a strand whose pane is gone.
// A reboot, a crash, a "tmux kill-server", or reed's own zombie-boot force-reap leaves that state.
// The entry "loom-status" in reed.json is left with a cleared pane binding.
// A presence-only check would report the strand already there and never re-add it,
// so the operator would lose the one-line read-out the bootstrap gives.
//
// A dead entry is replaced rather than added over, because reed's add has no upsert semantics:
// a second add under the role the dead entry still holds is refused.
//
// A live strand is kept only when the recorded sidecar names its GUID and equals the current build identity;
// a missing, unreadable or otherwise-mismatched sidecar means the strand may run a different lym build,
// so it is replaced.
func resolveStatusStrandAction(strands []reedengine.StrandStatus, sidecar *statuspane.Sidecar, current statuspane.BuildIdentity) (statusStrandAction, string) {
	strand, found := findStatusStrand(strands, statusStrandDisplayName)
	switch {
	case !found:
		return statusStrandAdd, ""
	case statuspane.StrandCurrent(strand.Live, strand.GUID, sidecar, current):
		return statusStrandKeep, strand.GUID
	default:
		return statusStrandReplace, strand.GUID
	}
}

// driverStrandAction is what the bootstrap must do about the loom driver session's own strand.
type driverStrandAction int

const (
	// driverStrandNone means no strand carries the driver strand's name: nothing to remove,
	// and the llm arm's own launch decision is unaffected by this signal.
	driverStrandNone driverStrandAction = iota
	// driverStrandLive means a strand carries the name and is live: a driver is already running,
	// and `mustSpawnDriver` must not launch a second one.
	driverStrandLive
	// driverStrandDead means a strand carries the name but is not live:
	// the corpse must be removed before a relaunch, since reed's add has no upsert semantics.
	driverStrandDead
	// driverStrandRetiring means a strand carries the name, is live and is marked retiring:
	// a caller of `reed remove --detach` already asked to remove it, so start replaces it instead of adopting it.
	driverStrandRetiring
)

// resolveDriverStrandAction decides what the bootstrap owes the driver strand, from reed's tracked strands.
// It returns the action and the matched strand's GUID;
// a driverStrandNone action carries no GUID, since nothing was found to act on.
// Nothing here removes anything: a driverStrandDead result only tells the caller that a corpse is present,
// so it can remove it before relaunching.
func resolveDriverStrandAction(strands []reedengine.StrandStatus) (driverStrandAction, string) {
	strand, found := findDriverStrand(strands)
	switch {
	case !found:
		return driverStrandNone, ""
	case strand.Live && strand.Retiring:
		return driverStrandRetiring, strand.GUID
	case strand.Live:
		return driverStrandLive, strand.GUID
	default:
		return driverStrandDead, strand.GUID
	}
}

// statusStrandCmd composes the command line the status strand's pane runs:
// `exe`, invoked through the shell seam, followed by the quoted words `loom` and `checklist`.
func statusStrandCmd(sh shell.Shell, exe string) string {
	return sh.Invoke(exe) + " " + sh.Quote("loom") + " " + sh.Quote("checklist")
}
