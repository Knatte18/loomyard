// gates.go implements this package's gate closures: NewDiscussionGate, NewDarnDiscussionGate, NewPlanGate, NewLensReviewPlanGate and NewReworkPlanGate.
// Each is the gate half of the Gate Self-Check Parity Invariant -- each calls the identical package
// function its CLI self-check verb does (ValidateLoomDiscussion, ValidateDarnDiscussion, ValidatePlan, ValidatePlanLensReview and
// ValidateReworkPlan, respectively), so the operator's own self-check verb and the automated gate can never disagree
// about what "valid" means.

package loomshed

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// formatDiscussionFindings renders findings as a single semicolon-separated list, using each
// Finding's own Error() rendering so the log line and the standalone verb's envelope describe a
// violation identically.
func formatDiscussionFindings(findings []discussionparser.Finding) string {
	parts := make([]string, len(findings))
	for i, f := range findings {
		parts[i] = f.Error()
	}
	return strings.Join(parts, "; ")
}

// formatPlanFindings renders findings as a single semicolon-separated list, each entry carrying
// its own Check[/Card]: Detail rendering plus its Severity, so the log line and "lyx loom
// validate-plan"'s envelope describe a violation identically and an informational
// create-new-unit is distinguishable from a blocking glyph-not-found in the one place this record
// exists.
// It calls each Finding's own Error() rather than re-deriving its layout, which is what makes the
// "describe a violation identically" promise above hold by construction rather than by two copies of
// one format string agreeing today; formatDiscussionFindings above does the same.
func formatPlanFindings(findings []planindex.Finding) string {
	parts := make([]string, len(findings))
	for i, f := range findings {
		parts[i] = f.Error()
	}
	return strings.Join(parts, "; ")
}

// writeInformationalFindings writes findings, one per line, to <gateName>-informational-findings.txt under dir, creating dir when it is missing, and returns the file's path.
func writeInformationalFindings(dir, gateName string, findings []planindex.Finding) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	lines := make([]string, len(findings))
	for i, f := range findings {
		lines[i] = formatPlanFindings([]planindex.Finding{f})
	}
	path := filepath.Join(dir, gateName+"-informational-findings.txt")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// hasBlockingFinding reports whether findings carries at least one entry that is not explicitly
// informational.
//
// It tests NOT-informational rather than equals-blocking, and that asymmetry is the point.
// planindex.Severity is an open string type, so an unrecognized value — or the zero value, which a
// hand-built Finding or a future producer that forgets to stamp one carries — took the informational
// branch, logged a Warn, and returned Done with the plan directory as its pointer: the run advanced
// past a finding that was meant to block it. That is the same "a validator's complaint reported as a
// clean plan" failure this file's own error-path comment records as deliberately rejected, still
// present on the severity path (crucible round opus-medium-r6, R6-27).
// Failing closed costs at most a spurious bounce on a severity nobody has defined yet; failing open
// costs a dispatched batch over a defect the gate saw.
func hasBlockingFinding(findings []planindex.Finding) bool {
	for _, f := range findings {
		if f.Severity != planindex.SeverityInformational {
			return true
		}
	}
	return false
}

// NewDiscussionGate returns the Discussion-Write row's gate closure: a shuttleengine.Gate that
// calls ValidateLoomDiscussion(decisionRecordPath, supportLogPath, relevantPath) once.
// It maps the result onto the gate contract.
//
// The three paths are told rather than derived, because loomengine's own accessors for them take a
// *lyxcwd.Location this package may not import.
// A relevantPath nobody wrote passes, since the relevant list is optional.
//
// A non-nil error from ValidateLoomDiscussion is returned verbatim alongside the zero shuttleengine.GateResult: a
// gate that could not read the artifact has found no defect to re-prompt over, it is an
// infrastructure fault, and it must never burn an attempt or reach the LLM. A non-empty findings
// slice produces GateResult{Passed: false, Findings: formatDiscussionFindings(findings)} and a nil
// error, preceded by a logger.Warn carrying the gate name, the decision record path, and the same
// formatted findings -- that warn line is the only durable record of why an artifact was refused,
// because the findings file itself lives in the ephemeral run directory finalize deletes on the Done
// cleanup. An empty slice produces GateResult{Passed: true} and a nil error.
func NewDiscussionGate(decisionRecordPath, supportLogPath, relevantPath string) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		findings, err := ValidateLoomDiscussion(decisionRecordPath, supportLogPath, relevantPath)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if len(findings) == 0 {
			return shuttleengine.GateResult{Passed: true}, nil
		}

		formatted := formatDiscussionFindings(findings)
		logger.Warn("loomshed: discussion gate failed validation", "gate", "Discussion-Gate", "decisionRecord", decisionRecordPath, "findings", formatted)
		return shuttleengine.GateResult{Passed: false, Findings: formatted}, nil
	}
}

