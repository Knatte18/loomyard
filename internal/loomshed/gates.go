// gates.go holds the gate closures of loomshed: NewDiscussionGate, NewPlanGate and NewLensReviewPlanGate.
// It also holds ValidateReworkPlan, the check that `lym loom validate-plan --rework` runs.
// Each gate calls the same check as its self-check verb.
// The verb and the automated gate can therefore never disagree about what is valid.
// This is the gate half of the Gate Self-Check Parity Invariant.

package loomshed

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// formatDiscussionFindings renders findings as one semicolon-separated list.
// Each finding renders through its own Error method.
// The log line and the verb's output therefore describe a violation identically.
func formatDiscussionFindings(findings []discussionparser.Finding) string {
	parts := make([]string, len(findings))
	for i, f := range findings {
		parts[i] = f.Error()
	}
	return strings.Join(parts, "; ")
}

// formatPlanFindings renders findings as one semicolon-separated list.
// Each finding renders through its own Error method, which carries its check, card, detail and severity.
// The log line and the validate-plan output therefore describe a violation identically.
// Calling Error keeps the two renderings identical by construction rather than by two copies of one format agreeing.
func formatPlanFindings(findings []planindex.Finding) string {
	parts := make([]string, len(findings))
	for i, f := range findings {
		parts[i] = f.Error()
	}
	return strings.Join(parts, "; ")
}

// writeInformationalFindings writes findings, one per line, to <gateName>-informational-findings.txt under dir.
// It creates dir when it is missing and returns the path of the file it wrote.
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

// hasBlockingFinding reports whether findings holds at least one entry that is not informational.
//
// It tests not-informational rather than blocking, because planindex.Severity is an open string type.
// A value the code does not recognize, and the zero value a hand-built Finding carries, must count as blocking:
// treated as informational, such a finding logs a warning and lets the run advance past a defect the gate saw.
// Failing closed costs at most a spurious bounce on a severity nobody has defined yet;
// failing open costs a dispatched batch over a defect the gate saw.
func hasBlockingFinding(findings []planindex.Finding) bool {
	for _, f := range findings {
		if f.Severity != planindex.SeverityInformational {
			return true
		}
	}
	return false
}

// DiscussionCheck carries the values the discussion check reads for one loom record.
// Every path is absolute.
type DiscussionCheck struct {
	// RecordPath is the decision record.
	RecordPath string
	// SupportLogPath is the support log.
	SupportLogPath string
	// RelevantPath is the relevant list, read only when Citations is set; a list nobody wrote passes.
	RelevantPath string
	// WorktreeRoot is the worktree the citation check tests the record's cited paths against; read only when Citations is set.
	WorktreeRoot string
	// Index resolves the symbols the record cites; read only when Citations is set.
	Index planindex.Index
	// History returns every committed version of the decision record, newest first.
	History func() ([][]byte, error)
	// Citations selects the citation and relevant-list checks, which a run whose record a person's discussion wrote takes.
	Citations bool
}

// NewDiscussionGate returns the gate closure of a loom record:
// a shuttleengine.Gate that calls ValidateDiscussion(check) once and maps the result onto the gate contract.
// It commits nothing.
//
// A non-nil error from ValidateDiscussion is returned with the zero shuttleengine.GateResult.
// The gate could not read the artifact, so it found no defect to re-prompt over: the error is an infrastructure fault,
// and it must never burn an attempt or reach the LLM.
// A non-empty findings slice produces GateResult{Passed: false, Findings: ...} and a nil error.
// It logs a logger.Warn with the gate name, the decision record path and the same formatted findings.
// That warn line is the only durable record of why the artifact was refused.
// The findings file lives in the ephemeral run directory that finalize deletes on the Done cleanup.
// An empty slice produces GateResult{Passed: true} and a nil error.
func NewDiscussionGate(check DiscussionCheck) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		findings, err := ValidateDiscussion(check)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if len(findings) == 0 {
			return shuttleengine.GateResult{Passed: true}, nil
		}

		formatted := formatDiscussionFindings(findings)
		logger.Warn("loomshed: discussion gate failed validation", "gate", "Discussion-Gate", "decisionRecord", check.RecordPath, "findings", formatted)
		return shuttleengine.GateResult{Passed: false, Findings: formatted}, nil
	}
}

