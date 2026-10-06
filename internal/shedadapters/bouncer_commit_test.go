// bouncer_commit_test.go covers BouncerConfig.Commit and BouncerConfig.Approve -- the two seams
// that let a segment whose round producer runs no git of its own still have its reviewed artifacts
// approved and committed by the loop owner. Every case below builds on the harvest vehicle:
// judgeFakeShuttle writes the round's verdict and ledger during the run, so judgeCall harvests and
// settle runs within the same Call that produced them, and nothing but the seam(s) is under test.

package shedadapters

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// seamLog is a call log both the Approve and Commit closures append a marker to; one shared log,
// never two independent booleans, since two booleans cannot distinguish the two orderings.
type seamLog struct{ calls []string }

func (l *seamLog) approve(err error) func() error {
	return func() error {
		l.calls = append(l.calls, "approve")
		return err
	}
}

func (l *seamLog) commit(err error) func() error {
	return func() error {
		l.calls = append(l.calls, "commit")
		return err
	}
}

// TestBouncer_ConvergedSettle_CallsApproveThenCommit pins that a CONVERGED verdict calls each
// non-nil seam exactly once, Approve strictly before Commit, before Done is returned with the
// ledger as pointer. A nil seam is not an error and does nothing: the no-seam row is what pins the
// shipped Webster-Bouncer row's behaviour as unchanged, since that row carries no commit_seam key
// -- its Burler partner commits its own fixes, so BouncerConfig never sets Commit for it.
//
//testtiming:keep pins that each non-nil seam runs exactly once, Approve strictly before Commit, and that a nil seam is not an error
func TestBouncer_ConvergedSettle_CallsApproveThenCommit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		approve bool
		commit  bool
		want    []string
	}{
		{"a Commit alone is called exactly once", false, true, []string{"commit"}},
		{"no seams is not an error", false, false, nil},
		{"Approve runs strictly before Commit", true, true, []string{"approve", "commit"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var log seamLog
			cfg := newBouncerFixture(t).Config
			cfg.Shuttle = judgeFakeShuttle(1, bouncerVerdictContent("CONVERGED"), bouncerLedgerContent(1), true)
			if tt.approve {
				cfg.Approve = log.approve(nil)
			}
			if tt.commit {
				cfg.Commit = log.commit(nil)
			}
			b, err := NewBouncer(cfg)
			if err != nil {
				t.Fatalf("NewBouncer(...) error = %v; want nil", err)
			}
			layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

			ptr := shedfake.RequireOutcome(t, b, shedengine.Done)
			if !slices.Equal(log.calls, tt.want) {
				t.Errorf("seam calls = %v; want %v", log.calls, tt.want)
			}
			if wantPointer := ledgerPath(cfg.RunDir, 1); ptr.Path != wantPointer {
				t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
			}
		})
	}
}

// TestBouncer_BlockingNeverCallsApproveOrCommit pins that a CONTINUE verdict calls neither seam:
// an unapproved artifact must not be approved or committed.
//
//testtiming:keep pins that a CONTINUE verdict calls neither the Approve nor the Commit seam
func TestBouncer_BlockingNeverCallsApproveOrCommit(t *testing.T) {
	t.Parallel()
	var log seamLog
	cfg := newBouncerFixture(t).Config
	cfg.Shuttle = &shedfake.Shuttle{}
	cfg.Approve = log.approve(nil)
	cfg.Commit = log.commit(nil)
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
		round:   1,
		report:  bouncerReport(1),
		verdict: bouncerVerdictContent("CONTINUE"),
		ledger:  bouncerLedgerContent(1),
	}})

	shedfake.RequireOutcome(t, b, shedengine.Stuck)
	if len(log.calls) != 0 {
		t.Errorf("seam calls = %v; want none", log.calls)
	}
}

// TestBouncer_FailingSeamIsAnError pins that a failing seam makes settle return that error rather
// than degrading to shedengine.Stuck, and that a failing Approve skips Commit. A regression that
// reroutes the failure through degrade is silent and its consequence is severe: an approved
// artifact would be bounced into a findings-free fixer round, re-approving and re-committing every
// bounce until the budget is spent, because judged(n) stays true on re-entry. The explicit
// non-Stuck assertion below is what catches that regression here rather than leaving it for a
// reader to notice.
func TestBouncer_FailingSeamIsAnError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		failApprove bool
		want        []string
	}{
		{"a failing Commit", false, []string{"commit"}},
		{"a failing Approve skips Commit", true, []string{"approve"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sentinel := errors.New("seam failed")
			var log seamLog
			cfg := newBouncerFixture(t).Config
			cfg.Shuttle = judgeFakeShuttle(1, bouncerVerdictContent("CONVERGED"), bouncerLedgerContent(1), true)
			if tt.failApprove {
				cfg.Approve = log.approve(sentinel)
				cfg.Commit = log.commit(nil)
			} else {
				cfg.Commit = log.commit(sentinel)
			}
			b, err := NewBouncer(cfg)
			if err != nil {
				t.Fatalf("NewBouncer(...) error = %v; want nil", err)
			}
			layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

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
				t.Error("Call() outcome = Stuck; want anything but Stuck -- a seam failure must not be routed through degrade")
			}
			if ptr != (shedengine.OutputPointer{}) {
				t.Errorf("Call() pointer = %+v; want empty", ptr)
			}
			if !slices.Equal(log.calls, tt.want) {
				t.Errorf("seam calls = %v; want %v", log.calls, tt.want)
			}
		})
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
//
//testtiming:keep pins that an already-cancelled context still commits an approved verdict, which no Call-driven test can reach
func TestBouncer_Commit_CancelledContextStillCommits(t *testing.T) {
	var log seamLog
	cfg := newBouncerFixture(t).Config
	cfg.Shuttle = &shedfake.Shuttle{}
	cfg.Commit = log.commit(nil)
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
		round:   1,
		report:  bouncerReport(1),
		verdict: bouncerVerdictContent("CONVERGED"),
		ledger:  bouncerLedgerContent(1),
	}})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	outcome, ptr, err := b.settle(ctx, 1, false)
	if err != nil {
		t.Fatalf("settle() error = %v; want nil (a genuinely parsed verdict survives cancellation)", err)
	}
	if !slices.Equal(log.calls, []string{"commit"}) {
		t.Errorf("seam calls = %v; want [commit]", log.calls)
	}
	if outcome != shedengine.Done {
		t.Errorf("settle() outcome = %q; want %q", outcome, shedengine.Done)
	}
	wantPointer := ledgerPath(cfg.RunDir, 1)
	if ptr.Path != wantPointer {
		t.Errorf("settle() pointer = %q; want %q", ptr.Path, wantPointer)
	}
}
