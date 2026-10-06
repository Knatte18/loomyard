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

// TestParseVerdict_Vocabulary covers the strict parse, which accepts only the three current words,
// and the recorded parse, which also aliases the two legacy words and flags them as legacy.
func TestParseVerdict_Vocabulary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		word string
		// strictOK is whether the strict parse accepts the word.
		strictOK bool
		// recordedErr is whether the recorded parse rejects the word; otherwise it yields
		// recorded, legacy.
		recordedErr bool
		recorded    bouncerVerdict
		legacy      bool
	}{
		{word: "CONVERGED", strictOK: true, recorded: verdictConverged},
		{word: "CONTINUE", strictOK: true, recorded: verdictContinue},
		{word: "CIRCLING", strictOK: true, recorded: verdictCircling},
		{word: "APPROVED", recorded: verdictConverged, legacy: true},
		{word: "BLOCKING", recorded: verdictContinue, legacy: true},
		{word: "converged", recordedErr: true},
		{word: "MAYBE", recordedErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			t.Parallel()
			content := []byte(bouncerVerdictContent(tt.word))

			strict, _, err := parseVerdict(content)
			if (err == nil) != tt.strictOK {
				t.Errorf("parseVerdict(%q) error = %v; want accepted = %v", tt.word, err, tt.strictOK)
			}
			if tt.strictOK && strict != bouncerVerdict(tt.word) {
				t.Errorf("parseVerdict(%q) = %q; want %q", tt.word, strict, tt.word)
			}

			got, _, legacy, err := parseRecordedVerdict(content)
			if tt.recordedErr {
				if err == nil {
					t.Errorf("parseRecordedVerdict(%q) error = nil; want a rejection", tt.word)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRecordedVerdict(%q) error = %v; want nil", tt.word, err)
			}
			if got != tt.recorded || legacy != tt.legacy {
				t.Errorf("parseRecordedVerdict(%q) = (%q, legacy %v); want (%q, legacy %v)", tt.word, got, legacy, tt.recorded, tt.legacy)
			}
		})
	}
}

//testtiming:keep pins the function-level split on a legacy word: the harvest refuses it while the recorded read aliases it
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

//testtiming:keep pins how a legacy verdict word on disk at Call entry is read: APPROVED clears and re-seeds, BLOCKING replays as a Stuck over the ledger
func TestBouncer_Verdict_LegacyWordAtEntry(t *testing.T) {
	tests := []struct {
		name string
		word string
		// clears is whether the legacy word settles the round as approved, so the entry clears
		// and re-seeds; otherwise it replays as a counted Stuck over the round's ledger.
		clears bool
	}{
		{"a legacy APPROVED clears and re-seeds", "APPROVED", true},
		{"a legacy BLOCKING replays as Stuck", "BLOCKING", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
			b, cfg := newBouncerFixture(t, withNestedRunDir(), withShuttle(shuttle)).Build()
			layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{
				round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent(tt.word), ledger: bouncerLedgerContent(1),
			}})

			ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
			if tt.clears {
				if ptr != (shedengine.OutputPointer{}) {
					t.Errorf("Call() pointer = %+v; want empty", ptr)
				}
				if !shuttle.Called {
					t.Error("Call() did not spawn the seed the clear falls through to")
				}
				if _, err := os.Stat(archivedRunDirPath(cfg.RunDir, bouncerJudgeTestClock, "")); err != nil {
					t.Errorf("expected the archived generation: %v", err)
				}
				return
			}
			if want := ledgerPath(cfg.RunDir, 1); ptr.Path != want {
				t.Errorf("Call() pointer = %q; want %q, not a degraded empty pointer", ptr.Path, want)
			}
			if shuttle.Called {
				t.Error("Call() spawned a run on a replay; want none")
			}
		})
	}
}

func TestBouncer_Verdict_JudgeWritingLegacyWordIsRejudged(t *testing.T) {
	tests := []struct {
		name string
		// attached is whether a live judge is attached to rather than a fresh one spawned.
		attached bool
	}{
		{"a spawned judge", false},
		{"an attached judge", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var shuttle *shedfake.Shuttle
			if tt.attached {
				shuttle = &shedfake.Shuttle{
					AttachFound:  true,
					AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, SessionID: "live-judge"},
				}
			} else {
				shuttle = judgeFakeShuttle(1, bouncerVerdictContent("APPROVED"), bouncerLedgerContent(1), true)
			}
			b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()
			layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1)}})
			if tt.attached {
				shuttle.DuringAttach = func() {
					_ = os.WriteFile(verdictPath(cfg.RunDir, 1), []byte(bouncerVerdictContent("APPROVED")), 0o644)
					_ = os.WriteFile(ledgerPath(cfg.RunDir, 1), []byte(bouncerLedgerContent(1)), 0o644)
				}
			}

			ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
			if !strings.Contains(ptr.Reason, "retired verdict word") {
				t.Errorf("Call() Reason = %q; want it to name the retired verdict word", ptr.Reason)
			}
			if b.judged(1) {
				t.Error("judged(1) = true; want false so the round is re-judged")
			}
			if tt.attached {
				return
			}
			if ptr.Path != "" {
				t.Errorf("Call() pointer = %+v; want an empty Path", ptr)
			}
			for _, path := range judgeOutputs(cfg.RunDir, 1) {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("os.Stat(%s) = %v; want the output archived away", path, err)
				}
			}
		})
	}
}

//testtiming:keep pins that a CIRCLING verdict returns Awaiting naming the round and cause, and that a fresh Bouncer over the same run dir replays it without spawning
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

//testtiming:keep pins that a CIRCLING verdict with a live judge attaches to it, never respawns, and keeps the judge's own next-round focus file
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
