// anomaly_test.go is the TDD driver for DetectCrashResume and DetectAnomalies: table tests over
// hand-built shedengine.Status values and hand-built observations, asserting the exact set of
// detected anomalies, their pinned order, and title stability/distinctness. It is untagged
// (Tier 1): no spawn, no git, no filesystem I/O -- the detector is pure.

package loomengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// baseEntry returns a fresh, non-anomalous EntryObservation baseline for testing.
func baseEntry() EntryObservation {
	return EntryObservation{
		Observed:        true,
		RunLockHeld:     false,
		State:           shedengine.StateDone,
		CurrentProducer: "Loom-Preflight",
		HistoryLength:   1,
		Slug:            "loom-contracts",
		Parent:          "main",
	}
}

// baseFinal returns a fresh, non-anomalous shedengine.Status baseline for testing.
func baseFinal() shedengine.Status {
	return shedengine.Status{
		CurrentProducer: "Loom-Preflight",
		State:           shedengine.StateDone,
		History: []shedengine.HistoryEntry{
			{Producer: "Loom-Preflight", Outcome: shedengine.Done, At: "2026-07-17T10:01:30Z"},
		},
	}
}

// baseProduct returns a fresh loom Status product baseline for testing.
func baseProduct() Status {
	return Status{
		Slug:   "loom-contracts",
		Parent: "main",
	}
}

func TestDetectCrashResume(t *testing.T) {
	tests := []struct {
		name  string
		entry EntryObservation
		want  bool
	}{
		{
			name: "RunningWithNonEmptyHistory_IsCrashResume",
			entry: EntryObservation{
				Observed:      true,
				RunLockHeld:   false,
				State:         shedengine.StateRunning,
				HistoryLength: 3,
			},
			want: true,
		},
		{
			name: "RunningWithEmptyHistory_None",
			entry: EntryObservation{
				Observed:      true,
				RunLockHeld:   false,
				State:         shedengine.StateRunning,
				HistoryLength: 0,
			},
			want: false,
		},
		{
			name: "Paused_None",
			entry: EntryObservation{
				Observed:      true,
				RunLockHeld:   false,
				State:         shedengine.StatePaused,
				HistoryLength: 3,
			},
			want: false,
		},
		{
			name: "Blocked_None",
			entry: EntryObservation{
				Observed:      true,
				RunLockHeld:   false,
				State:         shedengine.StateBlocked,
				HistoryLength: 3,
			},
			want: false,
		},
		{
			name: "Failed_None",
			entry: EntryObservation{
				Observed:      true,
				RunLockHeld:   false,
				State:         shedengine.StateFailed,
				HistoryLength: 3,
			},
			want: false,
		},
		{
			name: "RunningWithHistory_LockHeld_None",
			entry: EntryObservation{
				Observed:      true,
				RunLockHeld:   true,
				State:         shedengine.StateRunning,
				HistoryLength: 3,
			},
			want: false,
		},
		{
			name: "NotObserved_None",
			entry: EntryObservation{
				Observed:      false,
				RunLockHeld:   false,
				State:         shedengine.StateRunning,
				HistoryLength: 3,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := DetectCrashResume(tt.entry)
			if got != tt.want {
				t.Errorf("DetectCrashResume(%+v) ok = %v; want %v", tt.entry, got, tt.want)
			}
		})
	}
}

