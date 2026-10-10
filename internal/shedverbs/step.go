// step.go implements the generic `step` verb body -- the single-producer primitive an external
// supervisor drives one call at a time -- and declares the closed refusal-kind vocabulary its
// envelope's "kind" field reports.

package shedverbs

import (
	"bytes"
	"errors"
	"io"
	"strings"

	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/spf13/cobra"
)

// The closed refusal-kind vocabulary step reports on the envelope's "kind" field. This set is
// closed at six: StepKinds below lists all of them, and a test asserts the set is exactly this and
// no larger.
// The driver stencil (contracts/stencils/shed/shed-template-driver.md) is the single place a driver's
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
	// KindInterrupted means a child step ended without writing an envelope: it was killed, or it exited without one.
	KindInterrupted = "interrupted"
)

// StepKinds lists every value step's RunE can emit as the envelope's "kind" field. A test asserts
// this set is exactly the six declared constants above, so an undeclared seventh kind cannot ship
// silently.
var StepKinds = []string{KindBusy, KindUnseeded, KindOwnership, KindBootstrap, KindProducer, KindInterrupted}

// KindlessRefusal wraps an arming error whose envelope must carry no kind.
// ReportArmError prints it as a bare error line on every verb, step included.
type KindlessRefusal struct {
	Err error
}

// Error passes the wrapped error's text through.
func (r KindlessRefusal) Error() string { return r.Err.Error() }

// Unwrap returns the wrapped error.
func (r KindlessRefusal) Unwrap() error { return r.Err }

// armWayForward is the way-forward clause ReportArmError appends to a step arming refusal that names none of its own.
const armWayForward = "way forward: fix the cause this error names, then run the step again, escalating when the fix lies outside the repair verbs"

// ReportArmError prints the refusal err raised while verb was being armed, and returns the exit code.
// A KindlessRefusal, or any verb but step, prints a bare error envelope.
// For step it logs the refusal, then prints an error envelope carrying kind bootstrap and the trace file holding the log.
// That envelope's message is err's text followed by a way-forward clause, unless the text names one already.
func ReportArmError(out io.Writer, verb string, err error) int {
	var kindless KindlessRefusal
	if verb != "step" || errors.As(err, &kindless) {
		return output.Err(out, err.Error())
	}
	msg := bootstrapMessage(err)
	logger.Warn("shed: step arming refused", "error", msg)
	return output.ErrFields(out, msg, map[string]any{"kind": KindBootstrap, "trace_file": logger.TraceFile()})
}

// bootstrapMessage is err's text followed by the generic way-forward clause, unless the text names a way forward already.
func bootstrapMessage(err error) string {
	msg := err.Error()
	if !strings.Contains(msg, "way forward:") {
		msg += "; " + armWayForward
	}
	return msg
}

// childArgv builds the command line of a loop's child, after the executable, from where cmd sits in the command tree and its positional arguments:
// the subcommand names below the root, then the arguments, and no flag.
func childArgv(cmd *cobra.Command, args []string) []string {
	var names []string
	for current := cmd; current.HasParent(); current = current.Parent() {
		names = append([]string{current.Name()}, names...)
	}
	return append(names, args...)
}

// StepEnvelope builds step's full success envelope, held by the step record and printed under --full or when no record is kept, from res -- the StepResult shed.Step returned -- alongside nextPolicy (spec.Hooks.InterruptPolicyFor(res.Next), or the empty string when the hook is nil), statusFile (the shed's own StatusPath), friction (Hooks.AfterStep's return, or the empty string when the hook is nil) and progress (the recipe progress for res.Next, or nil when none is known).
// The returned map carries exactly the documented keys below;
// the key set is closed -- a key outside them has no test and no documented meaning:
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
//   - friction: friction
//   - trace_file: loc.TraceFile
//   - friction_dir: loc.FrictionDir
//   - scratch_dir: loc.ScratchDir
//   - trace_id: loc.TraceID
//   - run_id: loc.RunID
//   - progress: the compact form of progress -- step, steps and name, without the remaining steps and without any empty key -- or nil
//   - parent_notice: res.ParentNotice, present only when it is non-empty (an awaiting halt that carries one)
//
// "continue" is derived here, rather than left to the caller, so a thin external supervisor skill
// never carries its own copy of the State vocabulary -- it only ever branches on this one boolean.
func StepEnvelope(res shedengine.StepResult, nextPolicy, statusFile, friction string, loc StepLocations, progress *shedengine.Progress) map[string]any {
	var progressValue any
	if compact := compactProgress(progress); compact != nil {
		progressValue = compact
	}
	env := map[string]any{
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
		"friction":              friction,
		"trace_file":            loc.TraceFile,
		"friction_dir":          loc.FrictionDir,
		"scratch_dir":           loc.ScratchDir,
		"trace_id":              loc.TraceID,
		"run_id":                loc.RunID,
		"progress":              progressValue,
	}
	if res.ParentNotice != "" {
		env["parent_notice"] = res.ParentNotice
	}
	return env
}

