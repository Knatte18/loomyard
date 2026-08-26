// bouncer_replay_test.go covers Bouncer.Call's replay mode, focus synthesis over a malformed file,
// the pointer-discipline table stated in _mill/discussion.md's "Cancellation and the output
// pointer" section, and cancellation.
// The seed call, the re-bounce, the judge call's happy paths and degradations, harvest, and debris
// handling are covered by bouncer_seed_test.go (batch 3) and bouncer_judge_test.go (this batch's
// first file).

package shedadapters

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func TestBouncer_Replay_Approved(t *testing.T) {
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	b, cfg := newTestBouncer(t, shuttle)
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
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q (an already-APPROVED round clears and re-seeds rather than replaying)", outcome, shedengine.Stuck)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty (the seed path this clear falls through to reports an empty pointer)", ptr)
	}
	if !shuttle.called {
		t.Error("Call() did not invoke the shuttle seam; want the seed spawn the clear falls through to")
	}
	if _, err := os.Stat(verdictPath(cfg.RunDir, 1)); !os.IsNotExist(err) {
		t.Errorf("round 1's verdict file still exists in the recreated run dir: %v", err)
	}
}

func TestBouncer_Replay_Blocking(t *testing.T) {
	logBuf := captureBouncerWarnings(t)
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	b, cfg := newTestBouncer(t, shuttle)
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
		round:   1,
		report:  bouncerReport(1),
		verdict: bouncerVerdictContent("BLOCKING"),
		ledger:  bouncerLedgerContent(1),
	}})
	verdictBefore, err := os.ReadFile(verdictPath(cfg.RunDir, 1))
	if err != nil {
		t.Fatalf("ReadFile(verdict) = %v; want nil", err)
	}
	ledgerBefore, err := os.ReadFile(ledgerPath(cfg.RunDir, 1))
	if err != nil {
		t.Fatalf("ReadFile(ledger) = %v; want nil", err)
	}

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	wantPointer := ledgerPath(cfg.RunDir, 1)
	if ptr.Path != wantPointer {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
	}
	if shuttle.called {
		t.Error("Call() invoked the shuttle seam on a replay; want it never called")
	}
	if logBuf.Len() == 0 {
		t.Error("Call() did not log a warning on a BLOCKING replay")
	}

	if _, err := os.Stat(focusPath(cfg.RunDir, 2)); err != nil {
		t.Errorf("os.Stat(round-2-focus.md) = %v; want nil (synthesized when absent)", err)
	}

	verdictAfter, err := os.ReadFile(verdictPath(cfg.RunDir, 1))
	if err != nil {
		t.Fatalf("ReadFile(verdict) = %v; want nil", err)
	}
	if string(verdictAfter) != string(verdictBefore) {
		t.Errorf("verdict file changed across a replay: before %q, after %q", verdictBefore, verdictAfter)
	}
	ledgerAfter, err := os.ReadFile(ledgerPath(cfg.RunDir, 1))
	if err != nil {
		t.Fatalf("ReadFile(ledger) = %v; want nil", err)
	}
	if string(ledgerAfter) != string(ledgerBefore) {
		t.Errorf("ledger file changed across a replay: before %q, after %q", ledgerBefore, ledgerAfter)
	}
}

// TestBouncer_Judged_IgnoresFocusFile proves judged(N) does not treat an absent round-2-focus.md as
// debris: its fixture deliberately carries a BLOCKING verdict, not an APPROVED one, so this test's
// subject (judged's focus-file exclusion) is isolated from the clear trigger card 10 adds, which
// fires only on an APPROVED verdict and would otherwise collide with what this test proves.
func TestBouncer_Judged_IgnoresFocusFile(t *testing.T) {
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	b, cfg := newTestBouncer(t, shuttle)
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
		round:   1,
		report:  bouncerReport(1),
		verdict: bouncerVerdictContent("BLOCKING"),
		ledger:  bouncerLedgerContent(1),
	}})
	// round-2-focus.md is deliberately absent: judged(N) must not treat that as debris.

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q (verdict and ledger present and parsing is a replay, not a re-judge)", outcome, shedengine.Stuck)
	}
	wantPointer := ledgerPath(cfg.RunDir, 1)
	if ptr.Path != wantPointer {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
	}
	if shuttle.called {
		t.Error("Call() invoked the shuttle seam despite judged(N) holding; the absent focus file must not force a re-judge")
	}
}

