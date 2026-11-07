// bouncer_skip_test.go covers BouncerConfig.Skip.
// The seam settles a segment as approved without a seed or judge spawn when the caller says it needs no review.

package shedadapters

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestBouncer_Skip_TrueSpawnsNothingAndApprovesBeforeCommit covers a true seam.
// It spawns no seed or judge, calls Approve before Commit, and returns Done with an empty pointer.
func TestBouncer_Skip_TrueSpawnsNothingAndApprovesBeforeCommit(t *testing.T) {
	tiergate.Require(t, 0)
	var callLog []string
	shuttle := &shedfake.Shuttle{}
	cfg := newBouncerFixture(t).Config
	cfg.Shuttle = shuttle
	cfg.Skip = func() (bool, error) { return true, nil }
	cfg.Approve = func() error {
		callLog = append(callLog, "approve")
		return nil
	}
	cfg.Commit = func() error {
		callLog = append(callLog, "commit")
		return nil
	}
	recorder := carryOverRecorder{}
	recorder.install(&cfg)
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}

	ptr := shedfake.RequireOutcome(t, b, shedengine.Done)
	if !reflect.DeepEqual(ptr, shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
	}
	if shuttle.Called || shuttle.AttachCalled {
		t.Errorf("shuttle called = %v, attach called = %v; want neither", shuttle.Called, shuttle.AttachCalled)
	}
	if _, err := os.Stat(focusPath(cfg.RunDir, 1)); !os.IsNotExist(err) {
		t.Errorf("round 1 focus file stat error = %v; want not-exist", err)
	}
	if len(callLog) != 2 || callLog[0] != "approve" || callLog[1] != "commit" {
		t.Errorf("callLog = %v; want [approve commit]", callLog)
	}
	if len(recorder.entries) != 0 {
		t.Errorf("carry-over entries = %+v; want the seam uncalled when no round ran", recorder.entries)
	}
}

// TestBouncer_Skip_FalseOrErrorSeedsRoundOne covers a false seam and an erroring one.
// A false seam reviews as an unconfigured Bouncer does.
// An erroring seam warns and reviews the same way, never halting and never approving.
// The seed pass runs and the call returns Stuck.
func TestBouncer_Skip_FalseOrErrorSeedsRoundOne(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name string
		skip func() (bool, error)
	}{
		{"a false seam", func() (bool, error) { return false, nil }},
		{"an erroring seam", func() (bool, error) { return true, errors.New("classifier failed") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			approveCalls := 0
			shuttle := &shedfake.Shuttle{}
			cfg := newBouncerFixture(t).Config
			cfg.Shuttle = shuttle
			cfg.Skip = tt.skip
			cfg.Approve = func() error {
				approveCalls++
				return nil
			}
			b, err := NewBouncer(cfg)
			if err != nil {
				t.Fatalf("NewBouncer(...) error = %v; want nil", err)
			}

			shedfake.RequireOutcome(t, b, shedengine.Stuck)
			if !shuttle.Called {
				t.Error("shuttle Run was not called; want the round-1 seed spawn")
			}
			if approveCalls != 0 {
				t.Errorf("Approve call count = %d; want 0", approveCalls)
			}
			if _, err := os.Stat(focusPath(cfg.RunDir, 1)); err != nil {
				t.Errorf("round 1 focus file stat error = %v; want it present", err)
			}
		})
	}
}

// TestBouncer_Skip_FailingApproveSkipsCommit covers a failing Approve under a true seam.
// Commit is skipped, and the call fails with an error wrapping the cause, never Stuck.
func TestBouncer_Skip_FailingApproveSkipsCommit(t *testing.T) {
	tiergate.Require(t, 0)
	sentinel := errors.New("approve failed")
	commitCalls := 0
	cfg := newBouncerFixture(t).Config
	cfg.Shuttle = &shedfake.Shuttle{}
	cfg.Skip = func() (bool, error) { return true, nil }
	cfg.Approve = func() error { return sentinel }
	cfg.Commit = func() error {
		commitCalls++
		return nil
	}
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}

	outcome, ptr, err := b.Call(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("Call() error = %v; want errors.Is(err, sentinel)", err)
	}
	if outcome != "" {
		t.Errorf("Call() outcome = %q; want empty alongside a non-nil error", outcome)
	}
	if !reflect.DeepEqual(ptr, shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
	}
	if commitCalls != 0 {
		t.Errorf("Commit call count = %d; want 0 -- a failing Approve must skip Commit", commitCalls)
	}
}