// compactProgress maps p to the step envelope's progress object: step, steps and name, each omitted when zero or empty.
// It drops the remaining steps, which only the status envelope carries, and returns nil for a nil p.
func compactProgress(p *shedengine.Progress) map[string]any {
	if p == nil {
		return nil
	}
	compact := map[string]any{}
	if p.Step != 0 {
		compact["step"] = p.Step
	}
	if p.Steps != 0 {
		compact["steps"] = p.Steps
	}
	if p.Name != "" {
		compact["name"] = p.Name
	}
	return compact
}

// StepLocations carries the keys every step envelope, success or error, reports: trace_file
// (TraceFile), friction_dir (FrictionDir), scratch_dir (ScratchDir), trace_id (TraceID) and run_id
// (RunID).
// Every envelope also carries friction, which comes from Hooks.AfterStep and not from this struct.
// It is one struct so StepEnvelope and the error-envelope helper share a single source.
type StepLocations struct {
	TraceFile   string
	FrictionDir string
	ScratchDir  string
	TraceID     string
	RunID       string
}

// stepErrFields builds an error envelope's extra fields: kind, transient, friction and the five location keys.
// friction is the AfterStep hook's status, or the empty string where no step ran.
// transient is the class name shedengine.TransientOf reports for the failure, or the empty string when it is not transient;
// it is a key, not a kind.
func stepErrFields(kind, transient, friction string, loc StepLocations) map[string]any {
	return map[string]any{
		"kind":         kind,
		"transient":    transient,
		"friction":     friction,
		"trace_file":   loc.TraceFile,
		"friction_dir": loc.FrictionDir,
		"scratch_dir":  loc.ScratchDir,
		"trace_id":     loc.TraceID,
		"run_id":       loc.RunID,
	}
}

// shortStepEnvelope builds the short success envelope from the full one and the record's path.
// The key set is closed at: run_id, producer, outcome, state, continue, reason, output, next, progress,
// history_length, trace_file and envelope_path, plus friction and parent_notice only when non-empty.
func shortStepEnvelope(full map[string]any, envelopePath string) map[string]any {
	short := map[string]any{"envelope_path": envelopePath}
	for _, key := range []string{"run_id", "producer", "outcome", "state", "continue", "reason", "output", "next", "progress", "history_length", "trace_file"} {
		short[key] = full[key]
	}
	for _, key := range []string{"friction", "parent_notice"} {
		if value, _ := full[key].(string); value != "" {
			short[key] = value
		}
	}
	return short
}

// shortStepErrFields builds the short error envelope's extra fields from the full ones and the record's path.
// The key set is closed at: kind, transient, run_id, trace_file and envelope_path, plus friction only when non-empty.
// The error message travels beside them as output.ErrFields' own "error" key.
func shortStepErrFields(full map[string]any, envelopePath string) map[string]any {
	short := map[string]any{"envelope_path": envelopePath}
	for _, key := range []string{"kind", "transient", "run_id", "trace_file"} {
		short[key] = full[key]
	}
	if friction, _ := full["friction"].(string); friction != "" {
		short["friction"] = friction
	}
	return short
}

// emitStep renders the full envelope, hands its bytes to rec, and only then prints to out.
// ok selects output.Ok or output.ErrFields (msg is the error message); fields are the full envelope's;
// shorten builds the short envelope's fields from them and the record's path.
// When the record was written and full is false, the short envelope is printed;
// otherwise the full envelope bytes are, identical to the record.
// It returns the full envelope's exit code in every case.
func emitStep(out io.Writer, rec *stepRecorder, full, ok bool, msg string, fields map[string]any, shorten func(map[string]any, string) map[string]any) int {
	var buf bytes.Buffer
	var code int
	if ok {
		code = output.Ok(&buf, fields)
	} else {
		code = output.ErrFields(&buf, msg, fields)
	}
	path, recorded := rec.write(buf.Bytes())
	if !recorded || full {
		_, _ = out.Write(buf.Bytes())
		return code
	}
	if ok {
		output.Ok(out, shorten(fields, path))
	} else {
		output.ErrFields(out, msg, shorten(fields, path))
	}
	return code
}

// progressOf reports routing's progress at current, or nil when routing carries no producers.
func progressOf(routing shedengine.Routing, current string) *shedengine.Progress {
	if len(routing.Producers) == 0 {
		return nil
	}
	p := routing.ProgressAt(current)
	return &p
}