func TestDetectAnomalies(t *testing.T) {
	tests := []struct {
		name    string
		entry   EntryObservation
		final   shedengine.Status
		product Status
		ledgers []LedgerObservation
		want    []AnomalyKind
	}{
		{
			name:    "CleanDoneRun_NoLedgers_Empty",
			entry:   baseEntry(),
			final:   baseFinal(),
			product: baseProduct(),
			want:    nil,
		},
		{
			name: "BlockedEscalation",
			entry: func() EntryObservation {
				e := baseEntry()
				e.State = shedengine.StateBlocked
				return e
			}(),
			final: func() shedengine.Status {
				f := baseFinal()
				f.State = shedengine.StateBlocked
				f.Error = errorTextEscalation
				return f
			}(),
			product: baseProduct(),
			want:    []AnomalyKind{AnomalyEscalation},
		},
		{
			name: "BlockedBudgetExhausted",
			entry: func() EntryObservation {
				e := baseEntry()
				e.State = shedengine.StateBlocked
				return e
			}(),
			final: func() shedengine.Status {
				f := baseFinal()
				f.State = shedengine.StateBlocked
				f.Error = errorTextBudgetExhausted
				return f
			}(),
			product: baseProduct(),
			want:    []AnomalyKind{AnomalyBudgetExhausted},
		},
		{
			name: "BlockedOtherErrorText_None",
			entry: func() EntryObservation {
				e := baseEntry()
				e.State = shedengine.StateBlocked
				return e
			}(),
			final: func() shedengine.Status {
				f := baseFinal()
				f.State = shedengine.StateBlocked
				f.Error = "some other reason"
				return f
			}(),
			product: baseProduct(),
			want:    nil,
		},
		{
			name: "FailedArbitraryError_ProducerHardFailure",
			entry: func() EntryObservation {
				e := baseEntry()
				e.State = shedengine.StateFailed
				return e
			}(),
			final: func() shedengine.Status {
				f := baseFinal()
				f.State = shedengine.StateFailed
				f.Error = "arbitrary failure text"
				return f
			}(),
			product: baseProduct(),
			want:    []AnomalyKind{AnomalyProducerFailure},
		},
		{
			name:    "LedgerOpenThreeRounds_RecurringFinding",
			entry:   baseEntry(),
			final:   baseFinal(),
			product: baseProduct(),
			ledgers: []LedgerObservation{
				{Producer: "Discussion-Review", Key: "finding-a", Rounds: []int{1, 2, 3}, Status: "open"},
			},
			want: []AnomalyKind{AnomalyRecurringFinding},
		},
		{
			name:    "LedgerOpenTwoRounds_None",
			entry:   baseEntry(),
			final:   baseFinal(),
			product: baseProduct(),
			ledgers: []LedgerObservation{
				{Producer: "Discussion-Review", Key: "finding-a", Rounds: []int{1, 2}, Status: "open"},
			},
			want: nil,
		},
		{
			name:    "LedgerOpenFiveRounds_OneNotThree",
			entry:   baseEntry(),
			final:   baseFinal(),
			product: baseProduct(),
			ledgers: []LedgerObservation{
				{Producer: "Discussion-Review", Key: "finding-a", Rounds: []int{1, 2, 3, 4, 5}, Status: "open"},
			},
			want: []AnomalyKind{AnomalyRecurringFinding},
		},
		{
			name:    "LedgerResolvedLongRounds_None",
			entry:   baseEntry(),
			final:   baseFinal(),
			product: baseProduct(),
			ledgers: []LedgerObservation{
				{Producer: "Discussion-Review", Key: "finding-a", Rounds: []int{1, 2, 3, 4, 5}, Status: "resolved"},
			},
			want: nil,
		},
		{
			name: "MultipleAnomalies_PinnedOrder",
			entry: func() EntryObservation {
				e := baseEntry()
				e.State = shedengine.StateRunning
				e.HistoryLength = 2
				return e
			}(),
			final: func() shedengine.Status {
				f := baseFinal()
				f.State = shedengine.StateFailed
				f.Error = "arbitrary failure text"
				return f
			}(),
			product: baseProduct(),
			ledgers: []LedgerObservation{
				{Producer: "Discussion-Review", Key: "finding-a", Rounds: []int{1, 2, 3}, Status: "open"},
				{Producer: "Plan-Review", Key: "finding-b", Rounds: []int{1, 2, 3, 4}, Status: "open"},
			},
			want: []AnomalyKind{AnomalyCrashResume, AnomalyProducerFailure, AnomalyRecurringFinding, AnomalyRecurringFinding},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectAnomalies(tt.entry, tt.final, tt.product, tt.ledgers)
			if len(got) != len(tt.want) {
				t.Fatalf("DetectAnomalies() = %d anomalies %+v; want %d %+v", len(got), got, len(tt.want), tt.want)
			}
			for i, a := range got {
				if a.Kind != tt.want[i] {
					t.Errorf("DetectAnomalies()[%d].Kind = %q; want %q", i, a.Kind, tt.want[i])
				}
			}
		})
	}
}

// TestTitleStability_HaltKindSurvivesResumeAppend asserts that a blocked halt's title is
// byte-identical before and after a resume of the same unresolved halt appends one more stuck
// entry for the same producer -- a length-based discriminator would fail exactly this case.
func TestTitleStability_HaltKindSurvivesResumeAppend(t *testing.T) {
	product := baseProduct()
	first := shedengine.Status{
		CurrentProducer: "Discussion-Review",
		State:           shedengine.StateBlocked,
		Error:           errorTextEscalation,
		History: []shedengine.HistoryEntry{
			{Producer: "Discussion-Review", Outcome: shedengine.Stuck, At: "2026-07-17T10:01:30Z"},
		},
	}
	resumed := shedengine.Status{
		CurrentProducer: "Discussion-Review",
		State:           shedengine.StateBlocked,
		Error:           errorTextEscalation,
		History: append(append([]shedengine.HistoryEntry{}, first.History...), shedengine.HistoryEntry{
			Producer: "Discussion-Review", Outcome: shedengine.Stuck, At: "2026-07-17T10:05:00Z",
		}),
	}

	firstAnomalies := DetectAnomalies(EntryObservation{}, first, product, nil)
	resumedAnomalies := DetectAnomalies(EntryObservation{}, resumed, product, nil)

	if len(firstAnomalies) != 1 || len(resumedAnomalies) != 1 {
		t.Fatalf("expected exactly one anomaly each: first=%+v resumed=%+v", firstAnomalies, resumedAnomalies)
	}
	if firstAnomalies[0].Title != resumedAnomalies[0].Title {
		t.Errorf("titles differ across a resume append: %q vs %q", firstAnomalies[0].Title, resumedAnomalies[0].Title)
	}
}