func TestBouncer_FocusSynthesis_OverUnparseableFile(t *testing.T) {
	t.Run("JudgeBlockingReplay", func(t *testing.T) {
		shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
		b, cfg := newTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
			round:   1,
			report:  bouncerReport(1),
			verdict: bouncerVerdictContent("BLOCKING"),
			ledger:  bouncerLedgerContent(1),
		}})
		malformed := "garbage, not frontmatter (malformed round-2 focus)"
		if err := os.WriteFile(focusPath(cfg.RunDir, 2), []byte(malformed), 0o644); err != nil {
			t.Fatalf("WriteFile(malformed focus) = %v; want nil", err)
		}

		if _, _, err := b.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}

		archived := archivedSiblingPath(focusPath(cfg.RunDir, 2), bouncerJudgeTestClock)
		got, err := os.ReadFile(archived)
		if err != nil {
			t.Fatalf("ReadFile(archived sibling %q) = %v; want nil", archived, err)
		}
		if string(got) != malformed {
			t.Errorf("archived sibling content = %q; want %q (the malformed content survives)", got, malformed)
		}

		synthRaw, err := os.ReadFile(focusPath(cfg.RunDir, 2))
		if err != nil {
			t.Fatalf("ReadFile(round-2-focus.md) = %v; want nil", err)
		}
		synth, err := parseFocus(synthRaw)
		if err != nil {
			t.Fatalf("parseFocus(...) error = %v; want nil", err)
		}
		if synth.Round != 2 {
			t.Errorf("synthetic focus Round = %d; want 2", synth.Round)
		}
		if len(synth.ExcludeLenses) != 0 {
			t.Errorf("synthetic focus ExcludeLenses = %v; want empty", synth.ExcludeLenses)
		}
		if len(synth.Focus) != 0 {
			t.Errorf("synthetic focus Focus = %v; want empty", synth.Focus)
		}
	})

	t.Run("SeedPath", func(t *testing.T) {
		shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
		b, cfg := newTestBouncer(t, shuttle)
		malformed := "garbage, not frontmatter (malformed round-1 focus)"
		if err := os.WriteFile(focusPath(cfg.RunDir, 1), []byte(malformed), 0o644); err != nil {
			t.Fatalf("WriteFile(malformed focus) = %v; want nil", err)
		}

		if _, _, err := b.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}

		archived := archivedSiblingPath(focusPath(cfg.RunDir, 1), bouncerJudgeTestClock)
		got, err := os.ReadFile(archived)
		if err != nil {
			t.Fatalf("ReadFile(archived sibling %q) = %v; want nil", archived, err)
		}
		if string(got) != malformed {
			t.Errorf("archived sibling content = %q; want %q (the malformed content survives)", got, malformed)
		}

		synthRaw, err := os.ReadFile(focusPath(cfg.RunDir, 1))
		if err != nil {
			t.Fatalf("ReadFile(round-1-focus.md) = %v; want nil", err)
		}
		synth, err := parseFocus(synthRaw)
		if err != nil {
			t.Fatalf("parseFocus(...) error = %v; want nil", err)
		}
		if synth.Round != 1 {
			t.Errorf("synthetic focus Round = %d; want 1", synth.Round)
		}
		if len(synth.ExcludeLenses) != 0 {
			t.Errorf("synthetic focus ExcludeLenses = %v; want empty", synth.ExcludeLenses)
		}
		if len(synth.Focus) != 0 {
			t.Errorf("synthetic focus Focus = %v; want empty", synth.Focus)
		}
	})
}

