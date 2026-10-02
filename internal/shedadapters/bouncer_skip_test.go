// bouncer_skip_test.go covers BouncerConfig.Skip -- the optional seam that settles a segment as approved without a seed or judge spawn when the caller says the artifact needs no review.

package shedadapters

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestBouncer_Skip_TrueSpawnsNothingAndApprovesBeforeCommit pins that a true seam spawns no seed or judge, calls Approve strictly before Commit, and returns Done with an empty pointer.
func TestBouncer_Skip_TrueSpawnsNothingAndApprovesBeforeCommit(t *testing.T) {
	var callLog []string
	shuttle := &fakeShuttle{}
	cfg := testBouncerConfig(t)
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
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
	}
	if shuttle.called || shuttle.attachCalled {
		t.Errorf("shuttle called = %v, attach called = %v; want neither", shuttle.called, shuttle.attachCalled)
	}
	if _, err := os.Stat(focusPath(cfg.RunDir, 1)); !os.IsNotExist(err) {
		t.Errorf("round 1 focus file stat error = %v; want not-exist", err)
	}
	if len(callLog) != 2 || callLog[0] != "approve" || callLog[1] != "commit" {
		t.Errorf("callLog = %v; want [approve commit]", callLog)
	}
}

// TestBouncer_Skip_FalseSeedsRoundOne pins that a false seam reviews exactly as an unconfigured Bouncer does: the seed pass runs and the call returns Stuck.
func TestBouncer_Skip_FalseSeedsRoundOne(t *testing.T) {
	shuttle := &fakeShuttle{}
	cfg := testBouncerConfig(t)
	cfg.Shuttle = shuttle
	cfg.Skip = func() (bool, error) { return false, nil }
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}

	outcome, _, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if !shuttle.called {
		t.Error("shuttle Run was not called; want the round-1 seed spawn")
	}
	if _, err := os.Stat(focusPath(cfg.RunDir, 1)); err != nil {
		t.Errorf("round 1 focus file stat error = %v; want it present", err)
	}
}

// TestBouncer_Skip_ErrorFallsBackToReview pins that an erroring seam warns and seeds round 1 like a false one, never halting and never approving.
func TestBouncer_Skip_ErrorFallsBackToReview(t *testing.T) {
	approveCalls := 0
	shuttle := &fakeShuttle{}
	cfg := testBouncerConfig(t)
	cfg.Shuttle = shuttle
	cfg.Skip = func() (bool, error) { return true, errors.New("classifier failed") }
	cfg.Approve = func() error {
		approveCalls++
		return nil
	}
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}

	outcome, _, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if !shuttle.called {
		t.Error("shuttle Run was not called; want the round-1 seed spawn")
	}
	if approveCalls != 0 {
		t.Errorf("Approve call count = %d; want 0", approveCalls)
	}
	if _, err := os.Stat(focusPath(cfg.RunDir, 1)); err != nil {
		t.Errorf("round 1 focus file stat error = %v; want it present", err)
	}
}

// TestBouncer_Skip_FailingApproveSkipsCommit pins that a failing Approve under a true seam skips Commit and fails the way settle does: an error wrapping the cause, never Stuck.
func TestBouncer_Skip_FailingApproveSkipsCommit(t *testing.T) {
	sentinel := errors.New("approve failed")
	commitCalls := 0
	cfg := testBouncerConfig(t)
	cfg.Shuttle = &fakeShuttle{}
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
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
	}
	if commitCalls != 0 {
		t.Errorf("Commit call count = %d; want 0 -- a failing Approve must skip Commit", commitCalls)
	}
}
