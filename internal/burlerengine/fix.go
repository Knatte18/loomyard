// fix.go implements the fix half of a round: Engine.Fix runs one fresh fixer session as the chair of a one-seat table over a review the review half accepted, and ProbeFix and ResumeFix attach to a fixer a previous session left live.
// The half neither waits on, messages nor knows of the review half; it knows the review only by the digest it is told and the file it reads.
// A review with no findings whose gates pass spawns no fixer at all.

package burlerengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fixRolePrefix is the role prefix of the fix half's one seat; the seats engine forms its strand role from it.
const fixRolePrefix = "burler-fix"

// skippedFixerReport is the fixer report written when the fix half is skipped: a normal report saying nothing was fixed because the review held no findings and the round's gates passed.
var skippedFixerReport = "# Fixer report\n\n" +
	"## Changes\n\n" +
	"Nothing fixed: the review held no findings and the round's gates passed, so no fixer was spawned.\n\n" +
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
}

// fixTable builds the fix half's one-seat table for p under opts: the fixer as the chair, gated by opts.Gate with every entry wrapped by repairReportBeforeGate.
// The fixer reads the review file, writes the fixer-report and never forks.
// values carries the marker values the seat's stencil is filled from.
func (p *Profile) fixTable(opts RunOpts, values map[string]string) seatengine.Table {
	// A fresh copy, so the caller's slice is never mutated;
	// off entries are wrapped too, since they never run and the wrap is harmless there.
	fixGate := make(shuttleengine.GateSpec, 0, len(opts.Gate))
	for _, entry := range opts.Gate {
		entry.Gate = repairReportBeforeGate(entry.Gate, p.FixerReportPath)
		fixGate = append(fixGate, entry)
	}
	return seatengine.Table{
		RolePrefix: fixRolePrefix,
		Segment:    p.segment(),
		Gate:       fixGate,
		Round:      opts.Round,
		Timeout:    opts.Timeout,
		Seats: []seatengine.Seat{
			{
				Name:    seatengine.RoleChair,
				Stencil: "burler-template-fixer",
				Model:   opts.Fix.Model,
				Effort:  opts.Fix.Effort,
				Version: opts.Fix.Version,
				Inputs:  []string{p.ReviewPath},
				Outputs: []string{p.FixerReportPath},
				Skills:  burlerSkills,
				Values:  values,
			},
		},
	}
}

// Fix runs the fix half for p, tuned by opts, over the review whose accepted bytes hash to digest.
// It validates p, then reads the review through acceptedReview, which returns ErrReviewChanged when the file no longer hashes to digest.
// A review with findings of any severity spawns the fixer.
// A review with none runs opts.Gate once: when every gate passes, Fix writes the fixer report itself and returns a skipped OutcomeDone with no session;
// when a gate fails, the fixer is spawned with the failing gates' findings as its work; a gate whose closure errors is returned wrapped with nothing spawned.
// A fixer that never starts because its provider never came up is OutcomeDied with NotStarted set and a nil error.
// A fixer that died or timed out is its outcome with a nil error, and one whose gate failed returns its Gate with a nil error.
// A fixer that is done while the review file changed is ErrReviewChanged.
// A fixer that could not be stopped is returned wrapped in ErrHalfNotStopped.
func (e *Engine) Fix(p Profile, opts RunOpts, digest string) (FixResult, error) {
	if err := p.validateFix(e.geom.WorktreeRoot); err != nil {
		return FixResult{}, err
	}
	findings, err := acceptedReview(&p, digest)
	if err != nil {
		return FixResult{}, err
	}

	logger.Info("burler: fix starting", "round", opts.Round, "findings", len(findings), "fixerReportPath", p.FixerReportPath)

	gateFindings := ""
	if len(findings) == 0 {
		gateFindings, err = checkGates(opts.Gate)
		if err != nil {
			return FixResult{}, err
		}
		if gateFindings == "" {
			return e.skipFix(&p)
		}
	}

	// A friction.Directive error is swallowed, never returned:
	// it is optional bookkeeping, not a binding constraint, so a transient stencil read failure here must never kill the round.
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
	prompt, err := composeFixPrompt(e.stencilsDir, &p, frictionDirective, fixWork(p.ReviewPath, gateFindingsPath), paths)
	if err != nil {
		return FixResult{}, err
	}
	if err := writeInstructionFiles(prompt.Files); err != nil {
		return FixResult{}, err
	}

	seats, err := e.seats.Run(p.fixTable(opts, prompt.Values))
	return e.concludeFix(&p, opts, digest, seats, err)
}