func TestBouncer_PointerDiscipline(t *testing.T) {
	t.Run("JudgeCall_Approved", func(t *testing.T) {
		shuttle := judgeFakeShuttle(1, bouncerVerdictContent("APPROVED"), bouncerLedgerContent(1), true)
		b, cfg := newTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

		outcome, ptr, err := b.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Done {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
		}
		if _, err := os.Stat(ptr.Path); err != nil {
			t.Errorf("os.Stat(reported pointer %q) = %v; want nil", ptr.Path, err)
		}
	})

	t.Run("JudgeCall_Blocking", func(t *testing.T) {
		shuttle := judgeFakeShuttle(1, bouncerVerdictContent("BLOCKING"), bouncerLedgerContent(1), true)
		b, cfg := newTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

		outcome, ptr, err := b.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
		if _, err := os.Stat(ptr.Path); err != nil {
			t.Errorf("os.Stat(reported pointer %q) = %v; want nil", ptr.Path, err)
		}
	})

	t.Run("Harvest_Approved", func(t *testing.T) {
		shuttle := judgeFakeShuttle(1, bouncerVerdictContent("APPROVED"), bouncerLedgerContent(1), true)
		b, cfg := newTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

		outcome, ptr, err := b.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Done {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
		}
		if _, err := os.Stat(ptr.Path); err != nil {
			t.Errorf("os.Stat(reported pointer %q) = %v; want nil", ptr.Path, err)
		}
	})

	t.Run("Replay_Blocking", func(t *testing.T) {
		shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
		b, cfg := newTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
			round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("BLOCKING"), ledger: bouncerLedgerContent(1),
		}})

		outcome, ptr, err := b.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
		if _, err := os.Stat(ptr.Path); err != nil {
			t.Errorf("os.Stat(reported pointer %q) = %v; want nil", ptr.Path, err)
		}
	})

	t.Run("SeedCall_EmptyPointer", func(t *testing.T) {
		shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
		b, _ := newTestBouncer(t, shuttle)

		outcome, ptr, err := b.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
		if ptr != (shedengine.OutputPointer{}) {
			t.Errorf("Call() pointer = %+v; want empty", ptr)
		}
	})

	t.Run("ReBounce_EmptyPointer", func(t *testing.T) {
		shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
		b, cfg := newTestBouncer(t, shuttle)
		seeded := "---\nround: 1\nexclude_lenses: []\nfocus: [\"already seeded\"]\n---\n"
		if err := os.WriteFile(focusPath(cfg.RunDir, 1), []byte(seeded), 0o644); err != nil {
			t.Fatalf("WriteFile(...) = %v; want nil", err)
		}

		outcome, ptr, err := b.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
		if ptr != (shedengine.OutputPointer{}) {
			t.Errorf("Call() pointer = %+v; want empty", ptr)
		}
	})

	t.Run("DegradedJudgePath_EmptyPointer", func(t *testing.T) {
		shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
		b, cfg := newTestBouncer(t, shuttle)
		if err := os.WriteFile(filepath.Join(cfg.RunDir, cfg.ReportName(1)), []byte(""), 0o644); err != nil {
			t.Fatalf("WriteFile(empty report) = %v; want nil", err)
		}

		outcome, ptr, err := b.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
		if ptr != (shedengine.OutputPointer{}) {
			t.Errorf("Call() pointer = %+v; want empty", ptr)
		}
	})

	t.Run("ErrorReturn_EmptyPointer", func(t *testing.T) {
		shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
		b, _ := newTestBouncer(t, shuttle)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		outcome, ptr, err := b.Call(ctx)
		if err == nil {
			t.Fatal("Call() error = nil; want non-nil")
		}
		if outcome != "" {
			t.Errorf("Call() outcome = %q; want empty alongside a non-nil error", outcome)
		}
		if ptr != (shedengine.OutputPointer{}) {
			t.Errorf("Call() pointer = %+v; want empty", ptr)
		}
	})
}

