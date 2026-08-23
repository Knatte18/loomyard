// discussionvalidate.go implements the Discussion-Validate producer: a thin wrap over
// internal/discussionparser's own check and validate steps, and nothing more -- the two checks and
// the seven required headings are internal/discussionparser's; this file owns only the outcome
// mapping.

package loomshed

import (
	"context"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// discussionValidate is the Discussion-Validate producer: it runs discussionparser.Validate against
// decisionRecordPath and supportLogPath and maps the result onto shedengine's outcome contract.
//
// Both paths are told rather than derived, because loomengine's own accessors for them
// (DiscussionDecisionRecord/DiscussionSupportLog) take a *lyxcwd.Location, which this package may
// not import.
type discussionValidate struct {
	name               string
	decisionRecordPath string
	supportLogPath     string
}

var _ shedengine.ShedProducer = (*discussionValidate)(nil)

// NewDiscussionValidate returns a discussionValidate identified as name, checking
// decisionRecordPath and supportLogPath. The return type is shedengine.ShedProducer, the seam
// interface, so the internal/shedrecipe registry can call this constructor from outside this
// package while discussionValidate itself stays unexported.
func NewDiscussionValidate(name, decisionRecordPath, supportLogPath string) shedengine.ShedProducer {
	return &discussionValidate{name: name, decisionRecordPath: decisionRecordPath, supportLogPath: supportLogPath}
}

// Call implements shedengine.ShedProducer. It is a thin wrap and nothing more:
// discussionparser.Validate(p.decisionRecordPath, p.supportLogPath), once. A non-nil error maps to a
// returned error, never to Stuck; a non-empty findings slice maps to shedengine.Stuck with an empty
// pointer, the findings themselves discarded here; the clean case maps to shedengine.Done, reporting
// the decision record's path as the pointer.
//
// A read failure that is not a not-exist is a returned error, not Stuck -- a Stuck would bounce
// back to Discussion-Write, which cannot fix an I/O fault. See discussionparser.Validate for the
// two checks it runs, the seven required headings, and the three documented non-checks.
func (p *discussionValidate) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	findings, err := discussionparser.Validate(p.decisionRecordPath, p.supportLogPath)
	if err != nil {
		return p.nonDoneExit(ctx, "", err)
	}
	if len(findings) > 0 {
		return p.nonDoneExit(ctx, shedengine.Stuck, nil)
	}

	return shedengine.Done, shedengine.OutputPointer{Path: p.decisionRecordPath}, nil
}

// nonDoneExit consults cancelErr before returning a non-Done result: a cancelled context replaces
// outcome/err with the cancellation error; otherwise it returns outcome (with an empty pointer) and
// err verbatim.
func (p *discussionValidate) nonDoneExit(ctx context.Context, outcome shedengine.Outcome, err error) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if cerr := cancelErr(ctx, p.name); cerr != nil {
		return "", shedengine.OutputPointer{}, cerr
	}
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	return outcome, shedengine.OutputPointer{}, nil
}
