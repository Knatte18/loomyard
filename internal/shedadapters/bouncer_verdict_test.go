// bouncer_verdict_test.go covers the Bouncer's verdict vocabulary: the strict parse, the legacy aliases read only at Call entry, the harvest rule that refuses a legacy word, and the three verdicts' outcome mapping.

package shedadapters

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

func TestParseVerdict_Vocabulary(t *testing.T) {
	for _, word := range []bouncerVerdict{verdictConverged, verdictContinue, verdictCircling} {
		got, _, err := parseVerdict([]byte(bouncerVerdictContent(string(word))))
		if err != nil {
			t.Errorf("parseVerdict(%q) error = %v; want nil", word, err)
		}
		if got != word {
			t.Errorf("parseVerdict(%q) = %q; want %q", word, got, word)
		}
	}
	for _, word := range []string{"APPROVED", "BLOCKING", "converged"} {
		if _, _, err := parseVerdict([]byte(bouncerVerdictContent(word))); err == nil {
			t.Errorf("parseVerdict(%q) error = nil; want a rejection", word)
		}
	}
}

func TestParseRecordedVerdict_AliasesLegacyWords(t *testing.T) {
	tests := []struct {
		word       string
		want       bouncerVerdict
		wantLegacy bool
	}{
		{"APPROVED", verdictConverged, true},
		{"BLOCKING", verdictContinue, true},
		{"CONVERGED", verdictConverged, false},
		{"CIRCLING", verdictCircling, false},
	}
	for _, tt := range tests {
		got, _, legacy, err := parseRecordedVerdict([]byte(bouncerVerdictContent(tt.word)))
		if err != nil {
			t.Fatalf("parseRecordedVerdict(%q) error = %v; want nil", tt.word, err)
		}
		if got != tt.want || legacy != tt.wantLegacy {
			t.Errorf("parseRecordedVerdict(%q) = (%q, legacy %v); want (%q, legacy %v)", tt.word, got, legacy, tt.want, tt.wantLegacy)
		}
	}
	if _, _, _, err := parseRecordedVerdict([]byte(bouncerVerdictContent("MAYBE"))); err == nil {
		t.Error("parseRecordedVerdict(MAYBE) error = nil; want a rejection")
	}
}

func TestHarvestedVerdict_RefusesLegacyWords(t *testing.T) {
	dir := t.TempDir()
	writeVerdictAndLedger(t, dir, 1, 1)
	if err := os.WriteFile(verdictPath(dir, 1), []byte(bouncerVerdictContent("APPROVED")), 0o644); err != nil {
		t.Fatalf("WriteFile(verdict) = %v; want nil", err)
	}

	if _, ok := harvestedVerdict(dir, 1); ok {
		t.Error("harvestedVerdict(legacy APPROVED) ok = true; want false")
	}
	if got, ok := recordedVerdict(dir, 1); !ok || got != verdictConverged {
		t.Errorf("recordedVerdict(legacy APPROVED) = (%q, %v); want (%q, true)", got, ok, verdictConverged)
	}
}

func TestBouncer_Verdict_LegacyApprovedAtEntryClearsAndReseeds(t *testing.T) {
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	b, cfg := newBouncerFixture(t, withNestedRunDir(), withShuttle(shuttle)).Build()
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
		round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("APPROVED"), ledger: bouncerLedgerContent(1),
	}})

	ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
	}
	if !shuttle.Called {
		t.Error("Call() did not spawn the seed the clear falls through to")
	}
	if _, err := os.Stat(archivedRunDirPath(cfg.RunDir, bouncerJudgeTestClock, "")); err != nil {
		t.Errorf("expected the archived generation: %v", err)
	}
}

func TestBouncer_Verdict_LegacyBlockingAtEntryReplaysAsStuck(t *testing.T) {
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
		round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("BLOCKING"), ledger: bouncerLedgerContent(1),
	}})

	ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
	if want := ledgerPath(cfg.RunDir, 1); ptr.Path != want {
		t.Errorf("Call() pointer = %q; want %q, not a degraded empty pointer", ptr.Path, want)
	}
	if shuttle.Called {
		t.Error("Call() spawned a run on a replay; want none")
	}
}

func TestBouncer_Verdict_SpawnedJudgeWritingLegacyWordIsRejudged(t *testing.T) {
	shuttle := judgeFakeShuttle(1, bouncerVerdictContent("APPROVED"), bouncerLedgerContent(1), true)
	b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

	ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
	if ptr.Path != "" || !strings.Contains(ptr.Reason, "retired verdict word") {
		t.Errorf("Call() pointer = %+v; want an empty Path and a Reason naming the retired verdict word", ptr)
	}
	if b.judged(1) {
		t.Error("judged(1) = true; want false so the round is re-judged")
	}
	for _, path := range judgeOutputs(cfg.RunDir, 1) {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("os.Stat(%s) = %v; want the output archived away", path, err)
		}
	}
}

