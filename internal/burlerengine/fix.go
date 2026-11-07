// fix.go implements the fix half of a round.
// Engine.Fix runs one fresh fixer session as the chair of a one-seat table over an accepted review.
// ProbeFix and ResumeFix attach to a fixer a previous session left live.
// The half neither waits on, messages nor knows of the review half;
// it knows the review only by the digest it is told and the file it reads.
// A review with no findings whose gates pass spawns no fixer at all.

package burlerengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fixRolePrefix is the role prefix of the fix half's one seat; the seats engine forms its strand role from it.
const fixRolePrefix = "burler-fix"

// skippedFixerReport is the fixer report written when the fix half is skipped.
// It says nothing was fixed, because the review held no findings and the round's gates passed.
var skippedFixerReport = "# Fixer report\n\n" +
	"## Changes\n\n" +
	"Nothing fixed: the review held no findings and the round's gates passed, so no fixer was spawned.\n\n" +
	"## Disputed\n\n" +
	"(none)\n\n" +
	"## Deferred\n\n" +
	"(none)\n"

// skippedRoutedFixerReport is the fixer report written when the fix half is skipped.
// It applies when the review's only findings are decision-class ones its caller sent to the run's parent.
var skippedRoutedFixerReport = "# Fixer report\n\n" +
	"## Changes\n\n" +
	"Nothing fixed: the review held only decision findings, which went to the run's parent, and the round's gates passed, so no fixer was spawned.\n\n" +
	"## Disputed\n\n" +
	"(none)\n\n" +
	"## Deferred\n\n" +
	"(none)\n"

// FixResult is the fix half's outcome.
type FixResult struct {
	Outcome         shuttleengine.Outcome
	FixerReportPath string
	Fix             Half
	// Gate is a 1:1 passthrough of the fixer's shuttleengine.Result.Gate; nil means the fixer ran ungated or never ran.
	Gate *shuttleengine.GateOutcome
	// Skipped is true when no fixer was spawned: the review held no findings and the round's gates passed.
	// The fixer report was then written by the engine and Fix holds no session.
	Skipped bool
	// NotStarted is true when the fixer's provider never came up, so a producer can tell that from an agent that died mid-run.
	NotStarted bool
	// Lenses names the lenses of the fixer fan in fan order; nil for a single fixer.
	Lenses []string
	// Forks is the number of self-check forks the audit counted for a done twill fixer, 0 otherwise.
	Forks int
	// Warnings holds the audit's warnings for a done twill fixer.
	// It names a fork count other than the lens count and a fork that never returned a report.
	Warnings []string
}

// fixTable builds the fix half's one-seat table for p under opts: the fixer as the chair.
// It gates the table by opts.Gate, with every entry wrapped by repairReportBeforeGate.
// The fixer reads the review file, writes the fixer-report and forks exactly when the profile names a fixer fan.
// The seat's model key stays zero in every round; opts.FixKey is read only to name the key in an engine refusal.
// values carries the marker values the seat's stencil is filled from; skills lists the skills the seat loads.
func (p *Profile) fixTable(opts RunOpts, values map[string]string, skills []string) seatengine.Table {
	// A fresh copy, so the caller's slice is never mutated;
	// off entries are wrapped too, since they never run and the wrap is harmless there.
	fixGate := make(shuttleengine.GateSpec, 0, len(opts.Gate))
	for _, entry := range opts.Gate {
		entry.Gate = repairReportBeforeGate(entry.Gate, p.FixerReportPath)
		fixGate = append(fixGate, entry)
	}
	return seatengine.Table{
		RolePrefix: p.rolePrefix(fixRolePrefix),
		Segment:    p.segment(),
		Gate:       fixGate,
		Round:      opts.Round,
		Timeout:    opts.Timeout,
		Seats: []seatengine.Seat{
			{
				Stencil:       "burler-template-fixer",
				ModelChoice:   opts.Fix,
				Inputs:        []string{p.ReviewPath},
				Outputs:       []string{p.FixerReportPath},
				Skills:        skills,
				Values:        values,
				ForkSubagents: p.FixFan != "",
			},
		},
	}
}

