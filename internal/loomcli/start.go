// start.go implements the `start` loom verb: the session bootstrap.
// It resolves the recorded parent branch, seeds the status file when absent, commits that seed into the fabric, ensures the reed substrate and the session's status strand on every run, spawns the detached driver when none is already alive, waits for the handshake that confirms the driver took the run lock, and finally prints the success envelope.
// The verb never attaches or switches a tmux client;
// every step runs on the envelope.

package loomcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/spf13/cobra"
)

// bootstrapHandshakePollInterval and bootstrapHandshakeAttempts bound the handshake's wait for the
// just-spawned driver to take the run lock: a generous but finite deadline (30s) so a genuinely wedged
// spawn is reported rather than hung on forever, while an ordinary boot -- well under a second in
// practice -- never comes close to it.
const (
	bootstrapHandshakePollInterval = 100 * time.Millisecond
	bootstrapHandshakeAttempts     = 300
)

// mustUseLLMDriverArm reports whether the run takes the llm arm, from the run's recorded driver.
// It selects step 5's spawn: the llm arm's strand launch rather than the go arm's detached spawn.
//
// An unseeded run is seeded with the llm driver (resolveSeedDriver), while a seed file with no driver
// value still reads as the go driver, so this is a two-value switch -- any value other than
// shedrun.DriverLLM, empty included, selects the go arm.
func mustUseLLMDriverArm(driver string) bool {
	return driver == shedrun.DriverLLM
}

// startLLMDriverArm performs the llm arm's whole launch, in order: resolve the driver settings
// through the loom engine's driver resolver, using the config and registry the receiver already
// carries; when driverAction reports a dead driver strand, remove that corpse through the pane-probe
// seam before anything else -- a relaunch that started first and removed second would leave two
// strands under one name, which reed's add has no upsert semantics to reconcile; compose the report
// path and create its parent directory unconditionally with a mkdir-all immediately before composing
// the spec -- this arm's own mkdir, rather than leaning on step 4's, since whether that mkdir's
// parent covers this directory too is a premise this task would otherwise inherit unverified; compose
// the prompt and the spec, the prompt's parent-notification rule being the watched one when the run's batten-watched marker names a live pid; and start the run through the starter seam.
//
// It returns the started run's handle for the caller to log -- StartDriver already guarantees
// readiness, so a not-ready driver surfaces as StartDriver's own error, which
// runDriverSpawnAndWait's existing failed-start refusal path reports with shuttle's own message
// (naming the run dir and strand).
//
// Accepted residual: a readiness refusal removes the driver strand (shuttle's own not-ready teardown),
// so the next start spawns a fresh one. The strand is left in place only when shuttle's readiness
// check could not get an answer from reed at all (a startup mechanism failure), or when shuttle's
// not-ready teardown could not remove the strand, and a later start that finds that strand live
// resolves it as driverStrandLive and attaches without re-checking readiness. Accepted because reed
// being unable to answer says nothing about the agent, so tearing the strand down could kill a working
// driver.
func (c *loomCLI) startLLMDriverArm(driverAction driverStrandAction, driverGUID string, runID string) (driverHandle, error) {
	settings, err := loomengine.ResolveDriver(c.cfg, c.registry)
	if err != nil {
		return nil, err
	}

	if driverAction == driverStrandDead {
		if err := c.driverPaneProbe.RemoveDriverStrand(driverGUID); err != nil {
			return nil, err
		}
	}

	// The session addresses its run by slug, never the literal "self": the prompt and the report path
	// both carry the resolved run-id.
	resolvedRunID := shedrun.ResolveRunID(c.location, runID)
	reportPath := driverReportPath(c.location, resolvedRunID, time.Now, newDriverReportRand())
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return nil, err
	}

	watched := driverWatched(shedrun.BattenWatchedMarker(c.location, resolvedRunID), proc.IsAlive)
	prompt, err := driverPrompt(c.runDeps.Geom.StencilsDir, c.parentName, resolvedRunID, reportPath, watched)
	if err != nil {
		return nil, err
	}
	spec := driverSpec(prompt, reportPath, settings)

	run, err := c.driverStarter.StartDriver(spec)
	if err != nil {
		return nil, err
	}
	// Per the Live-Substrate Spawn Observability invariant, logged exactly as the go arm's own
	// detached spawn already is. StartDriver already guarantees readiness, so this single line
	// covers both the spawn and the "ready" observation the old AwaitStarted-driven log used to carry
	// separately.
	logger.Info("loom: driver strand is ready", "guid", run.StrandGUID(), "runDir", run.RunDir())
	return run, nil
}