// ValidateLoomDiscussion is the loom run's discussion check: discussionparser.Validate's decision record and support log check, then discussionparser.CheckRelevant's relevant list check.
// CheckRelevant runs even when Validate finds defects, so one re-prompt names every defect, and the findings are Validate's followed by CheckRelevant's.
// A missing relevant list adds no finding.
// Either function's error is returned as is, with nil findings.
func ValidateLoomDiscussion(decisionRecordPath, supportLogPath, relevantPath string) ([]discussionparser.Finding, error) {
	findings, err := discussionparser.Validate(decisionRecordPath, supportLogPath)
	if err != nil {
		return nil, err
	}
	relevantFindings, err := discussionparser.CheckRelevant(relevantPath)
	if err != nil {
		return nil, err
	}
	return append(findings, relevantFindings...), nil
}

// ValidateDarnDiscussion is the darn run's discussion check: discussionparser.Validate's section check, then the history check.
// A record Validate finds defects in comes back with those findings alone, since a missing or section-less record has no history worth comparing yet.
// Otherwise history supplies every committed version of the record, and the findings are the added-decision entries the record dropped from them.
// A read error and a history error are returned errors, as Validate's I/O errors are.
func ValidateDarnDiscussion(decisionRecordPath, supportLogPath string, history func() ([][]byte, error)) ([]discussionparser.Finding, error) {
	findings, err := discussionparser.Validate(decisionRecordPath, supportLogPath)
	if err != nil || len(findings) > 0 {
		return findings, err
	}

	record, err := os.ReadFile(decisionRecordPath)
	if err != nil {
		return nil, err
	}
	versions, err := history()
	if err != nil {
		return nil, err
	}

	findings = discussionparser.CheckAddedDecisionsKept(record, versions)
	for i := range findings {
		findings[i].Path = decisionRecordPath
	}
	return findings, nil
}

// NewDarnDiscussionGate returns the darn run's discussion gate closure: a shuttleengine.Gate that calls ValidateDarnDiscussion and commits the record once the check is clean.
// Findings fail the gate with the same formatted findings and warn line NewDiscussionGate produces, and commit nothing.
// A clean check calls commit and passes; a commit error is returned wrapped with the decision record path, so the handoff halts as a transient fault instead of passing over an uncommitted record.
// A repeat pass over an unchanged record commits nothing new when commit treats a clean path as a no-op.
func NewDarnDiscussionGate(decisionRecordPath, supportLogPath string, history func() ([][]byte, error), commit func() error) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		findings, err := ValidateDarnDiscussion(decisionRecordPath, supportLogPath, history)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if len(findings) > 0 {
			formatted := formatDiscussionFindings(findings)
			logger.Warn("loomshed: discussion gate failed validation", "gate", "Discussion-Gate", "decisionRecord", decisionRecordPath, "findings", formatted)
			return shuttleengine.GateResult{Passed: false, Findings: formatted}, nil
		}
		if err := commit(); err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("loomshed: commit decision record %s: %w", decisionRecordPath, err)
		}
		return shuttleengine.GateResult{Passed: true}, nil
	}
}

