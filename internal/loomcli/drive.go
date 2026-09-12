// drive.go implements the `drive` loom verb: the escape hatch that runs the phase machine in the
// foreground, for debugging and CI.
// It ensures the reed substrate and then runs the machine, adding no status strand and handing the
// terminal over to nothing -- those two omissions, not tmux itself, are what separate it from
// `lyx loom run`.

package loomcli

import (
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/frictionengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/spf13/cobra"
)

// driveCmd builds the `drive` subcommand.
func (c *loomCLI) driveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "drive",
		Short: "run loom's phase machine in the foreground, with no status strand and no terminal handover",
		Long: `drive runs loom's phase machine in the foreground: no status strand and
no terminal handover. It is the escape hatch for debugging and CI.

drive is NOT tmux-free. Every LLM row underneath it -- Discussion-Write,
Plan-Write, and all three review segments -- spawns its agent through
shuttle into a reed pane, so a live tmux session is required. drive
ensures that session itself, exactly as "lyx loom run" does, rather than
failing several producers deep once a row first tries to add a strand.
What drive does not do is add the status strand or hand the terminal over.

drive never seeds a status file and never commits anything -- only
"lyx loom run" seeds, because only it owns the commit-before-precondition
ordering the bootstrap needs.

Example:
  lyx loom drive`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			// Pre-flight: refuse on the envelope when the status file does not exist yet, naming
			// the two-word bootstrap verb as the remedy. This refusal exists so the operator is
			// told on the envelope rather than discovering it as the phase machine's own
			// seed-missing precondition failure buried in the detached driver's log -- only
			// "lyx loom run" may seed, because only it owns the commit-before-precondition
			// ordering.
			if _, err := os.Stat(c.shedPaths.StatusPath); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: no status file at "+c.shedPaths.StatusPath+"; run \"lyx loom run\" first to bootstrap this task"))
				return nil
			}
			// The status file must be THIS task's own, for the same reason "lyx loom run" checks:
			// a worktree forked from a task worktree inherits the old task's `_lyx` state, and the
			// phase machine would silently resume the inherited run under the wrong slug (crucible
			// round fable5-high-r3, F-B7).
			if err := loomengine.VerifySeedOwnership(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, seedSlug(c.location.WorktreeName)); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			// Ensure the reed substrate before the first producer call. drive adds no strand and
			// hands no terminal over, but the rows beneath it spawn agents into reed panes, so
			// without a live session the run gets several producers deep and then hard-errors on
			// "no reed session" -- after the Discussion-Bouncer's seed spawn has already failed
			// silently (runSeedSpawn degrades every failure to a warning), had a synthetic empty
			// focus file written over its real one, and consumed a unit of the segment's bounce
			// budget. Up is idempotent and is what "lyx loom run" already calls at its own step 4.
			if _, err := c.reed.Up(); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			handle, err := fabricengine.Open(c.location)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			taskBranch, err := handle.CurrentBranch()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			originURL, err := handle.OriginURL()
			if err != nil {
				// scalar-read-errors-refuse-or-defer-by-consumer: only Publish reads OriginURL, and only
				// when a pull request is actually required, so an unusable origin URL passes through as
				// an empty string rather than refusing drive itself.
				originURL = ""
			}
			recorded, found, err := fabricengine.ReadOrigin(c.location)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			parentBranch, err := resolveLandingParent(recorded, found, taskBranch)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			syncOpts := fabricengine.EnvSyncOptions()
			pushBranch := func() error {
				_, err := handle.PushBranch(syncOpts)
				return err
			}
			c.env.Landing = landingDeps(
				c.location,
				c.runDeps.Geom,
				taskBranch,
				originURL,
				parentBranch,
				syncOpts.SkipPush,
				pushBranch,
				c.registry,
				c.runner,
				c.landingCfg,
			)

			// Ensure the friction directory before the run starts, and never clear it: drive requires
			// an already-seeded task (VerifySeedOwnership above), so a drive-only invocation is by
			// definition a resume, and frictionengine.Reflect renames the directory away on a clean
			// reflection -- including on the RunBlocked trigger -- so a
			// blocked -> operator investigates -> lyx loom drive sequence would otherwise run with no
			// friction directory at all and lose every note silently.
			friction.EnsureDir(c.frictionDir)

			shed, err := loomrecipe.New(c.env, c.shedPaths)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			// Run's already-running sentinel (shedengine.ErrShedBusy) is treated as an ordinary
			// error envelope rather than a special case here: a second driver against the same
			// status file is a real refusal, not a race to tolerate.
			result, err := shed.Run(cmd.Context())
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			// The reflection step fires here, after shed.Run has already returned and after
			// RunDone has already merged and published: self-report filing is out-of-band
			// bookkeeping, never a gate on landing the work. It fires only on RunDone or
			// RunBlocked, never RunPaused (the run is not over -- re-filing on every pause would
			// be noise) and never when err is non-nil (an engine-level fault leaves the run's own
			// bookkeeping untrustworthy) -- structurally guaranteed here since a non-nil err from
			// shed.Run already returned above. It can never fire from a recipe row: shedengine.Run
			// returns immediately on RunBlocked without calling any further producer, so a row can
			// structurally never cover the stuck half of these two trigger points.
			frictionStatus := frictionengine.StatusSkipped
			if shouldReflectFriction(c.frictionDir, result.Outcome) {
				frictionStatus = c.reflectFriction()
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"outcome":         string(result.Outcome),
				"halted_producer": result.HaltedProducer,
				"reason":          result.Reason,
				"history_length":  len(result.History),
				"friction":        frictionStatus,
			}))
			return nil
		},
	}
}

// shouldReflectFriction reports whether driveCmd's RunE should fire the friction reflection step: a
// non-empty friction directory and an outcome of shedengine.RunDone or shedengine.RunBlocked.
// It is the pure decision the reflection call site gates on, factored out so a test can drive every
// outcome without a real Shed.
func shouldReflectFriction(frictionDir string, outcome shedengine.RunOutcome) bool {
	if frictionDir == "" {
		return false
	}
	return outcome == shedengine.RunDone || outcome == shedengine.RunBlocked
}

// reflectFriction builds Deps from c's own already-resolved fields and calls frictionengine.Reflect,
// returning the envelope's "friction" status. A non-nil error from Reflect is a Deps-validation
// failure -- a wiring bug -- and is logged rather than surfaced: failing a successful, already-merged
// run because an optional bookkeeping agent could not run is strictly worse than filing nothing, and
// RunBlocked is worse still -- an operator staring at a blocked run does not need a second, unrelated
// failure layered on top.
func (c *loomCLI) reflectFriction() string {
	report, err := frictionengine.Reflect(frictionengine.Deps{
		Shuttle:       c.runner,
		FrictionDir:   c.frictionDir,
		ArchivePrefix: loomengine.LoomFrictionArchivePrefix(c.location),
		StencilsDir:   c.runDeps.Geom.StencilsDir,
		FrictionSpec:  c.cfg.Friction,
		Registry:      c.registry,
		Timeout:       time.Duration(c.cfg.FrictionTimeoutMin) * time.Minute,
	})
	if err != nil {
		logger.Warn("loom: friction reflection failed", "dir", c.frictionDir, "error", err)
		return frictionengine.StatusFailed
	}
	return report.Status
}
