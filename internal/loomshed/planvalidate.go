// planvalidate.go implements the Plan-Validate producer: a thin wrap over planparser's own parse
// and validate steps, and nothing more -- the Planparser Sole-Parser Invariant means no plan parsing
// whatsoever may be written here.

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

// planValidate is the Plan-Validate producer: it parses the plan at anchorPath and runs
// planparser's own machine checks against it.
type planValidate struct {
	name         string
	anchorPath   string
	worktreeRoot string
}

var _ shedengine.ShedProducer = (*planValidate)(nil)

// NewPlanValidate returns a planValidate identified as name, validating the plan anchored at
// anchorPath against worktreeRoot. The two fields are separate because planparser.PlanDir takes the
// anchor path and planparser.Validate takes the worktree root, and they are not the same value.
// The return type is shedengine.ShedProducer, the seam interface, so the internal/shedrecipe
// registry can call this constructor from outside this package while planValidate itself stays
// unexported.
func NewPlanValidate(name, anchorPath, worktreeRoot string) shedengine.ShedProducer {
	return &planValidate{name: name, anchorPath: anchorPath, worktreeRoot: worktreeRoot}
}

// Call implements shedengine.ShedProducer. It is a thin wrap and nothing more:
// planparser.ParsePlan(planparser.PlanDir(p.anchorPath)), then planparser.Validate(plan,
// p.worktreeRoot). A non-empty []planparser.ValidationError maps to shedengine.Stuck with an empty
// pointer; an empty slice maps to shedengine.Done, reporting the plan directory as the pointer.
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

	if findings := planparser.Validate(plan, p.worktreeRoot); len(findings) > 0 {
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
