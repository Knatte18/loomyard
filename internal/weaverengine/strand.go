// strand.go declares how one batch attempt runs as a gated cold strand and how its end is judged.
// It holds the strand seam, the attempt's spawn spec, the batch's must-pass gates and the verdict of a waited result.

package weaverengine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// The gate keys a batch attempt's own gates render their calls from.
const (
	gateKeyCard  = "card"
	gateKeyBatch = "batch"
)

// batchGateAttempts is the re-prompt budget of each of a batch attempt's own gate entries.
const batchGateAttempts = 3

// batchGateSite labels the batch attempt gates' verify runs in the running marker.
const batchGateSite = "weaver batch gate"

// batchGateWayForward opens the findings of a failed batch gate entry.
const batchGateWayForward = "Fix every failure below, commit your fixes, and end your turn."

// The kinds of verdict classifyAttempt reaches over a waited attempt.
const (
	// verdictRecordable is a done attempt whose report is OK and whose gates all passed.
	verdictRecordable = "recordable"
	// verdictFailed is an attempt that ended without a recordable result.
	verdictFailed = "failed"
	// verdictLost is an attempt that ended on a machinery loss, which says nothing of its work.
	verdictLost = "lost"
	// verdictNeedsParent is an attempt that stopped on a step only the run's parent can take.
	verdictNeedsParent = "needs_parent"
)

