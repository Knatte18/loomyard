// checklist_test.go pins the checklist frame renderChecklist draws for a status and a recipe, and the verb's refusal outside a git worktree.

package loomcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/statuspane"
)

// checklistClock is the instant minute offsets in the checklist tests count from.
var checklistClock = time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)

// minuteMark renders the instant minutes after checklistClock as the RFC 3339 string a status entry carries.
func minuteMark(minutes int) string {
	return checklistClock.Add(time.Duration(minutes) * time.Minute).Format(time.RFC3339)
}

// recipeRows is a recipe of plain rows with the given names.
func recipeRows(names ...string) []loomrecipe.RecipeRow {
	rows := make([]loomrecipe.RecipeRow, 0, len(names))
	for _, name := range names {
		rows = append(rows, loomrecipe.RecipeRow{Name: name})
	}
	return rows
}

// doneEntry is a Done history entry of producer at the given minute.
func doneEntry(producer string, atMinute int) shedengine.HistoryEntry {
	return shedengine.HistoryEntry{Producer: producer, Outcome: shedengine.Done, At: minuteMark(atMinute)}
}

// stuckEntry is a Stuck history entry of producer at the given minute, bounced to routedTo or, with "", a halt.
func stuckEntry(producer string, atMinute int, routedTo string) shedengine.HistoryEntry {
	return shedengine.HistoryEntry{Producer: producer, Outcome: shedengine.Stuck, At: minuteMark(atMinute), RoutedTo: routedTo}
}

