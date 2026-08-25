// bouncer_commit_test.go covers BouncerConfig.Commit alone -- the seam that lets a segment whose
// round producer runs no git of its own still have its approved artifacts committed by the loop
// owner. Every case below builds on the replay path, following TestBouncer_Replay_Approved as its
// precedent: the replay path reaches settle with no shuttle spawn at all, so nothing but the seam
// is under test.

package shedadapters

import (
	"context"
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestBouncer_Commit_ApprovedCallsExactlyOnce pins that an APPROVED verdict calls Commit exactly
// once, before Done is returned.
func TestBouncer_Commit_ApprovedCallsExactlyOnce(t *testing.T) {
	calls := 0
	cfg := testBouncerConfig(t)
	cfg.Shuttle = &fakeShuttle{}
	cfg.Commit = func() error {
		calls++
		return nil
	}
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
		round:   1,
		report:  bouncerReport(1),
		verdict: bouncerVerdictContent("APPROVED"),
		ledger:  bouncerLedgerContent(1),
	}})

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if calls != 1 {
		t.Errorf("Commit call count = %d; want 1", calls)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
	wantPointer := ledgerPath(cfg.RunDir, 1)
	if ptr.Path != wantPointer {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
	}
}

// TestBouncer_Commit_BlockingNeverCalls pins that a BLOCKING verdict never calls Commit: an
// unapproved artifact must not be committed.
func TestBouncer_Commit_BlockingNeverCalls(t *testing.T) {
	calls := 0
	cfg := testBouncerConfig(t)
	cfg.Shuttle = &fakeShuttle{}
	cfg.Commit = func() error {
		calls++
		return nil
	}
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
		round:   1,
		report:  bouncerReport(1),
		verdict: bouncerVerdictContent("BLOCKING"),
		ledger:  bouncerLedgerContent(1),
	}})

	outcome, _, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if calls != 0 {
		t.Errorf("Commit call count = %d; want 0", calls)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
}

// TestBouncer_Commit_NilIsNotAnError pins that a nil Commit is not an error and commits nothing:
// this case is what pins the shipped Webster-Bouncer row's behaviour as unchanged, since that row
// carries no commit_seam key -- its Burler partner commits its own fixes, so BouncerConfig never
// sets Commit for this row.
func TestBouncer_Commit_NilIsNotAnError(t *testing.T) {
	cfg := testBouncerConfig(t)
	cfg.Shuttle = &fakeShuttle{}
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
		round:   1,
		report:  bouncerReport(1),
		verdict: bouncerVerdictContent("APPROVED"),
		ledger:  bouncerLedgerContent(1),
	}})

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}
	wantPointer := ledgerPath(cfg.RunDir, 1)
	if ptr.Path != wantPointer {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
	}
}

// TestBouncer_Commit_FailingCommitIsAnError pins that a failing Commit makes settle return that
// error rather than degrading to shedengine.Stuck. A regression that reroutes the failure through
// degrade is silent and its consequence is severe: an approved artifact would be bounced into a
// findings-free fixer round, re-approving and re-committing every bounce until the budget is
// spent, because judged(n) stays true on re-entry. The explicit non-Stuck assertion below is what
// catches that regression here rather than leaving it for a reader to notice.
func TestBouncer_Commit_FailingCommitIsAnError(t *testing.T) {
	sentinel := errors.New("commit failed")
	cfg := testBouncerConfig(t)
	cfg.Shuttle = &fakeShuttle{}
	cfg.Commit = func() error { return sentinel }
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
		round:   1,
		report:  bouncerReport(1),
		verdict: bouncerVerdictContent("APPROVED"),
		ledger:  bouncerLedgerContent(1),
	}})

	outcome, ptr, err := b.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("Call() error = %v; want errors.Is(err, sentinel)", err)
	}
	if outcome != "" {
		t.Errorf("Call() outcome = %q; want empty alongside a non-nil error", outcome)
	}
	if outcome == shedengine.Stuck {
		t.Error("Call() outcome = Stuck; want anything but Stuck -- a commit failure must not be routed through degrade")
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
	}
}

// TestBouncer_Commit_CancelledContextStillCommits pins that a cancelled context still runs
// Commit, and that the commit's own result -- not a cancellation error -- governs the return. An
// approved verdict is the one exception cancelErr never applies to, and that rule covers side
// effects, not just the returned outcome: leaving approved work uncommitted because an operator
// pressed Ctrl-C is precisely the dirt this seam exists to prevent.
//
// This case calls b.settle directly rather than b.Call: Call's own entryErr rejects an
// already-cancelled context before ResolveRound ever runs, which is the correct behaviour for a
// call that has not started anything yet, but it means the replay-via-Call vehicle every other
// case in this file uses cannot exercise cancellation once judged(n) already holds. settle itself
// never consults ctx on the approved branch, which is exactly the property under test here.
func TestBouncer_Commit_CancelledContextStillCommits(t *testing.T) {
	calls := 0
	cfg := testBouncerConfig(t)
	cfg.Shuttle = &fakeShuttle{}
	cfg.Commit = func() error {
		calls++
		return nil
	}
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
		round:   1,
		report:  bouncerReport(1),
		verdict: bouncerVerdictContent("APPROVED"),
		ledger:  bouncerLedgerContent(1),
	}})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	outcome, ptr, err := b.settle(ctx, 1, false)
	if err != nil {
		t.Fatalf("settle() error = %v; want nil (a genuinely parsed verdict survives cancellation)", err)
	}
	if calls != 1 {
		t.Errorf("Commit call count = %d; want 1", calls)
	}
	if outcome != shedengine.Done {
		t.Errorf("settle() outcome = %q; want %q", outcome, shedengine.Done)
	}
	wantPointer := ledgerPath(cfg.RunDir, 1)
	if ptr.Path != wantPointer {
		t.Errorf("settle() pointer = %q; want %q", ptr.Path, wantPointer)
	}
}
