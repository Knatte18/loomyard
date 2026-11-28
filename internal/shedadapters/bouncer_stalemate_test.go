// bouncer_stalemate_test.go covers the Bouncer's stalemate escalation.
// It covers the post-fix target id a judged round records.
// It covers the count of unchanged rounds in a row and the escalation itself.
// It also covers how the escalation yields to the soft-cap rule and how its decisions are settled.

package shedadapters

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	// stalemateFixerReport is a fixer-written report answering the one finding of a stalemate round's review.
	stalemateFixerReport = "---\ndispositions:\n  - id: F1\n    disposition: fixed\n---\n"
	// stalemateEngineReport is the report the engine writes itself when no fixer is spawned.
	stalemateEngineReport = "---\nspawned: false\ndispositions: []\n---\nnothing fixed\n"
	// stalemateIncompleteReport is a fixer-written report that leaves the review's finding unanswered.
	stalemateIncompleteReport = "---\ndispositions: []\n---\n"
)

// stalemateRound is the state a round leaves in the run directory.
// target and post are the ids its snapshot record holds, and report is its fixer-report, stalemateFixerReport when empty.
// noRecord leaves the round's snapshot record out.
type stalemateRound struct {
	target, post string
	report       string
	noRecord     bool
}

// stalemateHarness is a Bouncer over a run directory of rounds a fake judge settles one at a time.
// tree is the id the Bouncer's fake target snapshot answers.
type stalemateHarness struct {
	b       *Bouncer
	cfg     BouncerConfig
	shuttle *shedfake.Shuttle
	tree    string
}

// layOutStalemateRound writes round n's review, fixer-report and snapshot record under cfg.RunDir.
// A round before the judged one also gets its ledger.
func layOutStalemateRound(t *testing.T, cfg BouncerConfig, n int, round stalemateRound, judged bool) {
	t.Helper()
	review := decisionReviewContent(reviewFixtureFinding{"F1", "MEDIUM", "design"})
	for _, path := range []string{filepath.Join(cfg.RunDir, cfg.ReportName(n)), roundReviewPath(cfg.RunDir, n)} {
		if err := os.WriteFile(path, []byte(review), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) = %v; want nil", path, err)
		}
	}
	report := round.report
	if report == "" {
		report = stalemateFixerReport
	}
	if err := os.WriteFile(roundFixerReportPath(cfg.RunDir, n), []byte(report), 0o644); err != nil {
		t.Fatalf("WriteFile(fixer-report round %d) = %v; want nil", n, err)
	}
	if !judged {
		if err := os.WriteFile(ledgerPath(cfg.RunDir, n), []byte(bouncerLedgerContent(n)), 0o644); err != nil {
			t.Fatalf("WriteFile(ledger round %d) = %v; want nil", n, err)
		}
	}
	if round.noRecord {
		return
	}
	routed := false
	err := updateRoundSnapshot(cfg.RunDir, n, func(rec *roundSnapshot, _ bool) {
		*rec = roundSnapshot{FixScope: string(burlerengine.FixScopeSource), TargetID: round.target, PostFixTargetID: round.post, DecisionsRouted: &routed}
	})
	if err != nil {
		t.Fatalf("updateRoundSnapshot(round %d) = %v; want nil", n, err)
	}
}

