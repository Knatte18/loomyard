// gates.go implements this package's two gate closures: NewDiscussionGate and NewPlanGate.
// Both are the gate half of the Gate Self-Check Parity Invariant -- each calls the identical package
// function its CLI self-check verb does (discussionparser.Validate and planglyph.ValidateFormat,
// respectively), so the operator's own self-check verb and the automated gate can never disagree
// about what "valid" means.

package loomshed

import (
	"errors"
	"io/fs"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// NewDiscussionGate returns the Discussion-Write row's gate closure: a shuttleengine.Gate that
// calls discussionparser.Validate(decisionRecordPath, supportLogPath) once and maps the result onto
// the gate contract.
//
// Both paths are told rather than derived, because loomengine's own accessors for them take a
// *lyxcwd.Location this package may not import.
//
// A non-nil error from Validate is returned verbatim alongside the zero shuttleengine.GateResult: a
// gate that could not read the artifact has found no defect to re-prompt over, it is an
// infrastructure fault, and it must never burn an attempt or reach the LLM. A non-empty findings
// slice produces GateResult{Passed: false, Findings: formatDiscussionFindings(findings)} and a nil
// error, preceded by a logger.Warn carrying the gate name, the decision record path, and the same
// formatted findings -- that warn line is the only durable record of why an artifact was refused,
// because the findings file itself lives in the ephemeral run directory finalize deletes on the Done
// cleanup. An empty slice produces GateResult{Passed: true} and a nil error.
func NewDiscussionGate(decisionRecordPath, supportLogPath string) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		findings, err := discussionparser.Validate(decisionRecordPath, supportLogPath)
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

// NewPlanGate returns the Plan-Write row's gate closure: a shuttleengine.Gate that parses the plan
// through planparser.ParsePlan(planparser.PlanDir(anchorPath)) and then runs
// planglyph.ValidateFormat(plan, worktreeRoot), mapping the result onto the gate contract.
//
// The two path parameters are separate because planparser.PlanDir takes the anchor path while
// planglyph.ValidateFormat takes the worktree root, and they are not the same value.
//
// ValidateFormat is called, never planglyph.Validate: both plan gate sites run strictly before the
// Plan-Review segment's approve seam writes the approval flag, so demanding it would fail every
// single fix round.
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
// This is a reasoned reversal of the disposition the standing plan-validate producer takes: that
// producer's rationale -- a plan that will not parse is not a plan the bounce target can be asked to
// improve -- was about a cold respawn that knows nothing of the complaint, whereas the gate's bounce
// target is the live session that just wrote the file, holding its full context, so the premise no
// longer holds. This also makes both gates behave identically on a missing-or-malformed artifact,
// since discussionparser.Validate already reports a missing file as a finding and only a
// non-not-exist read failure as an error.
//
// Every planglyph error stays a returned error in full and is explicitly NOT part of the carve-out:
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
// hasBlockingFinding is called, never re-derived: it encodes a crucible-round finding that
// planglyph.Severity is an open string type, so testing not-informational rather than
// equals-blocking is what keeps an unrecognized or zero-valued severity from silently passing.
func NewPlanGate(anchorPath, worktreeRoot string) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		planDir := planparser.PlanDir(anchorPath)
		plan, err := planparser.ParsePlan(planDir)
		if err != nil {
			if errors.As(err, new(*fs.PathError)) {
				return shuttleengine.GateResult{}, err
			}
			return shuttleengine.GateResult{Passed: false, Findings: err.Error()}, nil
		}

		findings, err := planglyph.ValidateFormat(plan, worktreeRoot)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}

		if !hasBlockingFinding(findings) {
			logger.Warn("loomshed: plan gate surfaced informational findings", "gate", "Plan-Gate", "planDir", planDir, "findings", formatPlanFindings(findings))
			return shuttleengine.GateResult{Passed: true}, nil
		}

		formatted := formatPlanFindings(findings)
		logger.Warn("loomshed: plan gate failed validation", "gate", "Plan-Gate", "planDir", planDir, "findings", formatted)
		return shuttleengine.GateResult{Passed: false, Findings: formatted}, nil
	}
}
