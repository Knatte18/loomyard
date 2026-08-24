// discussionwrite_test.go exercises NewDiscussionWrite's own outcome mapping against a fake inner
// shedengine.ShedProducer: commit fires exactly once, and only, when the inner producer reports
// Done with a nil error, and every other outcome (Stuck, a non-nil error, or a commit failure)
// leaves the Fabric untouched by never invoking commit more than that once. No test in this file
// touches a filesystem or a real git repo -- see fakeInnerProducer and commitRecorder below.

package loomshed

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// fakeInnerProducer is a caller-settable shedengine.ShedProducer stand-in for the wrapped
// producer: Call returns whatever outcome/pointer/err the test configured, and records how many
// times it was called so a test can assert the decorator calls it exactly once per Call.
type fakeInnerProducer struct {
	outcome shedengine.Outcome
	pointer shedengine.OutputPointer
	err     error
	calls   int
}

func (f *fakeInnerProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	f.calls++
	return f.outcome, f.pointer, f.err
}

// commitRecorder is a caller-settable commit closure stand-in: Commit returns commitErr and
// records how many times it was invoked.
type commitRecorder struct {
	commitErr error
	calls     int
}

func (c *commitRecorder) Commit() error {
	c.calls++
	return c.commitErr
}

func TestDiscussionWrite_Call(t *testing.T) {
	t.Run("DoneWithOutputInvokesCommitOnce", func(t *testing.T) {
		inner := &fakeInnerProducer{outcome: shedengine.Done, pointer: shedengine.OutputPointer{Path: "decision-record.md"}}
		commit := &commitRecorder{}

		p := NewDiscussionWrite("Discussion-Write", inner, commit.Commit)
		outcome, pointer, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Done {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
		}
		if pointer != inner.pointer {
			t.Errorf("Call() pointer = %+v; want %+v", pointer, inner.pointer)
		}
		if commit.calls != 1 {
			t.Errorf("commit.calls = %d; want 1", commit.calls)
		}
		if inner.calls != 1 {
			t.Errorf("inner.calls = %d; want 1", inner.calls)
		}
	})

	t.Run("StuckLeavesCommitUninvoked", func(t *testing.T) {
		inner := &fakeInnerProducer{outcome: shedengine.Stuck}
		commit := &commitRecorder{}

		p := NewDiscussionWrite("Discussion-Write", inner, commit.Commit)
		outcome, pointer, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
		if pointer != (shedengine.OutputPointer{}) {
			t.Errorf("Call() pointer = %+v; want the zero value", pointer)
		}
		if commit.calls != 0 {
			t.Errorf("commit.calls = %d; want 0", commit.calls)
		}
	})

	t.Run("InnerErrorLeavesCommitUninvoked", func(t *testing.T) {
		innerErr := errors.New("inner producer failed")
		inner := &fakeInnerProducer{err: innerErr}
		commit := &commitRecorder{}

		p := NewDiscussionWrite("Discussion-Write", inner, commit.Commit)
		_, _, err := p.Call(context.Background())
		if !errors.Is(err, innerErr) {
			t.Errorf("Call() error = %v; want it to wrap %v", err, innerErr)
		}
		if commit.calls != 0 {
			t.Errorf("commit.calls = %d; want 0", commit.calls)
		}
	})

	t.Run("CommitErrorSurfacesAsErrorNotStuck", func(t *testing.T) {
		commitErr := errors.New("git commit failed")
		inner := &fakeInnerProducer{outcome: shedengine.Done, pointer: shedengine.OutputPointer{Path: "decision-record.md"}}
		commit := &commitRecorder{commitErr: commitErr}

		p := NewDiscussionWrite("Discussion-Write", inner, commit.Commit)
		outcome, pointer, err := p.Call(context.Background())
		if !errors.Is(err, commitErr) {
			t.Errorf("Call() error = %v; want it to wrap %v", err, commitErr)
		}
		if outcome != "" {
			t.Errorf("Call() outcome = %q; want the empty value, never %q", outcome, shedengine.Stuck)
		}
		if pointer != (shedengine.OutputPointer{}) {
			t.Errorf("Call() pointer = %+v; want the zero value", pointer)
		}
	})

	t.Run("CommitErrorIsWrappedWithProducerName", func(t *testing.T) {
		commitErr := errors.New("git commit failed")
		inner := &fakeInnerProducer{outcome: shedengine.Done, pointer: shedengine.OutputPointer{Path: "decision-record.md"}}
		commit := &commitRecorder{commitErr: commitErr}

		p := NewDiscussionWrite("Discussion-Write", inner, commit.Commit)
		_, _, err := p.Call(context.Background())
		if err == nil {
			t.Fatalf("Call() error = nil; want a non-nil error naming the producer")
		}
		if !strings.Contains(err.Error(), "Discussion-Write") {
			t.Errorf("Call() error = %q; want it to name the producer %q", err.Error(), "Discussion-Write")
		}
	})
}