// Fix runs the fix half for p, tuned by opts, over the review whose accepted bytes hash to digest.
// It validates p, then reads the review and returns ErrReviewChanged when the file no longer hashes to digest.
// A review with findings of any severity spawns the fixer.
// With opts.DecisionsRouted, decision-class findings are no fixer work: the fixer's work line names them as not its own,
// and a review holding only those counts as one with none.
// A review with none runs opts.Gate once.
// When every gate passes, Fix writes the fixer report itself and returns a skipped OutcomeDone with no session.
// When a gate fails, the fixer is spawned with the failing gates' findings as its work.
// A gate whose closure errors is returned wrapped, with nothing spawned.
// A fixer that never starts because its provider never came up is OutcomeDied with NotStarted set and a nil error.
// A fixer that died or timed out is its outcome with a nil error.
// A fixer whose gate failed returns its Gate with a nil error.
// A fixer that is done while the review file changed is ErrReviewChanged.
// A fixer that could not be stopped is returned wrapped in ErrHalfNotStopped.
func (e *Engine) Fix(p Profile, opts RunOpts, digest string) (FixResult, error) {
	if err := p.validateFix(e.geom.WorktreeRoot, e.cfg); err != nil {
		return FixResult{}, err
	}
	findings, err := acceptedReview(&p, digest)
	if err != nil {
		return FixResult{}, err
	}

	work := findings
	if opts.DecisionsRouted {
		work = withoutClass(findings, ClassDecision)
	}
	routed := len(findings) - len(work)

	logger.Info("burler: fix starting", "round", opts.Round, "findings", len(findings), "routedDecisions", routed, "fixerReportPath", p.FixerReportPath)

	gateFindings := ""
	if len(work) == 0 {
		gateFindings, err = checkGates(opts.Gate)
		if err != nil {
			return FixResult{}, err
		}
		if gateFindings == "" {
			report := skippedFixerReport
			if routed > 0 {
				report = skippedRoutedFixerReport
			}
			return e.skipFix(&p, report)
		}
	}

	// A friction.Directive error is swallowed, never returned:
	// it is optional bookkeeping, not a binding constraint,
	// so a transient stencil read failure here must never kill the round.
	notePath := friction.NotePath(e.frictionDir, opts.NoteID)
	frictionDirective, err := friction.Directive(notePath, e.stencilsDir, friction.RoleReviewFix)
	if err != nil {
		logger.Warn("burler: friction directive failed, continuing without one", "role", "review-fix", "stencil", "burler-step-fix", "error", err)
		frictionDirective = ""
	}

	dir, err := e.instructionDir("fix")
	if err != nil {
		return FixResult{}, err
	}
	paths := fixFilesIn(dir)
	gateFindingsPath := ""
	if gateFindings != "" {
		gateFindingsPath = filepath.Join(dir, "gate-findings.md")
		if err := os.WriteFile(gateFindingsPath, []byte(gateFindings), 0o644); err != nil {
			return FixResult{}, fmt.Errorf("burler: materialize instruction files: %w", err)
		}
	}
	prompt, err := composeFixPrompt(e.stencilsDir, e.geom.SpecsDir, e.seatEngine(opts.Fix), &p, frictionDirective, fixWork(p.ReviewPath, gateFindingsPath, routed > 0), paths)
	if err != nil {
		return FixResult{}, err
	}
	if err := writeInstructionFiles(prompt.Files); err != nil {
		return FixResult{}, err
	}

	seats, err := e.seats.Run(p.fixTable(opts, prompt.Values, e.seatSkills(&p)))
	return e.concludeFix(&p, opts, digest, seats, err)
}

// withoutClass returns the findings whose class is not class, in order.
func withoutClass(findings []Finding, class Class) []Finding {
	kept := make([]Finding, 0, len(findings))
	for _, f := range findings {
		if f.Class != class {
			kept = append(kept, f)
		}
	}
	return kept
}

// skipFix writes report as the fixer report for p and returns the skipped fix result.
func (e *Engine) skipFix(p *Profile, report string) (FixResult, error) {
	if err := os.MkdirAll(filepath.Dir(p.FixerReportPath), 0o755); err != nil {
		return FixResult{}, fmt.Errorf("burler: create the fixer report directory for %q: %w", p.FixerReportPath, err)
	}
	if err := os.WriteFile(p.FixerReportPath, []byte(report), 0o644); err != nil {
		return FixResult{}, fmt.Errorf("burler: write the skipped fixer report %q: %w", p.FixerReportPath, err)
	}
	logger.Info("burler: fix skipped, the review held no findings and the gates pass", "fixerReportPath", p.FixerReportPath)
	return FixResult{Outcome: shuttleengine.OutcomeDone, FixerReportPath: p.FixerReportPath, Skipped: true}, nil
}

