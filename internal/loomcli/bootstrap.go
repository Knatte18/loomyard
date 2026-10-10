// bootstrap.go implements the session bootstrap's pure decisions: whether a driver must be spawned,
// the handshake that waits for the spawned driver to take the run lock, the status strand's lookup
// by its fixed name, and the one command-string builder the bootstrap composes over. Every piece
// here is a pure function or takes injected seams, so the verb body in start.go is assembly over
// judgment that is already under test -- no real lock, no real process, and no real clock is needed
// to exercise any of it.

package loomcli

import (
	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
)

// statusStrandDisplayName is the status strand's stable identity, an explicit role.
// The reed engine's add operation has no upsert semantics and refuses an add whose explicit role another strand already holds.
// Every add and every lookup must use this exact constant,
// so the bootstrap's re-entrant call finds the pane it created on an earlier invocation instead of having its add refused.
const statusStrandDisplayName = "loom-status"

// statusStrandAddSpec builds the status strand's reedengine.AddSpec for the given pane command: a
// plain below-parent pane, sized by reed's one layout rule like every other strand. IfAbsent stays
// false because ensureStatusStrand does its own keep/replace/add dance.
func statusStrandAddSpec(cmd string) reedengine.AddSpec {
	return reedengine.AddSpec{
		NameOverride: statusStrandDisplayName,
		Cmd:          cmd,
		Display: render.Display{
			Anchor: render.AnchorBelowParent,
		},
	}
}

// driverStrandDisplayName is the loom driver session's own strand's stable identity.
// The value is declared once, as loomengine.LoomDriverStrandName, which carries the reason it must never vary.
const driverStrandDisplayName = loomengine.LoomDriverStrandName

// mustSpawnDriver reports whether the bootstrap must spawn a new driver, from the run lock's held
// state AND whether a live driver strand already exists.
//
// The conjunction is required on BOTH paths, not one signal per path, because each signal is blind
// to exactly the driver kind the other sees. An operator may run `lyx loom run` by hand against an
// `llm`-seeded run: that spawns no driver strand at all, so a bootstrap consulting the strand table
// alone would find none and launch a Claude driver alongside the live Go one. And a live loom driver
// session between two `lyx shed step` invocations holds no run lock -- it takes the lock only inside
// each step and releases it between them -- so a bootstrap consulting the lock alone would spawn a
// second driver into a worktree that already has one.
//
// The run lock is only ever probed non-blockingly and released immediately by the caller -- never
// held by the probe itself -- because holding it here would make this predicate indistinguishable
// from the very driver it is checking for.
func mustSpawnDriver(runLockHeld bool, driverStrandLive bool) bool {
	return !runLockHeld && !driverStrandLive
}

// startEnvelopeFields builds the success envelope `lyx loom start` prints: the run's driver, slug, resolved run id and status file.
// It is a pure function so a Tier 1 test can pin the key set.
func startEnvelopeFields(driver, slug, runID, statusFile string) map[string]any {
	return map[string]any{
		"driver":      driver,
		"slug":        slug,
		"run_id":      runID,
		"status_file": statusFile,
	}
}

// awaitRunLockResult is the four-way outcome of awaitRunLock.
type awaitRunLockResult int

const (
	// awaitRunLockReady means the run lock was observed held: a driver has taken it and is running.
	awaitRunLockReady awaitRunLockResult = iota
	// awaitRunLockChildDied means the spawned child process is no longer alive, before the lock was
	// ever observed held.
	awaitRunLockChildDied
	// awaitRunLockHalted means the child is still alive and has never been observed holding the
	// lock, but the machine's own persisted state has already left running: the driver's phase-machine
	// pass is over and the process is alive only for post-run bookkeeping.
	awaitRunLockHalted
	// awaitRunLockDeadline means none of the above happened within the attempt budget.
	awaitRunLockDeadline
)

