// checklist_test.go pins the checklist frame renderChecklist draws for a status and a recipe, its Fit, and the verb's refusal outside a git worktree.

package loomcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/statuspane"
)

// checklistClock is the instant minute offsets in the checklist tests count from.
var checklistClock = time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)

// minuteMark renders the instant minutes after checklistClock as the RFC 3339 string a status entry carries.
func minuteMark(minutes int) string {
	return checklistClock.Add(time.Duration(minutes) * time.Minute).Format(time.RFC3339)
}

// doneEntry is a Done history entry of producer at the given minute.
func doneEntry(producer string, atMinute int) shedengine.HistoryEntry {
	return shedengine.HistoryEntry{Producer: producer, Outcome: shedengine.Done, At: minuteMark(atMinute)}
}

// stuckEntry is a Stuck history entry of producer at the given minute.
func stuckEntry(producer string, atMinute int) shedengine.HistoryEntry {
	return shedengine.HistoryEntry{Producer: producer, Outcome: shedengine.Stuck, At: minuteMark(atMinute)}
}

// gotoEntry is the goto history entry that moved the run onto producer at the given minute.
func gotoEntry(producer string, atMinute int) shedengine.HistoryEntry {
	return shedengine.HistoryEntry{Producer: producer, Outcome: shedengine.OutcomeGoto, At: minuteMark(atMinute)}
}

// checklistStatus is a found poll of a status in state, on current, with the given history and the activity line "working".
func checklistStatus(state shedengine.State, current string, history ...shedengine.HistoryEntry) statuspane.StatusPoll {
	return statuspane.StatusPoll{
		Found: true,
		Status: shedengine.Status{
			CurrentProducer: current,
			State:           state,
			Activity:        shedengine.Activity{Now: "working"},
			History:         history,
		},
	}
}

