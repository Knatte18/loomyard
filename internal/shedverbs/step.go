// step.go implements the generic `step` verb body -- the single-producer primitive an external
// supervisor drives one call at a time -- and declares the closed refusal-kind vocabulary its
// envelope's "kind" field reports.

package shedverbs

import (
	"errors"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/spf13/cobra"
)

// The closed refusal-kind vocabulary step reports on the envelope's "kind" field. This set is
// closed at five: StepKinds below lists all of them, and a test asserts the set is exactly this and
// no larger.
// The ly-drive skill (plugins/ly/skills/ly-drive/SKILL.md) is the single place a driver's
// disposition per kind is stated.
const (
	// KindBusy means the run lock is already held by a live driver or another `step` invocation.
	KindBusy = "busy"
	// KindUnseeded means the status file specifically -- not seed.json, which "lyx shed seed"
	// writes and this vocabulary never reports on -- could not be seeded: the bootstrap's status
	// sub-step failed for a reason other than the file already existing.
	KindUnseeded = "unseeded"
	// KindOwnership means the seeded status file belongs to a different task's slug.
	KindOwnership = "ownership"
	// KindBootstrap means a BuildShed failure, or any other pre-producer setup failure a module's
	// own PreStep hook classifies as unclassifiable, occurred somewhere before the producer call.
	KindBootstrap = "bootstrap"
	// KindProducer means shed.Step's own producer call returned a hard error.
	KindProducer = "producer"
)

// StepKinds lists every value step's RunE can emit as the envelope's "kind" field. A test asserts
// this set is exactly the five declared constants above, so an undeclared sixth kind cannot ship
// silently.
var StepKinds = []string{KindBusy, KindUnseeded, KindOwnership, KindBootstrap, KindProducer}

// StepEnvelope builds step's success envelope from res -- the StepResult shed.Step returned --
// alongside nextPolicy (spec.Hooks.InterruptPolicyFor(res.Next), or the empty string when the hook
// is nil) and statusFile (the shed's own StatusPath). The returned map carries exactly the thirteen
// documented keys below; the key set is closed -- a key outside these thirteen has no test and no
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
//   - trace_file: loc.TraceFile
//   - friction_dir: loc.FrictionDir
//   - scratch_dir: loc.ScratchDir
//
// "continue" is derived here, rather than left to the caller, so a thin external supervisor skill
// never carries its own copy of the State vocabulary -- it only ever branches on this one boolean.
func StepEnvelope(res shedengine.StepResult, nextPolicy, statusFile string, loc StepLocations) map[string]any {
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
		"trace_file":            loc.TraceFile,
		"friction_dir":          loc.FrictionDir,
		"scratch_dir":           loc.ScratchDir,
	}
}

// StepLocations carries the three path keys every step envelope reports: trace_file
// (TraceFile), friction_dir (FrictionDir) and scratch_dir (ScratchDir).
// It is one struct so StepEnvelope and the error-envelope helper share a single source.
type StepLocations struct {
	TraceFile   string
	FrictionDir string
	ScratchDir  string
}

// stepErrFields builds an error envelope's extra fields: kind plus the three location keys.
func stepErrFields(kind string, loc StepLocations) map[string]any {
	return map[string]any{
		"kind":         kind,
		"trace_file":   loc.TraceFile,
		"friction_dir": loc.FrictionDir,
		"scratch_dir":  loc.ScratchDir,
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
			logger.Info("shed: step", "status_file", spec.StatusPath)
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			// locations is computed after the Warn so the trace file it names is the one holding it.
			locations := func() StepLocations {
				return StepLocations{TraceFile: logger.TraceFile(), FrictionDir: spec.FrictionDir, ScratchDir: spec.ScratchDir}
			}
			refuse := func(kind, msg string) {
				logger.Warn("shed: step refused", "kind", kind, "error", msg)
				clihelp.SetExit(ctx, output.ErrFields(out, msg, stepErrFields(kind, locations())))
			}

			if spec.Hooks.PreStep != nil {
				if kind, err := spec.Hooks.PreStep(ctx); err != nil {
					refuse(kind, err.Error())
					return nil
				}
			}

			if spec.BuildShed == nil {
				refuse(KindBootstrap, "shedverbs: step: no BuildShed constructor configured")
				return nil
			}
			shed, err := spec.BuildShed()
			if err != nil {
				refuse(KindBootstrap, err.Error())
				return nil
			}

			res, err := shed.Step(ctx)
			if err != nil {
				if errors.Is(err, shedengine.ErrShedBusy) {
					msg := err.Error()
					if spec.StepBusyMessage != "" {
						msg = spec.StepBusyMessage
					}
					refuse(spec.StepBusyKind, msg)
					return nil
				}
				refuse(KindProducer, err.Error())
				return nil
			}
			logger.Info("shed: step done", "producer", res.Producer, "outcome", string(res.Outcome), "state", string(res.State), "next", res.Next, "reason", res.Reason)

			if spec.Hooks.PostStep != nil {
				spec.Hooks.PostStep(res)
			}

			nextPolicy := ""
			if spec.Hooks.InterruptPolicyFor != nil {
				nextPolicy = spec.Hooks.InterruptPolicyFor(res.Next)
			}
			clihelp.SetExit(ctx, output.Ok(out, StepEnvelope(res, nextPolicy, spec.StatusPath, locations())))
			return nil
		},
	}
}