// awaitRunLock polls at most attempts times for the just-spawned driver to take the run lock.
//
// Each iteration consults the three seams in a fixed order — lockHeld, then alive, then halted —
// and only then calls wait and loops again.
//
// lockHeld comes first because the lock being held is the actual thing the handshake waits for:
// a child that took the lock and is about to exit must still be reported ready, not child-died, and
// checking liveness first would race a child that takes the lock and exits within one poll window.
//
// halted comes last because it is the weakest of the three signals — it says only that the machine
// is no longer running, which is also true of a status file a wedged driver never touched. It exists
// because the run lock alone stopped being able to tell "wedged spawn" from "finished, still
// working": shedengine.Run releases the lock on return, and after a blocked halt `lyx loom run`
// then spends up to friction_timeout_min in the Tier 2 reflection step with the lock free and the
// process very much alive. Without this signal a fast halt plus any friction note is reported as a wedged spawn and
// the bootstrap refuses — see dispositionForHandshake.
//
// lockHeld, alive, halted, and wait are all injected seams so a test can drive this whole poll with
// no real process, no real lock, no real status file, and no wall-clock sleep, which the Test Tier
// Purity Invariant's long-sleep guard would otherwise flag.
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

// handshakeDisposition is what the session bootstrap does with an awaitRunLockResult.
type handshakeDisposition int

const (
	// handshakeProceed means the bootstrap proceeds to its success envelope.
	handshakeProceed handshakeDisposition = iota
	// handshakeRefuse means the bootstrap reports a failure on the envelope.
	handshakeRefuse
)

// dispositionForHandshake maps an awaitRunLockResult onto what the bootstrap should do next.
//
// awaitRunLockChildDied proceeds, and that is the whole point of this function existing rather than
// the verb testing `result != awaitRunLockReady` inline. A child that is already gone before the
// handshake's first poll is a driver that RAN AND FINISHED -- the common case, since a run that
// halts fast (a blocked Preflight or Loom-Preflight, an exhausted bounce budget) exits within
// milliseconds, well inside the first poll interval. Refusing there tells the operator the bootstrap
// broke when in fact their task halted, and withholds the success envelope that is the bootstrap's entire job.
//
// awaitRunLockHalted proceeds for exactly the same reason, and covers the case Tier 2 introduced:
// the driver halted just as fast, but did NOT exit, because after a blocked halt `lyx loom run` runs
// the friction reflection after shedengine.Run has already returned and released the lock. That step spawns a
// real agent bounded by friction_timeout_min -- thirty minutes in the shipped template -- against a
// handshake budget of thirty seconds, so the child is alive, the lock is free, and the machine is
// done. Before this arm existed that combination landed on the refusal below, which turned every
// fast halt carrying a friction note into a reported bootstrap failure:
// precisely the outcome the paragraph above exists to prevent, reintroduced through a different door.
//
// Only awaitRunLockDeadline is a genuine refusal: the child is still alive after the whole attempt
// budget, has never taken the lock, AND the machine never left running -- which is a wedged spawn
// and nothing else.
func dispositionForHandshake(result awaitRunLockResult) handshakeDisposition {
	switch result {
	case awaitRunLockReady, awaitRunLockChildDied, awaitRunLockHalted:
		return handshakeProceed
	default:
		return handshakeRefuse
	}
}

// findStatusStrand returns the first strand in strands that agentname.Matches addresses as name:
// the strand's full name, its role segment, or a legacy exact name.
func findStatusStrand(strands []reedengine.StrandStatus, name string) (reedengine.StrandStatus, bool) {
	for _, s := range strands {
		if agentname.Matches(s.Name, name) {
			return s, true
		}
	}
	return reedengine.StrandStatus{}, false
}

// findDriverStrand returns the first strand loomengine.IsDriverStrand accepts, so a driver recorded under the legacy literal is still found.
func findDriverStrand(strands []reedengine.StrandStatus) (reedengine.StrandStatus, bool) {
	for _, s := range strands {
		if loomengine.IsDriverStrand(s.Name) {
			return s, true
		}
	}
	return reedengine.StrandStatus{}, false
}

// applyStatusStrandSurface is start's status-strand branch on the recorded driver:
// the llm arm calls remove and never ensure, because the loom driver strand is the driving surface and the status band is not wanted;
// the go arm calls ensure and returns its error.
func applyStatusStrandSurface(driver string, remove func(), ensure func() error) error {
	if mustUseLLMDriverArm(driver) {
		remove()
		return nil
	}
	return ensure()
}

