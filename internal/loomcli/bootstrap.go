// bootstrap.go implements the session bootstrap's pure decisions: whether a driver must be spawned,
// the handshake that waits for the spawned driver to take the run lock, the status strand's lookup
// by its fixed name, and the one command-string builder the bootstrap composes over. Every piece
// here is a pure function or takes injected seams, so the verb body in start.go is assembly over
// judgment that is already under test -- no real lock, no real process, and no real clock is needed
// to exercise any of it.

package loomcli

import (
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
)

// statusStrandDisplayName is the status strand's stable identity: the reed engine's add operation
// has no upsert semantics, so a second add with the same display name would append a second pane
// rather than replace the first. Every add and every lookup must use this exact constant so the
// bootstrap's re-entrant call finds the pane it created on an earlier invocation instead of
// duplicating it.
const statusStrandDisplayName = "loom-status"

// operatorStrandDisplayName is the operator's own strand's stable identity, pinned for the same
// three reasons statusStrandDisplayName is: it is the --if-absent match key operatorStrandAddSpec's
// IfAbsent option matches against, it is the name an operator sees for their own pane in
// `lyx reed status`, and it must stay byte-stable across versions -- reed's add has no upsert
// semantics, so a re-run under a changed literal would stack a second pane instead of matching the
// first.
const operatorStrandDisplayName = "loom-operator"

// driverStrandDisplayName is the ly-drive session's own strand's stable identity, pinned for the
// same reason statusStrandDisplayName and operatorStrandDisplayName are: reed's add has no upsert
// semantics, so a second add under this same display name would append a second pane rather than
// replace the first. Every add and every lookup must use this exact constant, or a re-entrant
// bootstrap stacks a second driver pane instead of matching the one already running.
const driverStrandDisplayName = "loom-driver"

// operatorStrandAddSpec builds the operator strand's reedengine.AddSpec: a below-parent pane that
// reuses an already-present entry rather than duplicating it, takes no focus, and never collapses.
//
// Four choices are non-default and each is pinned deliberately. Cmd is left at its zero value
// because a Strand's Cmd is typed into an already-running shell via send-keys, not passed as a
// trailing split-window argument -- naming a shell here would nest one shell inside another and make
// `lyx reed resume` stack a third; the pane instead runs whatever shell tmux gives a freshly split
// pane. IfAbsent is true because `lyx loom start` is explicitly re-entrant and the engine already
// implements the needed no-op / relaunch-dead / fall-through-to-add behaviour, so this must not
// repeat resolveStatusStrandAction's older manual keep/replace/add dance. Focus is false because
// Display.Focus is persisted on the strand and re-evaluated on every subsequent AddStrand, so a true
// value would re-capture focus on every agent-pane spawn for the rest of the run -- the operator
// still lands in their own pane at cold bootstrap for free, via the bottom-most default.
// ShrinkWhenWaitingOnChild is false as a declaration of intent rather than a rendering change: the
// flag is inert for a parentless, childless strand, and setting it true would accidentally encode
// that the operator's own pane may collapse to a one-row strip.
func operatorStrandAddSpec() reedengine.AddSpec {
	return reedengine.AddSpec{
		NameOverride: operatorStrandDisplayName,
		IfAbsent:     true,
		Display: render.Display{
			Anchor:                   render.AnchorBelowParent,
			Focus:                    false,
			ShrinkWhenWaitingOnChild: false,
		},
	}
}

// mustSpawnDriver reports whether the bootstrap must spawn a new driver, from the run lock's held
// state AND whether a live driver strand already exists.
//
// The conjunction is required on BOTH paths, not one signal per path, because each signal is blind
// to exactly the driver kind the other sees. An operator may run `lyx loom run` by hand against an
// `llm`-seeded run: that spawns no driver strand at all, so a bootstrap consulting the strand table
// alone would find none and launch a Claude driver alongside the live Go one. And a live ly-drive
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