// ValidateDiscussion is the discussion check every loom record runs.
// It runs discussionparser.Validate's section check over the record and support log.
// It then runs discussionparser.CheckDecisionIDs' decision id and acceptance check and the history check.
// With check.Citations set, it also runs the citation check and discussionparser.CheckRelevant's relevant list check.
// The history check runs only when the section check is clean.
// A missing or section-less record has no history worth comparing yet.
// The history check refuses a record that dropped an `Added after Discussion` entry.
// It also refuses one that dropped an appended PATTERN bullet its committed versions held.
// The citation check tests the record's cited paths against the worktree and its cited symbols through the index.
// Every other check runs even when an earlier one finds defects, so one re-prompt names every defect.
// The findings come in the order the checks run, each stamped with its path.
// A missing decision record adds no decision id or citation finding.
// A missing relevant list adds no relevant finding.
// A read error, a history error and a quarry outage are returned as is, with nil findings.
func ValidateDiscussion(check DiscussionCheck) ([]discussionparser.Finding, error) {
	findings, err := discussionparser.Validate(check.RecordPath, check.SupportLogPath)
	if err != nil {
		return nil, err
	}
	sectionsClean := len(findings) == 0

	record, found, err := readDecisionRecord(check.RecordPath)
	if err != nil {
		return nil, err
	}
	if found {
		findings = append(findings, stampFindings(discussionparser.CheckDecisionIDs(record), check.RecordPath)...)
	}
	if sectionsClean {
		versions, err := check.History()
		if err != nil {
			return nil, err
		}
		findings = append(findings, stampFindings(discussionparser.CheckAddedDecisionsKept(record, versions), check.RecordPath)...)
	}
	if !check.Citations {
		return findings, nil
	}

	if found {
		citations, err := citationFindings(check.WorktreeRoot, check.Index, discussionparser.Citations(record))
		if err != nil {
			return nil, err
		}
		findings = append(findings, stampFindings(citations, check.RecordPath)...)
	}
	relevantFindings, err := discussionparser.CheckRelevant(check.RelevantPath)
	if err != nil {
		return nil, err
	}
	return append(findings, relevantFindings...), nil
}

