// classify_test.go tests ParseStateChange and ClassifyRecords over hand-built batten records.

package battenshed

import (
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// minuteText returns the RFC 3339 text of minute n of a fixed hour.
func minuteText(n int) string {
	return time.Date(2026, 1, 1, 0, n, 0, 0, time.UTC).Format(time.RFC3339)
}

// changeEntry is a Run-Shed budget-exempt history entry recording text at minute n.
func changeEntry(n int, text string) shedengine.HistoryEntry {
	return shedengine.HistoryEntry{Producer: runShedRowName, Outcome: shedengine.Stuck, Output: text, At: minuteText(n), BudgetExempt: true}
}

// childEntry is a child history entry of producer with outcome at minute n.
func childEntry(n int, producer string, outcome shedengine.Outcome) shedengine.HistoryEntry {
	return shedengine.HistoryEntry{Producer: producer, Outcome: outcome, At: minuteText(n)}
}

// changesRecord is a record whose one snapshot holds the given change entries and whose child history is child.
func changesRecord(child []shedengine.HistoryEntry, changes ...shedengine.HistoryEntry) BattenRecord {
	return BattenRecord{Snapshots: []shedengine.Status{{History: changes}}, Child: child}
}

// rowSnapshot is a batten snapshot at row in state, with history.
func rowSnapshot(state shedengine.State, row string, history ...shedengine.HistoryEntry) shedengine.Status {
	return shedengine.Status{State: state, CurrentProducer: row, History: history}
}

// wantWake is the Wake of kind at producer for minute n.
func wantWake(kind, producer string, n int) Wake {
	return Wake{Kind: kind, Producer: producer, At: parseEntryTime(minuteText(n))}
}

func TestClassifyRecords(t *testing.T) {
	t.Parallel()
	rules := NoticeRules{
		VerifyRows:  []string{"Merge-In", "Publish"},
		GateRow:     "PR-Gate",
		EpisodeRows: []string{"Land", "Worktree-Teardown"},
	}
	escalating := rules
	escalating.Escalations = map[string]int{"Plan-Bouncer": 1}
	retryRecord := func(blocks ...shedengine.HistoryEntry) BattenRecord {
		return changesRecord([]shedengine.HistoryEntry{childEntry(2, "Merge-In", shedengine.OutcomeGoto)}, blocks...)
	}

	tests := []struct {
		name          string
		rules         NoticeRules
		record        BattenRecord
		wantWakes     []Wake
		wantAutomatic []AutomaticStep
		wantPaused    int
		wantAccounted []int
	}{
		{
			name:      "a halt at a review row with an escalation file is a review-escalation wake",
			rules:     escalating,
			record:    changesRecord(nil, changeEntry(1, "child running → awaiting at Plan-Bouncer")),
			wantWakes: []Wake{wantWake(WakeKindReviewEscalation, "Plan-Bouncer", 1)},
		},
		{
			name:      "a halt at a review row with no escalation file keeps the child's state as its kind",
			rules:     rules,
			record:    changesRecord(nil, changeEntry(1, "child running → awaiting at Plan-Bouncer")),
			wantWakes: []Wake{wantWake("awaiting", "Plan-Bouncer", 1)},
		},
		{
			name:      "an awaiting at the gate with no later reroute is a pr-gate wake",
			rules:     rules,
			record:    changesRecord([]shedengine.HistoryEntry{childEntry(0, "PR-Gate", shedengine.Awaiting)}, changeEntry(1, "child running → awaiting at PR-Gate")),
			wantWakes: []Wake{wantWake(WakeKindPRGate, "PR-Gate", 1)},
		},
		{
			name:  "an awaiting at the gate followed by a gate reroute is a moved-parent resume",
			rules: rules,
			record: changesRecord(
				[]shedengine.HistoryEntry{childEntry(0, "PR-Gate", shedengine.Awaiting), childEntry(2, "PR-Gate", shedengine.OutcomeReroute)},
				changeEntry(1, "child running → awaiting at PR-Gate"),
			),
			wantAutomatic: []AutomaticStep{{Kind: AutomaticKindParentMovedResume, Producer: "PR-Gate", At: parseEntryTime(minuteText(1))}},
			wantAccounted: []int{1},
		},
		{
			name:  "a first blocked at a verify row followed by a goto is a verify retry and its pause is no wake",
			rules: rules,
			record: retryRecord(
				changeEntry(1, "child running → blocked at Publish"),
				changeEntry(3, "child blocked → paused at Merge-In"),
				changeEntry(4, "child paused → running at Merge-In"),
			),
			wantAutomatic: []AutomaticStep{{Kind: AutomaticKindVerifyRetry, Producer: "Publish", At: parseEntryTime(minuteText(1))}},
			wantAccounted: []int{0},
		},
		{
			name:  "a second blocked at the same verify row is a wake",
			rules: rules,
			record: retryRecord(
				changeEntry(1, "child running → blocked at Merge-In"),
				changeEntry(3, "child blocked → paused at Merge-In"),
				changeEntry(4, "child paused → running at Merge-In"),
				changeEntry(6, "child running → blocked at Merge-In"),
			),
			wantWakes:     []Wake{wantWake("blocked", "Merge-In", 6)},
			wantAutomatic: []AutomaticStep{{Kind: AutomaticKindVerifyRetry, Producer: "Merge-In", At: parseEntryTime(minuteText(1))}},
			wantAccounted: []int{0},
		},
		{
			name:      "a blocked at a row that is no verify row is a wake",
			rules:     rules,
			record:    changesRecord(nil, changeEntry(1, "child running → blocked at Plan-Write")),
			wantWakes: []Wake{wantWake("blocked", "Plan-Write", 1)},
		},
		{
			name:       "a change into paused is recorded and is no wake",
			rules:      rules,
			record:     changesRecord(nil, changeEntry(1, "child running → paused at Plan-Write")),
			wantPaused: 1,
		},
		{
			name:  "a failure episode of a batten row is one wake however many failures it spans",
			rules: rules,
			record: BattenRecord{Snapshots: []shedengine.Status{
				rowSnapshot(shedengine.StateFailed, "Land", childEntry(5, "Land", shedengine.Stuck)),
				rowSnapshot(shedengine.StateFailed, "Land", childEntry(5, "Land", shedengine.Stuck), childEntry(6, "Land", shedengine.Stuck)),
				rowSnapshot(shedengine.StateDone, "Land", childEntry(5, "Land", shedengine.Stuck), childEntry(6, "Land", shedengine.Stuck), childEntry(7, "Land", shedengine.Done)),
			}},
			wantWakes: []Wake{wantWake(WakeKindFailed, "Land", 5)},
		},
		{
			name:  "episodes of two rows are one wake each, a done closing the first",
			rules: rules,
			record: BattenRecord{Snapshots: []shedengine.Status{
				rowSnapshot(shedengine.StateFailed, "Worktree-Teardown", childEntry(1, "Worktree-Teardown", shedengine.Stuck)),
				rowSnapshot(shedengine.StateFailed, "Worktree-Teardown", childEntry(1, "Worktree-Teardown", shedengine.Stuck), childEntry(2, "Worktree-Teardown", shedengine.Stuck)),
				rowSnapshot(shedengine.StateRunning, "Land", childEntry(1, "Worktree-Teardown", shedengine.Stuck), childEntry(3, "Worktree-Teardown", shedengine.Done)),
				rowSnapshot(shedengine.StateFailed, "Land", childEntry(3, "Worktree-Teardown", shedengine.Done), childEntry(4, "Land", shedengine.Stuck)),
			}},
			wantWakes: []Wake{wantWake(WakeKindFailed, "Land", 4), wantWake(WakeKindFailed, "Worktree-Teardown", 1)},
		},
		{
			name:   "history entries that are no recorded state change are ignored",
			rules:  rules,
			record: changesRecord(nil, changeEntry(1, "batten waiting"), shedengine.HistoryEntry{Producer: runShedRowName, Output: "child running → blocked at Plan-Write", At: minuteText(2)}),
		},
		{
			name:   "no snapshots classify to nothing",
			rules:  rules,
			record: BattenRecord{Child: []shedengine.HistoryEntry{childEntry(1, "PR-Gate", shedengine.OutcomeReroute)}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ClassifyRecords(tt.record, tt.rules)
			if !slices.Equal(got.Wakes, tt.wantWakes) {
				t.Errorf("Wakes = %+v; want %+v", got.Wakes, tt.wantWakes)
			}
			if !slices.Equal(got.Automatic, tt.wantAutomatic) {
				t.Errorf("Automatic = %+v; want %+v", got.Automatic, tt.wantAutomatic)
			}
			if len(got.Paused) != tt.wantPaused {
				t.Errorf("Paused = %+v; want %d entries", got.Paused, tt.wantPaused)
			}
			if !slices.Equal(got.AccountedChild, tt.wantAccounted) {
				t.Errorf("AccountedChild = %v; want %v", got.AccountedChild, tt.wantAccounted)
			}
		})
	}

	parseTests := []struct {
		output string
		want   StateChange
		wantOK bool
	}{
		{"child running → blocked", StateChange{From: shedengine.StateRunning, To: shedengine.StateBlocked}, true},
		{"child awaiting → running at Publish", StateChange{From: shedengine.StateAwaiting, To: shedengine.StateRunning, Producer: "Publish"}, true},
		{"child running → stuck at X", StateChange{}, false},
		{"batten waiting", StateChange{}, false},
	}
	for _, tt := range parseTests {
		t.Run("ParseStateChange "+tt.output, func(t *testing.T) {
			t.Parallel()
			got, ok := ParseStateChange(tt.output)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("ParseStateChange(%q) = (%+v, %v); want (%+v, %v)", tt.output, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