// skipFix writes the skipped fixer report for p and returns the skipped fix result.
func (e *Engine) skipFix(p *Profile) (FixResult, error) {
	if err := os.MkdirAll(filepath.Dir(p.FixerReportPath), 0o755); err != nil {
		return FixResult{}, fmt.Errorf("burler: create the fixer report directory for %q: %w", p.FixerReportPath, err)
	}
	if err := os.WriteFile(p.FixerReportPath, []byte(skippedFixerReport), 0o644); err != nil {
		return FixResult{}, fmt.Errorf("burler: write the skipped fixer report %q: %w", p.FixerReportPath, err)
	}
	logger.Info("burler: fix skipped, the review held no findings and the gates pass", "fixerReportPath", p.FixerReportPath)
	return FixResult{Outcome: shuttleengine.OutcomeDone, FixerReportPath: p.FixerReportPath, Skipped: true}, nil
}

// acceptedReview reads the review file at p.ReviewPath and returns its findings, refusing with ErrReviewChanged when the bytes no longer hash to digest.
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

// checkGates runs every entry of gate whose Attempts is above 0 once, in order, and returns the findings of the entries that did not pass, each under its entry name.
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

// ProbeFix reports the state of the fix half p describes under opts, without waiting on it:
// live when the seats engine finds a live fixer run, done when it has no live run and the fixer-report is present, and gone otherwise.
// A probe error, and an invalid p, are returned as is, a fixer that could not be stopped wrapped in ErrHalfNotStopped.
func (e *Engine) ProbeFix(p Profile, opts RunOpts) (LiveHalf, error) {
	if err := p.validateFix(e.geom.WorktreeRoot); err != nil {
		return LiveHalf{}, err
	}
	table := p.fixTable(opts, fixSeatValues(&p, fixFilesIn(e.probeDirOf())))
	return e.probeHalf(table, p.FixerReportPath)
}

// promptDeliverer is the optional capability of a live handle that can deliver a task prompt its agent never received.
// DeliverPendingPrompt types the prompt when the prompt's turn never started and reports whether it did.
type promptDeliverer interface {
	DeliverPendingPrompt() (bool, error)
}

// ResumeFix completes the fix half whose fixer is live, attaching to it instead of starting one, and concludes it as a fresh fix is, against the same digest.
// A LiveHalf that is not live is an error, and so is one with no probed table, which only a LiveHalf not taken from ProbeFix lacks; nothing is stopped or started.
// A live handle that can deliver a pending task prompt is asked to before the seats resume, and a delivery error is returned as the resume's error with nothing resumed.
func (e *Engine) ResumeFix(p Profile, opts RunOpts, digest string, live LiveHalf) (FixResult, error) {
	if live.State != HalfLive {
		return FixResult{}, fmt.Errorf("burler: resume needs the fix half live (state %d)", live.State)
	}
	if live.table == nil {
		return FixResult{}, fmt.Errorf("burler: resume needs the table ProbeFix found; way forward: take the LiveHalf from ProbeFix, or run the row again so it starts the fix fresh")
	}
	if err := p.validateFix(e.geom.WorktreeRoot); err != nil {
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

	table := p.fixTable(opts, fixSeatValues(&p, fixFilesIn(e.probeDirOf())))
	seats, err := e.seats.Resume(table, *live.table)
	return e.concludeFix(&p, opts, digest, seats, err)
}

// concludeFix turns the seats engine's answer, the fixer's result and the error it ran with, into the fix half's one result.
// Stops, start errors and a died or timed-out fixer are mapped as concludeReview maps them.
// A done fixer whose gate failed returns its Gate with a nil error.
// A done fixer with a passing or absent gate is accepted only when the review file still hashes to digest, and is ErrReviewChanged otherwise.
func (e *Engine) concludeFix(p *Profile, opts RunOpts, digest string, seats seatengine.Result, runErr error) (FixResult, error) {
	fixer := seats.Chair
	base := FixResult{Outcome: fixer.Outcome, FixerReportPath: p.FixerReportPath, Fix: halfOf(fixer, "", "")}
	switch {
	case runErr == nil:
	case errors.Is(runErr, seatengine.ErrSeatNotStopped):
		return base, fmt.Errorf("%w: %w", ErrHalfNotStopped, runErr)
	case errors.Is(runErr, shuttleengine.ErrNotStarted):
		return FixResult{Outcome: shuttleengine.OutcomeDied, FixerReportPath: p.FixerReportPath, Fix: Half{StartError: runErr.Error()}, NotStarted: true}, nil
	default:
		return FixResult{}, fmt.Errorf("burler: shuttle run: %w", runErr)
	}

	logger.Info("burler: fix joined", "round", opts.Round, "fixOutcome", fixer.Outcome)

	if fixer.Outcome != shuttleengine.OutcomeDone {
		return base, nil
	}
	base.Gate = fixer.Gate
	if base.Gate != nil && !base.Gate.Passed {
		return base, nil
	}
	if _, err := acceptedReview(p, digest); err != nil {
		return base, err
	}
	return base, nil
}
