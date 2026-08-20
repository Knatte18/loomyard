// bouncer_judge_test.go covers Bouncer.Call's judge mode: the happy paths (APPROVED and BLOCKING),
// the unconditional-OutputFiles guard against the conditional-output regression that would make
// shedengine.Done unreachable, the previous-ledger marker's three cases, every judge-call
// degradation, harvest, debris handling, and stale-output archival.
// The seed call, the re-bounce, replay, focus synthesis, pointer discipline, and cancellation are
// left to bouncer_seed_test.go (batch 3) and bouncer_replay_test.go (batch 4's own second file).

package shedadapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// bouncerJudgeFixture is one round's worth of on-disk state a judge-call test lays out before
// calling b.Call.
type bouncerJudgeFixture struct {
	round   int
	report  string // report content for this round; empty means "do not write"
	verdict string // verdict file content for this round; empty means "do not write"
	ledger  string // ledger file content for this round; empty means "do not write"
	focus   string // focus file content for this round; empty means "do not write"
}

// layoutBouncerRun writes cfg.RunDir's on-disk state for rounds 1..len(fixtures), each entry
// describing that round's report, verdict, ledger, and (existing, pre-judge) focus files.
func layoutBouncerRun(t *testing.T, cfg BouncerConfig, fixtures []bouncerJudgeFixture) {
	t.Helper()
	for _, f := range fixtures {
		if f.report != "" {
			path := filepath.Join(cfg.RunDir, cfg.ReportName(f.round))
			if err := os.WriteFile(path, []byte(f.report), 0o644); err != nil {
				t.Fatalf("WriteFile(report round %d) = %v; want nil", f.round, err)
			}
		}
		if f.verdict != "" {
			if err := os.WriteFile(verdictPath(cfg.RunDir, f.round), []byte(f.verdict), 0o644); err != nil {
				t.Fatalf("WriteFile(verdict round %d) = %v; want nil", f.round, err)
			}
		}
		if f.ledger != "" {
			if err := os.WriteFile(ledgerPath(cfg.RunDir, f.round), []byte(f.ledger), 0o644); err != nil {
				t.Fatalf("WriteFile(ledger round %d) = %v; want nil", f.round, err)
			}
		}
		if f.focus != "" {
			if err := os.WriteFile(focusPath(cfg.RunDir, f.round), []byte(f.focus), 0o644); err != nil {
				t.Fatalf("WriteFile(focus round %d) = %v; want nil", f.round, err)
			}
		}
	}
}

// bouncerReport, bouncerVerdictContent, and bouncerLedgerContent are minimal well-formed file
// bodies for the three file contracts, reused across this batch's fixtures.
func bouncerReport(round int) string {
	return fmt.Sprintf("---\nround: %d\n---\nfindings for round %d\n", round, round)
}

func bouncerVerdictContent(verdict string) string {
	return fmt.Sprintf("---\nverdict: %s\nrationale: because reasons\n---\n", verdict)
}

func bouncerLedgerContent(round int) string {
	return fmt.Sprintf("---\nround: %d\nledger: []\n---\nno open findings\n", round)
}

// judgeFakeShuttle returns a fakeShuttle whose duringRun writes verdict, ledger, and (unless
// omitted) a next-round focus file to the spec's declared OutputFiles, then reports OutcomeDone.
func judgeFakeShuttle(round int, verdictBody, ledgerBody string, writeFocus bool) *fakeShuttle {
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	shuttle.duringRun = func() {
		outputs := shuttle.gotSpec.OutputFiles
		if len(outputs) != 3 {
			return
		}
		_ = os.WriteFile(outputs[0], []byte(verdictBody), 0o644)
		_ = os.WriteFile(outputs[1], []byte(ledgerBody), 0o644)
		if writeFocus {
			_ = os.WriteFile(outputs[2], []byte(fmt.Sprintf("---\nround: %d\nexclude_lenses: []\nfocus: []\n---\n", round+1)), 0o644)
		}
	}
	return shuttle
}

func TestBouncer_JudgeCall_Approved(t *testing.T) {
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
	wantPointer := ledgerPath(cfg.RunDir, 1)
	if ptr.Path != wantPointer {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
	}
	if _, err := os.Stat(ptr.Path); err != nil {
		t.Errorf("os.Stat(reported pointer %q) = %v; want nil", ptr.Path, err)
	}

	if len(shuttle.gotSpec.OutputFiles) != 3 {
		t.Errorf("recorded Spec.OutputFiles has %d entries; want 3", len(shuttle.gotSpec.OutputFiles))
	}

	nextFocus := focusPath(cfg.RunDir, 2)
	raw, err := os.ReadFile(nextFocus)
	if err != nil {
		t.Fatalf("ReadFile(round-2-focus.md) = %v; want nil", err)
	}
	parsed, err := parseFocus(raw)
	if err != nil {
		t.Fatalf("parseFocus(...) error = %v; want nil", err)
	}
	if len(parsed.ExcludeLenses) != 0 {
		t.Errorf("parsed focus ExcludeLenses = %v; want empty", parsed.ExcludeLenses)
	}
	if len(parsed.Focus) != 0 {
		t.Errorf("parsed focus Focus = %v; want empty", parsed.Focus)
	}
}