// removeStatusStrands removes every strand named statusStrandDisplayName, for the llm arm, where the loom driver strand is the driving surface and the status band is not wanted.
// It removes every match rather than the first, because an older build may have left a duplicate.
// Removal is never recursive: reed refuses a non-recursive removal of a strand with children and removes nothing,
// so a status strand with anything parented beneath it stays up instead of cascading through strands this call never meant to touch.
// It never fails its caller: a failed status read or removal logs a warning and carries on, because a leftover band costs the operator screen rows, not the run (the same stance ensureStatusStrand takes on a failed ReplaceStrand).
func removeStatusStrands(status func() (reedengine.StatusResult, error), remove func(guid string, recursive bool) (reedengine.Removed, error)) {
	st, err := status()
	if err != nil {
		logger.Warn("loomcli: could not read the strand table to remove the status strand; leaving any in place", "cause", err)
		return
	}
	for _, s := range st.Strands {
		if !agentname.Matches(s.Name, statusStrandDisplayName) {
			continue
		}
		if _, err := remove(s.GUID, false); err != nil {
			logger.Warn("loomcli: could not remove a status strand; leaving it up", "guid", s.GUID, "cause", err)
		}
	}
}

// statusStrandAction is what the bootstrap must do about the status strand.
type statusStrandAction int

const (
	// statusStrandKeep means a live status strand is already present; add nothing.
	statusStrandKeep statusStrandAction = iota
	// statusStrandAdd means no strand carries the status strand's name; add one.
	statusStrandAdd
	// statusStrandReplace means a strand carries the name but is not live; remove that stale entry
	// first, then add.
	statusStrandReplace
)

// resolveStatusStrandAction decides what the bootstrap owes the status strand, from reed's tracked
// strands.
//
// Liveness is part of the question, not a detail. Presence alone used to answer it, and reed keeps
// tracking a strand whose pane is gone -- which is exactly the state any reed server restart leaves
// behind: a reboot, a crash, a "tmux kill-server", or reed's own zombie-boot force-reap all leave
// "loom-status" in reed.json with a cleared pane binding. A presence-only check then reported
// "already there" on every subsequent `lyx loom start` in that worktree, so the status strand was
// never re-added and the operator permanently lost the one-line read-out step 2 of the bootstrap
// exists to give them.
//
// A dead entry is replaced rather than simply added over, because reed's add has no upsert semantics:
// a second add under the role the dead entry still holds is refused, which is the very reason statusStrandDisplayName is a pinned constant.
//
// A live strand is kept only when the recorded sidecar names its GUID and equals the current build
// identity; a missing, unreadable or otherwise-mismatched sidecar means the strand may run a
// different lyx build, so it is replaced.
func resolveStatusStrandAction(strands []reedengine.StrandStatus, sidecar *statusSidecar, current buildIdentity) (statusStrandAction, string) {
	strand, found := findStatusStrand(strands, statusStrandDisplayName)
	switch {
	case !found:
		return statusStrandAdd, ""
	case strand.Live && sidecar != nil && sidecar.GUID == strand.GUID && sidecar.Build == current:
		return statusStrandKeep, strand.GUID
	default:
		return statusStrandReplace, strand.GUID
	}
}

// driverStrandAction is what the bootstrap must do about the loom driver session's own strand.
type driverStrandAction int

const (
	// driverStrandNone means no strand carries the driver strand's name: nothing to remove, and the
	// llm arm's own launch decision is unaffected by this signal.
	driverStrandNone driverStrandAction = iota
	// driverStrandLive means a strand carries the name and is live: a driver is already running, and
	// mustSpawnDriver must not launch a second one.
	driverStrandLive
	// driverStrandDead means a strand carries the name but is not live: the corpse must be removed
	// before a relaunch, since reed's add has no upsert semantics.
	driverStrandDead
	// driverStrandRetiring means a strand carries the name, is live and is marked retiring: some caller of "reed remove --detach" already asked to remove it,
	// so start replaces it with a fresh driver instead of adopting it.
	driverStrandRetiring
)

// resolveDriverStrandAction decides what the bootstrap owes the driver strand, from reed's tracked
// strands, modelled on resolveStatusStrandAction. It returns the action and the matched strand's
// GUID; a driverStrandNone action carries no GUID, since nothing was found to act on. Nothing here
// removes anything -- a driverStrandDead result only tells the caller a corpse is present so it can
// remove it before relaunching.
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

// statusStrandCmd composes the status strand's pane command line through the shell seam, exactly as
// the watchdog daemon's own spawn (ensureWatchdogSpawned, internal/reedcli/spawnwatchdog.go) composes
// its os.Executable() command line: exe invoked with the two-word status verb and the watch flag.
func statusStrandCmd(sh shell.Shell, exe string) string {
	return sh.Invoke(exe) + " " + sh.Quote("loom") + " " + sh.Quote("status") + " " + sh.Quote("--watch")
}
