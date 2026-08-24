// discussionwrite.go implements the DiscussionWrite commit decorator: a thin
// shedengine.ShedProducer that delegates to a wrapped producer and, on a Done outcome with a nil
// error, invokes an injected commit closure -- mirroring discussionvalidate.go's own file shape.

package loomshed

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// discussionWrite decorates inner with a post-Done commit step: once inner.Call reports Done with
// a nil error, discussionWrite invokes commit before returning that same verdict to the caller.
//
// discussionWrite does not consult entryErr or cancelErr itself. inner (a *SingleLLMProducer in
// practice) already entry-checks the context as its first act, so a second check here would be
// duplicate work at the same seam; the wrapped producer owns the whole cancellation obligation.
type discussionWrite struct {
	name   string
	inner  shedengine.ShedProducer
	commit func() error
}

var _ shedengine.ShedProducer = (*discussionWrite)(nil)

// NewDiscussionWrite returns a discussionWrite identified as name, delegating to inner and
// invoking commit once inner reports Done with a nil error. The return type is
// shedengine.ShedProducer, the seam interface, so the internal/shedrecipe registry can call this
// constructor from outside this package while discussionWrite itself stays unexported.
func NewDiscussionWrite(name string, inner shedengine.ShedProducer, commit func() error) shedengine.ShedProducer {
	return &discussionWrite{name: name, inner: inner, commit: commit}
}

// Call implements shedengine.ShedProducer: it calls p.inner.Call(ctx) exactly once and returns its
// three results verbatim whenever the error is non-nil or the outcome is anything other than
// shedengine.Done. Only a Done outcome with a nil error invokes p.commit before returning.
//
// A non-nil commit error maps to a returned error, never to shedengine.Stuck: a git fault is not
// something re-writing the discussion can fix, the same reasoning discussionvalidate.go already
// applies to a non-not-exist read failure. The commit fires before Discussion-Validate has judged
// the output, and that is intentional -- the commit keeps the weft clean and the artifact durable,
// it does not certify it.
func (p *discussionWrite) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	outcome, pointer, err := p.inner.Call(ctx)
	if err != nil || outcome != shedengine.Done {
		return outcome, pointer, err
	}

	if err := p.commit(); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: commit produced artifacts: %w", p.name, err)
	}

	return outcome, pointer, nil
}