// awaitingEntry is an Awaiting history entry of producer at the given minute.
func awaitingEntry(producer string, atMinute int) shedengine.HistoryEntry {
	return shedengine.HistoryEntry{Producer: producer, Outcome: shedengine.Awaiting, At: minuteMark(atMinute)}
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

// withActivityWait returns poll with its activity's wait set.
func withActivityWait(poll statuspane.StatusPoll, wait string) statuspane.StatusPoll {
	poll.Status.Activity.Wait = wait
	return poll
}

func TestRenderChecklist(t *testing.T) {
	t.Parallel()

	recipe := recipeRows("Plan", "Review", "Build")
	repeatedReview := stuckEntry("Review", 6, "")
	repeatedReview.LastAt = minuteMark(30)
	repeatedReview.Repeats = 1

	gatedBuild := doneEntry("Build", 5)
	gatedBuild.Output = "out/build.md"
	gatedAttempts := 2
	gatedBuild.GateAttempts = &gatedAttempts
	foldedBuild := stuckEntry("Build", 8, "")
	foldedBuild.LastAt = minuteMark(20)
	foldedBuild.Repeats = 2

	awaitingPlanBouncer := checklistStatus(shedengine.StateAwaiting, "Plan-Bouncer")
	awaitingPlanBouncer.Status.ParentNotice = "the plan needs a decision"
	awaitingPlanBouncer = withActivityWait(awaitingPlanBouncer, "waiting for the parent")
	longReason := "halted: " + strings.Repeat("a reason longer than any terminal is wide ", 8)

	tests := []struct {
		name    string
		rows    []loomrecipe.RecipeRow
		rowsErr error
		poll    statuspane.StatusPoll
		wait    string
		nowMin  int
		want    []string
	}{
		{
			name:   "a running run's wait line carries the note in full",
			rows:   recipe,
			poll:   checklistStatus(shedengine.StateRunning, "Review", doneEntry("Plan", 5)),
			wait:   "Review: waiting for the parent lock on main held by other 1m",
			nowMin: 12,
			want: []string{
				"[x] Plan 5m", "  10:00 5m done",
				"[>] Review 7m", "  10:05 7m running",
				"[ ] Build",
				"loom running | now working",
				"waiting Review: waiting for the parent lock on main held by other 1m",
			},
		},
		{
			name:   "a running run shows done, running and not reached rows in recipe order",
			rows:   recipe,
			poll:   checklistStatus(shedengine.StateRunning, "Review", doneEntry("Plan", 5)),
			nowMin: 12,
			want: []string{
				"[x] Plan 5m", "  10:00 5m done",
				"[>] Review 7m", "  10:05 7m running",
				"[ ] Build",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name: "a frame part-way through a run draws each row once with its visit lines under it",
			rows: recipeRows("Plan", "Review", "Build", "Publish"),
			poll: checklistStatus(shedengine.StateRunning, "Build",
				doneEntry("Plan", 5), doneEntry("Review", 10)),
			nowMin: 12,
			want: []string{
				"[x] Plan 5m", "  10:00 5m done",
				"[x] Review 5m", "  10:05 5m done",
				"[>] Build 2m", "  10:10 2m running",
				"[ ] Publish",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name: "a row reviewed twice sums its time over both visits and shows x2",
			rows: recipe,
			poll: checklistStatus(shedengine.StateRunning, "Review",
				doneEntry("Plan", 5), stuckEntry("Review", 8, ""), doneEntry("Plan", 15)),
			nowMin: 20,
			want: []string{
				"[x] Plan 12m x2", "  10:00 5m done", "  10:08 7m done",
				"[>] Review 8m x2", "  10:05 3m stuck", "  10:15 5m running",
				"[ ] Build",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name: "a folded repeat counts as further visits and a goto adds no time but starts the next visit",
			rows: recipe,
			poll: checklistStatus(shedengine.StateDone, "",
				doneEntry("Plan", 5), repeatedReview, gotoEntry("Plan", 40), doneEntry("Plan", 45)),
			nowMin: 60,
			want: []string{
				"[x] Plan 10m x2", "  10:00 5m done", "  10:40 5m done",
				"[x] Review 25m x2", "  10:05 25m stuck x2",
				"[ ] Build",
				"loom done | now working", "waiting on nothing",
			},
		},
		{
			name:   "a row resumed after a failed call counts the halt in its time",
			rows:   recipe,
			poll:   checklistStatus(shedengine.StateRunning, "Build", doneEntry("Plan", 5), doneEntry("Review", 60)),
			nowMin: 63,
			want: []string{
				"[x] Plan 5m", "  10:00 5m done",
				"[x] Review 55m", "  10:05 55m done",
				"[>] Build 3m", "  11:00 3m running",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name: "a row reached by goto after a halt counts only the time since the goto",
			rows: recipe,
			poll: checklistStatus(shedengine.StateRunning, "Plan",
				doneEntry("Plan", 5), stuckEntry("Review", 10, ""), gotoEntry("Plan", 120)),
			nowMin: 124,
			want: []string{
				"[>] Plan 9m x2", "  10:00 5m done", "  12:00 4m running",
				"[x] Review 5m", "  10:05 5m stuck",
				"[ ] Build",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name: "a stuck entry that records its routing reads bounced and a halted one reads stuck",
			rows: recipe,
			poll: checklistStatus(shedengine.StateRunning, "Review",
				stuckEntry("Review", 5, "Build"), stuckEntry("Review", 8, "")),
			nowMin: 10,
			want: []string{
				"[ ] Plan",
				"[>] Review 10m x3", "  10:00 5m bounced", "  10:05 3m stuck", "  10:08 2m running",
				"[ ] Build",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name:   "an awaiting entry reads escalated",
			rows:   recipe,
			poll:   checklistStatus(shedengine.StateAwaiting, "Review", awaitingEntry("Review", 5)),
			nowMin: 6,
			want: []string{
				"[ ] Plan",
				"[-] Review 6m x2", "  10:00 5m escalated", "  10:05 1m running",
				"[ ] Build",
				"loom awaiting | now working", "waiting on the parent: escalation from Review",
			},
		},
		{
			name:   "a blocked run's current row is halted with its time running to now",
			rows:   recipe,
			poll:   checklistStatus(shedengine.StateBlocked, "Review", doneEntry("Plan", 5)),
			nowMin: 35,
			want: []string{
				"[x] Plan 5m", "  10:00 5m done",
				"[-] Review 30m", "  10:05 30m running",
				"[ ] Build",
				"loom blocked | now working", "waiting on nothing",
			},
		},
		{
			name:   "a blocked run's wait line carries the halt reason in full",
			rows:   recipe,
			poll:   withActivityWait(checklistStatus(shedengine.StateBlocked, "Review"), longReason),
			nowMin: 1,
			want: []string{
				"[ ] Plan",
				"[-] Review 1m", "  10:00 1m running",
				"[ ] Build",
				"loom blocked | now working", "wait " + longReason,
			},
		},
		{
			name:   "an awaiting run's current row is halted",
			rows:   recipe,
			poll:   checklistStatus(shedengine.StateAwaiting, "Review", doneEntry("Plan", 5)),
			nowMin: 6,
			want: []string{
				"[x] Plan 5m", "  10:00 5m done",
				"[-] Review 1m", "  10:05 1m running",
				"[ ] Build",
				"loom awaiting | now working", "waiting on the parent: escalation from Review",
			},
		},
		{
			name:   "an awaiting run's wait line names the escalation to the parent",
			rows:   recipeRows("Plan-Bouncer"),
			poll:   awaitingPlanBouncer,
			nowMin: 2,
			want: []string{
				"[-] Plan-Bouncer 2m", "  10:00 2m running",
				"loom awaiting | now working",
				"waiting on the parent: escalation from Plan-Bouncer: the plan needs a decision | waiting for the parent",
			},
		},
		{
			name:   "a failed run's current row is failed",
			rows:   recipe,
			poll:   checklistStatus(shedengine.StateFailed, "Review", doneEntry("Plan", 5)),
			nowMin: 9,
			want: []string{
				"[x] Plan 5m", "  10:00 5m done",
				"[!] Review 4m", "  10:05 4m running",
				"[ ] Build",
				"loom failed | now working", "waiting on nothing",
			},
		},
		{
			name: "a visited row's lines show each visit with its start, time, outcome, output and gate attempts",
			rows: recipeRows("Build"),
			poll: checklistStatus(shedengine.StateRunning, "Build", gatedBuild, foldedBuild),
			// The folded entry spans minutes 8 to 20; the open visit starts where it ended.
			nowMin: 25,
			want: []string{
				"[>] Build 25m x5",
				"  10:00 5m done out/build.md gate attempts 2",
				"  10:05 15m stuck x3",
				"  10:20 5m running",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name:   "a history-only row follows the recipe rows, marked, with its visit line",
			rows:   recipe,
			poll:   checklistStatus(shedengine.StateRunning, "Plan", doneEntry("Retired", 5)),
			nowMin: 7,
			want: []string{
				"[>] Plan 2m", "  10:05 2m running",
				"[ ] Review",
				"[ ] Build",
				"[x] Retired 5m (not in recipe)", "  10:00 5m done",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name:    "an unreadable recipe shows recipe unreadable, then every visit in history order naming its row",
			rowsErr: errors.New("no seed is recorded"),
			poll: checklistStatus(shedengine.StateRunning, "Plan",
				doneEntry("Plan", 5), stuckEntry("Review", 8, "Plan")),
			nowMin: 10,
			want: []string{
				"recipe unreadable",
				"  10:00 5m Plan done", "  10:05 3m Review bounced", "  10:08 2m Plan running",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name: "an absent status shows every row not reached above the unavailable line",
			rows: recipe,
			want: []string{
				"[ ] Plan", "[ ] Review", "[ ] Build",
				"loom status unavailable (status file transiently unreadable)", "waiting on nothing",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			now := checklistClock.Add(time.Duration(tt.nowMin) * time.Minute)
			got := renderChecklist(checklistInput{rows: tt.rows, rowsErr: tt.rowsErr, poll: tt.poll, waitNote: tt.wait, startedAt: minuteMark(0)}, now)
			if diff := cmp.Diff(tt.want, got.Lines); diff != "" {
				t.Errorf("renderChecklist lines mismatch (-want +got):\n%s", diff)
			}
		})
	}
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
