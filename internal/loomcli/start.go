// start.go implements the `start` loom verb: the session bootstrap.
// It resolves the recorded parent branch, seeds the status file when absent, commits that seed
// into the fabric, ensures the reed substrate and its status strand, spawns the detached driver when none
// is already alive, waits for the handshake that confirms the driver took the run lock, and finally
// hands the operator's terminal to a tmux attach.
// Every fallible step runs pre-flight, on the envelope; only the terminal handover at the very end
// takes the CLI/Cobra Invariant's narrow interactive-handoff exception.

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
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// bootstrapHandshakePollInterval and bootstrapHandshakeAttempts bound the handshake's wait for the
// just-spawned driver to take the run lock: a generous but finite deadline (30s) so a genuinely wedged
// spawn is reported rather than hung on forever, while an ordinary boot -- well under a second in
// practice -- never comes close to it.
const (
	bootstrapHandshakePollInterval = 100 * time.Millisecond
	bootstrapHandshakeAttempts     = 300
)

// mustUseLLMDriverArm reports whether step 5 must take the llm arm's strand launch rather than the
// go arm's detached spawn, from the run's recorded driver.
//
// Per the seed contract an absent driver value already defaults to the go driver on read, so this is
// a two-value switch with the go driver as both the default and the zero-config answer -- any value
// other than shedrun.DriverLLM, empty included, selects the go arm.
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
// the prompt and the spec; and start the run through the starter seam.
//
// It returns the started run's handle so the caller can log the spawn and await the run's readiness
// through the handle's AwaitStarted.
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

	reportPath := driverReportPath(c.location, runID, time.Now, newDriverReportRand())
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return nil, err
	}

	prompt := driverPrompt(runID, reportPath)
	spec := driverSpec(prompt, reportPath, settings)

	run, err := c.driverStarter.StartDriver(spec)
	if err != nil {
		return nil, err
	}
	// Per the Live-Substrate Spawn Observability invariant, logged exactly as the go arm's own
	// detached spawn already is.
	logger.Info("loom: spawned driver strand", "guid", run.StrandGUID(), "runDir", run.RunDir())
	return run, nil
}

// runDriverSpawnAndWait performs steps 5 and 6 of the bootstrap: it probes the run lock and the
// driver strand table once, decides via mustSpawnDriver whether a spawn is needed, and -- when one
// is -- branches on driver into the go arm's detached spawn and run-lock handshake (both
// byte-for-byte unchanged from before this extraction, aside from taking lockHeld as a parameter
// rather than building it locally) or the llm arm's strand launch and readiness await.
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
		clihelp.SetExit(ctx, output.Err(out, err.Error()))
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
		clihelp.SetExit(ctx, output.Err(out, err.Error()))
		return false
	}
	driverAction, driverGUID := resolveDriverStrandAction(strands)
	mustSpawn := mustSpawnDriver(runLockHeld, driverAction == driverStrandLive)

	var childPID int
	var driverRun driverHandle
	if mustSpawn && mustUseLLMDriverArm(driver) {
		driverRun, err = c.startLLMDriverArm(driverAction, driverGUID, c.runID)
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
		// bookkeeping" now that the run lock is released before the friction reflection
		// runs. A read failure, and a status file that is not there at all, both report
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
			clihelp.SetExit(ctx, output.Err(out, "loom: driver did not take the run lock; see "+driverLogPath))
			return false
		}
		if result == awaitRunLockChildDied {
			// Not a failure: the driver ran to completion and exited before the handshake's
			// first poll, which is what every fast-halting run does. The tmux handover below
			// still happens, because the status strand in that session is where the halt is
			// legible. See dispositionForHandshake for the full argument.
			logger.Info("loom: driver exited before the handshake observed the run lock; its outcome is recorded in the driver log", "pid", childPID, "log", driverLogPath)
		}
		if result == awaitRunLockHalted {
			// The halted disposition's breadcrumb, symmetric with the child-died one above:
			// on every resume of an already-halted run this arm fires on the handshake's
			// first poll, before the driver has done anything, so without this line the log
			// carries zero evidence which handshake path the bootstrap took — including in
			// the narrow case where the child is not doing post-run bookkeeping but is
			// genuinely wedged before its first persist (crucible round 2, R2-F3).
			logger.Info("loom: driver is alive with the machine already halted; proceeding to the handover while it finishes post-run bookkeeping", "pid", childPID, "log", driverLogPath)
		}
	}

	// Step 6, llm arm: await the driver run's provider readiness through the handle's
	// AwaitStarted, in place of the go arm's run-lock handshake -- an ly-drive session
	// takes the run lock only inside each "lyx shed step" and releases it between steps,
	// so a handshake on it would either race the Claude boot or observe a free lock between
	// two perfectly healthy steps. AwaitStarted dismisses a recognized one-time startup gate
	// (the workspace-trust and bypass-permissions dialogs) that would otherwise park the
	// session forever on a live pane. Refuse when it reports not-ready, naming the run
	// directory and strand guid the handle reports -- never the driver log accessor, which
	// names only the detached go driver's captured output.
	//
	// Accepted residual, per the discussion's out-of-scope decision on already-live driver
	// strands: a not-ready refusal leaves the strand in place for diagnosis, so if its pane
	// is still live the next `start` resolves it as driverStrandLive through
	// resolveDriverStrandAction, spawns nothing, skips this step entirely and succeeds
	// without re-checking readiness; the operator sees what the pane is stuck on by
	// attaching, and only a newly spawned driver is awaited.
	//
	// bootstrapLock stays held across this await, up to startup_timeout_s, exactly as it was
	// held across the old probe, and the ly-drive session's own first `lyx shed step` waits
	// on the same lock until this bootstrap releases it.
	if mustSpawn && mustUseLLMDriverArm(driver) {
		ready, err := driverRun.AwaitStarted()
		if err != nil {
			_ = bootstrapLock.Release()
			clihelp.SetExit(ctx, output.Err(out, err.Error()))
			return false
		}
		if !ready {
			_ = bootstrapLock.Release()
			clihelp.SetExit(ctx, output.Err(out, fmt.Sprintf("loom: driver strand did not come up; see run dir %s (strand %s)", driverRun.RunDir(), driverRun.StrandGUID())))
			return false
		}
		logger.Info("loom: driver strand is ready", "guid", driverRun.StrandGUID(), "runDir", driverRun.RunDir())
	}

	return true
}