// StrandRunner is the part of *shuttleengine.Runner that Weaver drives:
// starting a gated strand, probing for a live one, and stopping one by guid.
// EngineName names the engine a rung's model choice launches on, which the attempt's prompt is rendered for.
type StrandRunner interface {
	shuttleengine.EngineNamer
	// StartGated starts spec with gate and returns once the strand is past its provider's startup gates.
	StartGated(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (*shuttleengine.Run, error)
	// ProbeGated finds the live run matching spec's output files and reports whether one exists, without waiting on it.
	ProbeGated(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (*shuttleengine.Run, bool, error)
	// StopStrand ends the strand guid names and records the stop on its run.
	StopStrand(guid string) error
}

var _ StrandRunner = (*shuttleengine.Runner)(nil)

// attemptLaunch is what attemptSpec builds one attempt's spawn from.
type attemptLaunch struct {
	// Prompt is the rendered batch prompt.
	Prompt []byte
	// Engine names the engine Prompt was rendered for, the one Choice launches on.
	Engine string
	// ReportPath is the attempt's report file, the strand's only output file.
	ReportPath string
	// Choice is the model choice of the attempt's ladder rung.
	Choice shuttleengine.ModelChoice
	// Label names the batch as NN-<slug> and names the strand's run directory.
	Label string
	// Skills are the hub's language skills, loaded after the implementer role's own.
	Skills []string
}

// attemptSpec builds the spawn of one batch attempt.
// The attempt runs autonomously under the implementer role's skills, in a pane with no operator.
func attemptSpec(in attemptLaunch) shuttleengine.Spec {
	return shuttleengine.Spec{
		Prompt:       string(in.Prompt),
		PromptEngine: in.Engine,
		OutputFiles:  []string{in.ReportPath},
		ModelChoice:  in.Choice,
		ConfigRole:   agentconfig.RoleWebster,
		Skills:       in.Skills,
		Role:         BatchStrandRole,
		Segment:      segmentcolor.Webster,
		Round:        in.Label,
	}
}

// batchGateCalls returns the gate calls of one batch attempt, in the order the attempt must pass them.
// There is one `card` call per card in card order, then one `batch` call over every card of the batch.
// The last batch's `batch` call also runs the module's main packages.
// A gate config without the `card` or `batch` key refuses with the config's way forward.
func batchGateCalls(geom Geometry, cards []planparser.Card, last bool) ([]string, error) {
	if geom.GateConfig == nil {
		return nil, errors.New("weaver: the geometry carries no gate config reader; way forward: the CLI layer must set Geometry.GateConfig")
	}
	cfg, err := geom.GateConfig()
	if err != nil {
		return nil, fmt.Errorf("weaver: read the gate config for the batch gates: %w", err)
	}
	calls := make([]string, 0, len(cards)+1)
	ids := make([]string, 0, len(cards))
	for _, card := range cards {
		call, err := cfg.GateCall(gateKeyCard, card.ID())
		if err != nil {
			return nil, err
		}
		calls = append(calls, call)
		ids = append(ids, card.ID())
	}
	batchCall, err := cfg.GateCall(gateKeyBatch, ids...)
	if err != nil {
		return nil, err
	}
	if last {
		batchCall += " --" + gateslot.MainPackagesFlag
	}
	return append(calls, batchCall), nil
}

// batchGateSpec returns the must-pass gates of one batch attempt's strand, so the agent fixes its own gate failures.
// There is one entry per call batchGateCalls returns, in its order, then rowGates unchanged.
// Each entry runs its call through verifytree.Verify under the geometry's pause hold, with a budget of re-prompts.
func batchGateSpec(geom Geometry, cards []planparser.Card, last bool, rowGates shuttleengine.GateSpec) (shuttleengine.GateSpec, error) {
	calls, err := batchGateCalls(geom, cards, last)
	if err != nil {
		return nil, err
	}
	spec := make(shuttleengine.GateSpec, 0, len(calls)+len(rowGates))
	for i, call := range calls {
		name := gateKeyBatch
		if i < len(cards) {
			name = gateKeyCard + " " + cards[i].ID()
		}
		gate := batchGateClosure(geom, name, call)
		if i >= len(cards) && geom.ReachPending != nil {
			gate = verifytree.WithReachTurn(gate, verifytree.ReachTurn{
				Pending:    func() (verifytree.ReachPending, bool, error) { return geom.ReachPending(gateKeyBatch) },
				Attempts:   batchGateAttempts,
				ListedPath: verifytree.ReachListedPath(geom.VerifyDir, batchGateSite),
			})
		}
		spec = append(spec, shuttleengine.GateEntry{Name: name, Gate: gate, Attempts: batchGateAttempts})
	}
	return append(spec, rowGates...), nil
}

// batchGateClosure returns the gate that runs call through verifytree.Verify at each arrival.
// A dirty tree and a failed run fail with findings the agent reads, and a crashed test runner fails terminally.
func batchGateClosure(geom Geometry, name, call string) shuttleengine.Gate {
	paths := verifytree.NewPaths(geom.PaneCwd, geom.VerifyDir)
	attempt := 0
	return func() (shuttleengine.GateResult, error) {
		attempt++
		site := verifytree.Site{Label: batchGateSite, Attempt: attempt, Hold: geom.VerifyHold}
		res, err := verifytree.Verify(context.Background(), paths, site, call)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("weaver: batch gate %s: %w", name, err)
		}
		switch res.Status {
		case verifytree.StatusPassed, verifytree.StatusSkipped:
			return shuttleengine.GateResult{Passed: true}, nil
		case verifytree.StatusDirty:
			return shuttleengine.GateResult{Findings: verifytree.UncommittedFindings(res.Dirty)}, nil
		}
		if res.RunnerCrash {
			return shuttleengine.GateResult{Terminal: true, Findings: runnerCrashFindings(res)}, nil
		}
		output, err := readVerifyLog(res.Log)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		report := failureReport(call, attempt, batchGateAttempts, res, output)
		return shuttleengine.GateResult{Findings: runplan.RenderVerifyGateFindings(report, batchGateWayForward)}, nil
	}
}

// runnerCrashFindings words a verify whose every failed run crashed its test runner, which no code change mends.
func runnerCrashFindings(res verifytree.Result) string {
	return fmt.Sprintf("The verify command's test runner crashed in its run and its one rerun, so no test judged the code: %s. No change to the code mends it; output: %s.", res.Detail, res.Log)
}

// readVerifyLog returns the content of the verify log at path.
func readVerifyLog(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("weaver: read verify log %s: %w", path, err)
	}
	return string(data), nil
}

// failureReport builds the verify-failure record of a failed run of command.
// A timed-out run carries its timeout and the log tail.
// A failure that names no test carries the log tail alone, and any other carries its failing identities.
// A log whose failure report holds triage lines under its failed tests adds them to the record.
func failureReport(command string, attempt, budget int, res verifytree.Result, output string) runplan.VerifyGateReport {
	report := runplan.VerifyGateReport{Attempt: attempt, Cap: budget, Command: command, ExitCode: res.ExitCode, LogPath: res.Log}
	if res.TimedOut {
		report.TimedOut = res.Detail
		report.LogTail = runplan.LogTail(output)
		return report
	}
	report.Triage = gateslot.TriageLines(output)
	failures := runplan.ParseVerifyRun(output, false)
	if len(failures) == 1 && failures[0].Kind == runplan.FailureKindOpaque {
		report.LogTail = runplan.LogTail(output)
		return report
	}
	report.Failures = failures
	return report
}