// stepCmd builds the generic `step` subcommand: the single-producer primitive an external
// supervisor drives. This body owns nothing above shed.Step -- any run-lock probe, bootstrap, or
// status-strand work a module needs belongs entirely to its own PreStep hook.
func stepCmd(texts VerbTexts, spec *Spec) *cobra.Command {
	cmd := &cobra.Command{
		Use:         texts.Step.Use,
		Short:       texts.Step.Short,
		Long:        texts.Step.Long,
		Annotations: map[string]string{clihelp.AudienceAnnotation: texts.Step.Audience},
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()

			// The loop branch returns before the in-flight record is written.
			// The invocation carrying the flag therefore keeps no record of its own, and last_step only ever names a child's step.
			if untilStop, _ := cmd.Flags().GetBool(UntilStopFlag); untilStop {
				if spec.Loop.EnvelopePath == "" {
					msg := "shedverbs: this recipe arms no loop; way forward: run the step without --" + UntilStopFlag
					logger.Warn("shed: step refused", "kind", KindBootstrap, "error", msg)
					clihelp.SetExit(ctx, output.ErrFields(cmd.OutOrStdout(), msg, map[string]any{"kind": KindBootstrap, "trace_file": logger.TraceFile()}))
					return nil
				}
				argv := childArgv(cmd, args)
				if loopID, _ := cmd.Flags().GetString(LoopDetachedFlag); loopID != "" {
					if !validLoopID(loopID) {
						clihelp.SetExit(ctx, output.Err(cmd.OutOrStdout(), "shedverbs: --"+LoopDetachedFlag+" takes the loop id its waiter minted; way forward: run the step with --"+UntilStopFlag+" alone"))
						return nil
					}
					clihelp.SetExit(ctx, runLoop(ctx, spec, loopID, argv, cmd.OutOrStdout()))
					return nil
				}
				clihelp.SetExit(ctx, runUntilStop(ctx, spec, argv, cmd.OutOrStdout()))
				return nil
			}

			logger.Info("shed: step", "status_file", spec.StatusPath)

			// The in-flight record is written before anything can refuse, and the full envelope,
			// success or refusal, is written to its own record before stdout is printed.
			rec := newStepRecorder(spec.StepsDir, logger.TraceID(), buildvcs.Running())
			rec.begin()
			full, _ := cmd.Flags().GetBool("full")
			out := cmd.OutOrStdout()

			// locations is computed after the Warn so the trace file it names is the one holding it.
			locations := func() StepLocations {
				return StepLocations{TraceFile: logger.TraceFile(), FrictionDir: spec.FrictionDir, ScratchDir: spec.ScratchDir, TraceID: logger.TraceID(), RunID: spec.RunID}
			}
			refuse := func(kind, transient, friction, msg string) {
				logger.Warn("shed: step refused", "kind", kind, "transient", transient, "error", msg)
				clihelp.SetExit(ctx, emitStep(out, rec, full, false, msg, stepErrFields(kind, transient, friction, locations()), shortStepErrFields))
			}

			if spec.Hooks.PreStep != nil {
				if kind, err := spec.Hooks.PreStep(ctx); err != nil {
					refuse(kind, string(shedengine.TransientOf(err)), "", err.Error())
					return nil
				}
			}

			if spec.BuildShed == nil {
				refuse(KindBootstrap, "", "", "shedverbs: step: no BuildShed constructor configured")
				return nil
			}
			shed, err := spec.BuildShed()
			if err != nil {
				refuse(KindBootstrap, string(shedengine.TransientOf(err)), "", err.Error())
				return nil
			}

			res, err := shed.Step(ctx)
			if err != nil {
				friction := ""
				if spec.Hooks.AfterStep != nil {
					friction = spec.Hooks.AfterStep(ctx, res, err)
				}
				if errors.Is(err, shedengine.ErrShedBusy) {
					msg := err.Error()
					if spec.StepBusyMessage != "" {
						msg = spec.StepBusyMessage
					}
					refuse(spec.StepBusyKind, "", friction, msg)
					return nil
				}
				refuse(KindProducer, string(shedengine.TransientOf(err)), friction, err.Error())
				return nil
			}
			logger.Info("shed: step done", "producer", res.Producer, "outcome", string(res.Outcome), "state", string(res.State), "next", res.Next, "reason", res.Reason)

			if spec.Hooks.PostStep != nil {
				spec.Hooks.PostStep(res)
			}

			friction := ""
			if spec.Hooks.AfterStep != nil {
				friction = spec.Hooks.AfterStep(ctx, res, nil)
			}

			nextPolicy := ""
			if spec.Hooks.InterruptPolicyFor != nil {
				nextPolicy = spec.Hooks.InterruptPolicyFor(res.Next)
			}
			clihelp.SetExit(ctx, emitStep(out, rec, full, true, "", StepEnvelope(res, nextPolicy, spec.StatusPath, friction, locations(), progressOf(spec.Routing, res.Next)), shortStepEnvelope))
			return nil
		},
	}
	cmd.Flags().Bool("full", false, "print the full envelope on stdout instead of the short one; the record, exit code and run state are unchanged")
	cmd.Flags().Bool(UntilStopFlag, false, "run steps one after another in a detached loop until the run halts, errors or reaches a stop condition, and print the one envelope of that stop")
	cmd.Flags().String(LoopDetachedFlag, "", "marks the detached loop process itself; set only by the waiter that spawns it")
	_ = cmd.Flags().MarkHidden(LoopDetachedFlag)
	return cmd
}