func TestBouncer_Verdict_AttachedJudgeWritingLegacyWordIsRejudged(t *testing.T) {
	attach := &shedfake.Shuttle{
		AttachFound:  true,
		AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, SessionID: "live-judge"},
	}
	b, cfg := newBouncerFixture(t, withShuttle(attach)).Build()
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
	attach.DuringAttach = func() {
		_ = os.WriteFile(verdictPath(cfg.RunDir, 1), []byte(bouncerVerdictContent("APPROVED")), 0o644)
		_ = os.WriteFile(ledgerPath(cfg.RunDir, 1), []byte(bouncerLedgerContent(1)), 0o644)
	}

	ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
	if !strings.Contains(ptr.Reason, "retired verdict word") {
		t.Errorf("Call() Reason = %q; want it to name the retired verdict word", ptr.Reason)
	}
	if b.judged(1) {
		t.Error("judged(1) = true; want false so the round is re-judged")
	}
}

func TestBouncer_Verdict_ConvergedApprovesCommitsAndNextCallClears(t *testing.T) {
	var order []string
	shuttle := judgeFakeShuttle(1, bouncerVerdictContent("CONVERGED"), bouncerLedgerContent(1), true)
	cfg := newBouncerFixture(t, withNestedRunDir(), withShuttle(shuttle)).Config
	cfg.Approve = func() error { order = append(order, "approve"); return nil }
	cfg.Commit = func() error { order = append(order, "commit"); return nil }
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

	ptr := shedfake.RequireOutcome(t, b, shedengine.Done)
	if want := ledgerPath(cfg.RunDir, 1); ptr.Path != want {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
	}
	if got := strings.Join(order, ","); got != "approve,commit" {
		t.Errorf("seam order = %q; want approve,commit", got)
	}

	shedfake.RequireOutcome(t, b, shedengine.Stuck)
	if _, err := os.Stat(archivedRunDirPath(cfg.RunDir, bouncerJudgeTestClock, "")); err != nil {
		t.Errorf("expected the next Call to archive the settled generation: %v", err)
	}
}

func TestBouncer_Verdict_ContinueReturnsStuckWithLedgerPointer(t *testing.T) {
	shuttle := judgeFakeShuttle(1, bouncerVerdictContent("CONTINUE"), bouncerLedgerContent(1), true)
	b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})

	ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
	if want := ledgerPath(cfg.RunDir, 1); ptr.Path != want {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
	}
}

func TestBouncer_Verdict_CirclingReturnsAwaitingAndReplaysWithoutSpawning(t *testing.T) {
	shuttle := judgeFakeShuttle(2, bouncerVerdictContent("CIRCLING"), openGatingLedgerContent(2), true)
	b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{
		{round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("CONTINUE"), ledger: openGatingLedgerContent(1)},
		{round: 2, report: bouncerReport(2)},
	})

	ptr := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
	if want := ledgerPath(cfg.RunDir, 2); ptr.Path != want {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
	}
	if !strings.Contains(ptr.Reason, "round 2") || !strings.Contains(ptr.Reason, "cause: circling") {
		t.Errorf("Call() Reason = %q; want it to name the round and the circling cause", ptr.Reason)
	}

	again := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	cfg.Shuttle = again
	b2, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	ptr2 := shedfake.RequireOutcome(t, b2, shedengine.Awaiting)
	if ptr2.Path != ptr.Path {
		t.Errorf("second Call() pointer = %q; want %q", ptr2.Path, ptr.Path)
	}
	if again.Called {
		t.Error("second Call() spawned a run; want a replay with none")
	}
}

func TestBouncer_Verdict_CirclingWithLiveJudgeAttachesAndHarvests(t *testing.T) {
	attach := &shedfake.Shuttle{
		AttachFound:  true,
		AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, SessionID: "live-judge"},
	}
	b, cfg := newBouncerFixture(t, withShuttle(attach)).Build()
	layoutBouncerRun(t, cfg, []bouncerJudgeFixture{
		{round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("CONTINUE"), ledger: openGatingLedgerContent(1)},
		{round: 2, report: bouncerReport(2), verdict: bouncerVerdictContent("CIRCLING"), ledger: openGatingLedgerContent(2)},
	})
	realFocus := "---\nround: 3\nexclude_lenses: []\nfocus: [\"the judge's own targeting\"]\n---\n"
	attach.DuringAttach = func() {
		_ = os.WriteFile(focusPath(cfg.RunDir, 3), []byte(realFocus), 0o644)
	}

	shedfake.RequireOutcome(t, b, shedengine.Awaiting)
	if !attach.AttachCalled {
		t.Error("Attach was not called; want the entry probe to run first")
	}
	if attach.Called {
		t.Error("Run was called; want a live judge attached to, never respawned over")
	}
	got, err := os.ReadFile(focusPath(cfg.RunDir, 3))
	if err != nil {
		t.Fatalf("ReadFile(round-3-focus.md) = %v; want nil", err)
	}
	if string(got) != realFocus {
		t.Errorf("round-3-focus.md = %q; want the live judge's own targeting %q", got, realFocus)
	}
}
