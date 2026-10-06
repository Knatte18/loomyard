// bouncer_circling_test.go covers the Bouncer's handling of a CIRCLING round: the Awaiting Reason it writes for the parent and the way it acts on the recorded decision.

package shedadapters

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// circlingSeams counts the Approve and Commit calls a test's Bouncer makes.
type circlingSeams struct {
	order     []string
	commitErr error
}

func (s *circlingSeams) install(cfg *BouncerConfig) {
	cfg.Approve = func() error { s.order = append(s.order, "approve"); return nil }
	cfg.Commit = func() error { s.order = append(s.order, "commit"); return s.commitErr }
}

// circlingBouncer builds a Bouncer over a run directory whose round 2 is judged CIRCLING over a gating key open in rounds 1 and 2.
// The shuttle is a fake that fails the test if a Run is attempted.
func circlingBouncer(t *testing.T, tweak func(*BouncerConfig)) (*Bouncer, *BouncerConfig, *shedfake.Shuttle) {
	t.Helper()
	return layoutCirclingBouncer(t, 2, 2, tweak, func(dir string) {
		gating := evidenceEntry{"alpha", "open", "design", "MEDIUM"}
		writeEvidenceLedger(t, dir, 1, gating)
		writeEvidenceLedger(t, dir, 2, gating)
	})
}

// layoutCirclingBouncer builds a Bouncer over a run directory whose rounds 1..round are judged, the last CIRCLING and the earlier ones CONTINUE.
// writeLedgers then replaces the placeholder ledgers with the test's own.
// The shuttle is a fake that fails the test if a Run is attempted.
func layoutCirclingBouncer(t *testing.T, checkpoint, round int, tweak func(*BouncerConfig), writeLedgers func(dir string)) (*Bouncer, *BouncerConfig, *shedfake.Shuttle) {
	t.Helper()
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	cfg := newBouncerFixture(t, withNestedRunDir(), withShuttle(shuttle)).Config
	cfg.CirclingCheckpoint = checkpoint
	if tweak != nil {
		tweak(&cfg)
	}
	var fixtures []bouncerJudgeFixture
	for n := 1; n <= round; n++ {
		verdict := "CONTINUE"
		if n == round {
			verdict = "CIRCLING"
		}
		fixtures = append(fixtures, bouncerJudgeFixture{
			round: n, report: bouncerReport(n), verdict: bouncerVerdictContent(verdict), ledger: bouncerLedgerContent(n),
		})
	}
	layoutBouncerRun(t, cfg, fixtures)
	writeLedgers(cfg.RunDir)
	// RecordCirclingDecision resolves the latest round from the review file, not the fixture's report name.
	for n := 1; n <= round; n++ {
		if err := os.WriteFile(roundReviewPath(cfg.RunDir, n), []byte("review\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(review round %d) = %v; want nil", n, err)
		}
	}
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	return b, &cfg, shuttle
}

func requireReasonContains(t *testing.T, reason string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(reason, want) {
			t.Errorf("Reason = %q; want it to contain %q", reason, want)
		}
	}
}

//testtiming:keep pins the awaiting reason a circling round with no decision writes: both verbs, each with the slug argument or without it
func TestBouncer_Circling_NoDecisionAwaitsNamingBothVerbs(t *testing.T) {
	tests := []struct {
		name      string
		slug      string
		wantReasn []string
	}{
		{
			name:      "a slug is named in both verbs",
			slug:      "my-task",
			wantReasn: []string{"round 2", "lyx loom circling accept my-task", "lyx loom circling continue my-task", "lyx loom start"},
		},
		{
			name:      "an empty slug omits the argument",
			wantReasn: []string{"`lyx loom circling accept`", "`lyx loom circling continue`"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, cfg, shuttle := circlingBouncer(t, func(c *BouncerConfig) { c.Slug = tt.slug })

			ptr := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
			if want := ledgerPath(cfg.RunDir, 2); ptr.Path != want {
				t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
			}
			requireReasonContains(t, ptr.Reason, tt.wantReasn...)
			if shuttle.Called {
				t.Error("Call() spawned a run; want none")
			}
		})
	}
}