// newStalemateHarness lays out the earlier rounds and the judged one that follows them, and builds the Bouncer.
// The judged round's record holds lastTarget and no post-fix id; the fake judge writes verdict and ledger(round).
func newStalemateHarness(t *testing.T, stalemateRounds int, earlier []stalemateRound, lastTarget, tree, verdict string, ledger func(round int) string, tweak func(*BouncerConfig)) *stalemateHarness {
	t.Helper()
	h := &stalemateHarness{tree: tree, shuttle: &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}}
	h.shuttle.DuringRun = func() {
		outputs := h.shuttle.GotSpec.OutputFiles
		if len(outputs) != 3 {
			return
		}
		round, _ := strconv.Atoi(h.shuttle.GotSpec.Round)
		_ = os.WriteFile(outputs[0], []byte(bouncerVerdictContent(verdict)), 0o644)
		_ = os.WriteFile(outputs[1], []byte(ledger(round)), 0o644)
		_ = os.WriteFile(outputs[2], []byte(fmt.Sprintf("---\nround: %d\nexclude_lenses: []\nfocus: []\n---\n", round+1)), 0o644)
	}
	h.cfg = newBouncerFixture(t, withNestedRunDir(), withShuttle(h.shuttle), withEscalationInputs()).Config
	h.cfg.CirclingCheckpoint = 2
	h.cfg.StalemateRounds = stalemateRounds
	h.cfg.Snapshot = &TargetSnapshot{Scope: burlerengine.FixScopeSource, SourceTreeID: func() (string, error) { return h.tree, nil }}
	if tweak != nil {
		tweak(&h.cfg)
	}
	rounds := append(slices.Clone(earlier), stalemateRound{target: lastTarget})
	for i, round := range rounds {
		layOutStalemateRound(t, h.cfg, i+1, round, i == len(rounds)-1)
	}
	b, err := NewBouncer(h.cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	h.b = b
	return h
}

// escalationOf returns round's escalation cause and notice, and whether the round holds a record.
func (h *stalemateHarness) escalationOf(t *testing.T, round int) (EscalationCause, string, bool) {
	t.Helper()
	cause, notice, exists, err := readEscalation(h.cfg.RunDir, round)
	if err != nil {
		t.Fatalf("readEscalation(round %d) = %v; want nil", round, err)
	}
	return cause, notice, exists
}