// mustAttach reports whether the bootstrap must hand the terminal over to the tmux session, from the
// operator's own --no-attach choice. It is the twin of mustSpawnDriver: both are the whole of a
// re-entrancy or handoff decision, expressed as one pure predicate rather than written inline in the
// verb body.
//
// The terminal handover this predicate gates is the CLI/Cobra Invariant's narrow interactive-handoff
// exception for `lyx loom start`/`lyx start`. Skipping it on noAttach's say-so removes that exception
// for this one invocation -- every step before it, including the run-lock handshake, still runs --
// rather than adding a new exception of its own.
func mustAttach(noAttach bool) bool {
	return !noAttach
}

// noAttachFields builds the success envelope `lyx loom start --no-attach` prints once the driver is
// confirmed up: the run's driver, slug and status file, with "attached": false stating which tail
// was skipped. It is a pure function so a Tier 1 test can pin the key set.
func noAttachFields(driver, slug, statusFile string) map[string]any {
	return map[string]any{
		"attached":    false,
		"driver":      driver,
		"slug":        slug,
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
// the bootstrap skips its own terminal handover — see dispositionForHandshake.
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
	// handshakeProceed means the bootstrap continues to its terminal handover.
	handshakeProceed handshakeDisposition = iota
	// handshakeRefuse means the bootstrap reports a failure and hands the terminal over to nothing.
	handshakeRefuse
)

// dispositionForHandshake maps an awaitRunLockResult onto what the bootstrap should do next.
//
// awaitRunLockChildDied proceeds, and that is the whole point of this function existing rather than
// the verb testing `result != awaitRunLockReady` inline. A child that is already gone before the
// handshake's first poll is a driver that RAN AND FINISHED -- the common case, since a run that
// halts fast (a blocked Preflight or Loom-Preflight, an exhausted bounce budget) exits within
// milliseconds, well inside the first poll interval. Refusing there tells the operator the bootstrap
// broke when in fact their task halted, and skips the tmux handover that is the bootstrap's entire
// job, so the one place the halt is legible -- the status strand sitting in the session -- is the
// one place they are not put.
//
// awaitRunLockHalted proceeds for exactly the same reason, and covers the case Tier 2 introduced:
// the driver halted just as fast, but did NOT exit, because after a blocked halt `lyx loom run` runs
// the friction reflection after shedengine.Run has already returned and released the lock. That step spawns a
// real agent bounded by friction_timeout_min -- thirty minutes in the shipped template -- against a
// handshake budget of thirty seconds, so the child is alive, the lock is free, and the machine is
// done. Before this arm existed that combination landed on the refusal below, which turned every
// fast halt carrying a friction note into a reported bootstrap failure with no terminal handover:
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

// findStatusStrand returns the first strand in strands whose Name exactly matches name.
func findStatusStrand(strands []reedengine.StrandStatus, name string) (reedengine.StrandStatus, bool) {
	for _, s := range strands {
		if s.Name == name {
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
// A dead entry is replaced rather than simply added over, because reed's add has no upsert
// semantics: a second add under the same display name appends a second pane instead of replacing the
// first, which is the very reason statusStrandDisplayName is a pinned constant.
func resolveStatusStrandAction(strands []reedengine.StrandStatus) (statusStrandAction, string) {
	strand, found := findStatusStrand(strands, statusStrandDisplayName)
	switch {
	case !found:
		return statusStrandAdd, ""
	case strand.Live:
		return statusStrandKeep, strand.GUID
	default:
		return statusStrandReplace, strand.GUID
	}
}

// driverStrandAction is what the bootstrap must do about the ly-drive session's own strand.
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
)

// resolveDriverStrandAction decides what the bootstrap owes the driver strand, from reed's tracked
// strands, modelled on resolveStatusStrandAction. It returns the action and the matched strand's
// GUID; a driverStrandNone action carries no GUID, since nothing was found to act on. Nothing here
// removes anything -- a driverStrandDead result only tells the caller a corpse is present so it can
// remove it before relaunching.
func resolveDriverStrandAction(strands []reedengine.StrandStatus) (driverStrandAction, string) {
	strand, found := findStatusStrand(strands, driverStrandDisplayName)
	switch {
	case !found:
		return driverStrandNone, ""
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