// readRunStatus reads the run's status file, reporting found false when the file is absent or does not decode.
// An undecodable file is logged rather than refused, since the driver's own read gate diagnoses it in the driver log and hand-editing a status file is no way forward.
func (c *loomCLI) readRunStatus() (shedengine.Status, bool, error) {
	st, found, err := state.ReadJSONStrict[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
	if errors.Is(err, state.ErrDecode) {
		logger.Warn("loom: the status file does not decode; deferring its diagnosis to the driver", "path", c.shedPaths.StatusPath, "error", err)
		return shedengine.Status{}, false, nil
	}
	return st, found, err
}

// statusReadFailedMessage is the refusal for a status-file read that failed for a reason other than a file that does not decode.
func statusReadFailedMessage(err error) string {
	return "loom: could not read the run's status file: " + err.Error() + `; re-run "` + retryStart + `", and if the failure persists, fix the file or directory the message names`
}

// rerunMessage is the refusal for a transient failure err that mutated nothing, naming retry as the verb to re-run.
func rerunMessage(err error, retry string) string {
	return err.Error() + `; re-run "` + retry + `"`
}

// runHaltedAtHandBack returns the run's persisted state when it is one a parking driver parks at (awaiting, blocked, paused or failed), and "" otherwise.
// An absent or undecodable status file returns "": nothing says the run halted.
func (c *loomCLI) runHaltedAtHandBack() (shedengine.State, error) {
	st, found, err := c.readRunStatus()
	if err != nil {
		return "", err
	}
	if !found {
		return "", nil
	}
	switch st.State {
	case shedengine.StateAwaiting, shedengine.StateBlocked, shedengine.StatePaused, shedengine.StateFailed:
		return st.State, nil
	}
	return "", nil
}

// retryStart and retryResume are the two verbs that put a parked or dead driver back to work, as a refusal message names one for the caller's retry.
const (
	retryStart  = "lyx loom start"
	retryResume = "lyx loom resume"
)

// driverNotParkedMessage is the refusal for a run halted at a hand-back whose driver has not written its park marker yet;
// retry is the verb the message names for the caller's retry.
func driverNotParkedMessage(handBack shedengine.State, retry string) string {
	return "loom: the driver has not parked yet (the run is " + string(handBack) + " and its driver is still writing its stop report and committing its records); retry `" + retry + "` in a few seconds"
}

// isLandingProducer reports whether producer is one of the two rows that abort and redo their own parked merge-in.
func isLandingProducer(producer string) bool {
	return producer == loomshed.NamePublish || producer == loomshed.NameFinalize
}

// ownMergeInLeftover reports whether st is the parked merge-in of parentBranch that producer's row aborts and redoes itself:
// a parked `merge-in` sourced from the parent branch, at Publish or Finalize.
func ownMergeInLeftover(st fabricengine.MidMergeState, producer, parentBranch string) bool {
	return st.Kind == fabricengine.MidMergeParked && st.Verb == fabricengine.MergeVerbMergeIn && st.Source == parentBranch && isLandingProducer(producer)
}

// readRecordedParentBranch returns the parent branch the pair's fabric origin record names, or an error when the record is absent.
func readRecordedParentBranch(l *lyxcwd.Location) (string, error) {
	origin, found, err := fabricengine.ReadOrigin(l)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("the pair has no fabric origin record")
	}
	return origin.ParentBranch, nil
}

// currentProducer returns the producer the run's status file names as current, or "" when the run has no status file or the file does not decode.
func (c *loomCLI) currentProducer() (string, error) {
	st, found, err := c.readRunStatus()
	if err != nil {
		return "", err
	}
	if !found {
		return "", nil
	}
	return st.CurrentProducer, nil
}