// TestTitleDistinctness_HaltKind asserts two escalations at different producers yield different
// titles, and an escalation at a producer with one prior done entry differs from one at a
// producer with none.
func TestTitleDistinctness_HaltKind(t *testing.T) {
	product := baseProduct()

	atProducerA := shedengine.Status{
		CurrentProducer: "Discussion-Review",
		State:           shedengine.StateBlocked,
		Error:           errorTextEscalation,
	}
	atProducerB := shedengine.Status{
		CurrentProducer: "Plan-Review",
		State:           shedengine.StateBlocked,
		Error:           errorTextEscalation,
	}
	aAnomalies := DetectAnomalies(EntryObservation{}, atProducerA, product, nil)
	bAnomalies := DetectAnomalies(EntryObservation{}, atProducerB, product, nil)
	if aAnomalies[0].Title == bAnomalies[0].Title {
		t.Errorf("expected distinct titles at different producers, got same: %q", aAnomalies[0].Title)
	}

	noPriorDone := shedengine.Status{
		CurrentProducer: "Discussion-Review",
		State:           shedengine.StateBlocked,
		Error:           errorTextEscalation,
	}
	onePriorDone := shedengine.Status{
		CurrentProducer: "Discussion-Review",
		State:           shedengine.StateBlocked,
		Error:           errorTextEscalation,
		History: []shedengine.HistoryEntry{
			{Producer: "Discussion-Review", Outcome: shedengine.Done, At: "2026-07-17T09:00:00Z"},
		},
	}
	noDoneAnomalies := DetectAnomalies(EntryObservation{}, noPriorDone, product, nil)
	oneDoneAnomalies := DetectAnomalies(EntryObservation{}, onePriorDone, product, nil)
	if noDoneAnomalies[0].Title == oneDoneAnomalies[0].Title {
		t.Errorf("expected distinct titles at different done counts, got same: %q", noDoneAnomalies[0].Title)
	}
}

// TestTitleDistinctness_Trigger5 asserts two ledger observations carrying the same key but
// different Producer values yield two distinct titles, and the same producer-and-key pair
// recurring after a resolved round yields the same title, asserted as intended identity-dedupe
// behaviour rather than a bug.
func TestTitleDistinctness_Trigger5(t *testing.T) {
	product := baseProduct()

	ledgerA := LedgerObservation{Producer: "Discussion-Review", Key: "finding-x", Rounds: []int{1, 2, 3}, Status: "open"}
	ledgerB := LedgerObservation{Producer: "Plan-Review", Key: "finding-x", Rounds: []int{1, 2, 3}, Status: "open"}
	anomaliesA := DetectAnomalies(EntryObservation{}, shedengine.Status{}, product, []LedgerObservation{ledgerA})
	anomaliesB := DetectAnomalies(EntryObservation{}, shedengine.Status{}, product, []LedgerObservation{ledgerB})
	if anomaliesA[0].Title == anomaliesB[0].Title {
		t.Errorf("expected distinct titles for same key, different producer, got same: %q", anomaliesA[0].Title)
	}

	recurringSame := LedgerObservation{Producer: "Discussion-Review", Key: "finding-x", Rounds: []int{4, 5, 6}, Status: "open"}
	anomaliesRecurring := DetectAnomalies(EntryObservation{}, shedengine.Status{}, product, []LedgerObservation{recurringSame})
	if anomaliesA[0].Title != anomaliesRecurring[0].Title {
		t.Errorf("expected identical titles for same producer+key recurring after resolve, got %q vs %q", anomaliesA[0].Title, anomaliesRecurring[0].Title)
	}
}

// TestDetectAnomalies_Deterministic asserts the same input detected twice yields byte-identical
// titles.
func TestDetectAnomalies_Deterministic(t *testing.T) {
	entry := baseEntry()
	entry.State = shedengine.StateRunning
	entry.HistoryLength = 2
	final := baseFinal()
	final.State = shedengine.StateFailed
	final.Error = "boom"
	product := baseProduct()
	ledgers := []LedgerObservation{
		{Producer: "Discussion-Review", Key: "finding-a", Rounds: []int{1, 2, 3}, Status: "open"},
	}

	first := DetectAnomalies(entry, final, product, ledgers)
	second := DetectAnomalies(entry, final, product, ledgers)

	if len(first) != len(second) {
		t.Fatalf("non-deterministic anomaly count: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Title != second[i].Title {
			t.Errorf("non-deterministic title at index %d: %q vs %q", i, first[i].Title, second[i].Title)
		}
	}
}
