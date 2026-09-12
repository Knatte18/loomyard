// run.go implements the `run` loom verb: the session bootstrap.
// It resolves the recorded parent branch, seeds the status file when absent, commits that seed
// into the fabric, ensures the reed substrate and its status strand, spawns the detached driver when none
// is already alive, waits for the handshake that confirms the driver took the run lock, and finally
// hands the operator's terminal to a tmux attach.
// Every fallible step runs pre-flight, on the envelope; only the terminal handover at the very end
// takes the CLI/Cobra Invariant's narrow interactive-handoff exception.

package loomcli

import (
	"errors"
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

// runCmd builds the `run` subcommand: the session bootstrap.
func (c *loomCLI) runCmd() *cobra.Command {
	var parentFlag string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "bootstrap this worktree's loom task and hand the terminal to the driver session",
		Long: `run is the session bootstrap. It performs four steps in order:

  1. resolve the recorded parent branch, seed the status file when it is
     absent, and commit that seed into the fabric before anything else touches it
  2. ensure the worktree's tmux session is up and its status strand exists
  3. spawn the detached loom driver, unless one is already alive -- a second
     invocation while a driver is running ensures substrate and attaches
     rather than spawning a second one
  4. hand the terminal to the tmux session

The detached driver's own stdout/stderr go to the log the ephemeral-tree
driver-log accessor names, never to this command's own output.

Example:
  lyx loom run
  lyx loom run --parent main`,
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
			_, _, err := c.seedAndCommitBootstrap(slug, parentFlag)
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

			// Step 5: probe the run lock non-blockingly -- releasing it immediately when it was
			// free, never holding it across this probe -- and spawn the detached driver only when
			// mustSpawnDriver says no driver is already alive.
			runLockPath := c.shedPaths.LockPath
			probe, runLockFree, err := lock.TryAcquireWriteLock(runLockPath)
			if err != nil {
				_ = bootstrapLock.Release()
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			if runLockFree {
				_ = probe.Release()
			}
			runLockHeld := !runLockFree

			var childPID int
			if mustSpawnDriver(runLockHeld) {
				exe, err := os.Executable()
				if err != nil {
					_ = bootstrapLock.Release()
					clihelp.SetExit(ctx, output.Err(out, err.Error()))
					return nil
				}
				driverLogPath := loomengine.LoomDriverLog(c.location)
				logFile, err := os.OpenFile(driverLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
				if err != nil {
					_ = bootstrapLock.Release()
					clihelp.SetExit(ctx, output.Err(out, err.Error()))
					return nil
				}
				driveCmd := exec.Command(exe, "loom", "drive")
				driveCmd.Stdout = logFile
				driveCmd.Stderr = logFile
				proc.Detach(driveCmd)
				if err := driveCmd.Start(); err != nil {
					_ = logFile.Close()
					_ = bootstrapLock.Release()
					clihelp.SetExit(ctx, output.Err(out, err.Error()))
					return nil
				}
				logger.Info("loom: spawned detached driver", "pid", driveCmd.Process.Pid, "log", driverLogPath)
				childPID = driveCmd.Process.Pid
				// The log file handle is safe to close here: the child inherited its own
				// duplicated descriptor at Start, so this process's copy is no longer needed.
				_ = logFile.Close()
				// Reap the child as soon as it exits, in the background: this process is still the
				// driver's direct parent (Detach's Setsid only puts it in a new session; the child
				// is re-parented away only once THIS process itself exits), so a driver that
				// finishes before this bootstrap invocation does -- the common case, since a fresh
				// task's Discussion-Validate has nothing to validate yet and bounces to its budget
				// within milliseconds -- would otherwise sit as a zombie. A zombie's pid still
				// answers kill(pid, 0) as "alive", which is exactly the probe proc.IsAlive uses, so
				// leaving this unreaped would make the handshake below spin its entire deadline and
				// falsely refuse a bootstrap whose driver actually completed cleanly.
				go func() { _ = driveCmd.Wait() }()
			}

			// Step 6: still holding the bootstrap lock, wait for the driver to take the run lock.
			if mustSpawnDriver(runLockHeld) {
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
				alive := func() bool { return proc.IsAlive(childPID) }
				wait := func() { time.Sleep(bootstrapHandshakePollInterval) }

				result, err := awaitRunLock(lockHeld, alive, wait, bootstrapHandshakeAttempts)
				if err != nil {
					_ = bootstrapLock.Release()
					clihelp.SetExit(ctx, output.Err(out, err.Error()))
					return nil
				}
				driverLogPath := loomengine.LoomDriverLog(c.location)
				if dispositionForHandshake(result) == handshakeRefuse {
					_ = bootstrapLock.Release()
					clihelp.SetExit(ctx, output.Err(out, "loom: driver did not take the run lock; see "+driverLogPath))
					return nil
				}
				if result == awaitRunLockChildDied {
					// Not a failure: the driver ran to completion and exited before the handshake's
					// first poll, which is what every fast-halting run does. The tmux handover below
					// still happens, because the status strand in that session is where the halt is
					// legible. See dispositionForHandshake for the full argument.
					logger.Info("loom: driver exited before the handshake observed the run lock; its outcome is recorded in the driver log", "pid", childPID, "log", driverLogPath)
				}
			}

			// Step 7: this tail is the CLI/Cobra Invariant's interactive-handoff exception. Steps
			// 1 through 6 are pre-flight precisely so every fallible thing has already been
			// reported before stdio is handed away here.
			_ = bootstrapLock.Release()

			if _, err := c.reed.Status(); err != nil {
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

	return cmd
}

// RunAliasCommand returns the run verb registered a second time, as a bare root child ("lyx run"),
// alongside the full "lyx loom run" subtree.
//
// It builds a fresh receiver and takes that receiver's own runCmd unchanged -- it carries no seam
// functions of its own, because it delegates entirely into the subtree's verb -- and attaches that
// same receiver's resolvePersistentPreRun as the returned command's own PersistentPreRunE. That
// attachment is necessary here, not optional: a root child gets no parent group's PersistentPreRunE
// to inherit, so without it the alias would run with location, cwd, env, and shedPaths all left
// unresolved.
// The group short-circuit inside resolvePersistentPreRun (its cmd.Name() == "loom" check) does not
// fire for this command, since this command's own Name() is "run", never "loom" -- so the alias
// always resolves the full engine stack exactly as "lyx loom run" does.
//
// The alias is not registered inside Command(); the root command registers it as a sibling of the
// "loom" group, in a later batch.
func RunAliasCommand() *cobra.Command {
	c := &loomCLI{}
	cmd := c.runCmd()
	cmd.PersistentPreRunE = c.resolvePersistentPreRun
	return cmd
}