func TestBouncer_JudgeCall_Blocking(t *testing.T) {
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
	wantPointer := ledgerPath(cfg.RunDir, 1)
	if ptr.Path != wantPointer {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, wantPointer)
	}

	if len(shuttle.gotSpec.OutputFiles) != 3 {
		t.Errorf("recorded Spec.OutputFiles has %d entries; want 3", len(shuttle.gotSpec.OutputFiles))
	}

	for _, path := range []string{verdictPath(cfg.RunDir, 1), ledgerPath(cfg.RunDir, 1), focusPath(cfg.RunDir, 2)} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("os.Stat(%q) = %v; want nil (all three files exist)", path, err)
		}
	}
}

func TestBouncer_JudgeCall_RoundThree_UsesRoundTwoLedger(t *testing.T) {
	shuttle := judgeFakeShuttle(3, bouncerVerdictContent("APPROVED"), bouncerLedgerContent(3), true)
	b, cfg := newTestBouncer(t, shuttle)
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{
		{round: 1, report: bouncerReport(1)},
		{round: 2, report: bouncerReport(2)},
		{round: 3, report: bouncerReport(3)},
	})
	// Ledgers for rounds 1 and 2 already exist on disk (a fully judged round 1 and round 2).
	if err := os.WriteFile(ledgerPath(cfg.RunDir, 1), []byte(bouncerLedgerContent(1)), 0o644); err != nil {
		t.Fatalf("WriteFile(round-1 ledger) = %v; want nil", err)
	}
	if err := os.WriteFile(ledgerPath(cfg.RunDir, 2), []byte(bouncerLedgerContent(2)), 0o644); err != nil {
		t.Fatalf("WriteFile(round-2 ledger) = %v; want nil", err)
	}
	if err := os.WriteFile(verdictPath(cfg.RunDir, 1), []byte(bouncerVerdictContent("BLOCKING")), 0o644); err != nil {
		t.Fatalf("WriteFile(round-1 verdict) = %v; want nil", err)
	}
	if err := os.WriteFile(verdictPath(cfg.RunDir, 2), []byte(bouncerVerdictContent("BLOCKING")), 0o644); err != nil {
		t.Fatalf("WriteFile(round-2 verdict) = %v; want nil", err)
	}

	outcome, _, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}

	wantReport := filepath.Join(cfg.RunDir, cfg.ReportName(3))
	if !strings.Contains(shuttle.gotSpec.Prompt, wantReport) {
		t.Errorf("recorded Spec.Prompt does not contain round 3's report path %q", wantReport)
	}
	wantPrevLedger := ledgerPath(cfg.RunDir, 2)
	if !strings.Contains(shuttle.gotSpec.Prompt, wantPrevLedger) {
		t.Errorf("recorded Spec.Prompt does not contain round 2's ledger path %q (previous_ledger)", wantPrevLedger)
	}
}

func TestBouncer_JudgeCall_PreviousLedgerHandling(t *testing.T) {
	t.Run("ValidPriorLedger", func(t *testing.T) {
		shuttle := judgeFakeShuttle(2, bouncerVerdictContent("APPROVED"), bouncerLedgerContent(2), true)
		b, cfg := newTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{
			{round: 1, report: bouncerReport(1)},
			{round: 2, report: bouncerReport(2)},
		})
		if err := os.WriteFile(ledgerPath(cfg.RunDir, 1), []byte(bouncerLedgerContent(1)), 0o644); err != nil {
			t.Fatalf("WriteFile(round-1 ledger) = %v; want nil", err)
		}
		if err := os.WriteFile(verdictPath(cfg.RunDir, 1), []byte(bouncerVerdictContent("BLOCKING")), 0o644); err != nil {
			t.Fatalf("WriteFile(round-1 verdict) = %v; want nil", err)
		}

		if _, _, err := b.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		wantPrevLedger := ledgerPath(cfg.RunDir, 1)
		if !strings.Contains(shuttle.gotSpec.Prompt, wantPrevLedger) {
			t.Errorf("recorded Spec.Prompt does not contain the valid prior ledger's absolute path %q", wantPrevLedger)
		}
	})

	t.Run("MalformedPriorLedger", func(t *testing.T) {
		shuttle := judgeFakeShuttle(2, bouncerVerdictContent("APPROVED"), bouncerLedgerContent(2), true)
		b, cfg := newTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{
			{round: 1, report: bouncerReport(1)},
			{round: 2, report: bouncerReport(2)},
		})
		if err := os.WriteFile(ledgerPath(cfg.RunDir, 1), []byte("not frontmatter at all"), 0o644); err != nil {
			t.Fatalf("WriteFile(round-1 ledger) = %v; want nil", err)
		}

		outcome, _, err := b.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Done {
			t.Errorf("Call() outcome = %q; want %q (the judge still runs)", outcome, shedengine.Done)
		}
		if !strings.Contains(shuttle.gotSpec.Prompt, "(none)") {
			t.Error("recorded Spec.Prompt does not contain the (none) literal for a malformed prior ledger")
		}
	})

	t.Run("NoPriorLedger", func(t *testing.T) {
		shuttle := judgeFakeShuttle(1, bouncerVerdictContent("APPROVED"), bouncerLedgerContent(1), true)
		b, cfg := newTestBouncer(t, shuttle)
		layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

		if _, _, err := b.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if !strings.Contains(shuttle.gotSpec.Prompt, "(none)") {
			t.Error("recorded Spec.Prompt does not contain the (none) literal when no prior ledger exists")
		}
	})
}