func TestBouncer_Cancellation_AlreadyCancelled(t *testing.T) {
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	b, _ := newTestBouncer(t, shuttle)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := b.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want non-nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
	}
	if shuttle.called {
		t.Error("Call() invoked the shuttle seam with an already-cancelled context")
	}
}

func TestBouncer_Cancellation_DuringRun_ParsedVerdictSurvives(t *testing.T) {
	t.Run("Approved", func(t *testing.T) {
		shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
		b, cfg := newTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

		ctx, cancel := context.WithCancel(context.Background())
		shuttle.duringRun = func() {
			outputs := shuttle.gotSpec.OutputFiles
			_ = os.WriteFile(outputs[0], []byte(bouncerVerdictContent("APPROVED")), 0o644)
			_ = os.WriteFile(outputs[1], []byte(bouncerLedgerContent(1)), 0o644)
			_ = os.WriteFile(outputs[2], []byte("---\nround: 2\nexclude_lenses: []\nfocus: []\n---\n"), 0o644)
			cancel()
		}

		outcome, ptr, err := b.Call(ctx)
		if err != nil {
			t.Fatalf("Call() error = %v; want nil (a genuinely parsed verdict survives cancellation)", err)
		}
		if outcome != shedengine.Done {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
		}
		wantPointer := ledgerPath(cfg.RunDir, 1)
		if ptr.Path != wantPointer {
			t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
		}
	})

	t.Run("Blocking", func(t *testing.T) {
		shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
		b, cfg := newTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

		ctx, cancel := context.WithCancel(context.Background())
		shuttle.duringRun = func() {
			outputs := shuttle.gotSpec.OutputFiles
			_ = os.WriteFile(outputs[0], []byte(bouncerVerdictContent("BLOCKING")), 0o644)
			_ = os.WriteFile(outputs[1], []byte(bouncerLedgerContent(1)), 0o644)
			_ = os.WriteFile(outputs[2], []byte("---\nround: 2\nexclude_lenses: []\nfocus: []\n---\n"), 0o644)
			cancel()
		}

		outcome, ptr, err := b.Call(ctx)
		if err != nil {
			t.Fatalf("Call() error = %v; want nil (a genuinely parsed verdict survives cancellation)", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
		wantPointer := ledgerPath(cfg.RunDir, 1)
		if ptr.Path != wantPointer {
			t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
		}
	})
}

func TestBouncer_Cancellation_DuringRun_OrdinaryRuleReturnsError(t *testing.T) {
	t.Run("SeedCall", func(t *testing.T) {
		shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
		b, _ := newTestBouncer(t, shuttle)
		ctx, cancel := context.WithCancel(context.Background())
		shuttle.duringRun = cancel

		outcome, ptr, err := b.Call(ctx)
		if err == nil {
			t.Fatal("Call() error = nil; want non-nil")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
		}
		if outcome != "" {
			t.Errorf("Call() outcome = %q; want empty alongside a non-nil error", outcome)
		}
		if ptr != (shedengine.OutputPointer{}) {
			t.Errorf("Call() pointer = %+v; want empty", ptr)
		}
	})

	t.Run("DegradedJudgePath", func(t *testing.T) {
		shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
		b, cfg := newTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
		ctx, cancel := context.WithCancel(context.Background())
		// duringRun cancels without writing anything usable, so judged(1) stays false and the
		// run's own outcome (OutcomeDone with no verdict/ledger written) degrades.
		shuttle.duringRun = cancel

		outcome, ptr, err := b.Call(ctx)
		if err == nil {
			t.Fatal("Call() error = nil; want non-nil")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Call() error = %v; want errors.Is(err, context.Canceled)", err)
		}
		if outcome != "" {
			t.Errorf("Call() outcome = %q; want empty alongside a non-nil error", outcome)
		}
		if ptr != (shedengine.OutputPointer{}) {
			t.Errorf("Call() pointer = %+v; want empty", ptr)
		}
		if !strings.Contains(err.Error(), "gate") {
			t.Errorf("Call() error %q does not name the producer", err.Error())
		}
	})
}