// TestBouncer_Circling_RecordedDecision covers the way a recorded decision settles a CIRCLING
// round: continue returns Stuck, runs no seam and writes the next round's focus file; accept
// settles, approves and commits, returns Done, and the next Call clears the settled generation.
//
//testtiming:keep pins settling a decision recorded on a CIRCLING round that never wrote an escalation record, which the escalation tests never reach
func TestBouncer_Circling_RecordedDecision(t *testing.T) {
	tests := []struct {
		name     string
		decision CirclingDecision
	}{
		{"continue returns Stuck and runs no seam", CirclingContinue},
		{"accept settles, approves, commits then clears", CirclingAccept},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seams := &circlingSeams{}
			b, cfg, shuttle := circlingBouncer(t, seams.install)
			if _, _, err := RecordCirclingDecision(cfg.RunDir, tt.decision); err != nil {
				t.Fatalf("RecordCirclingDecision(%s) = %v; want nil", tt.decision, err)
			}

			if tt.decision == CirclingContinue {
				ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
				if want := ledgerPath(cfg.RunDir, 2); ptr.Path != want {
					t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
				}
				if len(seams.order) != 0 {
					t.Errorf("seams ran %v; want none", seams.order)
				}
				if shuttle.Called {
					t.Error("Call() spawned a run; want none")
				}
				if _, err := os.Stat(focusPath(cfg.RunDir, 3)); err != nil {
					t.Errorf("round-3 focus file = %v; want ensureFocus to have written it", err)
				}
				return
			}

			ptr := shedfake.RequireOutcome(t, b, shedengine.Done)
			if want := ledgerPath(cfg.RunDir, 2); ptr.Path != want {
				t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
			}
			if got := strings.Join(seams.order, ","); got != "approve,commit" {
				t.Errorf("seam order = %q; want approve,commit", got)
			}
			if _, _, settled, _, err := readCirclingDecision(cfg.RunDir, 2); err != nil || !settled {
				t.Errorf("readCirclingDecision settled = %v, err = %v; want settled true", settled, err)
			}

			shedfake.RequireOutcome(t, b, shedengine.Stuck)
			if _, err := os.Stat(archivedRunDirPath(cfg.RunDir, bouncerJudgeTestClock, "")); err != nil {
				t.Errorf("expected the next Call to archive the settled generation: %v", err)
			}
			if len(seams.order) != 2 {
				t.Errorf("seams ran %v; want approve and commit once each", seams.order)
			}
		})
	}
}

func TestBouncer_Circling_AcceptWithFailedCommitResumesByClearingWithoutApprove(t *testing.T) {
	seams := &circlingSeams{commitErr: errors.New("commit refused")}
	b, cfg, _ := circlingBouncer(t, seams.install)
	if _, _, err := RecordCirclingDecision(cfg.RunDir, CirclingAccept); err != nil {
		t.Fatalf("RecordCirclingDecision(accept) = %v; want nil", err)
	}

	if _, _, err := b.Call(t.Context()); err == nil || !strings.Contains(err.Error(), "commit refused") {
		t.Fatalf("Call() error = %v; want the commit failure", err)
	}
	if _, _, settled, _, err := readCirclingDecision(cfg.RunDir, 2); err != nil || !settled {
		t.Errorf("readCirclingDecision settled = %v, err = %v; want the accept already settled", settled, err)
	}

	shedfake.RequireOutcome(t, b, shedengine.Stuck)
	if got := strings.Join(seams.order, ","); got != "approve,commit" {
		t.Errorf("seam order = %q; want no second Approve on the resume", got)
	}
	if _, err := os.Stat(archivedRunDirPath(cfg.RunDir, bouncerJudgeTestClock, "")); err != nil {
		t.Errorf("expected the resumed Call to archive the settled generation: %v", err)
	}
}

func TestBouncer_Circling_SettledAcceptReachedInSettleDegrades(t *testing.T) {
	seams := &circlingSeams{}
	b, cfg, _ := circlingBouncer(t, seams.install)
	if _, _, err := RecordCirclingDecision(cfg.RunDir, CirclingAccept); err != nil {
		t.Fatalf("RecordCirclingDecision(accept) = %v; want nil", err)
	}
	if err := settleCirclingAccept(cfg.RunDir, 2); err != nil {
		t.Fatalf("settleCirclingAccept = %v; want nil", err)
	}

	outcome, ptr, err := b.settle(t.Context(), 2, true)
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("settle(...) = (%q, %v); want Stuck and nil", outcome, err)
	}
	requireReasonContains(t, ptr.Reason, "already settled", "way forward:", "lyx loom start")
	if len(seams.order) != 0 {
		t.Errorf("seams ran %v; want none", seams.order)
	}
}

