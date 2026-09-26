package loomshed

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

func TestFrictionReflect_CallReturnsDoneForEveryStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
	}{
		{"reflected", "reflected"},
		{"skipped", "skipped"},
		{"failed", "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			p, err := NewFrictionReflect("Friction-Reflect", func() string {
				calls++
				return tt.status
			})
			if err != nil {
				t.Fatalf("NewFrictionReflect error = %v; want nil", err)
			}
			outcome, pointer, err := p.Call(context.Background())
			if err != nil {
				t.Fatalf("Call error = %v; want nil", err)
			}
			if outcome != shedengine.Done {
				t.Errorf("Call outcome = %q; want %q", outcome, shedengine.Done)
			}
			if pointer != (shedengine.OutputPointer{}) {
				t.Errorf("Call pointer = %+v; want empty", pointer)
			}
			if calls != 1 {
				t.Errorf("closure calls = %d; want 1", calls)
			}
		})
	}
}

func TestNewFrictionReflect_NilClosureRejected(t *testing.T) {
	p, err := NewFrictionReflect("Friction-Reflect", nil)
	if err == nil {
		t.Fatal("NewFrictionReflect(nil) error = nil; want non-nil")
	}
	if p != nil {
		t.Errorf("NewFrictionReflect(nil) producer = %v; want nil", p)
	}
}

func TestFrictionReflect_CancelledContextSkipsClosure(t *testing.T) {
	calls := 0
	p, err := NewFrictionReflect("Friction-Reflect", func() string {
		calls++
		return "reflected"
	})
	if err != nil {
		t.Fatalf("NewFrictionReflect error = %v; want nil", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := p.Call(ctx); err == nil {
		t.Error("Call(cancelled) error = nil; want non-nil")
	}
	if calls != 0 {
		t.Errorf("closure calls = %d; want 0", calls)
	}
}
