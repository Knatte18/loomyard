// start.go implements the `start` loom verb: the session bootstrap.
// It resolves the recorded parent branch, seeds the status file when absent, and commits that seed into the fabric.
// It ensures the reed substrate and the session's status strand on either driver.
// It spawns the detached driver when none is alive.
// It waits for the handshake that confirms the driver took the run lock, and it prints the success envelope.
// The verb never attaches or switches a tmux client;
// every step runs on the envelope.

package loomcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/spf13/cobra"
)

// bootstrapHandshakePollInterval and bootstrapHandshakeAttempts bound the handshake's wait for the
// just-spawned driver to take the run lock.
// The deadline, 30s, is generous but finite, so a wedged spawn is reported rather than hung on forever.
// An ordinary boot, well under a second in practice, never comes close to it.
const (
	bootstrapHandshakePollInterval = 100 * time.Millisecond
	bootstrapHandshakeAttempts     = 300
)

// mustUseLLMDriverArm reports whether the run takes the llm arm, from the run's recorded driver.
// It selects the llm arm's strand launch over the go arm's detached spawn.
//
// An unseeded run is seeded with the driver its driver_kind resolves to, llm by default (resolveSeedDriver).
// A seed file with no driver value still reads as the go driver.
// So this is a two-value switch: any value other than shedrun.DriverLLM, empty included, selects the go arm.
func mustUseLLMDriverArm(driver string) bool {
	return driver == shedrun.DriverLLM
}

// startLLMDriverArm performs the llm arm's whole launch.
// It resolves the driver model choice through the loom engine's driver resolver.
// It uses the receiver's config and registry for that.
// When driverAction reports a dead driver strand, it removes that corpse through the pane-probe seam.
// This happens before anything else.
// A relaunch that started first and removed second would leave two strands under one name.
// Reed's add has no upsert semantics to reconcile them.
// It runs its own mkdir-all on the report path's parent directory right before composing the spec.
// It does not rely on an earlier step's mkdir.
// It composes the prompt and the spec.
// The parent-notification rule takes the watched form when the run's batten-watched marker names a live pid.
// Then it starts the run through the starter seam.
//
// It returns the started run's handle for the caller to log.
// StartDriver already guarantees readiness, so a not-ready driver surfaces as StartDriver's own error.
// runDriverSpawnAndWait's failed-start refusal path reports it.
// Shuttle's own message names the run dir and strand.
//
// Accepted residual: a readiness refusal removes the driver strand (shuttle's own not-ready teardown),
// so the next start spawns a fresh one.
// The strand is left in place only in two cases.
// In the first, shuttle's readiness check could not get an answer from reed at all, a startup mechanism failure.
// In the second, shuttle's not-ready teardown could not remove the strand.
// A later start that finds that strand live resolves it as driverStrandLive and attaches without re-checking readiness.
// This is accepted because reed being unable to answer says nothing about the agent.
// Tearing the strand down could kill a working driver.
func (c *loomCLI) startLLMDriverArm(driverAction driverStrandAction, driverGUID string, runID string) (driverHandle, error) {
	choice, err := loomengine.ResolveDriver(c.cfg, c.registry)
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
	spec := driverSpec(prompt, reportPath, choice)

	run, err := c.driverStarter.StartDriver(spec)
	if err != nil {
		return nil, shuttleengine.ModelKey{Module: "loom", Key: "driver"}.Wrap(err)
	}
	// The spawn is logged the way the Live-Substrate Spawn Observability invariant requires.
	// StartDriver guarantees readiness, so this one line records both the spawn and the ready observation.
	logger.Info("loom: driver strand is ready", "guid", run.StrandGUID(), "runDir", run.RunDir())
	return run, nil
}