func TestRenderChecklist(t *testing.T) {
	t.Parallel()

	recipe := []string{"Plan", "Review", "Build"}
	repeatedReview := stuckEntry("Review", 6)
	repeatedReview.LastAt = minuteMark(30)
	repeatedReview.Repeats = 1

	tests := []struct {
		name    string
		rows    []string
		rowsErr error
		poll    statuspane.StatusPoll
		wait    string
		nowMin  int
		want    []string
	}{
		{
			name:   "a wait note follows the state line",
			rows:   recipe,
			poll:   checklistStatus(shedengine.StateRunning, "Review", doneEntry("Plan", 5)),
			wait:   "Review: waiting for the parent lock on main held by other 1m",
			nowMin: 12,
			want: []string{
				"loom running | now working | waiting Review: waiting for the parent lock on main held by other 1m",
				"[x] Plan 5m", "[>] Review 7m", "[ ] Build",
			},
		},
		{
			name:   "a running run shows done, running and not reached rows in recipe order",
			rows:   recipe,
			poll:   checklistStatus(shedengine.StateRunning, "Review", doneEntry("Plan", 5)),
			nowMin: 12,
			want:   []string{"loom running | now working", "[x] Plan 5m", "[>] Review 7m", "[ ] Build"},
		},
		{
			name: "a row reviewed twice sums its time over both visits and shows x2",
			rows: recipe,
			poll: checklistStatus(shedengine.StateRunning, "Review",
				doneEntry("Plan", 5), stuckEntry("Review", 8), doneEntry("Plan", 15)),
			nowMin: 20,
			want:   []string{"loom running | now working", "[x] Plan 12m x2", "[>] Review 8m x2", "[ ] Build"},
		},
		{
			name: "a folded repeat counts as further visits and a goto adds no time but starts the next visit",
			rows: recipe,
			poll: checklistStatus(shedengine.StateDone, "",
				doneEntry("Plan", 5), repeatedReview, gotoEntry("Plan", 40), doneEntry("Plan", 45)),
			nowMin: 60,
			want:   []string{"loom done | now working", "[x] Plan 10m x2", "[x] Review 25m x2", "[ ] Build"},
		},
		{
			name:   "a row resumed after a failed call counts the halt in its time",
			rows:   recipe,
			poll:   checklistStatus(shedengine.StateRunning, "Build", doneEntry("Plan", 5), doneEntry("Review", 60)),
			nowMin: 63,
			want:   []string{"loom running | now working", "[x] Plan 5m", "[x] Review 55m", "[>] Build 3m"},
		},
		{
			name: "a row reached by goto after a halt counts only the time since the goto",
			rows: recipe,
			poll: checklistStatus(shedengine.StateRunning, "Plan",
				doneEntry("Plan", 5), stuckEntry("Review", 10), gotoEntry("Plan", 120)),
			nowMin: 124,
			want:   []string{"loom running | now working", "[>] Plan 9m x2", "[x] Review 5m", "[ ] Build"},
		},
		{
			name:   "a blocked run's current row is halted with its time running to now",
			rows:   recipe,
			poll:   checklistStatus(shedengine.StateBlocked, "Review", doneEntry("Plan", 5)),
			nowMin: 35,
			want:   []string{"loom blocked | now working", "[x] Plan 5m", "[-] Review 30m", "[ ] Build"},
		},
		{
			name:   "an awaiting run's current row is halted",
			rows:   recipe,
			poll:   checklistStatus(shedengine.StateAwaiting, "Review", doneEntry("Plan", 5)),
			nowMin: 6,
			want:   []string{"loom awaiting | now working", "[x] Plan 5m", "[-] Review 1m", "[ ] Build"},
		},
		{
			name:   "a failed run's current row is failed",
			rows:   recipe,
			poll:   checklistStatus(shedengine.StateFailed, "Review", doneEntry("Plan", 5)),
			nowMin: 9,
			want:   []string{"loom failed | now working", "[x] Plan 5m", "[!] Review 4m", "[ ] Build"},
		},
		{
			name:   "a history entry naming a removed row follows the recipe rows",
			rows:   recipe,
			poll:   checklistStatus(shedengine.StateRunning, "Plan", doneEntry("Retired", 5)),
			nowMin: 7,
			want:   []string{"loom running | now working", "[>] Plan 2m", "[ ] Review", "[ ] Build", "[x] Retired 5m (not in recipe)"},
		},
		{
			name:    "an unreadable recipe shows the state line and recipe unreadable",
			rowsErr: errors.New("no seed is recorded"),
			poll:    checklistStatus(shedengine.StateRunning, "Plan"),
			want:    []string{"loom running | now working", "recipe unreadable"},
		},
		{
			name: "an absent status shows the unavailable line and every row not reached",
			rows: recipe,
			want: []string{"loom status unavailable (status file transiently unreadable)", "[ ] Plan", "[ ] Review", "[ ] Build"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			now := checklistClock.Add(time.Duration(tt.nowMin) * time.Minute)
			got := renderChecklist(tt.rows, tt.rowsErr, tt.poll, tt.wait, minuteMark(0), now)
			if diff := cmp.Diff(tt.want, got.Lines); diff != "" {
				t.Errorf("renderChecklist lines mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRenderChecklist_Fit(t *testing.T) {
	t.Parallel()

	rows := []string{"A", "B", "C", "D", "E", "F"}
	running := checklistStatus(shedengine.StateRunning, "D", doneEntry("A", 1), doneEntry("B", 2), doneEntry("C", 3))
	frame := renderChecklist(rows, nil, running, "", minuteMark(0), checklistClock.Add(4*time.Minute))
	if len(frame.Lines) != 1+len(rows) {
		t.Fatalf("frame lines = %d; want the state line and %d rows", len(frame.Lines), len(rows))
	}

	tests := []struct {
		name string
		rows int
		want []string
	}{
		{"every line fits", 7, frame.Lines},
		{"the rows nearest the running row stay and the later row wins a tie", 5, []string{
			frame.Lines[0], frame.Lines[3], frame.Lines[4], frame.Lines[5], "… 3 more rows not shown",
		}},
		{"the state line, the running row and the summary are the least that stays", 3, []string{
			frame.Lines[0], frame.Lines[4], "… 5 more rows not shown",
		}},
		{"two rows keep the state line and the running row", 2, []string{frame.Lines[0], frame.Lines[4]}},
		{"one row keeps the state line", 1, []string{frame.Lines[0]}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := frame.Fit(tt.rows)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Fit(%d) mismatch (-want +got):\n%s", tt.rows, diff)
			}
			if len(got) > tt.rows {
				t.Errorf("Fit(%d) returned %d lines; want at most %d", tt.rows, len(got), tt.rows)
			}
		})
	}

	t.Run("without a running row the last visited row anchors the fit", func(t *testing.T) {
		t.Parallel()
		done := checklistStatus(shedengine.StateDone, "", doneEntry("A", 1), doneEntry("B", 2), doneEntry("C", 3))
		fitted := renderChecklist(rows, nil, done, "", minuteMark(0), checklistClock.Add(4*time.Minute)).Fit(3)
		if want := "[x] C 1m"; !slices.Contains(fitted, want) {
			t.Errorf("Fit(3) = %q; want it to keep the last visited row %q", fitted, want)
		}
	})
}

func TestChecklistCmd_ReportsAResolutionFailureOnTheEnvelope(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if code := RunCLIIn(t.TempDir(), &out, []string{"checklist"}); code != 1 {
		t.Errorf("RunCLIIn(checklist) outside a worktree = %d; want 1", code)
	}
	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("output %q is not one JSON envelope: %v", out.String(), err)
	}
	if envelope["ok"] != false {
		t.Errorf("envelope ok = %v; want false", envelope["ok"])
	}
}
