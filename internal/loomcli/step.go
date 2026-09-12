// step.go implements the `step` loom verb: the single-producer primitive an external supervisor
// drives. It bootstraps idempotently exactly as `run` does, probes the run lock before doing that
// bootstrap work so a live driver is never disturbed, then calls shedengine.Shed's own Step exactly
// once and reports the result as a JSON envelope. Unlike `run`, it spawns no detached driver, runs no
// handshake, and hands the terminal to nothing.

package loomcli

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/spf13/cobra"
)

// The closed refusal-kind vocabulary `step` reports on the envelope's "kind" field. This set is
// closed at five: stepKinds below lists all of them, and a test asserts the set is exactly this and
// no larger. The skill's one-retry rule applies to stepKindProducer alone -- every other kind is
// handed straight back to the operator with no retry, because none of them can be fixed by running
// the same command again: stepKindBusy means a driver already holds the run lock, stepKindUnseeded
// and stepKindOwnership mean the bootstrap needs an operator decision (a --parent flag, or a
// mismatched worktree), and stepKindBootstrap covers every other pre-producer failure, none of which
// a bare re-invocation resolves.
const (
	// stepKindBusy means the run lock is already held by a live driver or another `step` invocation.
	stepKindBusy = "busy"
	// stepKindUnseeded means the status file could not be seeded -- the bootstrap's seed sub-step
	// failed for a reason other than the file already existing.
	stepKindUnseeded = "unseeded"
	// stepKindOwnership means the seeded status file belongs to a different task's slug.
	stepKindOwnership = "ownership"
	// stepKindBootstrap means an origin-record, commit, lock, or reed-substrate failure occurred
	// somewhere in the pre-producer setup, outside the seed and ownership sub-steps that get their
	// own more specific kinds.
	stepKindBootstrap = "bootstrap"
	// stepKindProducer means shed.Step's own producer call returned a hard error. This is the one
	// kind the supervisor skill may retry once, since the status file already records the failure
	// verbatim and a retry re-calls the same producer from the same persisted state.
	stepKindProducer = "producer"
)

// stepKinds lists every value stepCmd's RunE can emit as the envelope's "kind" field. A test asserts
// this set is exactly the five declared constants, so an undeclared sixth kind cannot ship silently.
var stepKinds = []string{stepKindBusy, stepKindUnseeded, stepKindOwnership, stepKindBootstrap, stepKindProducer}

// stepKindForBootstrapStage maps a bootstrapStage -- seedAndCommitBootstrap's own classification of
// where in the bootstrap a failure occurred -- onto step's refusal-kind vocabulary.
// bootstrapStageSeed maps to stepKindUnseeded and bootstrapStageOwnership maps to stepKindOwnership,
// the two sub-steps with a more specific kind; bootstrapStageOrigin and bootstrapStageCommit both map
// to stepKindBootstrap, as does every other value including bootstrapStageNone, so an unclassifiable
// failure still carries a kind rather than an empty one.
func stepKindForBootstrapStage(stage bootstrapStage) string {
	switch stage {
	case bootstrapStageSeed:
		return stepKindUnseeded
	case bootstrapStageOwnership:
		return stepKindOwnership
	default:
		return stepKindBootstrap
	}
}

// stepEnvelope builds step's success envelope from res -- the StepResult shed.Step returned --
// alongside nextPolicy (loomshed.InterruptPolicyFor(res.Next)) and statusFile (the shed's own
// StatusPath). The returned map carries exactly the ten documented keys below; the key set is closed
// -- a key outside these ten has no test and no documented meaning:
//
//   - producer: res.Producer
//   - outcome: string(res.Outcome)
//   - output: res.Output
//   - next: res.Next
//   - state: string(res.State)
//   - reason: res.Reason
//   - continue: derived as res.State == shedengine.StateRunning
//   - history_length: len(res.History)
//   - next_interrupt_policy: nextPolicy
//   - status_file: statusFile
//
// "continue" is derived here, rather than left to the caller, so a thin external supervisor skill
// never carries its own copy of the State vocabulary -- it only ever branches on this one boolean.
func stepEnvelope(res shedengine.StepResult, nextPolicy, statusFile string) map[string]any {
	return map[string]any{
		"producer":              res.Producer,
		"outcome":               string(res.Outcome),
		"output":                res.Output,
		"next":                  res.Next,
		"state":                 string(res.State),
		"reason":                res.Reason,
		"continue":              res.State == shedengine.StateRunning,
		"history_length":        len(res.History),
		"next_interrupt_policy": nextPolicy,
		"status_file":           statusFile,
	}
}

