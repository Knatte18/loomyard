// planvalidate.go implements the Plan-Validate/Plan-Revalidate producer: a thin wrap over
// planglyph's own resolve-backed validation entry points, and nothing more -- the Planparser
// Sole-Parser Invariant means no plan parsing whatsoever may be written here. The two rows share
// this one engine and are distinguished only by a requireApproved mode: Plan-Validate runs
// planglyph.ValidateFormat before review, Plan-Revalidate runs planglyph.Validate after review
// settles.

package loomshed

import (
	"context"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// formatPlanFindings renders findings as a single semicolon-separated list, each entry carrying
// its own Check[/Card]: Detail rendering plus its Severity, so the log line and "lyx loom
// validate-plan"'s envelope describe a violation identically and an informational
// create-new-unit is distinguishable from a blocking glyph-not-found in the one place this record
// exists.
// It calls each Finding's own Error() rather than re-deriving its layout, which is what makes the
// "describe a violation identically" promise above hold by construction rather than by two copies of
// one format string agreeing today; formatDiscussionFindings in discussionvalidate.go does the same.
func formatPlanFindings(findings []planglyph.Finding) string {
	parts := make([]string, len(findings))
	for i, f := range findings {
		parts[i] = f.Error()
	}
	return strings.Join(parts, "; ")
}

// hasBlockingFinding reports whether findings carries at least one entry that is not explicitly
// informational.
//
// It tests NOT-informational rather than equals-blocking, and that asymmetry is the point.
// planglyph.Severity is an open string type, so an unrecognized value — or the zero value, which a
// hand-built Finding or a future producer that forgets to stamp one carries — took the informational
// branch, logged a Warn, and returned Done with the plan directory as its pointer: the run advanced
// past a finding that was meant to block it. That is the same "a validator's complaint reported as a
// clean plan" failure this file's own error-path comment records as deliberately rejected, still
// present on the severity path (crucible round opus-medium-r6, R6-27).
// Failing closed costs at most a spurious bounce on a severity nobody has defined yet; failing open
// costs a dispatched batch over a defect the gate saw.
func hasBlockingFinding(findings []planglyph.Finding) bool {
	for _, f := range findings {
		if f.Severity != planglyph.SeverityInformational {
			return true
		}
	}
	return false
}

// planValidate is the Plan-Validate/Plan-Revalidate producer: it parses the plan at anchorPath and
// runs planglyph's own resolve-backed checks against it, in one of two modes selected by
// requireApproved. When requireApproved is false it runs planglyph.ValidateFormat, the pre-review
// format-only check set that must not demand a flag only the review segment can produce; when true
// it runs planglyph.Validate, the full check set including the plan-unapproved approval gate.
type planValidate struct {
	name            string
	anchorPath      string
	worktreeRoot    string
	requireApproved bool
}

var _ shedengine.ShedProducer = (*planValidate)(nil)

// NewPlanValidate returns a planValidate identified as name, validating the plan anchored at
// anchorPath against worktreeRoot. The two path fields are separate because planparser.PlanDir
// takes the anchor path and planparser.Validate/ValidateFormat take the worktree root, and they are
// not the same value. requireApproved selects which of the two rows sharing this engine name is
// being built: false is Plan-Validate, which runs before review and must not demand the
// plan-unapproved flag; true is Plan-Revalidate, which runs after the review segment settles and
// must confirm the flag is there. The return type is shedengine.ShedProducer, the seam interface,
// so the internal/shedrecipe registry can call this constructor from outside this package while
// planValidate itself stays unexported.
func NewPlanValidate(name, anchorPath, worktreeRoot string, requireApproved bool) shedengine.ShedProducer {
	return &planValidate{name: name, anchorPath: anchorPath, worktreeRoot: worktreeRoot, requireApproved: requireApproved}
}

// Call implements shedengine.ShedProducer. It is a thin wrap and nothing more:
// planparser.ParsePlan(planparser.PlanDir(p.anchorPath)), then either planglyph.Validate or
// planglyph.ValidateFormat, selected by p.requireApproved, against p.worktreeRoot. A findings slice
// carrying at least one SeverityBlocking entry maps to shedengine.Stuck with an empty pointer; a
// slice that is empty, or that holds only SeverityInformational entries, maps to shedengine.Done,
// reporting the plan directory as the pointer.
//
// A ParsePlan error maps to a returned error, never to Stuck: a plan that will not parse is not a
// plan the Plan-Write bounce target can be asked to improve, and the two dispositions differ
// materially -- Stuck persists blocked, a returned error persists failed and aborts the run. EVERY
// error from planglyph maps to the same returned-error disposition, for the same reason: a gate that
// could not read the code has not found a plan defect to bounce. That includes the quarry-unavailable
// one this gate used to single out -- singling it out dropped every other error on the floor and
// reported the plan clean over a validation that never finished.
func (p *planValidate) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	planDir := planparser.PlanDir(p.anchorPath)
	plan, err := planparser.ParsePlan(planDir)
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, err
	}

	var findings []planglyph.Finding
	if p.requireApproved {
		findings, err = planglyph.Validate(plan, p.worktreeRoot)
	} else {
		findings, err = planglyph.ValidateFormat(plan, p.worktreeRoot)
	}
	// Any validator error fails the gate, not only the quarry-named one. Conjoining errors.Is here
	// dropped every other error on the floor, and an unrecognized error with an empty findings set
	// then returned Done with the plan directory as its pointer — "the plan is clean" reported for a
	// validator that never finished, which is exactly the failure mode internal/planglyph/repo.go's
	// own rationale names as deliberately rejected.
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, err
	}

	if len(findings) > 0 {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		if !hasBlockingFinding(findings) {
			// Informational-only: surfaced for visibility, but this is a pass -- a set that is
			// entirely informational (e.g. card 18's create-new-unit on a brand-new package) is not
			// something Plan-Write can fix, so bouncing on it would resubmit an unchanged plan until
			// the bounce budget escalated to a human over a condition that was never wrong.
			logger.Warn("loomshed: plan validation surfaced informational findings", "producer", p.name, "planDir", planDir, "findings", formatPlanFindings(findings))
			return shedengine.Done, shedengine.OutputPointer{Path: planDir}, nil
		}
		// Surfaced rather than discarded. This row's bounce target is Plan-Write, respawned with no
		// knowledge of which of loom-plan-spec.md's check IDs fired, so this line is the only record
		// of it anywhere -- and both the Plan-Validate and Plan-Revalidate rows run this same
		// producer, so it covers the fixer-introduced regression case too. The producer name
		// distinguishes which row spoke.
		logger.Warn("loomshed: plan failed validation", "producer", p.name, "planDir", planDir, "findings", formatPlanFindings(findings))
		return shedengine.Stuck, shedengine.OutputPointer{}, nil
	}

	return shedengine.Done, shedengine.OutputPointer{Path: planDir}, nil
}
