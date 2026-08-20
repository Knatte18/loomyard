// preflight.go implements NewPreflight, the general Preflight producer: a content-free
// shedengine.ShedProducer wrapping internal/preflight.Check, constructed with a told name and cwd.

package preflightshed

import (
	"context"

	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// preflightProducer is the general Preflight producer: it wraps preflight.Check, mapping its
// determined Report onto shedengine's contract.
type preflightProducer struct {
	name string
	cwd  string
}

var _ shedengine.ShedProducer = (*preflightProducer)(nil)

// NewPreflight returns a shedengine.ShedProducer that validates cwd via preflight.Check.
//
// name is told rather than hardcoded because a second product names this row independently.
// Per internal/shedadapters' own package doc, the name is used only as a log field and in error
// text -- never compared, parsed, or used for control flow.
func NewPreflight(name, cwd string) shedengine.ShedProducer {
	return &preflightProducer{name: name, cwd: cwd}
}

// Call implements shedengine.ShedProducer: it invokes preflight.Check(p.cwd) and maps its result --
// a Report with OK true to shedengine.Done with an empty pointer, a Report with OK false to
// shedengine.Stuck with an empty pointer, and a non-nil error to a returned error. That mapping is
// the whole producer -- Check reports a determined verdict rather than erroring on anything short
// of an infra failure, so its OK false is a verdict to route and its error is an undetermined
// failure to escalate. The resolved *lyxcwd.Location Check also returns is discarded: this package
// never touches the resolved location.
func (p *preflightProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	report, _, err := preflight.Check(p.cwd)
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
