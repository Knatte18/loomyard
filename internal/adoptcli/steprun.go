// steprun.go runs adopt's steps in order and records each one.
// It holds the step run the steps share, the loop that drives them, the error that stops one,
// and the outcome part each step contributes to the adopt record and to the end envelope merged from it.

package adoptcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/adoptrecord"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// stepRun is one adopt in flight, shared by its steps.
// Outcomes are the slug outcomes merged from the steps that ended in this process.
// Part and PartSlugs are the working outcome of the step running now, which the loop merges into them when the step ends.
// A step reads earlier steps' marks from Outcomes and writes its own into Part, through slot for a slug.
// The last group carries the reconcile step's decisions to the steps up to the owner write, which share one process.
// Save, when set, persists the record after each step ends and when one starts.
type stepRun struct {
	Deps      adoptDeps
	Req       request
	Here      string
	Slugs     []string
	RootNamed bool
	Owners    boardengine.Owners
	Outcomes  map[string]*slugOutcome
	Mutations *fabricengine.Mutations
	Report    fabricengine.OriginReport
	Record    adoptrecord.Record
	Save      func(adoptrecord.Record) error
	Now       func() time.Time
	// Stopped is the error that ended the record stopped, nil while the record runs or after it ended done.
	Stopped error

	Part      outcome
	PartSlugs map[string]*slugOutcome

	LocalRuns []string
	RunSlugs  []string
	Acted     []string
	Projects  map[string]string
	Stops     map[string]bool
	Held      map[string]bool
	HeldBack  map[string]bool
}

// stopError is the error that ends a step: what went wrong, and the clause that names what to do next.
type stopError struct {
	err        error
	wayForward string
}

func (e *stopError) Error() string { return e.err.Error() + "; " + e.wayForward }

func (e *stopError) Unwrap() error { return e.err }

// stopWith returns the error that stops a step with the message format and args describe, followed by wayForward.
func stopWith(wayForward, format string, args ...any) error {
	return &stopError{err: fmt.Errorf(format, args...), wayForward: wayForward}
}

// outcomePart is the JSON of the outcome fields one step set.
// Its keys are the envelope's own, except slugs, an object keyed by slug that holds only the entries the step touched.
// A list field holds only the items the step appended.
type outcomePart struct {
	Reconcile        *fabricengine.OriginReport `json:"reconcile,omitempty"`
	Mutations        []fabricengine.Mutation    `json:"mutations,omitempty"`
	Deploy           *deployOutcome             `json:"deploy,omitempty"`
	PrimeRecorded    bool                       `json:"prime_recorded,omitempty"`
	PrimeLost        string                     `json:"prime_lost_to,omitempty"`
	Slugs            map[string]slugOutcome     `json:"slugs,omitempty"`
	Warnings         []string                   `json:"warnings,omitempty"`
	PrimeBeamSteps   *beamStepsOutcome          `json:"prime_beam_steps,omitempty"`
	PrimeBeamWarning string                     `json:"prime_beam_warning,omitempty"`
	Checks           []suiteStart               `json:"checks,omitempty"`
	SuiteMessage     string                     `json:"message,omitempty"`
	CheckWarnings    []string                   `json:"check_warnings,omitempty"`
	Runs             []runReport                `json:"runs,omitempty"`
}

// runSteps runs the steps of adoptrecord.Steps from the step named from, in order, and records each in run.Record.
// It seeds an outcome for each slug the record names that no outcome holds yet.
// It does so before the first step and after each step, so a later step never meets a missing one.
// On every call it then reads the local runs and refuses an anchor change standing under the subjects or those runs;
// a refusal ends the record stopped at from and runs no step.
// A step's error ends the record stopped at that step, with the part the step gathered before it failed,
// and no later step runs.
// The step's error is left in run.Stopped; runSteps itself returns only an error from saving the record.
func runSteps(run *stepRun, from string) error {
	run.seedOutcomes()
	run.Stopped = nil
	run.Record.State = adoptrecord.StateRunning
	run.Record.Error, run.Record.WayForward = "", ""

	if err := run.refuseBeforeSteps(); err != nil {
		return run.stop(from, err, nil)
	}
	for _, name := range adoptrecord.Steps[slices.Index(adoptrecord.Steps, from):] {
		run.Record.InFlight = name
		if err := run.save(); err != nil {
			return err
		}

		run.beginPart()
		mutated := run.Mutations.Len()
		stepErr := stepFunc(name)(run)
		part := run.endPart(mutated)
		raw, err := json.Marshal(part)
		if err != nil {
			return fmt.Errorf("lym adopt: encode the part of the %s step: %w", name, err)
		}
		if err := mergeSlugs(run.Outcomes, part.Slugs); err != nil {
			return err
		}
		run.seedOutcomes()
		if stepErr != nil {
			return run.stop(name, stepErr, raw)
		}

		run.recordStep(name, raw)
		if err := run.save(); err != nil {
			return err
		}
	}
	run.Record.State = adoptrecord.StateDone
	return run.save()
}

