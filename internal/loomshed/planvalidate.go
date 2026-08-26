// planvalidate.go implements the Plan-Validate/Plan-Revalidate producer: a thin wrap over
// planparser's own parse and validate steps, and nothing more -- the Planparser Sole-Parser
// Invariant means no plan parsing whatsoever may be written here. The two rows share this one
// engine and are distinguished only by a requireApproved mode: Plan-Validate runs
// planparser.ValidateFormat before review, Plan-Revalidate runs planparser.Validate after review
// settles.

package loomshed

import (
	"context"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// formatPlanFindings renders findings as a single semicolon-separated list, using each
// ValidationError's own Error() rendering so the log line and "lyx loom validate-plan"'s envelope
// describe a violation identically.
func formatPlanFindings(findings []planparser.ValidationError) string {
	parts := make([]string, len(findings))
	for i, f := range findings {
		parts[i] = f.Error()
	}
	return strings.Join(parts, "; ")
}

// planValidate is the Plan-Validate/Plan-Revalidate producer: it parses the plan at anchorPath and
// runs planparser's own machine checks against it, in one of two modes selected by
// requireApproved. When requireApproved is false it runs planparser.ValidateFormat, the pre-review
// format-only check set that must not demand a flag only the review segment can produce; when true
// it runs planparser.Validate, the full check set including the plan-unapproved approval gate.
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
// planparser.ParsePlan(planparser.PlanDir(p.anchorPath)), then either planparser.Validate or
// planparser.ValidateFormat, selected by p.requireApproved, against p.worktreeRoot. A non-empty
// []planparser.ValidationError maps to shedengine.Stuck with an empty pointer; an empty slice maps
// to shedengine.Done, reporting the plan directory as the pointer.
//
// A ParsePlan error maps to a returned error, never to Stuck: a plan that will not parse is not a
// plan the Plan-Write bounce target can be asked to improve, and the two dispositions differ
// materially -- Stuck persists blocked, a returned error persists failed and aborts the run.
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

	var findings []planparser.ValidationError
	if p.requireApproved {
		findings = planparser.Validate(plan, p.worktreeRoot)
	} else {
		findings = planparser.ValidateFormat(plan, p.worktreeRoot)
	}
	if len(findings) > 0 {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
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