// readRunStatus reads the run's status file, reporting found false when the file is absent or does not decode.
// An undecodable file is logged rather than refused.
// The driver's own read gate diagnoses it in the driver log, and hand-editing a status file is no way forward.
func (c *loomCLI) readRunStatus() (shedengine.Status, bool, error) {
	st, found, err := state.ReadJSON[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
	if errors.Is(err, state.ErrDecode) {
		logger.Warn("loom: the status file does not decode; deferring its diagnosis to the driver", "path", c.shedPaths.StatusPath, "error", err)
		return shedengine.Status{}, false, nil
	}
	return st, found, err
}

// statusReadFailedMessage is the refusal for a status-file read that failed for a reason other than decoding.
func statusReadFailedMessage(err error) string {
	return "loom: could not read the run's status file: " + err.Error() + `; re-run "` + retryStart + `", and if the failure persists, fix the file or directory the message names`
}

// rerunMessage is the refusal for a transient failure err that mutated nothing, naming retry as the verb to re-run.
func rerunMessage(err error, retry string) string {
	return err.Error() + `; re-run "` + retry + `"`
}

// runHaltedAtHandBack returns the run's persisted state when it is a state a parking driver parks at.
// Those states are awaiting, blocked, paused and failed; any other state yields "".
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

// retryStart and retryResume are the two verbs that put a parked or dead driver back to work.
// A refusal message names one of them for the caller's retry.
const (
	retryStart  = "lym loom start"
	retryResume = "lym loom resume"
)

// driverNotParkedMessage is the refusal for a run halted at a hand-back whose driver has not parked yet;
// retry is the verb the message names for the caller's retry.
func driverNotParkedMessage(handBack shedengine.State, retry string) string {
	return "loom: the driver has not parked yet (the run is " + string(handBack) + " and its driver is still writing its stop report and committing its records); retry `" + retry + "` in a few seconds"
}

// isLandingProducer reports whether producer is one of the two rows that abort and redo their own parked merge-in.
func isLandingProducer(producer string) bool {
	return producer == loomshed.NameParentAlign || producer == loomshed.NameFinalize
}

// ownMergeInLeftover reports whether st is a parked merge-in of parentBranch that the producer aborts and redoes itself:
// a parked `merge-in` sourced from the parent branch, at Parent-Align or Finalize.
func ownMergeInLeftover(st fabricengine.MidMergeState, producer, parentBranch string) bool {
	return st.Kind == fabricengine.MidMergeParked && st.Verb == fabricengine.MergeVerbMergeIn && st.Source == parentBranch && isLandingProducer(producer)
}

// readRecordedParentBranch returns the parent branch the pair's fabric origin record names.
// It returns an error when the record is absent.
func readRecordedParentBranch(l *lymcwd.Location) (string, error) {
	origin, found, err := fabricengine.ReadOrigin(l)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("the pair has no fabric origin record")
	}
	return origin.ParentBranch, nil
}

// currentProducer returns the producer the run's status file names as current.
// It returns "" when the run has no status file or the file does not decode.
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

// parentBranchUnreadableMessage is the refusal for a parked merge-in at Parent-Align or Finalize.
// It applies when the pair's recorded parent branch could not be read.
// The merge then cannot be told apart from the row's own leftover.
// The message names retry, the verb the caller re-runs.
func parentBranchUnreadableMessage(err error, retry string) string {
	return "loom: could not read the pair's recorded parent branch to tell this parked merge-in from the row's own leftover: " + err.Error() + `; with no fabric origin record, run "lym loom start --parent <branch>" in the task worktree, which writes it, otherwise fix the file the message names and re-run "` + retry + `"`
}