// stepFunc returns the function of the step named name.
func stepFunc(name string) func(*stepRun) error {
	switch name {
	case adoptrecord.StepReconcile:
		return stepReconcile
	case adoptrecord.StepPairs:
		return stepPairs
	case adoptrecord.StepPause:
		return stepPause
	case adoptrecord.StepOwner:
		return stepOwner
	case adoptrecord.StepDeploy:
		return stepDeploy
	case adoptrecord.StepBeam:
		return stepBeam
	case adoptrecord.StepCheck:
		return stepCheck
	case adoptrecord.StepDone:
		return stepDone
	}
	panic("adoptcli: no adopt step is named " + name)
}

// seedOutcomes gives each slug the record's subjects, runs and paused slugs name an outcome, when none holds one.
// The word `root` gets none.
func (run *stepRun) seedOutcomes() {
	seedOutcomes(run.Outcomes, run.Record)
}

// seedOutcomes gives each slug the record's subjects, runs and paused slugs name an outcome in outcomes.
// A slug that already holds one keeps it, and the word `root` gets none.
func seedOutcomes(outcomes map[string]*slugOutcome, record adoptrecord.Record) {
	inputs := record.Inputs
	for _, slug := range slices.Concat(record.Subjects, inputs.Runs, inputs.Paused) {
		if slug == boardengine.RootSubject {
			continue
		}
		if outcomes[slug] == nil {
			outcomes[slug] = &slugOutcome{Slug: slug}
		}
	}
}

// refuseBeforeSteps reads the runs under the subjects as this machine's records hold them.
// It refuses an anchor change standing in a worktree the steps would act under.
// It keeps the runs for the reconcile step to record.
func (run *stepRun) refuseBeforeSteps() error {
	localRuns, err := movedRuns(run.Deps, run.Owners, run.Slugs, run.RootNamed)
	if err != nil {
		return stopWith(reentry, "lym adopt: read the runs under the adopted projects, so nothing ran: %w", err)
	}
	if err := refuseStandingMove(run.Deps, append(slices.Clone(run.Slugs), localRuns...)); err != nil {
		return err
	}
	run.LocalRuns = localRuns
	return nil
}

// beginPart gives the step about to run an empty working outcome.
func (run *stepRun) beginPart() {
	run.Part = outcome{}
	run.PartSlugs = map[string]*slugOutcome{}
	for slug := range run.Outcomes {
		run.PartSlugs[slug] = &slugOutcome{Slug: slug}
	}
}

// slot returns the working outcome of slug in the step running now, creating it when the slug has none.
func (run *stepRun) slot(slug string) *slugOutcome {
	out := run.PartSlugs[slug]
	if out == nil {
		out = &slugOutcome{Slug: slug}
		run.PartSlugs[slug] = out
	}
	return out
}

// endPart returns the part the step that just ended contributes.
// mutated is the length of the mutation record when the step began.
func (run *stepRun) endPart(mutated int) outcomePart {
	part := outcomePart{
		Mutations:        run.Mutations.Entries()[mutated:],
		Deploy:           run.Part.Deploy,
		PrimeRecorded:    run.Part.PrimeRecorded,
		PrimeLost:        run.Part.PrimeLost,
		Warnings:         run.Part.Warnings,
		PrimeBeamSteps:   run.Part.PrimeBeamSteps,
		PrimeBeamWarning: run.Part.PrimeBeamWarning,
		Checks:           run.Part.Checks,
		SuiteMessage:     run.Part.SuiteMessage,
		CheckWarnings:    run.Part.CheckWarnings,
		Runs:             run.Part.Runs,
	}
	if !reflect.DeepEqual(run.Part.Reconcile, fabricengine.OriginReport{}) {
		report := run.Part.Reconcile
		part.Reconcile = &report
	}
	for slug, out := range run.PartSlugs {
		if reflect.DeepEqual(*out, slugOutcome{Slug: slug}) {
			continue
		}
		if part.Slugs == nil {
			part.Slugs = map[string]slugOutcome{}
		}
		part.Slugs[slug] = *out
	}
	return part
}

