package shedengine

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestMarkTransient(t *testing.T) {
	t.Parallel()
	t.Run("a marked error unwraps and keeps its text", func(t *testing.T) {
		t.Parallel()
		inner := errors.New("connection reset")
		marked := MarkTransient(TransientGitTransport, inner)
		wrapped := fmt.Errorf("push: %w", marked)

		if got := TransientOf(wrapped); got != TransientGitTransport {
			t.Errorf("TransientOf(wrapped) = %q; want %q", got, TransientGitTransport)
		}
		if marked.Error() != inner.Error() {
			t.Errorf("marked.Error() = %q; want %q", marked.Error(), inner.Error())
		}
		if !errors.Is(wrapped, inner) {
			t.Errorf("errors.Is(wrapped, inner) = false; want true")
		}
	})
	t.Run("an empty class, a nil error and an unmarked error stay unmarked", func(t *testing.T) {
		t.Parallel()
		inner := errors.New("boom")
		if got := MarkTransient("", inner); got != inner {
			t.Errorf("MarkTransient(empty class) = %v; want the error unchanged", got)
		}
		if got := MarkTransient(TransientGitHubAPI, nil); got != nil {
			t.Errorf("MarkTransient(nil err) = %v; want nil", got)
		}
		if got := TransientOf(nil); got != "" {
			t.Errorf("TransientOf(nil) = %q; want empty", got)
		}
		if got := TransientOf(inner); got != "" {
			t.Errorf("TransientOf(unmarked) = %q; want empty", got)
		}
	})
}

func failingProducer() *funcProducer {
	return &funcProducer{fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
		return "", OutputPointer{}, errors.New("push failed")
	}}
}

func TestStep_TransientClassification(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		classifier func(error) TransientClass
		wantClass  TransientClass
	}{
		{"a class is marked on the error and persisted", func(error) TransientClass { return TransientGitTransport }, TransientGitTransport},
		{"an empty class persists empty", func(error) TransientClass { return "" }, ""},
		{"a nil classifier leaves the error unmarked", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shed, statusPath, _, statusLockPath := newTestShed(t)
			shed.Producers = []ProducerDef{{Name: "A", Producer: failingProducer()}}
			shed.Transient = tt.classifier
			seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

			_, err := shed.Step(context.Background())
			if err == nil {
				t.Fatalf("Step(...) = nil error; want the producer error")
			}
			if got := TransientOf(err); got != tt.wantClass {
				t.Errorf("TransientOf(err) = %q; want %q", got, tt.wantClass)
			}
			got := readStatus(t, statusPath, statusLockPath)
			if got.State != StateFailed {
				t.Errorf("persisted State = %q; want %q", got.State, StateFailed)
			}
			if got.Transient != string(tt.wantClass) {
				t.Errorf("persisted Transient = %q; want %q", got.Transient, tt.wantClass)
			}
		})
	}
}

func TestStep_ResumeClearsTransient(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	fail := true
	shed.Producers = []ProducerDef{{Name: "A", Producer: &funcProducer{fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
		if fail {
			return "", OutputPointer{}, errors.New("push failed")
		}
		return Done, OutputPointer{}, nil
	}}}}
	shed.Transient = func(error) TransientClass { return TransientGitHubAPI }
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	if _, err := shed.Step(context.Background()); err == nil {
		t.Fatalf("first Step(...) = nil error; want the producer error")
	}
	if got := readStatus(t, statusPath, statusLockPath); got.Transient != "github-api" {
		t.Fatalf("persisted Transient = %q; want github-api", got.Transient)
	}

	fail = false
	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("second Step(...) = _, %v; want nil error", err)
	}
	if res.State != StateDone {
		t.Errorf("State = %q; want %q", res.State, StateDone)
	}
	if got := readStatus(t, statusPath, statusLockPath); got.Transient != "" {
		t.Errorf("persisted Transient after resume = %q; want empty", got.Transient)
	}
}

func TestStep_VerdictsNeverClassify(t *testing.T) {
	tests := []struct {
		name     string
		producer *funcProducer
		pause    bool
	}{
		{"Stuck", fixedOutcomeProducer(Stuck, ""), false},
		{"Awaiting", awaitingProducer("waiting"), false},
		{"Pause", fixedOutcomeProducer(Done, ""), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shed, statusPath, _, statusLockPath := newTestShed(t)
			shed.Producers = []ProducerDef{{Name: "A", Producer: tt.producer, OnStuck: "A"}}
			calls := 0
			shed.Transient = func(error) TransientClass {
				calls++
				return TransientGitTransport
			}
			seed := commonSeed("A")
			seed.PauseRequested = tt.pause
			seedStatus(t, statusPath, statusLockPath, seed)

			if _, err := shed.Step(context.Background()); err != nil {
				t.Fatalf("Step(...) = _, %v; want nil error", err)
			}
			if calls != 0 {
				t.Errorf("classifier calls = %d; want 0", calls)
			}
			if got := readStatus(t, statusPath, statusLockPath); got.Transient != "" {
				t.Errorf("persisted Transient = %q; want empty", got.Transient)
			}
		})
	}
}