// NewPlanGate returns the Plan-Write row's gate closure, a shuttleengine.Gate that parses the plan through planparser.ParsePlan(planparser.PlanDir(anchorPath)) and then runs ValidatePlan(plan, anchorPath, worktreeRoot, index), mapping the result onto the gate contract.
//
// The two path parameters are separate because planparser.PlanDir and webster's run record take the anchor path while index.ValidateFormat takes the worktree root, and they are not the same value.
//
// ValidateFormat is called, never the require_approved-aware validation.
// Both plan gate sites run strictly before the Plan-Review segment's approve seam writes the approval flag, so demanding it would fail every single fix round.
//
// The ParsePlan error set is split by shape rather than treated whole. An error satisfying
// errors.As(err, new(*fs.PathError)) is a returned error: ParsePlan has exactly two %w-wrapped
// os.ReadFile faults, the overview read and the per-card read reached through parseCardFile, and
// this predicate matches both and only those, because every other error it returns is a plain
// fmt.Errorf value. Every other ParsePlan error produces GateResult{Passed: false, Findings:
// err.Error()} and a nil error: the not-exist branch and every structural or format error are plain
// fmt.Errorf values by construction, and they describe the bytes the agent wrote, which is the
// single most LLM-fixable defect class there is.
//
// This is a reasoned reversal of the disposition the now-removed Plan-Validate row's producer took:
// that producer's rationale -- a plan that will not parse is not a plan the bounce target can be
// asked to improve -- was about a cold respawn that knows nothing of the complaint, whereas the
// gate's bounce target is the live session that just wrote the file, holding its full context, so the
// premise no longer holds. This also makes both gates behave identically on a missing-or-malformed
// artifact, since discussionparser.Validate already reports a missing file as a finding and only a
// non-not-exist read failure as an error.
//
// Every index error stays a returned error in full and is explicitly NOT part of the carve-out:
// a resolve or quarry failure means the gate could not read the code, not that it found a defect,
// and three absurd re-prompts over a missing quarry binary would hide the real fault.
//
// A findings slice carrying no entry that fails hasBlockingFinding is a pass: it is logged as a
// logger.Warn naming it informational and returns GateResult{Passed: true}, because a
// create-new-unit finding on a brand-new package is not something the writer can fix and
// re-prompting on it would burn the whole budget on a condition that was never wrong. A slice with
// at least one blocking entry produces GateResult{Passed: false, Findings:
// formatPlanFindings(findings)} after a logger.Warn carrying the same formatted text.
//
// hasBlockingFinding is called, never re-derived.
// It encodes a crucible-round finding that planindex.Severity is an open string type, so testing not-informational rather than equals-blocking is what keeps an unrecognized or zero-valued severity from silently passing.
//
// scratchDir is the run's told ephemeral scratch directory, where an informational pass records its findings; empty keeps them inline in the log line.
func NewPlanGate(anchorPath, worktreeRoot, scratchDir string, index planindex.Index) shuttleengine.Gate {
	return planGate("Plan-Gate", anchorPath, scratchDir, func(plan *planparser.Plan) ([]planindex.Finding, error) {
		return ValidatePlan(plan, anchorPath, worktreeRoot, index)
	})
}

// ValidatePlan is the plan gate's check set: index.ValidateFormat over plan, told the cards webster's run record under anchorPath holds done.
// A run moved back to the plan review after Webster executed batches keeps its run record, and a done card's work is already in the tree, so the index skips its tree-dependent checks.
// With no run record every card is checked.
// A run record that cannot be read is a returned error, never a finding.
//
// A done card whose file no longer hashes to what its batch recorded at begin adds one blocking done-card-edited finding, since a done card is frozen.
// It reads the record and the card bytes alone; an unreadable card file is a returned error.
func ValidatePlan(plan *planparser.Plan, anchorPath, worktreeRoot string, index planindex.Index) ([]planindex.Finding, error) {
	st, err := websterengine.LoadState(websterengine.Dir(anchorPath), websterengine.ScratchDir(anchorPath))
	if err != nil {
		return nil, err
	}
	findings, err := index.ValidateFormat(plan, worktreeRoot, websterengine.DoneCards(plan, st))
	if err != nil {
		return nil, err
	}
	edited, err := websterengine.EditedDoneCards(plan, st)
	if err != nil {
		return nil, err
	}
	for _, id := range edited {
		findings = append(findings, planindex.Finding{
			Check:    doneCardEditedCheck,
			Card:     id,
			Detail:   "a done card is frozen: its work has landed, so a finding against it goes in a follow-up card placed after the last begun batch, with its Card Index line in 00-overview.md",
			Severity: planindex.SeverityBlocking,
		})
	}
	return findings, nil
}

// doneCardEditedCheck names the finding ValidatePlan appends for an edited done card.
const doneCardEditedCheck = "done-card-edited"

// NewLensReviewPlanGate returns the plan gate closure a twill-spawned Plan-Write run evaluates in place of NewPlanGate's, chosen by shuttle from the run's recorded fork authorization.
// It parses the plan under anchorPath and checks it with ValidatePlanLensReview over index.
//
// Parse failures, the blocking-versus-informational split and the logging follow the plan gate's contract.
// It only adds a refusal over NewPlanGate's set: a writer that never records the lens review spends the gate's existing re-prompt budget.
func NewLensReviewPlanGate(anchorPath, worktreeRoot, scratchDir string, index planindex.Index) shuttleengine.Gate {
	return planGate("Plan-Gate", anchorPath, scratchDir, func(plan *planparser.Plan) ([]planindex.Finding, error) {
		return ValidatePlanLensReview(plan, anchorPath, worktreeRoot, index)
	})
}