// startCmd builds the `start` subcommand: the session bootstrap.
func (c *loomCLI) startCmd() *cobra.Command {
	var parentFlag string
	var noAttachFlag bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "bootstrap this worktree's loom task and hand the terminal to the driver session",
		Long: `start is the session bootstrap. It performs four steps in order:

  1. resolve the recorded parent branch, seed the status file when it is
     absent, and commit that seed into the fabric before anything else touches it
  2. ensure the worktree's tmux session is up and its status strand exists,
     then spawn the per-hub watchdog daemon, best-effort -- --no-attach
     still performs this spawn
  3. read this run's seed and, unless a driver is already alive, spawn the
     driver its recorded choice selects -- the detached Go runner, or a
     Claude strand running ly-drive in this worktree's own reed session --
     a second invocation while a driver is running ensures substrate and
     attaches rather than spawning a second one; which driver runs is the
     seed's recorded choice, never a flag on this command
  4. add the operator's own strand and then hand the terminal to the tmux session

The detached Go driver's own stdout/stderr go to the log the ephemeral-tree
driver-log accessor names, never to this command's own output -- an ly-drive
strand writes no such log, since its own pane is where its output already
lives.

--no-attach performs steps 1 through 3 and returns once the driver's
readiness signal confirms it is up, instead of running step 4 -- skipping
the terminal handover this way skips the operator's own strand with it. That
readiness signal is the run lock being taken for the Go driver; for an
ly-drive driver, it is the driver's provider TUI coming up ready, with any
one-time startup gate (such as the workspace-trust dialog) dismissed along
the way, within shuttle's startup_timeout_s. This signal is checked only for
a driver this invocation spawns, so an ly-drive strand already live from an
earlier invocation -- including one left in place by an earlier readiness
refusal -- is attached to, or returned over with --no-attach, without
re-checking readiness. The documented meaning is the same on both paths:
perform every bootstrap step, confirm the driver is up by that path's own
signal, and return without the terminal handover.

Example:
  lyx loom start
  lyx loom start --parent main
  lyx loom start --no-attach`,
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
			// vocabulary. The returned driver is step 5's branch condition below -- this call is
			// the only read of it: seedAndCommitBootstrap has just written or found this run's
			// seed, so a second shedrun.ReadSeed here would re-read a value already in hand.
			_, driver, _, err := c.seedAndCommitBootstrap(slug, parentFlag)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			// Step 4: take the bootstrap lock, then ensure the reed substrate and its status
			// strand. The lock's parent directory is the same ephemeral-tree directory the run
			// lock and driver log also live in, so creating it here also covers those.
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

			if err := c.ensureStatusStrand(); err != nil {
				_ = bootstrapLock.Release()
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			// Still part of step 4, not a step of its own: this call reports nothing on the
			// envelope, so it earns no "// Step N:" marker, and giving it one would leave a reader
			// wondering why the numbering appears to skip something. Three placement facts matter
			// here. First, it sits outside this RunE's own mustAttach gate below, unlike the
			// operator strand added at step 7 -- the daemon is per-hub and reconciles a session
			// that exists on every invocation, --no-attach included, where the detached driver
			// still spawns agent strands that need reconciling. Second, it is called here rather
			// than from inside ensureStatusStrand, because that helper lives in
			// sharedbootstrap.go and `lyx loom step` calls it too, and widening the watchdog spawn
			// onto `step` is out of this task's scope. Third, it stays inside the region where the
			// bootstrap lock is still held, deliberately: the spawn is a MkdirAll, an
			// os.Executable(), and a detached Start with no Wait, so it is bounded and cannot
			// extend the hold the way a wait could, while releasing the lock earlier to place this
			// call outside it would mean releasing before the driver-spawn and handshake steps the
			// lock exists to serialise. The call returns nothing and is never error-checked or
			// reported on the envelope: every failure path inside the seam logs and returns, and
			// up, attach and resume already treat it as best-effort.
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

			// Step 7: this tail is the CLI/Cobra Invariant's interactive-handoff exception. Steps
			// 1 through 6 are pre-flight precisely so every fallible thing has already been
			// reported before stdio is handed away here.
			_ = bootstrapLock.Release()

			if !mustAttach(noAttachFlag) {
				return nil
			}

			if _, err := c.reed.Status(); err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			// Add the operator's own strand before the attach below. The position is load-bearing
			// in three ways. First, it is after the bootstrap lock's release, which is already
			// documented above as deliberate. Second, it is inside this mustAttach gate: an
			// invocation that hands no terminal over has no operator to give a pane to, and a
			// tracked idle shell in every CI worktree is debris the watchdog would then keep
			// alive. Third, it is before the term.GetSize call and the AttachArgv it feeds below,
			// because that argv chains a select-layout computed for the current pane count, so
			// adding the strand afterwards would compute a layout for a pane count about to
			// change. A failed add is an ordinary pre-flight error and must not be swallowed into
			// the handover -- doing so would widen the CLI/Cobra Invariant's deliberately narrow
			// interactive-handoff exception.
			if _, err := c.reed.AddStrand(operatorStrandAddSpec()); err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			// Read the operator's own terminal size against stdout, exactly as
			// internal/reedcli's own attach verb does. On error (piped output, no
			// controlling terminal) this does not report on the envelope and does not
			// abort: AttachArgv answers a non-positive cols/rows with the bare argv,
			// exactly today's behaviour, so nothing regresses on a non-TTY. This adds
			// no new fallible step that reports on the envelope, so step 7 keeps its
			// interactive-handoff exception unchanged.
			cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
			if err != nil {
				logger.Warn("loom: no terminal size available, attaching without a chained layout", "err", err)
				cols, rows = 0, 0
			}

			attach := exec.Command(c.reed.TmuxPath(), c.reed.AttachArgv(cols, rows)...)
			attach.Stdin = os.Stdin
			attach.Stdout = os.Stdout
			attach.Stderr = os.Stderr
			logger.Info("loomcli: spawning tmux attach", "tmux", c.reed.TmuxPath(), "cols", cols, "rows", rows)
			if err := attach.Run(); err != nil {
				exitCode := 1
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					exitCode = exitErr.ExitCode()
				}
				logger.Info("loomcli: tmux attach exited", "tmux", c.reed.TmuxPath(), "exitCode", exitCode)
				clihelp.SetExit(ctx, exitCode)
			} else {
				logger.Info("loomcli: tmux attach exited", "tmux", c.reed.TmuxPath(), "exitCode", 0)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&parentFlag, "parent", "", "write the pair's provenance record once for a worktree created before that record existed; refused when it disagrees with an already-recorded value")
	cmd.Flags().BoolVar(&noAttachFlag, "no-attach", false, "return once a driver this invocation spawns is confirmed up (the Go driver has taken the run lock; an ly-drive driver's provider TUI is ready, with any one-time startup gate dismissed), instead of handing the terminal to the session")

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
