// preflight.go wires internal/loomengine.Preflight in behind an exported constructor, so this task
// has a Preflight row something can actually construct.

package loomshed

import (
	"context"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// preflightProducer is the Preflight producer: it wraps loomengine.Preflight, mapping its
// determined Report onto shedengine's contract.
type preflightProducer struct {
	name string
	cwd  string
}

var _ shedengine.ShedProducer = (*preflightProducer)(nil)

// NewPreflightProducer returns a shedengine.ShedProducer named "Preflight" that validates cwd via
// loomengine.Preflight.
//
// The constructor is exported and owned here, rather than left to the not-yet-built
// session-bootstrap caller, so this task has a Preflight row something can actually construct.
// Deps.Preflight (see loomshed.go) stays typed as a bare shedengine.ShedProducer so a Tier-1 test
// can inject a fake instead.
//
// This adds the production import of internal/loomengine that the Told-Geometry guard test in
// seam_enforcement_test.go already allowlists -- that import does not compromise this package's
// Told-Geometry position, since the invariant's membership predicate is about a direct production
// import of internal/lyxcwd and transitive is explicitly fine.
func NewPreflightProducer(cwd string) shedengine.ShedProducer {
	return &preflightProducer{name: "Preflight", cwd: cwd}
}

// Call implements shedengine.ShedProducer: it invokes loomengine.Preflight(p.cwd) and maps its
// result -- a Report with OK true to shedengine.Done with an empty pointer, a Report with OK false
// to shedengine.Stuck with an empty pointer, and a non-nil error to a returned error. That mapping
// is the whole producer -- Preflight reports a determined verdict rather than erroring on anything
// short of an infra failure, so its OK false is a verdict to route and its error is an undetermined
// failure to escalate.
func (p *preflightProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	report, err := loomengine.Preflight(p.cwd)
	if err != nil {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return "", shedengine.OutputPointer{}, err
	}

	if !report.OK {
		if cerr := cancelErr(ctx, p.name); cerr != nil {
			return "", shedengine.OutputPointer{}, cerr
		}
		return shedengine.Stuck, shedengine.OutputPointer{}, nil
	}

	return shedengine.Done, shedengine.OutputPointer{}, nil
}