// parentBranchUnreadableMessage is the refusal for a parked merge-in at Publish or Finalize whose pair's recorded parent branch could not be read, so the merge cannot be told apart from the row's own leftover;
// retry is the verb the message names for the caller's retry.
func parentBranchUnreadableMessage(err error, retry string) string {
	return "loom: could not read the pair's recorded parent branch to tell this parked merge-in from the row's own leftover: " + err.Error() + `; with no fabric origin record, run "lyx loom start --parent <branch>" in the task worktree, which writes it, otherwise fix the file the message names and re-run "` + retry + `"`
}

// refuseOverUnfinishedMerge probes the pair's merge state and reports whether the verb named retry may proceed.
// producer is the run's current producer: a parked merge-in of the recorded parent branch at Publish or Finalize is that row's own leftover, which the row aborts and redoes, so it proceeds.
// The parent branch is read only for a parked merge-in at one of those rows, and a read failure refuses like a probe error.
// On a refusal or a probe error it releases bootstrapLock, records the envelope and returns false, leaving the park marker on disk.
func (c *loomCLI) refuseOverUnfinishedMerge(ctx context.Context, out io.Writer, bootstrapLock *lock.FileLock, retry, producer string) bool {
	st, err := c.midMerge(c.location)
	if err != nil {
		_ = bootstrapLock.Release()
		clihelp.SetExit(ctx, output.Err(out, rerunMessage(err, retry)))
		return false
	}
	var msg string
	switch st.Kind {
	case fabricengine.MidMergeNone:
		return true
	case fabricengine.MidMergeParked:
		if st.Verb == fabricengine.MergeVerbMergeIn && isLandingProducer(producer) {
			parentBranch, err := c.recordedParentBranch(c.location)
			if err != nil {
				_ = bootstrapLock.Release()
				clihelp.SetExit(ctx, output.Err(out, parentBranchUnreadableMessage(err, retry)))
				return false
			}
			if ownMergeInLeftover(st, producer, parentBranch) {
				return true
			}
		}
		msg = `loom: a fabric merge is in progress in this worktree; resolve each listed path, mark it resolved with "lyx fabric merge-stage <path>...", then run "lyx fabric merge --continue" (or "lyx fabric merge --abort" to discard the merge), then re-run "` + retry + `"`
	default:
		msg = `loom: a git merge, cherry-pick or squash that fabric did not start is in progress in this worktree; conclude or abort it with git, then re-run "` + retry + `"`
	}
	conflicts := st.Conflicts
	if conflicts == nil {
		conflicts = []string{}
	}
	_ = bootstrapLock.Release()
	clihelp.SetExit(ctx, output.ErrFields(out, msg, map[string]any{"kind": shedrun.StartMergeInProgressKind, "conflicts": conflicts}))
	return false
}