// recordStep lists the step as ended with the part raw, replacing an entry of the same name.
// It clears the part a stop left, since the step that gathered it has run again.
func (run *stepRun) recordStep(name string, raw json.RawMessage) {
	ended := adoptrecord.StepRecord{Name: name, EndedAt: run.Now(), Outcome: raw}
	index := slices.IndexFunc(run.Record.Steps, func(done adoptrecord.StepRecord) bool { return done.Name == name })
	if index < 0 {
		run.Record.Steps = append(run.Record.Steps, ended)
	} else {
		run.Record.Steps[index] = ended
	}
	run.Record.InFlight = ""
	run.Record.StoppedPart = nil
}

// stop ends the record stopped at step with err.
// raw is the part the step gathered before it failed, nil for a refusal that ran no step.
// InFlight keeps the step the record stopped at.
func (run *stepRun) stop(step string, err error, raw json.RawMessage) error {
	run.Stopped = err
	run.Record.State = adoptrecord.StateStopped
	run.Record.InFlight = step
	var stopErr *stopError
	if errors.As(err, &stopErr) {
		run.Record.Error, run.Record.WayForward = stopErr.err.Error(), stopErr.wayForward
	} else {
		run.Record.Error, run.Record.WayForward = err.Error(), ""
	}
	run.Record.StoppedPart = nil
	if len(raw) > 0 && string(raw) != "{}" {
		run.Record.StoppedPart = raw
	}
	return run.save()
}

// save persists the record through Save, when one is set.
func (run *stepRun) save() error {
	if run.Save == nil {
		return nil
	}
	if err := run.Save(run.Record); err != nil {
		return fmt.Errorf("lym adopt: save the adopt record: %w", err)
	}
	return nil
}

// mergePart merges part into result and outcomes.
// A list field is appended and a scalar or object field the part holds replaces the merged one.
// A slug's entry overlays only the fields the part holds.
func mergePart(result *outcome, outcomes map[string]*slugOutcome, part outcomePart) error {
	if part.Reconcile != nil {
		result.Reconcile = *part.Reconcile
	}
	result.Mutations = append(result.Mutations, part.Mutations...)
	result.Partial = len(result.Mutations) > 0
	if part.Deploy != nil {
		result.Deploy = part.Deploy
	}
	result.PrimeRecorded = result.PrimeRecorded || part.PrimeRecorded
	if part.PrimeLost != "" {
		result.PrimeLost = part.PrimeLost
	}
	result.Warnings = append(result.Warnings, part.Warnings...)
	if part.PrimeBeamSteps != nil {
		result.PrimeBeamSteps = part.PrimeBeamSteps
	}
	if part.PrimeBeamWarning != "" {
		result.PrimeBeamWarning = part.PrimeBeamWarning
	}
	result.Checks = append(result.Checks, part.Checks...)
	if part.SuiteMessage != "" {
		result.SuiteMessage = part.SuiteMessage
	}
	result.CheckWarnings = append(result.CheckWarnings, part.CheckWarnings...)
	if part.Runs != nil {
		result.Runs = part.Runs
	}
	return mergeSlugs(outcomes, part.Slugs)
}

// mergeSlugs overlays each entry of entries onto the outcome of its slug in outcomes.
// It creates the outcome when none stands.
// An entry overlays only the fields it holds.
func mergeSlugs(outcomes map[string]*slugOutcome, entries map[string]slugOutcome) error {
	for slug, entry := range entries {
		merged := outcomes[slug]
		if merged == nil {
			merged = &slugOutcome{Slug: slug}
			outcomes[slug] = merged
		}
		data, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("lym adopt: encode the outcome of %s: %w", slug, err)
		}
		if err := json.Unmarshal(data, merged); err != nil {
			return fmt.Errorf("lym adopt: merge the outcome of %s: %w", slug, err)
		}
	}
	return nil
}