func TestBouncer_Circling_MalformedDecisionReturnsStuckAndRunsNoSeam(t *testing.T) {
	seams := &circlingSeams{}
	b, cfg, _ := circlingBouncer(t, seams.install)
	if err := os.WriteFile(circlingDecisionPath(cfg.RunDir, 2), []byte("not a decision"), 0o644); err != nil {
		t.Fatalf("WriteFile(decision) = %v; want nil", err)
	}

	ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
	requireReasonContains(t, ptr.Reason, "circling decision", "way forward:", circlingDecisionPath(cfg.RunDir, 2), "lyx loom start")
	if len(seams.order) != 0 {
		t.Errorf("seams ran %v; want none", seams.order)
	}
}

func TestBouncer_CirclingGuard(t *testing.T) {
	gating := func(key string) evidenceEntry { return evidenceEntry{key, "open", "design", "MEDIUM"} }

	tests := []struct {
		name       string
		checkpoint int
		round      int
		ledgers    func(t *testing.T, dir string)
		want       shedengine.Outcome
	}{
		{
			name:       "below the checkpoint reads as CONTINUE even over recurring evidence",
			checkpoint: 3, round: 2,
			ledgers: func(t *testing.T, dir string) {
				writeEvidenceLedger(t, dir, 1, gating("alpha"))
				writeEvidenceLedger(t, dir, 2, gating("alpha"))
			},
			want: shedengine.Stuck,
		},
		{
			name:       "at the checkpoint with no recurring open key reads as CONTINUE",
			checkpoint: 2, round: 2,
			ledgers: func(t *testing.T, dir string) {
				writeEvidenceLedger(t, dir, 1, gating("alpha"))
				writeEvidenceLedger(t, dir, 2, gating("beta"))
			},
			want: shedengine.Stuck,
		},
		{
			name:       "a gating key open in rounds 1 and 3 reaches Awaiting",
			checkpoint: 3, round: 3,
			ledgers: func(t *testing.T, dir string) {
				writeEvidenceLedger(t, dir, 1, gating("alpha"))
				writeEvidenceLedger(t, dir, 2, evidenceEntry{"alpha", "resolved", "", ""})
				writeEvidenceLedger(t, dir, 3, gating("alpha"))
			},
			want: shedengine.Awaiting,
		},
		{
			name:       "a non-gating recurring key alone reads as CONTINUE",
			checkpoint: 2, round: 2,
			ledgers: func(t *testing.T, dir string) {
				low := evidenceEntry{"alpha", "open", "scope", "LOW"}
				writeEvidenceLedger(t, dir, 1, low)
				writeEvidenceLedger(t, dir, 2, low)
			},
			want: shedengine.Stuck,
		},
		{
			name:       "a rising count over new keys reads as CONTINUE",
			checkpoint: 2, round: 2,
			ledgers: func(t *testing.T, dir string) {
				writeEvidenceLedger(t, dir, 1, gating("alpha"))
				writeEvidenceLedger(t, dir, 2, gating("beta"), gating("gamma"), gating("delta"))
			},
			want: shedengine.Stuck,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, cfg, shuttle := layoutCirclingBouncer(t, tt.checkpoint, tt.round, nil, func(dir string) { tt.ledgers(t, dir) })

			ptr := shedfake.RequireOutcome(t, b, tt.want)
			if want := ledgerPath(cfg.RunDir, tt.round); ptr.Path != want {
				t.Errorf("Call() pointer = %q; want %q", ptr.Path, want)
			}
			if shuttle.Called {
				t.Error("Call() spawned a run; want none")
			}
			_, _, _, exists, err := readCirclingDecision(cfg.RunDir, tt.round)
			if err != nil || exists {
				t.Errorf("readCirclingDecision exists = %v, err = %v; want the guard to record no decision", exists, err)
			}
			if tt.want == shedengine.Stuck {
				if _, err := os.Stat(focusPath(cfg.RunDir, tt.round+1)); err != nil {
					t.Errorf("next-round focus file = %v; want ensureFocus to have written it", err)
				}
			}
		})
	}
}