// ValidatePlanLensReview is the check set of a twill Plan-Write run's gate: ValidatePlan's set, plus one blocking lens-review-missing finding when the plan directory's lens review file is absent or holds only whitespace.
// An error from ValidatePlan is returned unchanged.
// The file is checked for presence only and never parsed; its content and timing are not checked.
// Any read error but not-exist is a returned error, never a finding.
func ValidatePlanLensReview(plan *planparser.Plan, anchorPath, worktreeRoot string, index planindex.Index) ([]planindex.Finding, error) {
	findings, err := ValidatePlan(plan, anchorPath, worktreeRoot, index)
	if err != nil {
		return nil, err
	}
	lensReviewPath := filepath.Join(plan.Dir, planparser.LensReviewFileName)
	content, err := os.ReadFile(lensReviewPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err != nil || strings.TrimSpace(string(content)) == "" {
		findings = append(findings, planindex.Finding{
			Check:    lensReviewMissingCheck,
			Detail:   fmt.Sprintf("no lens review is recorded at %s: record it there as the lens review directive describes", lensReviewPath),
			Severity: planindex.SeverityBlocking,
		})
	}
	return findings, nil
}

// lensReviewMissingCheck names the finding ValidatePlanLensReview appends for an absent or empty lens review.
const lensReviewMissingCheck = "lens-review-missing"

// NewReworkPlanGate returns the PR-Rework row's gate closure: a shuttleengine.Gate that parses the plan under anchorPath and checks it with ValidateReworkPlan over index.
// The whole new plan is held to the plan-format checks plus the told first_card.
// readCommitted returns an anchor-relative file as committed at HEAD, with found false when HEAD has no such file.
//
// Parse failures, the blocking-versus-informational split and the logging follow the plan gate's contract.
// A plan that cannot be read as committed at HEAD is a returned error, never a finding: the rework session cannot fix a missing baseline.
func NewReworkPlanGate(anchorPath, worktreeRoot, scratchDir string, index planindex.Index, readCommitted func(anchorRel string) ([]byte, bool, error)) shuttleengine.Gate {
	return planGate("Rework-Plan-Gate", anchorPath, scratchDir, func(plan *planparser.Plan) ([]planindex.Finding, error) {
		return ValidateReworkPlan(plan, worktreeRoot, index, readCommitted)
	})
}

// ValidateReworkPlan checks the whole new plan with index.ValidateRework: the plan gate's format-only set, plus the check that first_card equals the number Go told the session.
// The told number is NextReworkCardNumber over the plan committed at HEAD, which still holds the retired generation until the round commit, so it is stable for the gate's whole window.
// readCommitted returns an anchor-relative file as committed at HEAD, with found false when HEAD has no such file.
// A plan that cannot be read as committed at HEAD is a returned error.
func ValidateReworkPlan(plan *planparser.Plan, worktreeRoot string, index planindex.Index, readCommitted func(anchorRel string) ([]byte, bool, error)) ([]planindex.Finding, error) {
	told, err := NextReworkCardNumber(plan.Dir, readCommitted)
	if err != nil {
		return nil, fmt.Errorf("read the plan committed at HEAD: %w", err)
	}
	return index.ValidateRework(plan, worktreeRoot, told)
}

// planGate builds a plan gate closure named gateName in its log lines: it parses the plan under anchorPath, runs validate over it, and maps the result onto the gate contract.
// On an informational pass it records the findings in a file under scratchDir and logs their count and the file's path;
// with no scratchDir, or when the file cannot be written, the log line carries the findings inline.
func planGate(gateName, anchorPath, scratchDir string, validate func(*planparser.Plan) ([]planindex.Finding, error)) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		planDir := planparser.PlanDir(anchorPath)
		plan, err := planparser.ParsePlan(planDir)
		if err != nil {
			if errors.As(err, new(*fs.PathError)) {
				return shuttleengine.GateResult{}, err
			}
			return shuttleengine.GateResult{Passed: false, Findings: err.Error()}, nil
		}

		findings, err := validate(plan)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}

		if !hasBlockingFinding(findings) {
			if scratchDir != "" {
				path, writeErr := writeInformationalFindings(scratchDir, gateName, findings)
				if writeErr == nil {
					logger.Warn("loomshed: plan gate surfaced informational findings", "gate", gateName, "planDir", planDir, "count", len(findings), "file", path)
					return shuttleengine.GateResult{Passed: true}, nil
				}
				logger.Warn("loomshed: plan gate could not record its informational findings", "gate", gateName, "error", writeErr.Error())
			}
			logger.Warn("loomshed: plan gate surfaced informational findings", "gate", gateName, "planDir", planDir, "findings", formatPlanFindings(findings))
			return shuttleengine.GateResult{Passed: true}, nil
		}

		formatted := formatPlanFindings(findings)
		logger.Warn("loomshed: plan gate failed validation", "gate", gateName, "planDir", planDir, "findings", formatted)
		return shuttleengine.GateResult{Passed: false, Findings: formatted}, nil
	}
}