// acceptedReview reads the review file at p.ReviewPath and returns its findings.
// It refuses with ErrReviewChanged when the bytes no longer hash to digest.
// A review that no longer parses strictly is returned as that parse error.
func acceptedReview(p *Profile, digest string) ([]Finding, error) {
	content, err := os.ReadFile(p.ReviewPath)
	if err != nil {
		return nil, fmt.Errorf("burler: read review file %q: %w", p.ReviewPath, err)
	}
	if reviewDigest(content) != digest {
		return nil, fmt.Errorf("%w: %q no longer holds the bytes the review half accepted; only the review half writes it", ErrReviewChanged, p.ReviewPath)
	}
	_, findings, err := ParseCurrentReview(content)
	if err != nil {
		return nil, fmt.Errorf("burler: the accepted review file is invalid: %w", err)
	}
	return findings, nil
}

// checkGates runs every entry of gate whose Attempts is above 0 once, in order.
// It returns the findings of the entries that did not pass, each under its entry name.
// It returns "" when every entry passes.
// A closure error is returned wrapped with the entry's name.
func checkGates(gate shuttleengine.GateSpec) (string, error) {
	var failed []string
	for _, entry := range gate {
		if entry.Attempts <= 0 {
			continue
		}
		result, err := entry.Gate()
		if err != nil {
			return "", fmt.Errorf("burler: gate %q could not run: %w", entry.Name, err)
		}
		if !result.Passed {
			failed = append(failed, fmt.Sprintf("## Gate %s\n\n%s\n", entry.Name, strings.TrimSpace(result.Findings)))
		}
	}
	return strings.Join(failed, "\n"), nil
}

// ProbeFix reports the state of the fix half p describes under opts, without waiting on it.
// The state is live when the seats engine finds a live fixer run.
// It is done when there is no live run and the fixer report is present; otherwise it is gone.
// A probe error and an invalid p are returned as is.
// A fixer that could not be stopped is wrapped in ErrHalfNotStopped.
func (e *Engine) ProbeFix(p Profile, opts RunOpts) (LiveHalf, error) {
	if err := p.validateFix(e.geom.WorktreeRoot, e.cfg); err != nil {
		return LiveHalf{}, err
	}
	table := p.fixTable(opts, fixSeatValues(&p, fixFilesIn(e.probeDirOf())), e.seatSkills(&p))
	return e.probeHalf(table, p.FixerReportPath)
}

// promptDeliverer is the optional capability of a live handle that can deliver a task prompt its agent never received.
// DeliverPendingPrompt types the prompt when the prompt's turn never started and reports whether it did.
type promptDeliverer interface {
	DeliverPendingPrompt() (bool, error)
}

// ResumeFix completes the fix half whose fixer is live, attaching to that fixer instead of starting one.
// It then concludes the half against digest.
// A LiveHalf that is not live is an error, and so is one without the table from its probe.
// Nothing is stopped or started.
// A live handle that can deliver a pending task prompt is asked to before the seats resume,
// and a delivery error is returned as the resume's error with nothing resumed.
func (e *Engine) ResumeFix(p Profile, opts RunOpts, digest string, live LiveHalf) (FixResult, error) {
	if live.State != HalfLive {
		return FixResult{}, fmt.Errorf("burler: resume needs the fix half live (state %d)", live.State)
	}
	if live.table == nil {
		return FixResult{}, fmt.Errorf("burler: resume needs the table ProbeFix found; way forward: take the LiveHalf from ProbeFix, or run the row again so it starts the fix fresh")
	}
	if err := p.validateFix(e.geom.WorktreeRoot, e.cfg); err != nil {
		return FixResult{}, err
	}

	logger.Info("burler: resuming a live fix", "round", opts.Round, "fixStrand", live.Handle.StrandGUID())

	if deliverer, ok := live.Handle.(promptDeliverer); ok {
		sent, err := deliverer.DeliverPendingPrompt()
		if err != nil {
			return FixResult{}, fmt.Errorf("burler: deliver the fix's task prompt before resuming: %w", err)
		}
		if sent {
			logger.Info("burler: delivered the fix's task prompt before resuming", "round", opts.Round, "fixStrand", live.Handle.StrandGUID())
		}
	}

	table := p.fixTable(opts, fixSeatValues(&p, fixFilesIn(e.probeDirOf())), e.seatSkills(&p))
	seats, err := e.seats.Resume(table, *live.table)
	return e.concludeFix(&p, opts, digest, seats, err)
}