// readDecisionRecord reads the decision record, and reports found false for a record that does not exist.
func readDecisionRecord(decisionRecordPath string) (record []byte, found bool, err error) {
	record, err = os.ReadFile(decisionRecordPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return record, err == nil, err
}

// stampFindings sets the path of each of findings and returns them.
func stampFindings(findings []discussionparser.Finding, path string) []discussionparser.Finding {
	for i := range findings {
		findings[i].Path = path
	}
	return findings
}

// NewPlanGate returns the gate closure of the Plan-Write row.
// It is a shuttleengine.Gate that parses the plan through planparser.ParsePlan(planparser.PlanDir(anchorPath)).
// It then runs ValidatePlan(plan, anchorPath, worktreeRoot, index, pc) and maps the result onto the gate contract.
//
// The two path parameters are separate because planparser.PlanDir and the webster run record take the anchor path.
// index.ValidateFormat takes the worktree root.
//
// It calls ValidateFormat and not the validation that demands `require_approved`.
// Both plan gate sites run before the Plan-Review approve seam writes the approval flag.
// Demanding that flag would fail every fix round.
//
// The ParsePlan error set is split by shape rather than treated whole.
// An error satisfying errors.As(err, new(*fs.PathError)) is a returned error.
// ParsePlan wraps its file read faults with %w, and every other error it returns is a plain fmt.Errorf value.
// Every other ParsePlan error produces GateResult{Passed: false, Findings: err.Error()} and a nil error.
//
// Those errors describe the bytes the agent wrote, the most LLM-fixable defect class there is:
// the live session that wrote the file still holds its full context, so it can be asked to fix the plan.
// discussionparser.Validate reports a missing file as a finding.
// It returns only other read failures as errors, and the plan gate matches it.
//
// An index error is always returned as an error, never as a finding.
// A resolve or quarry failure means the gate could not read the code, not that it found a defect,
// and re-prompting over a missing quarry binary would hide the real fault.
//
// A findings slice with no blocking entry passes.
// The pass is logged as a logger.Warn that names the findings informational.
// A create-new-unit finding on a brand-new package is something the writer cannot fix.
// Re-prompting on it would burn the whole budget on a condition that was never wrong.
// A slice with at least one blocking entry fails the gate, with Findings set to formatPlanFindings(findings).
// The gate logs the same formatted text in a logger.Warn.
//
// A finding with an unrecognized or zero-valued severity counts as blocking.
//
// scratchDir is the ephemeral scratch directory of the run, where an informational pass records its findings.
// An empty scratchDir keeps them inline in the log line.
func NewPlanGate(anchorPath, worktreeRoot, scratchDir string, index planindex.Index, pc PlanContext) shuttleengine.Gate {
	return planGate("Plan-Gate", planparser.PlanDir(anchorPath), scratchDir, func(plan *planparser.Plan) ([]planindex.Finding, error) {
		return ValidatePlan(plan, anchorPath, worktreeRoot, index, pc)
	})
}

// ValidatePlan checks plan with index.ValidateFormat, told the done cards from Webster's run record under anchorPath.
// A run moved back to plan review after Webster executed batches keeps its run record,
// and a done card's work is already in the tree, so the index skips its tree-dependent checks for those cards.
// With no run record, every card is checked.
// A run record that cannot be read is a returned error, never a finding.
//
// A done card whose file no longer hashes to what its batch recorded at begin is one blocking done-card-edited finding.
// A done card is frozen, so any change to its file is reported.
// It reads the record and the card bytes alone; an unreadable card file is a returned error.
//
// The findings of the plan context checks over pc follow.
// They cover only the cards not recorded done, since a done card's rename already ran.
func ValidatePlan(plan *planparser.Plan, anchorPath, worktreeRoot string, index planindex.Index, pc PlanContext) ([]planindex.Finding, error) {
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

	done := make(map[int]bool)
	for _, c := range websterengine.DoneCards(plan, st) {
		done[c.Number] = true
	}
	var pending []planparser.Card
	for _, c := range plan.Cards {
		if !done[c.Number] {
			pending = append(pending, c)
		}
	}
	contextFindings, err := planContextFindings(plan, pending, worktreeRoot, index, pc)
	if err != nil {
		return nil, err
	}
	return append(findings, contextFindings...), nil
}

// doneCardEditedCheck names the finding ValidatePlan appends for an edited done card.
const doneCardEditedCheck = "done-card-edited"

// NewLensReviewPlanGate returns the plan gate closure for a twill-spawned Plan-Write run.
// Shuttle chooses it from the run's recorded fork authorization.
// It parses the plan under anchorPath and checks it with ValidatePlanLensReview over index.
//
// A parse failure fails the gate with its error text, except a read failure, which is returned as an error.
// An informational pass is logged, and only a blocking finding fails the gate.
// It adds one refusal to the plan checks: a writer that never records the lens review spends the re-prompt budget.
func NewLensReviewPlanGate(anchorPath, worktreeRoot, scratchDir string, index planindex.Index, pc PlanContext) shuttleengine.Gate {
	return planGate("Plan-Gate", planparser.PlanDir(anchorPath), scratchDir, func(plan *planparser.Plan) ([]planindex.Finding, error) {
		return ValidatePlanLensReview(plan, anchorPath, worktreeRoot, index, pc)
	})
}

// ValidatePlanLensReview checks plan for a twill Plan-Write run.
// It runs the plan checks and adds one blocking lens-review-missing finding.
// That finding is added when the plan directory's lens review file is absent or holds only whitespace.
// An error from the plan checks is returned unchanged.
// The file is checked for presence and for non-whitespace content only;
// it is never parsed, and its timing is not checked.
// A read error other than not-exist is returned, never a finding.
func ValidatePlanLensReview(plan *planparser.Plan, anchorPath, worktreeRoot string, index planindex.Index, pc PlanContext) ([]planindex.Finding, error) {
	findings, err := ValidatePlan(plan, anchorPath, worktreeRoot, index, pc)
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

// lensReviewMissingCheck names the check of a finding that reports an absent or empty lens review.
const lensReviewMissingCheck = "lens-review-missing"

// planFindingHead matches the start of one finding in a plan gate's findings text:
// its check, an optional card and its severity.
var planFindingHead = regexp.MustCompile(`(?:^|; )([a-z][a-z0-9-]*)(?:/[^\[;\s]+)?\[([a-z]*)\]: `)

// LensReviewOnly reports whether the findings text of a failed plan gate holds a blocking finding,
// and every blocking finding is the missing lens review.
// The text is the gate's findings joined by "; ", each rendered as check[/card][severity]: detail.
// Such a failure is the writer's own to clear, since only the writer records the lens review.
// A finding that is informational is not counted, and a text with no blocking finding is false.
func LensReviewOnly(findings string) bool {
	blocking := false
	for _, head := range planFindingHead.FindAllStringSubmatch(findings, -1) {
		if head[2] == string(planindex.SeverityInformational) {
			continue
		}
		if head[1] != lensReviewMissingCheck {
			return false
		}
		blocking = true
	}
	return blocking
}

// ValidateReworkPlan checks the whole new plan with index.ValidateRework:
// the format checks, and the check that first_card equals one past the highest card of the plan committed at HEAD.
// readCommitted returns an anchor-relative file as committed at HEAD, with found false when HEAD has no such file.
// A plan that cannot be read as committed at HEAD is a returned error.
// The findings of the plan context checks over pc, run over every card of the fresh generation, follow.
func ValidateReworkPlan(plan *planparser.Plan, worktreeRoot string, index planindex.Index, readCommitted func(anchorRel string) ([]byte, bool, error), pc PlanContext) ([]planindex.Finding, error) {
	told, err := NextReworkCardNumber(plan.Dir, readCommitted)
	if err != nil {
		return nil, fmt.Errorf("read the plan committed at HEAD: %w", err)
	}
	findings, err := index.ValidateRework(plan, worktreeRoot, told)
	if err != nil {
		return nil, err
	}
	contextFindings, err := planContextFindings(plan, plan.Cards, worktreeRoot, index, pc)
	if err != nil {
		return nil, err
	}
	return append(findings, contextFindings...), nil
}

// planGate builds a plan gate closure named gateName in its log lines.
// It parses the plan in planDir, runs validate over it, and maps the result onto a shuttleengine.GateResult.
// On an informational pass it records the findings in a file under scratchDir and logs their count and the file's path;
// with no scratchDir, or when the file cannot be written, the log line carries the findings inline.
func planGate(gateName, planDir, scratchDir string, validate func(*planparser.Plan) ([]planindex.Finding, error)) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
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
