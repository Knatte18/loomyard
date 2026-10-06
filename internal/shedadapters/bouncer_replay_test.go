// bouncer_replay_test.go covers Bouncer.Call's replay mode, focus synthesis over a malformed file,
// and cancellation.
// The seed call, the re-bounce, the judge call's happy paths and degradations, harvest, and debris
// handling are covered by bouncer_seed_test.go (batch 3) and bouncer_judge_test.go (this batch's
// first file).

package shedadapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// TestBouncer_Replay_Blocking also pins that judged(N) does not treat an absent next-round focus file as debris: the CONTINUE verdict replays rather than re-judging.
//
//testtiming:keep pins the replay's warning log and that the verdict and ledger are left byte-identical, which the tests that reach a replay do not check
func TestBouncer_Replay_Blocking(t *testing.T) {
	logBuf := logcapture.Capture(t)
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
		round:   1,
		report:  bouncerReport(1),
		verdict: bouncerVerdictContent("CONTINUE"),
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

	ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
	wantPointer := ledgerPath(cfg.RunDir, 1)
	if ptr.Path != wantPointer {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
	}
	if shuttle.Called {
		t.Error("Call() invoked the shuttle seam on a replay; want it never called")
	}
	if logBuf.String() == "" {
		t.Error("Call() did not log a warning on a CONTINUE replay")
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

// TestBouncer_FocusSynthesis_OverUnparseableFile covers an unparseable focus file standing at the
// path the call is about to write: it is archived byte-identical beside the original and a
// well-formed, empty focus file for the round replaces it, on the judge's replay (round 2's file)
// and on the seed path (round 1's).
//
//testtiming:keep pins that an unparseable focus file is archived byte-identical and replaced by a well-formed empty one, on both the replay and the seed path
func TestBouncer_FocusSynthesis_OverUnparseableFile(t *testing.T) {
	tests := []struct {
		name string
		// replay is whether round 1 is already judged CONTINUE, so the call replays and
		// synthesizes round 2's focus; otherwise the call seeds round 1.
		replay bool
	}{
		{"JudgeBlockingReplay", true},
		{"SeedPath", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
			b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()
			round := 1
			if tt.replay {
				round = 2
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
					round:   1,
					report:  bouncerReport(1),
					verdict: bouncerVerdictContent("CONTINUE"),
					ledger:  bouncerLedgerContent(1),
				}})
			}
			malformed := fmt.Sprintf("garbage, not frontmatter (malformed round-%d focus)", round)
			if err := os.WriteFile(focusPath(cfg.RunDir, round), []byte(malformed), 0o644); err != nil {
				t.Fatalf("WriteFile(malformed focus) = %v; want nil", err)
			}

			shedfake.CallOK(t, b)

			archived := archivedSiblingPath(focusPath(cfg.RunDir, round), bouncerJudgeTestClock)
			got, err := os.ReadFile(archived)
			if err != nil {
				t.Fatalf("ReadFile(archived sibling %q) = %v; want nil", archived, err)
			}
			if string(got) != malformed {
				t.Errorf("archived sibling content = %q; want %q (the malformed content survives)", got, malformed)
			}

			synthRaw, err := os.ReadFile(focusPath(cfg.RunDir, round))
			if err != nil {
				t.Fatalf("ReadFile(round-%d-focus.md) = %v; want nil", round, err)
			}
			synth, err := parseFocus(synthRaw)
			if err != nil {
				t.Fatalf("parseFocus(...) error = %v; want nil", err)
			}
			if synth.Round != round {
				t.Errorf("synthetic focus Round = %d; want %d", synth.Round, round)
			}
			if len(synth.ExcludeLenses) != 0 {
				t.Errorf("synthetic focus ExcludeLenses = %v; want empty", synth.ExcludeLenses)
			}
			if len(synth.Focus) != 0 {
				t.Errorf("synthetic focus Focus = %v; want empty", synth.Focus)
			}
		})
	}
}

// TestBouncer_Cancellation_ParsedVerdictSurvives pins that a verdict the judge genuinely wrote
// survives a cancellation that arrives during the run: the call returns the verdict's outcome with
// the ledger as pointer, never the cancellation error.
//
//testtiming:keep pins that a verdict the judge wrote survives a cancellation during the run, for both an approving and a blocking verdict
func TestBouncer_Cancellation_ParsedVerdictSurvives(t *testing.T) {
	tests := []struct {
		name        string
		verdict     string
		wantOutcome shedengine.Outcome
	}{
		{"Approved", "CONVERGED", shedengine.Done},
		{"Blocking", "CONTINUE", shedengine.Stuck},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
			b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()
			layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

			ctx, cancel := context.WithCancel(context.Background())
			shuttle.DuringRun = func() {
				outputs := shuttle.GotSpec.OutputFiles
				_ = os.WriteFile(outputs[0], []byte(bouncerVerdictContent(tt.verdict)), 0o644)
				_ = os.WriteFile(outputs[1], []byte(bouncerLedgerContent(1)), 0o644)
				_ = os.WriteFile(outputs[2], []byte("---\nround: 2\nexclude_lenses: []\nfocus: []\n---\n"), 0o644)
				cancel()
			}

			outcome, ptr, err := b.Call(ctx)
			if err != nil {
				t.Fatalf("Call() error = %v; want nil (a genuinely parsed verdict survives cancellation)", err)
			}
			if outcome != tt.wantOutcome {
				t.Errorf("Call() outcome = %q; want %q", outcome, tt.wantOutcome)
			}
			wantPointer := ledgerPath(cfg.RunDir, 1)
			if ptr.Path != wantPointer {
				t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
			}
		})
	}
}

// TestBouncer_Cancellation_ReturnsErrorAndEmptyPointer covers every cancellation that is not a
// parsed verdict surviving: an already-cancelled context never spawns, and a cancellation during
// the seed run or a degrading judge run returns the context error with no outcome and an empty
// pointer.
func TestBouncer_Cancellation_ReturnsErrorAndEmptyPointer(t *testing.T) {
	tests := []struct {
		name string
		// alreadyCancelled cancels the context before the call; otherwise the run's DuringRun
		// hook cancels it.
		alreadyCancelled bool
		// judge lays out a round-1 report so the call is a judge call; otherwise it seeds.
		judge bool
	}{
		{name: "AlreadyCancelled", alreadyCancelled: true},
		{name: "DuringRun_SeedCall"},
		// DuringRun cancels without writing anything usable, so judged(1) stays false and the run's
		// own outcome (OutcomeDone with no verdict/ledger written) degrades.
		{name: "DuringRun_DegradedJudgePath", judge: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
			b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()
			if tt.judge {
				layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
			}
			ctx, cancel := context.WithCancel(context.Background())
			if tt.alreadyCancelled {
				cancel()
			} else {
				shuttle.DuringRun = cancel
			}

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
			if tt.alreadyCancelled && shuttle.Called {
				t.Error("Call() invoked the shuttle seam with an already-cancelled context")
			}
		})
	}
}