// concludeFix turns the seats answer, the fixer's result and the run error into the fix half's one result.
// A seat that could not be stopped returns ErrHalfNotStopped wrapped around the run error.
// A fixer that never started is reported as died, with NotStarted set and no error.
// A twill fixer whose engine lacks fork support returns a refusal that names the model key and the fan key.
// A done fixer of a twill round is audited first.
// A fork violation is returned as an error whether or not the gate passed.
// A done fixer whose gate failed returns its Gate with a nil error.
// A done fixer with a passing or absent gate is accepted only when the review file still hashes to digest;
// otherwise it is ErrReviewChanged.
func (e *Engine) concludeFix(p *Profile, opts RunOpts, digest string, seats seatengine.Result, runErr error) (FixResult, error) {
	fixer := seats.Chair
	base := FixResult{Outcome: fixer.Outcome, FixerReportPath: p.FixerReportPath, Fix: halfOf(fixer, "", "")}
	if p.FixFan != "" {
		base.Lenses = lensNames(p.fixLenses)
	}
	switch {
	case runErr == nil:
	case errors.Is(runErr, seatengine.ErrSeatNotStopped):
		return base, fmt.Errorf("%w: %w", ErrHalfNotStopped, runErr)
	case errors.Is(runErr, shuttleengine.ErrNotStarted):
		return FixResult{Outcome: shuttleengine.OutcomeDied, FixerReportPath: p.FixerReportPath, Fix: Half{StartError: runErr.Error()}, NotStarted: true}, nil
	case p.FixFan != "" && errors.Is(runErr, shuttleengine.ErrRequirementsUnmet):
		return FixResult{}, twillEngineRefusal(opts, runErr)
	default:
		return FixResult{}, fmt.Errorf("burler: shuttle run: %w", runErr)
	}

	logger.Info("burler: fix joined", "round", opts.Round, "fixOutcome", fixer.Outcome)

	if fixer.Outcome != shuttleengine.OutcomeDone {
		return base, nil
	}
	base.Gate = fixer.Gate
	if p.FixFan != "" {
		warnings, forks, err := auditFixRound(fixer.ForkAudit, len(p.fixLenses))
		for _, warning := range warnings {
			logger.Warn("burler: "+warning, "round", opts.Round)
		}
		base.Forks, base.Warnings = forks, warnings
		if err != nil {
			return base, err
		}
	}
	if base.Gate != nil && !base.Gate.Passed {
		return base, nil
	}
	if _, err := acceptedReview(p, digest); err != nil {
		return base, err
	}
	return base, nil
}

// lensNames returns the names of resolved in order.
func lensNames(resolved []lenses.Lens) []string {
	names := make([]string, len(resolved))
	for i, lens := range resolved {
		names[i] = lens.Name
	}
	return names
}

// engineRefusalError is a refusal whose text is its message.
// It matches shuttleengine.ErrRequirementsUnmet under errors.Is.
type engineRefusalError struct{ message string }

func (e *engineRefusalError) Error() string { return e.message }

func (e *engineRefusalError) Unwrap() error { return shuttleengine.ErrRequirementsUnmet }

// twillEngineRefusal words the refusal of a twill fixer whose engine cannot run fork subagents.
// It names the model key and the fan key from opts.
// err is the start error wrapping shuttleengine.ErrRequirementsUnmet.
// Its text is cut at the last way-forward clause, so the message carries only one.
// That clause is the one for the caller that supplied the keys.
// A fixer told a fixer model key (a shed row) is moved to an engine with forks and resumed;
// one told none (lym burler run) takes the default engine of shuttle.yaml or drops the fan from its profile.
func twillEngineRefusal(opts RunOpts, err error) error {
	const wayForwardMarker = "; way forward: "
	refusal := err.Error()
	if cut := strings.LastIndex(refusal, wayForwardMarker); cut >= 0 {
		refusal = refusal[:cut]
	}
	modelKey := "engine: in shuttle.yaml"
	step := fmt.Sprintf("set engine: in shuttle.yaml to an engine with forks, or remove %s from the profile, and re-run", opts.FixFanKey)
	if opts.FixKey.Key != "" {
		modelKey = fmt.Sprintf("%s in %s.yaml", opts.FixKey.Key, opts.FixKey.Module)
		step = fmt.Sprintf("move %s to an engine with forks, then resume; setting the %s entry to \"\" reaches only rounds whose reviewer is spawned after the edit", modelKey, opts.FixFanKey)
	}
	return &engineRefusalError{message: fmt.Sprintf("burler: the twill fixer's fan from %s needs fork subagents, which the engine of %s cannot run: %s%s%s", opts.FixFanKey, modelKey, refusal, wayForwardMarker, step)}
}
