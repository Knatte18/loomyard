// discussionwrite.go implements the DiscussionWrite commit decorator: a thin
// shedengine.ShedProducer that delegates to a wrapped producer and, on a Done outcome with a nil
// error, invokes an injected commit closure.

package loomshed

import (
	"context"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// discussionWrite decorates inner with a post-Done commit step: once inner.Call reports Done with
// a nil error, discussionWrite invokes commit before returning that same verdict to the caller.
//
// discussionWrite does not consult entryErr or cancelErr itself. inner (a *MultiLLMProducer in
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

// Call implements shedengine.ShedProducer: it runs the wrapped producer once and commits its outputs through runAndCommit.
// The commit fires for a non-empty output pointer, Done or a gate-failed Stuck, since the wrapped producer holds the handoff inside the gate loop and the verdict is settled by the time it returns.
// Committing a gate-failed Stuck leaves the invalid artifact committed and diagnosable for the human the run halted for, instead of uncommitted in a dirty working tree.
// The commit seam is checked at call time rather than at construction because the constructor returns the bare seam interface and so has no error to refuse a nil through.
func (p *discussionWrite) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return runAndCommit(ctx, p.name, p.inner, p.commit)
}
