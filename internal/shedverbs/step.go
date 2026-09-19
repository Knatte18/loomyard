// step.go implements the generic `step` verb body -- the single-producer primitive an external
// supervisor drives one call at a time -- and declares the closed refusal-kind vocabulary its
// envelope's "kind" field reports.

package shedverbs

import (
	"errors"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/spf13/cobra"
)

// The closed refusal-kind vocabulary step reports on the envelope's "kind" field. This set is
// closed at five: StepKinds below lists all of them, and a test asserts the set is exactly this and
// no larger. The skill's one-retry rule applies to KindProducer alone -- every other kind is handed
// straight back to the operator with no retry, because none of them can be fixed by running the same
// command again: KindBusy means a driver already holds the run lock, KindUnseeded and KindOwnership
// mean the bootstrap needs an operator decision (a --parent flag, or a mismatched worktree), and
// KindBootstrap covers every other pre-producer failure, none of which a bare re-invocation resolves.
const (
	// KindBusy means the run lock is already held by a live driver or another `step` invocation.
	KindBusy = "busy"
	// KindUnseeded means the status file could not be seeded -- the bootstrap's seed sub-step
	// failed for a reason other than the file already existing.
	KindUnseeded = "unseeded"
	// KindOwnership means the seeded status file belongs to a different task's slug.
	KindOwnership = "ownership"
	// KindBootstrap means a BuildShed failure, or any other pre-producer setup failure a module's
	// own PreStep hook classifies as unclassifiable, occurred somewhere before the producer call.
	KindBootstrap = "bootstrap"
	// KindProducer means shed.Step's own producer call returned a hard error. This is the one kind
	// the supervisor skill may retry once, since the status file already records the failure
	// verbatim and a retry re-calls the same producer from the same persisted state.
	KindProducer = "producer"
)

// StepKinds lists every value step's RunE can emit as the envelope's "kind" field. A test asserts
// this set is exactly the five declared constants above, so an undeclared sixth kind cannot ship
// silently.
var StepKinds = []string{KindBusy, KindUnseeded, KindOwnership, KindBootstrap, KindProducer}

// StepEnvelope builds step's success envelope from res -- the StepResult shed.Step returned --
// alongside nextPolicy (spec.Hooks.InterruptPolicyFor(res.Next), or the empty string when the hook
// is nil) and statusFile (the shed's own StatusPath). The returned map carries exactly the ten
// documented keys below; the key set is closed -- a key outside these ten has no test and no
// documented meaning:
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
func StepEnvelope(res shedengine.StepResult, nextPolicy, statusFile string) map[string]any {
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

// stepCmd builds the generic `step` subcommand: the single-producer primitive an external
// supervisor drives. This body owns nothing above shed.Step -- any run-lock probe, bootstrap, or
// status-strand work a module needs belongs entirely to its own PreStep hook.
func stepCmd(texts VerbTexts, spec *Spec) *cobra.Command {
	return &cobra.Command{
		Use:   texts.Step.Use,
		Short: texts.Step.Short,
		Long:  texts.Step.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			if spec.Hooks.PreStep != nil {
				if kind, err := spec.Hooks.PreStep(ctx); err != nil {
					clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), map[string]any{"kind": kind}))
					return nil
				}
			}

			if spec.BuildShed == nil {
				clihelp.SetExit(ctx, output.ErrFields(out, "shedverbs: step: no BuildShed constructor configured", map[string]any{"kind": KindBootstrap}))
				return nil
			}
			shed, err := spec.BuildShed()
			if err != nil {
				clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), map[string]any{"kind": KindBootstrap}))
				return nil
			}

			res, err := shed.Step(ctx)
			if err != nil {
				if errors.Is(err, shedengine.ErrShedBusy) {
					msg := err.Error()
					if spec.StepBusyMessage != "" {
						msg = spec.StepBusyMessage
					}
					clihelp.SetExit(ctx, output.ErrFields(out, msg, map[string]any{"kind": spec.StepBusyKind}))
					return nil
				}
				clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), map[string]any{"kind": KindProducer}))
				return nil
			}

			if spec.Hooks.PostStep != nil {
				spec.Hooks.PostStep(res)
			}

			nextPolicy := ""
			if spec.Hooks.InterruptPolicyFor != nil {
				nextPolicy = spec.Hooks.InterruptPolicyFor(res.Next)
			}
			clihelp.SetExit(ctx, output.Ok(out, StepEnvelope(res, nextPolicy, spec.StatusPath)))
			return nil
		},
	}
}
