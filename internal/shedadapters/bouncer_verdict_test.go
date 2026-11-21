// bouncer_verdict_test.go covers the Bouncer's verdict vocabulary.
// It tests the strict parse and the outcome each of the three verdicts maps to.

package shedadapters

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestParseVerdict_Vocabulary covers the strict parse, which accepts only the three current words.
// A retired word, a wrong-case word and an unknown word are each refused.
func TestParseVerdict_Vocabulary(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		word string
		// accepted reports whether the parse accepts the word.
		accepted bool
	}{
		{word: "CONVERGED", accepted: true},
		{word: "CONTINUE", accepted: true},
		{word: "CIRCLING", accepted: true},
		{word: "APPROVED"},
		{word: "BLOCKING"},
		{word: "converged"},
		{word: "MAYBE"},
	}
	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			t.Parallel()
			got, _, err := parseVerdict([]byte(bouncerVerdictContent(tt.word)))
			if (err == nil) != tt.accepted {
				t.Errorf("parseVerdict(%q) error = %v; want accepted = %v", tt.word, err, tt.accepted)
			}
			if tt.accepted && got != bouncerVerdict(tt.word) {
				t.Errorf("parseVerdict(%q) = %q; want %q", tt.word, got, tt.word)
			}
		})
	}
}

//testtiming:keep pins that a CIRCLING verdict returns Awaiting naming the round and cause, and that a fresh Bouncer over the same run dir replays it without spawning
func TestBouncer_Verdict_CirclingReturnsAwaitingAndReplaysWithoutSpawning(t *testing.T) {
	tiergate.Require(t, 0)
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
	tiergate.Require(t, 0)
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