// stepCmd builds the `step` subcommand: the single-producer primitive an external supervisor drives.
func (c *loomCLI) stepCmd() *cobra.Command {
	var parentFlag string

	cmd := &cobra.Command{
		Use:   "step",
		Short: "bootstrap idempotently and drive exactly one producer, reporting a JSON envelope",
		Long: `step bootstraps this worktree's loom task exactly as "lyx loom run" does --
seeding the status file when absent and committing it into the fabric --
and then drives exactly one producer through shedengine.Shed's own Step.

step spawns no detached driver, hands the terminal to nothing, and loops
over nothing: it is the single-producer primitive an external supervisor
drives, one invocation at a time, reading the returned envelope's
"continue" and "next_interrupt_policy" fields to decide what to do next.

Example:
  lyx loom step
  lyx loom step --parent main`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			slug := seedSlug(c.location.WorktreeName)

			// The early run-lock probe, before the bootstrap. The MkdirAll here is part of the
			// probe rather than an accident of ordering: the run lock lives in the ephemeral
			// tree, internal/lock opens with O_CREATE but never creates a parent, and `run`
			// creates that same directory at its own step 4 before its own step-5 probe -- so
			// hoisting only the probe would run it on a fresh worktree whose parent directory
			// does not exist. Treating a missing-parent error as "lock free" is explicitly
			// rejected here, because it would convert a filesystem fault into a false green
			// light on the one check guarding against a second driver.
			if err := os.MkdirAll(filepath.Dir(c.shedPaths.LockPath), 0o755); err != nil {
				clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), map[string]any{"kind": stepKindBootstrap}))
				return nil
			}
			// Without this early probe, step would seed, commit, bring up reed, and churn the
			// status strand against a task a live driver owns -- and the strand work in
			// particular is a real side effect on a running session, since it can remove and
			// re-add the pane a driver's operator is watching.
			probe, runLockFree, err := lock.TryAcquireWriteLock(c.shedPaths.LockPath)
			if err != nil {
				clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), map[string]any{"kind": stepKindBootstrap}))
				return nil
			}
			if runLockFree {
				_ = probe.Release()
			} else {
				clihelp.SetExit(ctx, output.ErrFields(out, "loom: a driver already holds the run lock; run \"lyx loom pause\" to request a pause at the next producer boundary", map[string]any{"kind": stepKindBusy}))
				return nil
			}

			_, stage, err := c.seedAndCommitBootstrap(slug, parentFlag)
			if err != nil {
				clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), map[string]any{"kind": stepKindForBootstrapStage(stage)}))
				return nil
			}

			bootstrapLockPath := loomengine.LoomBootstrapLock(c.location)
			if err := os.MkdirAll(filepath.Dir(bootstrapLockPath), 0o755); err != nil {
				clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), map[string]any{"kind": stepKindBootstrap}))
				return nil
			}
			bootstrapLock, err := lock.AcquireWriteLock(bootstrapLockPath)
			if err != nil {
				clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), map[string]any{"kind": stepKindBootstrap}))
				return nil
			}
			if err := c.ensureStatusStrand(); err != nil {
				_ = bootstrapLock.Release()
				clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), map[string]any{"kind": stepKindBootstrap}))
				return nil
			}
			// Released immediately, before the producer call: the lock must never be held across
			// a minutes-long LLM row, which is why it is released here rather than deferred to
			// RunE's return.
			_ = bootstrapLock.Release()

			shed, err := c.buildLoomShed()
			if err != nil {
				clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), map[string]any{"kind": stepKindBootstrap}))
				return nil
			}

			// The early probe above is an optimisation, not the authority -- shedengine.Step's
			// own acquisition is what guarantees mutual exclusion, so a driver starting in the
			// window between the probe and that acquisition is benign: the run is still
			// refused, and the only cost is bootstrap work that is idempotent.
			res, err := shed.Step(ctx)
			if err != nil {
				if errors.Is(err, shedengine.ErrShedBusy) {
					clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), map[string]any{"kind": stepKindBusy}))
					return nil
				}
				// A producer hard error mirrors drive's handling of the same condition, and the
				// status file already records state: "failed" with the error text, so the
				// supervisor loses nothing.
				clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), map[string]any{"kind": stepKindProducer}))
				return nil
			}

			nextPolicy := loomshed.InterruptPolicyFor(res.Next)
			clihelp.SetExit(ctx, output.Ok(out, stepEnvelope(res, nextPolicy, c.shedPaths.StatusPath)))
			return nil
		},
	}

	cmd.Flags().StringVar(&parentFlag, "parent", "", "write the pair's provenance record once for a worktree created before that record existed; refused when it disagrees with an already-recorded value")

	return cmd
}