// attemptVerdict is how one waited attempt ended.
type attemptVerdict struct {
	// Kind is one of the verdict constants.
	Kind string
	// Cause says why a failed or lost attempt ended so.
	Cause string
	// Need is the step only the parent can take, set for a needs-parent verdict.
	Need string
	// Report is the attempt's parsed report, nil when none parsed.
	Report *runplan.Report
}

// classifyAttempt judges one waited result into a verdict.
// A wait error from reed, a dead strand and a provider API error are machinery losses.
// A timeout, a missing or unparseable report, a FAILED report and a gate that failed after its re-prompts are failures.
// A NEEDS_PARENT report stops for its need, and an OK report with every gate passed is recordable.
func classifyAttempt(result shuttleengine.Result, waitErr error, reportPath string) attemptVerdict {
	if waitErr != nil {
		return attemptVerdict{Kind: verdictLost, Cause: fmt.Sprintf("waiting on the strand failed: %v", waitErr)}
	}
	switch result.Outcome {
	case shuttleengine.OutcomeDied:
		return attemptVerdict{Kind: verdictLost, Cause: "the strand died before it wrote its report"}
	case shuttleengine.OutcomeAPIError:
		return attemptVerdict{Kind: verdictLost, Cause: fmt.Sprintf("the provider's API ended the strand's turn on an error: %s", result.APIError)}
	case shuttleengine.OutcomeTimeout:
		return attemptVerdict{Kind: verdictFailed, Cause: "the attempt timed out before it wrote its report"}
	case shuttleengine.OutcomeDone:
	default:
		return attemptVerdict{Kind: verdictFailed, Cause: fmt.Sprintf("the strand ended with outcome %q", result.Outcome)}
	}

	report, err := runplan.ParseReport(reportPath)
	if errors.Is(err, fs.ErrNotExist) {
		return attemptVerdict{Kind: verdictFailed, Cause: fmt.Sprintf("the attempt wrote no report at %s", reportPath)}
	}
	if err != nil {
		return attemptVerdict{Kind: verdictFailed, Cause: fmt.Sprintf("the attempt's report does not parse: %v", err)}
	}
	switch report.Status {
	case runplan.ReportStatusNeedsParent:
		return attemptVerdict{Kind: verdictNeedsParent, Need: report.Need, Report: report}
	case runplan.ReportStatusFailed:
		return attemptVerdict{Kind: verdictFailed, Cause: "the attempt reported status FAILED", Report: report}
	}
	if result.Gate != nil && !result.Gate.Passed {
		return attemptVerdict{Kind: verdictFailed, Cause: gateFailureCause(result.Gate), Report: report}
	}
	return attemptVerdict{Kind: verdictRecordable, Report: report}
}

// gateFailureCause words the gate failure of an attempt that ended with a failing gate.
func gateFailureCause(gate *shuttleengine.GateOutcome) string {
	cause := "a batch gate still failed after its re-prompts"
	for _, entry := range gate.Entries {
		if entry.State == shuttleengine.GateEntryFailed {
			cause += fmt.Sprintf(": entry %q after %d re-prompt(s)", entry.Name, entry.Attempts)
			break
		}
	}
	findings := gate.Findings
	if gate.Reason != "" {
		findings = gate.Reason
	}
	if findings = strings.TrimSpace(findings); findings != "" {
		cause += "\n" + findings
	}
	return cause
}

// StrandStopError is a strand that could not be stopped, with its guid and the cause.
type StrandStopError struct {
	GUID string
	Err  error
}

// Error names the strand and the way forward: remove it with reed, then re-step the row.
func (e *StrandStopError) Error() string {
	return fmt.Sprintf("weaver: could not stop strand %s: %v; way forward: remove it with `lym reed remove %s`, then re-step the row", e.GUID, e.Err, e.GUID)
}

// Unwrap returns the stop's own error.
func (e *StrandStopError) Unwrap() error { return e.Err }

// stopAttempt stops the strand guid names through the runner's stop verb, which records the stop on the strand's run.
// A failure returns a *StrandStopError.
// An empty guid names no strand, so nothing is stopped.
func stopAttempt(runner StrandRunner, guid string) error {
	if guid == "" {
		return nil
	}
	if err := runner.StopStrand(guid); err != nil {
		return &StrandStopError{GUID: guid, Err: err}
	}
	return nil
}
