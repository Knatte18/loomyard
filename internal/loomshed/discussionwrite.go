// discussionwrite.go implements the DiscussionWrite commit decorator.
// It is a thin shedengine.ShedProducer that delegates to a wrapped producer.
// On a Done outcome with a nil error, it invokes an injected commit closure.

package loomshed

import (
	"context"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// discussionWrite decorates inner with a commit step that runs after a Done verdict.
// Once inner.Call reports Done with a nil error, discussionWrite invokes commit.
// It then returns that same verdict to the caller.
//
// discussionWrite does not consult entryErr or cancelErr itself.
// inner (a *shedadapters.MultiLLMProducer in practice) already entry-checks the context as its first act,
// so a second check here would be duplicate work at the same seam;
// the wrapped producer owns the whole cancellation obligation.
type discussionWrite struct {
	name   string
	inner  shedengine.ShedProducer
	commit func() error
}

var _ shedengine.ShedProducer = (*discussionWrite)(nil)

// NewDiscussionWrite returns a discussionWrite identified as name, delegating to inner.
// It invokes commit once inner reports Done with a nil error.
// The return type is shedengine.ShedProducer, the seam interface.
// The internal/shedrecipe registry calls this constructor from outside this package.
// discussionWrite itself stays unexported.
func NewDiscussionWrite(name string, inner shedengine.ShedProducer, commit func() error) shedengine.ShedProducer {
	return &discussionWrite{name: name, inner: inner, commit: commit}
}

// Call implements shedengine.ShedProducer.
// It runs the wrapped producer once and commits its outputs through runAndCommit.
// The commit fires for a non-empty output pointer, whether the verdict is Done or a gate-failed Stuck.
// The wrapped producer holds the handoff inside the gate loop, so the verdict is settled when it returns.
// Committing a gate-failed Stuck keeps the invalid artifact diagnosable for the human the run halted for.
// Without that commit it would sit uncommitted in a dirty working tree.
// The commit seam is checked at call time, not at construction.
// The constructor has no error to refuse a nil seam through.
func (p *discussionWrite) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return runAndCommit(ctx, p.name, p.inner, p.commit)
}