// refuseOverUnfinishedMerge probes the pair's merge state and reports whether the verb named retry may proceed.
// producer is the run's current producer.
// A parked merge-in of the recorded parent branch at Parent-Align or Finalize is that row's own leftover;
// the row aborts and redoes it, so it proceeds.
// The parent branch is read only for a parked merge-in at one of those rows,
// and a read failure refuses like a probe error.
// On a refusal or a probe error it releases bootstrapLock and records the envelope;
// it returns false and leaves the park marker on disk.
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
		msg = `loom: a fabric merge is in progress in this worktree; resolve each listed path, mark it resolved with "lym fabric merge-stage <path>...", then run "lym fabric merge --continue" (or "lym fabric merge --abort" to discard the merge), then re-run "` + retry + `"; a session lym refuses the verb from reports status: FAILED and the orch runs it`
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
	st, found, err := state.ReadJSON[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
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

// runDriverSpawnAndWait decides whether the bootstrap must spawn a driver, and starts one or resumes a parked one.
// It probes the run lock, the driver lock and the driver strand table once.
// mustSpawnDriver then decides whether a spawn is needed.
// When one is, it branches on driver.
// The go arm runs a detached `lym loom drive` spawn and the driver-lock handshake;
// the llm arm launches a strand;
// StartDriver already blocks through the provider's readiness gates before it returns.
//
// Before any spawn it removes a stale park marker (shedrun.ParkMarker).
// When no spawn is needed and the driver strand is live with the marker present, the driver is parked,
// so it spawns nothing and resumes that driver through resumeParkedDriver instead.
// A live strand with no marker, or a held driver lock with no live strand, is refused over a run halted at a hand-back;
// the refusal uses the shedrun.StartNotParkedKind kind.
// That driver has not parked yet, or its Go driver is still settling.
// Over a running run it is a no-op, since the driver is working.
//
// On the spawn path only, it writes the handoff voucher right before either arm spawns.
// The write follows the stale-marker removal, for the status file as it stands, when Tier 2 is on.
// The driver's first entry observation then does not read this resume as a crash.
// The voucher suppresses at most one entry observation, the first;
// it does so only when its history length and state equal what start recorded.
// It lives under `.lym`, so it never crosses machines or survives a fabric re-wire.
// A voucher whose spawn then fails its readiness check stays until the next start or completed step overwrites it,
// and it suppresses at most one later matching observation;
// a driver that start spawned and that then dies mid-run is not noted;
// its evidence stays in the driver log and trace.
// The parked-driver resume, the live-strand no-op and every refusal write no voucher.
//
// The pair's merge state is probed before the stale-marker removal, the resume line or any spawn.
// It is probed only when this invocation would put a driver to work:
// mustSpawn, or a live strand with the park marker present.
// An unfinished merge is refused with the shedrun.StartMergeInProgressKind kind;
// a live strand with no marker is never probed, so a working driver is attached to without a merge-state read.
//
// lockHeld is the go arm's handshake seam, built by the caller over the real driver lock in production.
// A test substitutes a counting fake to prove the handshake is never consulted on an llm-seeded bootstrap.
//
// Every failure return releases bootstrapLock before it reports on out's envelope, and returns false;
// the caller then returns at once, without a second release.
// On success it returns true and leaves bootstrapLock held for the caller to release.
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
	driverLockHeld, err := c.driverLockHeld()
	if err != nil {
		_ = bootstrapLock.Release()
		clihelp.SetExit(ctx, output.Err(out, rerunMessage(err, retryStart)))
		return false
	}

	// The strand table is read once here, through the driverPaneProbe.Strands() seam.
	// The one slice feeds both the spawn predicate below and, on the llm arm, the corpse check.
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
	mustSpawn := mustSpawnDriver(runLockHeld, driverLockHeld, driverAction == driverStrandLive)

	// The resume branch reads only strand liveness and the park marker;
	// it never reads the seed's driver value a second time (Driver Choice Single-Site Invariant).
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
		for _, stale := range []string{markerPath, shedrun.ResumeReport(c.location, shedrun.ResolveRunID(c.location, c.runID))} {
			if err := os.Remove(stale); err != nil && !errors.Is(err, os.ErrNotExist) {
				_ = bootstrapLock.Release()
				clihelp.SetExit(ctx, output.Err(out, rerunMessage(err, retryStart)))
				return false
			}
		}
		c.vouchForSpawnedResume()
	} else if driverAction == driverStrandLive || driverLockHeld {
		if parkedLive {
			if _, err := c.resumeParkedDriver(driverGUID, retryStart); err != nil {
				_ = bootstrapLock.Release()
				if errors.Is(err, shuttleengine.ErrSessionBusy) {
					clihelp.SetExit(ctx, output.ErrFields(out, driverBusyMessage(err, retryStart), map[string]any{"kind": shedrun.StartDriverBusyKind}))
					return false
				}
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return false
			}
		} else {
			// No marker: a running run's driver is working, so there is nothing to do.
			// A run halted at a hand-back means the driver is between its stop and its park;
			// a silent success here would resume nothing.
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
		// The seam reaps the child in the background.
		// That matters for a driver that exits within milliseconds, such as a precondition failure on a fresh task.
		// proc.IsAlive would read a zombie as alive, and the handshake below would spin its whole deadline.
		pid, err := c.spawnGoDriver()
		if err != nil {
			_ = bootstrapLock.Release()
			clihelp.SetExit(ctx, output.Err(out, err.Error()))
			return false
		}
		childPID = pid
	}

	// Step 6, go arm: still holding the bootstrap lock, wait for the driver to take the run lock.
	// The handshake stays the go path's alone.
	// This block never runs on the llm arm, is never widened to cover it,
	// and is never moved into a shared helper that reaches it.
	if mustSpawn && !mustUseLLMDriverArm(driver) {
		alive := func() bool { return proc.IsAlive(childPID) }
		// halted reads the machine's own persisted state, the only thing that separates these two cases:
		// a wedged spawn, and a finished pass whose driver still does post-run bookkeeping once the run lock is released.
		// A read failure, and a status file that is not there at all, both report false rather than true:
		// neither is evidence the machine halted, so neither may shortcut the handshake.
		// The deadline stays the arbiter in that case.
		halted := func() bool {
			st, found, readErr := state.ReadJSON[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
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
			clihelp.SetExit(ctx, output.Err(out, "loom: driver did not take its driver lock; see "+driverLogPath+" for why the driver exited; way forward: lym loom start"))
			return false
		}
		if result == awaitRunLockChildDied {
			// Not a failure: the driver ran to completion and exited before the handshake's first poll.
			// Every fast-halting run does this.
			// The bootstrap still reports success, because the status strand in that session is where the halt is legible.
			logger.Info("loom: driver exited before the handshake observed the run lock; its outcome is recorded in the driver log", "pid", childPID, "log", driverLogPath)
		}
		if result == awaitRunLockHalted {
			// This line records which handshake path the bootstrap took when the machine is already halted.
			// On every resume of an already-halted run this arm fires on the handshake's first poll.
			// The driver has not done anything by then, so without this line the log holds no evidence of that path.
			// That also covers a child genuinely wedged before its first persist, not only one doing post-run bookkeeping.
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
		Use:         "start",
		Short:       "bootstrap this worktree's task and launch its driver session",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `start bootstraps this worktree's task and launches its driver: it seeds
the status file and commits it into the fabric when absent, brings up the
worktree's tmux session, and spawns the driver the run's seed records, the
detached Go runner or a Claude strand running the loom driver. With a driver
already alive it ensures the substrate instead of spawning a second one, and
a live loom driver parked at a hand-back is resumed in its own pane. A live
driver strand reed marked retiring is never adopted: start removes it and
spawns a fresh driver in its place.

Reach for it in a task worktree to begin the task, and again after any
hand-back that waits on start to carry the run on.

It returns once the driver is up and never attaches to the session; watch it
with "lym reed attach". The envelope carries the run's driver, slug, run id
and status file.

It refuses with the kind "merge_in_progress" while the worktree holds an
unfinished merge, naming the conflicted paths and the remedy, and with the
kind "driver_not_parked" while a live loom driver over a halted run is still
writing its stop report; retry that one a few seconds later.
It refuses with the kind "owned_elsewhere" over a run another machine owns,
naming the owner and "lym adopt".
While a pause covers this task it refuses and names "lym tender resume".

Example:
  lym loom start
  lym loom start --parent main`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			if err := c.refuseCovered(`run "lym loom start" again`); err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			if c.refuseOwnedElsewhere(ctx, out, `run "lym loom start" again`) {
				return nil
			}

			slug := seedSlug(c.location.WorktreeName)

			// Steps 1 through 3: resolve the parent branch, seed the status file, and verify seed ownership.
			// They also commit the seed and origin record into the fabric.
			// The stage this helper returns is discarded here.
			// `run` writes the same envelope on any failure, whichever sub-step produced it.
			// `step` is the caller that maps the stage onto its own refusal-kind vocabulary.
			// The returned driver is the branch condition for step 4's strand branch and step 5's spawn below.
			// This call is the only read of it: seedAndCommitBootstrap has just written or found this run's seed,
			// so a second shedrun.ReadSeed here would re-read a value already in hand.
			_, driver, _, err := c.seedAndCommitBootstrap(slug, parentFlag)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			// Step 4: take the bootstrap lock, then bring the reed substrate and the status strand up.
			// The lock's parent directory is the ephemeral-tree directory that also holds the run lock and driver log.
			// Creating it here also covers those.
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
			// Released explicitly, not deferred: it must stay held across the spawn and the handshake below,
			// and it is released only once the run lock is observed held.
			// A plain defer would release it at RunE return, far too early.
			// The steps below release it themselves, at the points where it is no longer needed.

			if _, err := c.reed.Up(); err != nil {
				_ = bootstrapLock.Release()
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			if err := c.ensureStatusStrand(); err != nil {
				_ = bootstrapLock.Release()
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			// Every start entry re-arms the viewer's follow state and selects the run's relevant window.
			// This holds whether the status strand was kept, replaced or added.
			// A failure never fails the start: the follow state belongs to the viewer, not to the run.
			if err := c.reed.FollowRun(); err != nil {
				logger.Warn("loom: could not turn the viewer's follow state on", "err", err)
			}

			// This call belongs to step 4 and reports nothing on the envelope.
			// Three placement facts matter here.
			// First, the daemon is per-hub and reconciles a session that exists on every invocation.
			// The detached driver still spawns agent strands that need reconciling.
			// Second, it sits after the status strand is ensured, so both arms reach it,
			// and `step` does not spawn the watchdog.
			// Third, it stays inside the region where the bootstrap lock is still held.
			// The spawn is a MkdirAll, an os.Executable(), and a detached Start with no Wait,
			// so it is bounded and cannot extend the hold the way a wait could.
			// Placing this call outside the lock would release it before the driver-spawn and handshake steps.
			// The lock exists to serialise those steps.
			// The call returns nothing and is never error-checked or reported on the envelope.
			// Every failure path inside the seam logs and returns, and up, attach and resume already treat it as best-effort.
			c.spawnWatchdog(c.location.HubPath, c.reed.TmuxPath(), c.reed.ShellPath(), c.suppressWatchdogSpawn)

			// Steps 5 and 6 probe the run lock and the driver strand table and decide whether a spawn is needed.
			// They then run the arm-specific launch and wait.
			// The go arm's handshake seam is the real driver lock, which `lym loom drive` takes first and holds for life.
			// The lock-held check is a parameter so a test can substitute a counting fake.
			// The fake proves the handshake is never consulted on an llm-seeded bootstrap.
			if !c.runDriverSpawnAndWait(ctx, out, driver, bootstrapLock, c.driverLockHeld) {
				return nil
			}

			// Step 7: release the bootstrap lock and report the success envelope.
			// The verb never attaches or switches a tmux client;
			// attaching is `lym reed attach`.
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