func TestBouncer_Stalemate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	emptyLedger := bouncerLedgerContent
	unchanged := []stalemateRound{{target: "T0", post: "T0"}}

	tests := []struct {
		name            string
		stalemateRounds int
		earlier         []stalemateRound
		lastTarget      string
		tree            string
		// verdict is what the fake judge rules, CONTINUE when empty; ledger is the ledger it writes, an empty one when nil.
		verdict string
		ledger  func(round int) string
		tweak   func(*BouncerConfig)
		// want is the outcome of the one Call.
		// wantCause is the cause of the judged round's escalation record, none when empty.
		want      shedengine.Outcome
		wantCause EscalationCause
		// wantPost is the post-fix target id the judged round's record must hold; noPost asserts it holds none.
		wantPost string
		noPost   bool
		// run replaces the single Call with a scenario of its own.
		run func(t *testing.T, h *stalemateHarness)
	}{
		{name: "SecondUnchangedRoundEscalates", stalemateRounds: 2, earlier: unchanged, lastTarget: "T0", tree: "T0", want: shedengine.Awaiting, wantCause: EscalationStalemate, wantPost: "T0"},
		{name: "OneUnchangedRoundDoesNotEscalate", stalemateRounds: 2, earlier: []stalemateRound{{target: "T0", post: "T9"}}, lastTarget: "T9", tree: "T9", want: shedengine.Stuck},
		{
			name: "ChangedRoundBetweenTwoUnchangedOnesResetsTheCount", stalemateRounds: 2,
			earlier:    []stalemateRound{{target: "T0", post: "T0"}, {target: "T0", post: "T1"}},
			lastTarget: "T1", tree: "T1", want: shedengine.Stuck,
		},
		{name: "ZeroStalemateRoundsNeverEscalates", earlier: unchanged, lastTarget: "T0", tree: "T0", want: shedengine.Stuck},
		{
			name: "NilSnapshotNeverEscalates", stalemateRounds: 2, earlier: unchanged, lastTarget: "T0", tree: "T0", want: shedengine.Stuck, noPost: true,
			tweak: func(c *BouncerConfig) { c.Snapshot = nil },
		},
		{name: "ConvergedVerdictOnAnUnchangedRoundConverges", stalemateRounds: 2, earlier: unchanged, lastTarget: "T0", tree: "T0", verdict: "CONVERGED", want: shedengine.Done},
		{name: "RoundWithNoRecordCountsAsChanged", stalemateRounds: 2, earlier: []stalemateRound{{noRecord: true}}, lastTarget: "T0", tree: "T0", want: shedengine.Stuck},
		{name: "RoundWithNoPostFixIdCountsAsChanged", stalemateRounds: 2, earlier: []stalemateRound{{target: "T0"}}, lastTarget: "T0", tree: "T0", want: shedengine.Stuck},
		{name: "RoundWithNoTargetIdCountsAsChanged", stalemateRounds: 2, earlier: []stalemateRound{{target: "", post: ""}}, lastTarget: "T0", tree: "T0", want: shedengine.Stuck},
		{name: "SkippedFixerCountsAsChanged", stalemateRounds: 2, earlier: []stalemateRound{{target: "T0", post: "T0", report: stalemateEngineReport}}, lastTarget: "T0", tree: "T0", want: shedengine.Stuck},
		{name: "ReportMissingADispositionCountsAsChanged", stalemateRounds: 2, earlier: []stalemateRound{{target: "T0", post: "T0", report: stalemateIncompleteReport}}, lastTarget: "T0", tree: "T0", want: shedengine.Stuck},
		{name: "UnparseableReportCountsAsChanged", stalemateRounds: 2, earlier: []stalemateRound{{target: "T0", post: "T0", report: "not a report"}}, lastTarget: "T0", tree: "T0", want: shedengine.Stuck},
		{
			name: "ReplayAfterTheTargetIdChangesReadsTheRecordedId", stalemateRounds: 2, earlier: unchanged, lastTarget: "T0", tree: "T9",
			run: func(t *testing.T, h *stalemateHarness) {
				shedfake.RequireOutcome(t, h.b, shedengine.Stuck)
				h.tree = "T0"
				shedfake.RequireOutcome(t, h.b, shedengine.Stuck)
				if _, _, exists := h.escalationOf(t, 2); exists {
					t.Error("round 2 holds an escalation record; want the replay to settle as the harvest did")
				}
				if rec, _, err := readRoundSnapshot(h.cfg.RunDir, 2); err != nil || rec.PostFixTargetID != "T9" {
					t.Errorf("round 2's post-fix id = %q, %v; want the recorded T9", rec.PostFixTargetID, err)
				}
			},
		},
		{
			name: "UnchangedRoundAtTheSoftCapWithAutoAcceptIsAutoAccepted", stalemateRounds: 2, earlier: unchanged, lastTarget: "T0", tree: "T0", want: shedengine.Done,
			tweak: func(c *BouncerConfig) { c.AutoAccept, c.Bounces = true, spentBounces },
		},
		{
			name: "UnchangedRoundInTheWindowWithOnlyALowEntryIsAcceptedBySoftCapRule", stalemateRounds: 2, earlier: unchanged, lastTarget: "T0", tree: "T0", want: shedengine.Done,
			ledger: func(round int) string {
				return taggedLedgerContent(round, taggedEntry{key: "K", status: "open", class: "design", severity: "LOW"})
			},
			tweak: func(c *BouncerConfig) { c.Bounces = spentBounces },
		},
		{
			name: "UnchangedRoundInTheWindowWithADesignWrongEntryStillEscalatesStalemate", stalemateRounds: 2, earlier: unchanged, lastTarget: "T0", tree: "T0",
			want: shedengine.Awaiting, wantCause: EscalationStalemate,
			ledger: func(round int) string {
				return taggedLedgerContent(round, taggedEntry{key: "K", status: "open", class: "design", severity: "MEDIUM"})
			},
			tweak: func(c *BouncerConfig) { c.Bounces = spentBounces },
		},
		{
			name: "ReplayOfAStalemateRecordEscalatesAgainWithNoJudge", stalemateRounds: 2, earlier: unchanged, lastTarget: "T0", tree: "T0",
			run: func(t *testing.T, h *stalemateHarness) {
				shedfake.RequireOutcome(t, h.b, shedengine.Awaiting)
				shedfake.RequireOutcome(t, h.b, shedengine.Awaiting)
				off := h.cfg
				off.StalemateRounds = 0
				b, err := NewBouncer(off)
				if err != nil {
					t.Fatalf("NewBouncer(off) = %v; want nil", err)
				}
				shedfake.RequireOutcome(t, b, shedengine.Awaiting)
				if cause, _, exists := h.escalationOf(t, 2); !exists || cause != EscalationStalemate {
					t.Errorf("round 2's escalation = %q, %v; want stalemate", cause, exists)
				}
				requireSpawns(t, h.shuttle, 1)
			},
		},
		{
			name: "RoundWithACirclingRecordIsNotAStalemate", stalemateRounds: 2, earlier: unchanged, lastTarget: "T0", tree: "T0",
			run: func(t *testing.T, h *stalemateHarness) {
				if err := writeEscalation(h.cfg.RunDir, 2, EscalationCircling, "brief\n", "notice"); err != nil {
					t.Fatalf("writeEscalation() = %v; want nil", err)
				}
				shedfake.RequireOutcome(t, h.b, shedengine.Stuck)
				if cause, _, exists := h.escalationOf(t, 2); !exists || cause != EscalationCircling {
					t.Errorf("round 2's escalation = %q, %v; want the circling record kept", cause, exists)
				}
			},
		},
		{
			name: "AfterAContinueTheNextUnchangedRoundAloneDoesNotEscalate", stalemateRounds: 2, earlier: unchanged, lastTarget: "T0", tree: "T0",
			run: func(t *testing.T, h *stalemateHarness) {
				shedfake.RequireOutcome(t, h.b, shedengine.Awaiting)
				if round, cause, err := RecordCirclingDecision(h.cfg.RunDir, CirclingContinue); err != nil || round != 2 || cause != EscalationStalemate {
					t.Fatalf("RecordCirclingDecision(continue) = (%d, %q, %v); want (2, stalemate, nil)", round, cause, err)
				}
				shedfake.RequireOutcome(t, h.b, shedengine.Stuck)
				layOutStalemateRound(t, h.cfg, 3, stalemateRound{target: "T0"}, true)

				shedfake.RequireOutcome(t, h.b, shedengine.Stuck)

				if _, _, exists := h.escalationOf(t, 3); exists {
					t.Error("round 3 holds an escalation record; want the count to restart after the stalemate round")
				}
			},
		},
		{
			name: "RecordedAcceptOnAStalemateRoundReturnsDone", stalemateRounds: 2, earlier: unchanged, lastTarget: "T0", tree: "T0",
			run: func(t *testing.T, h *stalemateHarness) {
				shedfake.RequireOutcome(t, h.b, shedengine.Awaiting)
				if round, cause, err := RecordCirclingDecision(h.cfg.RunDir, CirclingAccept); err != nil || round != 2 || cause != EscalationStalemate {
					t.Fatalf("RecordCirclingDecision(accept) = (%d, %q, %v); want (2, stalemate, nil)", round, cause, err)
				}
				shedfake.RequireOutcome(t, h.b, shedengine.Done)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			verdict, ledger := tt.verdict, tt.ledger
			if verdict == "" {
				verdict = "CONTINUE"
			}
			if ledger == nil {
				ledger = emptyLedger
			}
			h := newStalemateHarness(t, tt.stalemateRounds, tt.earlier, tt.lastTarget, tt.tree, verdict, ledger, tt.tweak)
			if tt.run != nil {
				tt.run(t, h)
				return
			}

			ptr := shedfake.RequireOutcome(t, h.b, tt.want)

			round := len(tt.earlier) + 1
			cause, notice, exists := h.escalationOf(t, round)
			if (tt.wantCause != "") != exists || cause != tt.wantCause {
				t.Fatalf("round %d's escalation = %q, %v; want cause %q", round, cause, exists, tt.wantCause)
			}
			if tt.wantCause == EscalationStalemate {
				for _, want := range []string{"lym loom circling accept my-task", "lym loom circling continue my-task"} {
					if !strings.Contains(notice, want) || !strings.Contains(ptr.ParentNotice, want) {
						t.Errorf("notice = %q, ParentNotice = %q; want both to name %q", notice, ptr.ParentNotice, want)
					}
				}
			}
			if rec, _, err := readRoundSnapshot(h.cfg.RunDir, round); err != nil || (tt.wantPost != "" && rec.PostFixTargetID != tt.wantPost) || (tt.noPost && rec.PostFixTargetID != "") {
				t.Errorf("round %d's record = %+v, %v; want post-fix id %q (none: %v)", round, rec, err, tt.wantPost, tt.noPost)
			}
		})
	}
}