// vouchForSpawnedResume records the handoff voucher for the status file as it stands, replacing any earlier voucher.
// It writes only when Tier 2 is on, the gate under which the entry observation consumes the voucher;
// a missing or unreadable status file, or a failed write, is warned and changes nothing else.
func (c *loomCLI) vouchForSpawnedResume() {
	if c.frictionDir == "" {
		return
	}
	st, found, err := state.ReadJSONStrict[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
	if err != nil {
		logger.Warn("loom: could not read the status file to vouch for the spawned resume", "path", c.shedPaths.StatusPath, "cause", err)
		return
	}
	if !found {
		logger.Warn("loom: no status file to vouch for the spawned resume", "path", c.shedPaths.StatusPath)
		return
	}
	recordHandoffVoucher(loomengine.LoomHandoffVoucher(c.location), loomengine.LoomHandoffVoucherLock(c.location), len(st.History), st.State)
}

// runDriverSpawnAndWait performs steps 5 and 6 of the bootstrap: it probes the run lock and the
// driver strand table once, decides via mustSpawnDriver whether a spawn is needed, and -- when one
// is -- branches on driver into the go arm's detached spawn and run-lock handshake (both
// byte-for-byte unchanged from before this extraction, aside from taking lockHeld as a parameter
// rather than building it locally) or the llm arm's strand launch, whose StartDriver already blocks
// through the provider's readiness gates before returning.
//
// Before any spawn it removes a stale park marker (shedrun.ParkMarker).
// When no spawn is needed and the driver strand is live with the marker present, the driver is parked,
// so it spawns nothing and resumes that driver through resumeParkedDriver instead.
// A live strand with no marker over a run halted at a hand-back is refused with the shedrun.StartNotParkedKind kind, since that driver has not parked yet;
// over a running run it is a no-op, since the driver is working.
//
// On the spawn path only, after the stale-marker removal and right before either arm spawns, it writes the handoff voucher for the status file as it stands, when Tier 2 is on, so the driver's first entry observation does not read this deliberate resume as a crash.
// The voucher suppresses at most one entry observation, the first, and only when its history length and state equal what start recorded;
// it lives under `.lyx`, so it never crosses machines or survives a fabric re-wire.
// A voucher whose spawn then fails its readiness check stays until the next start or completed step overwrites it, and suppresses at most one later matching observation;
// a driver that start spawned and that then dies mid-run is not noted, and its evidence stays in the driver log and trace.
// The parked-driver resume, the live-strand no-op and every refusal write no voucher.
//
// Before the stale-marker removal, the resume line or any spawn it probes the pair's merge state, but only when this invocation would put a driver to work: mustSpawn, or a live strand with the park marker present.
// An unfinished merge is refused with the shedrun.StartMergeInProgressKind kind;
// a live strand with no marker is never probed, so a working driver is attached to as before.
//
// lockHeld is the go arm's handshake seam, built by the caller over the real run lock in production;
// a test substitutes a counting fake to prove the handshake is never consulted on an llm-seeded
// bootstrap -- the assertion this batch's own scope note names.
//
// Every failure return releases bootstrapLock explicitly, before reporting on out's envelope, and
// then returns false so the caller returns immediately without a second release. On success it
// returns true, leaving bootstrapLock held for the caller's own step 7 release.
func (c *loomCLI) runDriverSpawnAndWait(ctx context.Context, out io.Writer, driver string, bootstrapLock *lock.FileLock, lockHeld func() (bool, error)) bool {
	runLockPath := c.shedPaths.LockPath
	probe, runLockFree, err := lock.TryAcquireWriteLock(runLockPath)
	if err != nil {
		_ = bootstrapLock.Release()
		clihelp.SetExit(ctx, output.Err(out, rerunMessage(err, retryStart)))
		return false
	}
	if runLockFree {
		_ = probe.Release()
	}
	runLockHeld := !runLockFree

	// The strand table is read once here, through the driverPaneProbe.Strands() seam, and the one
	// slice feeds both consumers: the widened spawn predicate below, and, on the llm arm, the
	// corpse check -- there is no pre-branch strand read in today's start.go to reuse, so this is a
	// new call, made exactly once.
	strands, err := c.driverPaneProbe.Strands()
	if err != nil {
		_ = bootstrapLock.Release()
		clihelp.SetExit(ctx, output.Err(out, rerunMessage(err, retryStart)))
		return false
	}
	driverAction, driverGUID := resolveDriverStrandAction(strands)
	if driverAction == driverStrandRetiring {
		// The detached remover of the old strand no-ops against the fresh strand's new guid.
		if err := c.driverPaneProbe.RemoveDriverStrand(driverGUID); err != nil {
			_ = bootstrapLock.Release()
			clihelp.SetExit(ctx, output.Err(out, rerunMessage(err, retryStart)))
			return false
		}
		driverAction, driverGUID = driverStrandNone, ""
	}
	mustSpawn := mustSpawnDriver(runLockHeld, driverAction == driverStrandLive)

	// The resume branch reads only strand liveness and the park marker, never the seed's driver value a second time (Driver Choice Single-Site Invariant).
	// A spawn first removes a stale marker;
	// a live strand with the marker is a parked driver, resumed with one typed line.
	markerPath := shedrun.ParkMarker(c.location, shedrun.ResolveRunID(c.location, c.runID))
	_, markerStatErr := os.Stat(markerPath)
	parkedLive := !mustSpawn && driverAction == driverStrandLive && markerStatErr == nil

	// A spawn or a resume puts a driver to work, so both refuse over an unfinished merge;
	// a live strand without a marker is working (or still parking) and is left alone.
	if mustSpawn || parkedLive {
		producer, err := c.currentProducer()
		if err != nil {
			_ = bootstrapLock.Release()
			clihelp.SetExit(ctx, output.Err(out, statusReadFailedMessage(err)))
			return false
		}
		if !c.refuseOverUnfinishedMerge(ctx, out, bootstrapLock, retryStart, producer) {
			return false
		}
	}
	if mustSpawn {
		if err := os.Remove(markerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = bootstrapLock.Release()
			clihelp.SetExit(ctx, output.Err(out, rerunMessage(err, retryStart)))
			return false
		}
		c.vouchForSpawnedResume()
	} else if driverAction == driverStrandLive {
		if parkedLive {
			if _, err := c.resumeParkedDriver(driverGUID, retryStart); err != nil {
				_ = bootstrapLock.Release()
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return false
			}
		} else {
			// No marker: a running run's driver is working, so there is nothing to do.
			// A run halted at a hand-back means the driver is between its stop and its park, and a silent success here would resume nothing.
			handBack, err := c.runHaltedAtHandBack()
			if err != nil {
				_ = bootstrapLock.Release()
				clihelp.SetExit(ctx, output.Err(out, statusReadFailedMessage(err)))
				return false
			}
			if handBack != "" {
				_ = bootstrapLock.Release()
				clihelp.SetExit(ctx, output.ErrFields(out, driverNotParkedMessage(handBack, retryStart), map[string]any{"kind": shedrun.StartNotParkedKind}))
				return false
			}
		}
	}

	var childPID int
	if mustSpawn && mustUseLLMDriverArm(driver) {
		_, err = c.startLLMDriverArm(driverAction, driverGUID, c.runID)
		if err != nil {
			_ = bootstrapLock.Release()
			clihelp.SetExit(ctx, output.Err(out, err.Error()))
			return false
		}
	} else if mustSpawn {
		exe, err := os.Executable()
		if err != nil {
			_ = bootstrapLock.Release()
			clihelp.SetExit(ctx, output.Err(out, err.Error()))
			return false
		}
		driverLogPath := loomengine.LoomDriverLog(c.location)
		logFile, err := os.OpenFile(driverLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			_ = bootstrapLock.Release()
			clihelp.SetExit(ctx, output.Err(out, err.Error()))
			return false
		}
		runCmd := exec.Command(exe, "loom", "run")
		runCmd.Stdout = logFile
		runCmd.Stderr = logFile
		proc.Detach(runCmd)
		if err := runCmd.Start(); err != nil {
			_ = logFile.Close()
			_ = bootstrapLock.Release()
			clihelp.SetExit(ctx, output.Err(out, err.Error()))
			return false
		}
		logger.Info("loom: spawned detached driver", "pid", runCmd.Process.Pid, "log", driverLogPath)
		childPID = runCmd.Process.Pid
		// The log file handle is safe to close here: the child inherited its own
		// duplicated descriptor at Start, so this process's copy is no longer needed.
		_ = logFile.Close()
		// Reap the child as soon as it exits, in the background: this process is still the
		// driver's direct parent (Detach's Setsid only puts it in a new session; the child
		// is re-parented away only once THIS process itself exits), so a driver that
		// finishes before this bootstrap invocation does -- the common case, since a fresh
		// task's Preflight and Loom-Preflight gates run in milliseconds and a precondition
		// failure there halts the whole run just as fast -- would otherwise sit as a zombie. A zombie's pid still
		// answers kill(pid, 0) as "alive", which is exactly the probe proc.IsAlive uses, so
		// leaving this unreaped would make the handshake below spin its entire deadline and
		// falsely refuse a bootstrap whose driver actually completed cleanly.
		go func() { _ = runCmd.Wait() }()
	}

	// Step 6, go arm: still holding the bootstrap lock, wait for the driver to take the run
	// lock. The handshake stays the go path's alone -- see this batch's own scope note --
	// so this block is never reached on the llm arm, is never widened to cover it, and is
	// never refactored into a shared helper that reaches it.
	if mustSpawn && !mustUseLLMDriverArm(driver) {
		alive := func() bool { return proc.IsAlive(childPID) }
		// halted reads the machine's own persisted state, which is the only thing that can
		// still separate "wedged spawn" from "pass finished, driver still doing post-run
		// bookkeeping" now that a blocked halt's friction reflection runs after the run lock is
		// released. A read failure, and a status file that is not there at all, both report
		// false rather than true: neither is evidence the machine halted, so neither may
		// shortcut the handshake -- the deadline stays the arbiter in that case.
		halted := func() bool {
			st, found, readErr := state.ReadJSONStrict[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
			if readErr != nil || !found {
				return false
			}
			return st.State != shedengine.StateRunning
		}
		wait := func() { time.Sleep(bootstrapHandshakePollInterval) }

		result, err := awaitRunLock(lockHeld, alive, halted, wait, bootstrapHandshakeAttempts)
		if err != nil {
			_ = bootstrapLock.Release()
			clihelp.SetExit(ctx, output.Err(out, err.Error()))
			return false
		}
		driverLogPath := loomengine.LoomDriverLog(c.location)
		if dispositionForHandshake(result) == handshakeRefuse {
			_ = bootstrapLock.Release()
			clihelp.SetExit(ctx, output.Err(out, "loom: driver did not take the run lock; see "+driverLogPath+" for why the driver exited; way forward: lyx loom start"))
			return false
		}
		if result == awaitRunLockChildDied {
			// Not a failure: the driver ran to completion and exited before the handshake's first poll, which is what every fast-halting run does.
			// The bootstrap still reports success, because the status strand in that session is where the halt is legible.
			// See dispositionForHandshake for the full argument.
			logger.Info("loom: driver exited before the handshake observed the run lock; its outcome is recorded in the driver log", "pid", childPID, "log", driverLogPath)
		}
		if result == awaitRunLockHalted {
			// The halted disposition's breadcrumb, symmetric with the child-died one above:
			// on every resume of an already-halted run this arm fires on the handshake's
			// first poll, before the driver has done anything, so without this line the log
			// carries zero evidence which handshake path the bootstrap took — including in
			// the narrow case where the child is not doing post-run bookkeeping but is
			// genuinely wedged before its first persist (crucible round 2, R2-F3).
			logger.Info("loom: driver is alive with the machine already halted; proceeding to the success envelope while it finishes post-run bookkeeping", "pid", childPID, "log", driverLogPath)
		}
	}

	return true
}

// startCmd builds the `start` subcommand: the session bootstrap.
func (c *loomCLI) startCmd() *cobra.Command {
	var parentFlag string
	var noAttachFlag bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "bootstrap this worktree's loom task and launch its driver session",
		Long: `start is the session bootstrap. It performs four steps in order:

  1. resolve the recorded parent branch, seed the status file when it is
     absent, and commit that seed into the fabric before anything else touches it
  2. ensure the worktree's tmux session is up and its status strand exists;
     then spawn the per-hub watchdog daemon, best-effort
  3. read this run's seed and, unless a driver is already alive, spawn the
     driver its recorded choice selects -- the detached Go runner, or a
     Claude strand running the loom driver in this worktree's own reed session --
     a second invocation while a driver is running ensures substrate
     rather than spawning a second one; which driver runs is the
     seed's recorded choice, never a flag on this command; a live loom
     driver that parked at a hand-back is resumed by typing one line into its
     pane, and start refuses after a bounded wait when that pane is not
     ready, since the driver may be busy having resumed on its own; a live
     loom driver over a run halted at a hand-back (awaiting, blocked,
     paused or failed) that has not written its park marker yet is still
     writing its stop report, so start refuses with the kind
     "driver_not_parked" and is retried a few seconds later, while a live
     driver over a running run is left working; before spawning or resuming
     a driver, start refuses with the kind "merge_in_progress" when the
     worktree carries an unfinished merge, naming the conflicted paths and the
     remedy (the fabric verbs for a fabric merge, git for one fabric did not
     start), except that a run whose current producer is Publish or Finalize
     goes through over a parked fabric merge-in of its own parent branch, which
     that row aborts and redoes, while a live driver that is working is left alone; a live
     loom driver strand that reed marked retiring (some caller of
     "reed remove --detach" already asked to remove it) is never adopted:
     start removes it and spawns a fresh driver in its place
  4. print the success envelope

The detached Go driver's own stdout/stderr go to the log the ephemeral-tree
driver-log accessor names, never to this command's own output -- a loom driver
strand writes no such log, since its own pane is where its output already
lives.

A loom driver strand launches from the driver stencil, read from the
stencils directory at start time.

A worktree opened through "lyx ide spawn"'s generated VS Code task starts
"lyx reed up", then "lyx reed add --if-absent --cmd claude --name claude
--focus", then "lyx reed attach", so the operator's own session is the
strand named "claude" and the panes the run spawns are its siblings. To
self-check, compare $TMUX_PANE against the tracked strands "lyx reed status"
reports: tracked is fine; set but untracked means relaunch through that
chain or proceed without reed supervision; unset is unconfirmed, not
failed, since psmux on Windows may not export it. A worktree whose
.vscode/tasks.json predates this convention is upgraded by deleting that
file and re-running "lyx ide spawn".

start never attaches to the session or switches a tmux client, with or without
$TMUX; "lyx reed attach" is the way to watch the session.
It returns once the driver's readiness signal confirms the driver is up; for a
parked loom driver it returns once the delivery of the resume line is verified.
That readiness signal is the run lock being taken for the Go driver; for a
loom driver, it is the driver's provider TUI coming up ready, with any
one-time startup gate its provider requires dismissed along the way (shuttle's
engine seam owns which gates exist), within shuttle's startup_timeout_s. A
readiness refusal removes the driver strand, so the next start spawns a
fresh one; the signal is checked only for a driver this invocation spawns,
and the two cases that can still leave an unready loom driver strand live --
shuttle could not get a liveness answer from reed at all, or its teardown
could not remove the strand -- are returned over by a later start without
re-checking readiness. The success envelope carries the run's driver, slug,
run id and status file.

Example:
  lyx loom start
  lyx loom start --parent main`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			slug := seedSlug(c.location.WorktreeName)

			// Steps 1 through 3: resolve the parent branch, seed the status file, verify seed
			// ownership, and commit the seed and origin record into the fabric. The stage this
			// helper also returns is deliberately discarded here: `run` writes the same envelope on
			// any failure regardless of which sub-step produced it, exactly as before this
			// extraction; `step` is the caller that maps the stage onto its own refusal-kind
			// vocabulary.
			// The returned driver is the branch condition for step 5's spawn below -- this call is the only read of it:
			// seedAndCommitBootstrap has just written or found this run's seed,
			// so a second shedrun.ReadSeed here would re-read a value already in hand.
			_, driver, _, err := c.seedAndCommitBootstrap(slug, parentFlag)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			// Step 4: take the bootstrap lock, then bring the reed substrate up and ensure the session's status strand.
			// The lock's parent directory is the same ephemeral-tree directory the run lock and driver log also live in,
			// so creating it here also covers those.
			bootstrapLockPath := loomengine.LoomBootstrapLock(c.location)
			if err := os.MkdirAll(filepath.Dir(bootstrapLockPath), 0o755); err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			bootstrapLock, err := lock.AcquireWriteLock(bootstrapLockPath)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			// Released explicitly, not deferred: it must stay held across the spawn AND the
			// handshake below, and is released only once the run lock is observed held -- a plain
			// defer here would release it far too early, at RunE return, rather than at the exact
			// points the steps below release it themselves.

			if _, err := c.reed.Up(); err != nil {
				_ = bootstrapLock.Release()
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			// Both arms keep the status strand: the driver's pane no longer shows the run's steps once the loop runs detached.
			if err := c.ensureStatusStrand(); err != nil {
				_ = bootstrapLock.Release()
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			// Still part of step 4, not a step of its own: this call reports nothing on the
			// envelope, so it earns no "// Step N:" marker, and giving it one would leave a reader
			// wondering why the numbering appears to skip something.
			// Three placement facts matter here.
			// First, the daemon is per-hub and reconciles a session that exists on every invocation,
			// where the detached driver still spawns agent strands that need reconciling.
			// Second, it sits after the status strand, so every run reaches it,
			// and `step` does not spawn the watchdog.
			// Third, it stays inside the region where the bootstrap lock is still held, deliberately: the
			// spawn is a MkdirAll, an os.Executable(), and a detached Start with no Wait, so it is
			// bounded and cannot extend the hold the way a wait could, while releasing the lock
			// earlier to place this call outside it would mean releasing before the driver-spawn
			// and handshake steps the lock exists to serialise. The call returns nothing and is
			// never error-checked or reported on the envelope: every failure path inside the seam
			// logs and returns, and up, attach and resume already treat it as best-effort.
			c.spawnWatchdog(c.location.HubPath, c.reed.TmuxPath(), c.reed.ShellPath(), c.suppressWatchdogSpawn)

			// Steps 5 and 6: probe the run lock and the driver strand table, decide whether a spawn
			// is needed, and run the arm-specific launch and wait. lockHeld is the go arm's
			// handshake seam, built here over the real run lock -- see runDriverSpawnAndWait's own
			// doc comment for why it is a parameter rather than built inside that function.
			runLockPath := c.shedPaths.LockPath
			lockHeld := func() (bool, error) {
				fl, acquired, err := lock.TryAcquireWriteLock(runLockPath)
				if err != nil {
					return false, err
				}
				if acquired {
					_ = fl.Release()
					return false, nil
				}
				return true, nil
			}
			if !c.runDriverSpawnAndWait(ctx, out, driver, bootstrapLock, lockHeld) {
				return nil
			}

			// Step 7: release the bootstrap lock and report the success envelope.
			// The verb never attaches or switches a tmux client;
			// attaching is `lyx reed attach`.
			_ = bootstrapLock.Release()

			runID := shedrun.ResolveRunID(c.location, c.runID)
			output.Ok(out, startEnvelopeFields(driver, slug, runID, c.shedPaths.StatusPath))
			return nil
		},
	}

	cmd.Flags().StringVar(&parentFlag, "parent", "", "write the pair's provenance record once for a worktree created before that record existed; refused when it disagrees with an already-recorded value")
	// noAttachFlag is read by nothing: the verb never attaches,
	// and the flag stays accepted for existing callers.
	cmd.Flags().BoolVar(&noAttachFlag, "no-attach", false, "accepted and ignored; kept for existing callers")
	_ = cmd.Flags().MarkHidden("no-attach")

	return cmd
}

// StartAliasCommand returns the start verb registered a second time, as a bare root child ("lyx
// start"), alongside the full "lyx loom start" subtree.
//
// It builds a fresh receiver and takes that receiver's own startCmd unchanged -- it carries no seam
// functions of its own, because it delegates entirely into the subtree's verb -- and attaches that
// same receiver's resolvePersistentPreRun as the returned command's own PersistentPreRunE. That
// attachment is necessary here, not optional: a root child gets no parent group's PersistentPreRunE
// to inherit, so without it the alias would run with location, cwd, env, and shedPaths all left
// unresolved.
// The group short-circuit inside resolvePersistentPreRun (its cmd.Name() == "loom" check) does not
// fire for this command, since this command's own Name() is "start", never "loom" -- so the alias
// always resolves the full engine stack exactly as "lyx loom start" does.
//
// The alias is not registered inside Command(); the root command registers it as a sibling of the
// "loom" group, in a later batch.
func StartAliasCommand() *cobra.Command {
	c := newLoomCLI()
	cmd := c.startCmd()
	cmd.PersistentPreRunE = c.resolvePersistentPreRun
	return cmd
}
