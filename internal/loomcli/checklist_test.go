// checklist_test.go pins the checklist frame renderChecklist draws for a status and a recipe, and the verb's refusal outside a git worktree.

package loomcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/loomengine"
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

// reviewRecipe is a recipe holding a review segment, Rev, of a Bouncer, a review and a fix row, and a segment, PR, with no Bouncer row.
var reviewRecipe = []loomrecipe.RecipeRow{
	{Name: "Build"},
	{Name: "Rev-Bouncer", Segment: "Rev", OnStuck: "Rev-Review", Bouncer: true},
	{Name: "Rev-Review", Segment: "Rev", OnStuck: "Rev-Fix"},
	{Name: "Rev-Fix", Segment: "Rev", OnStuck: "Rev-Bouncer"},
	{Name: "Publish"},
	{Name: "PR-Review", Segment: "PR"},
	{Name: "PR-Rework", Segment: "PR"},
}

// bouncedEntry is the Rev-Bouncer entry at the given minute that bounced the run to the review row.
func bouncedEntry(atMinute int) shedengine.HistoryEntry {
	return stuckEntry("Rev-Bouncer", atMinute, "Rev-Review")
}

// roundEntry is a Done entry of producer at the given minute whose output is the round file of the given kind, such as round-2-review.md.
func roundEntry(producer string, atMinute, round int, kind string) shedengine.HistoryEntry {
	entry := doneEntry(producer, atMinute)
	entry.Output = fmt.Sprintf("round-%d-%s.md", round, kind)
	return entry
}

// handOnEntry is the budget-exempt Stuck entry of the review row at the given minute, whose output names the round.
func handOnEntry(atMinute, round int) shedengine.HistoryEntry {
	entry := stuckEntry("Rev-Review", atMinute, "")
	entry.Output = fmt.Sprintf("round-%d-review.md", round)
	entry.BudgetExempt = true
	return entry
}

// resolvedArbiter is an attempt spawned spawnMinute after checklistClock whose envelope carries verdict and cause.
func resolvedArbiter(number, spawnMinute, historyLength int, verdict, cause string) checklistArbiter {
	return checklistArbiter{
		number:        number,
		spawnedAt:     checklistClock.Add(time.Duration(spawnMinute) * time.Minute),
		historyLength: historyLength,
		envelope:      loomengine.ArbiterEnvelope{Verdict: verdict, Cause: cause},
		hasEnvelope:   true,
	}
}

// pendingArbiter is an attempt spawned spawnMinute after checklistClock that has no envelope yet and whose deadline is deadlineMinute after it.
func pendingArbiter(number, spawnMinute, historyLength, deadlineMinute int) checklistArbiter {
	return checklistArbiter{
		number:        number,
		spawnedAt:     checklistClock.Add(time.Duration(spawnMinute) * time.Minute),
		historyLength: historyLength,
		deadline:      checklistClock.Add(time.Duration(deadlineMinute) * time.Minute),
	}
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
		arbiter []checklistArbiter
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
			name: "a review segment renders one block grouped by round, with the closing Bouncer done outside any round",
			rows: reviewRecipe,
			poll: checklistStatus(shedengine.StateRunning, "Publish",
				doneEntry("Build", 5),
				bouncedEntry(6), roundEntry("Rev-Review", 10, 1, "review"), roundEntry("Rev-Fix", 15, 1, "fixer-report"),
				bouncedEntry(16), roundEntry("Rev-Review", 20, 2, "review"), roundEntry("Rev-Fix", 25, 2, "fixer-report"),
				doneEntry("Rev-Bouncer", 26)),
			nowMin: 30,
			want: []string{
				"[x] Build 5m", "  10:00 5m done",
				"[x] Rev 21m rounds 2",
				"  round 1",
				"    10:05 1m Rev-Bouncer bounced",
				"    10:06 4m Rev-Review done round-1-review.md",
				"    10:10 5m Rev-Fix done round-1-fixer-report.md",
				"  round 2",
				"    10:15 1m Rev-Bouncer bounced",
				"    10:16 4m Rev-Review done round-2-review.md",
				"    10:20 5m Rev-Fix done round-2-fixer-report.md",
				"  10:25 1m Rev-Bouncer done",
				"[>] Publish 4m", "  10:26 4m running",
				"[ ] PR-Review", "[ ] PR-Rework",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name: "a hand-on naming an opened round joins it and one naming an unopened round opens it",
			rows: reviewRecipe,
			poll: checklistStatus(shedengine.StateRunning, "Rev-Fix",
				doneEntry("Build", 5),
				bouncedEntry(6), roundEntry("Rev-Review", 10, 1, "review"), handOnEntry(12, 1), handOnEntry(14, 2)),
			nowMin: 16,
			want: []string{
				"[x] Build 5m", "  10:00 5m done",
				"[>] Rev 11m rounds 2",
				"  round 1",
				"    10:05 1m Rev-Bouncer bounced",
				"    10:06 4m Rev-Review done round-1-review.md",
				"    10:10 2m Rev-Review stuck round-1-review.md",
				"  round 2",
				"    10:12 2m Rev-Review stuck round-2-review.md",
				"    10:14 2m Rev-Fix running",
				"[ ] Publish", "[ ] PR-Review", "[ ] PR-Rework",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name: "a goto into the review row ends round 1 and round 2 opens with its review line and no Bouncer line",
			rows: reviewRecipe,
			poll: checklistStatus(shedengine.StateRunning, "Rev-Fix",
				doneEntry("Build", 5),
				bouncedEntry(6), roundEntry("Rev-Review", 10, 1, "review"), roundEntry("Rev-Fix", 15, 1, "fixer-report"),
				gotoEntry("Rev-Review", 16), roundEntry("Rev-Review", 20, 2, "review")),
			nowMin: 22,
			want: []string{
				"[x] Build 5m", "  10:00 5m done",
				"[>] Rev 16m rounds 2",
				"  round 1",
				"    10:05 1m Rev-Bouncer bounced",
				"    10:06 4m Rev-Review done round-1-review.md",
				"    10:10 5m Rev-Fix done round-1-fixer-report.md",
				"    10:16 0m Rev-Review goto",
				"  round 2",
				"    10:16 4m Rev-Review done round-2-review.md",
				"    10:20 2m Rev-Fix running",
				"[ ] Publish", "[ ] PR-Review", "[ ] PR-Rework",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name: "a reroute into the review row renders the same shape as a goto into it",
			rows: reviewRecipe,
			poll: checklistStatus(shedengine.StateRunning, "Rev-Fix",
				doneEntry("Build", 5),
				bouncedEntry(6), roundEntry("Rev-Review", 10, 1, "review"), roundEntry("Rev-Fix", 15, 1, "fixer-report"),
				shedengine.HistoryEntry{Producer: "Rev-Fix", Outcome: shedengine.OutcomeReroute, At: minuteMark(16), RoutedTo: "Rev-Review"},
				roundEntry("Rev-Review", 20, 2, "review")),
			nowMin: 22,
			want: []string{
				"[x] Build 5m", "  10:00 5m done",
				"[>] Rev 17m rounds 2",
				"  round 1",
				"    10:05 1m Rev-Bouncer bounced",
				"    10:06 4m Rev-Review done round-1-review.md",
				"    10:10 5m Rev-Fix done round-1-fixer-report.md",
				"    10:15 1m Rev-Fix reroute",
				"  round 2",
				"    10:16 4m Rev-Review done round-2-review.md",
				"    10:20 2m Rev-Fix running",
				"[ ] Publish", "[ ] PR-Review", "[ ] PR-Rework",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name: "a goto into the segment before any round opened stands before round 1",
			rows: reviewRecipe,
			poll: checklistStatus(shedengine.StateRunning, "Rev-Bouncer",
				doneEntry("Build", 5), gotoEntry("Rev-Bouncer", 6)),
			nowMin: 8,
			want: []string{
				"[x] Build 5m", "  10:00 5m done",
				"[>] Rev 2m rounds 0",
				"  10:06 0m Rev-Bouncer goto",
				"  10:06 2m Rev-Bouncer running",
				"[ ] Publish", "[ ] PR-Review", "[ ] PR-Rework",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name: "a Bouncer that bounced while the review row still runs shows its line and the open review visit under the next round",
			rows: reviewRecipe,
			poll: checklistStatus(shedengine.StateRunning, "Rev-Review",
				doneEntry("Build", 5),
				bouncedEntry(6), roundEntry("Rev-Review", 10, 1, "review"), roundEntry("Rev-Fix", 15, 1, "fixer-report"),
				bouncedEntry(16)),
			nowMin: 18,
			want: []string{
				"[x] Build 5m", "  10:00 5m done",
				"[>] Rev 13m rounds 2",
				"  round 1",
				"    10:05 1m Rev-Bouncer bounced",
				"    10:06 4m Rev-Review done round-1-review.md",
				"    10:10 5m Rev-Fix done round-1-fixer-report.md",
				"  round 2",
				"    10:15 1m Rev-Bouncer bounced",
				"    10:16 2m Rev-Review running",
				"[ ] Publish", "[ ] PR-Review", "[ ] PR-Rework",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name: "a Bouncer awaiting line reads escalated as the last line of its round",
			rows: reviewRecipe,
			poll: checklistStatus(shedengine.StateAwaiting, "Rev-Bouncer",
				doneEntry("Build", 5),
				bouncedEntry(6), roundEntry("Rev-Review", 10, 1, "review"), roundEntry("Rev-Fix", 15, 1, "fixer-report"),
				awaitingEntry("Rev-Bouncer", 16)),
			nowMin: 17,
			want: []string{
				"[x] Build 5m", "  10:00 5m done",
				"[-] Rev 12m rounds 1",
				"  round 1",
				"    10:05 1m Rev-Bouncer bounced",
				"    10:06 4m Rev-Review done round-1-review.md",
				"    10:10 5m Rev-Fix done round-1-fixer-report.md",
				"    10:15 1m Rev-Bouncer escalated",
				"    10:16 1m Rev-Bouncer running",
				"[ ] Publish", "[ ] PR-Review", "[ ] PR-Rework",
				"loom awaiting | now working", "waiting on the parent: escalation from Rev-Bouncer",
			},
		},
		{
			name: "a segment without a Bouncer row renders its rows as per-row lines and an unvisited review segment is one bare line",
			rows: reviewRecipe,
			poll: checklistStatus(shedengine.StateRunning, "PR-Rework",
				doneEntry("Build", 5), doneEntry("PR-Review", 8)),
			nowMin: 10,
			want: []string{
				"[x] Build 5m", "  10:00 5m done",
				"[ ] Rev",
				"[ ] Publish",
				"[x] PR-Review 3m", "  10:05 3m done",
				"[>] PR-Rework 2m", "  10:08 2m running",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name: "a round-3 escalation and its arbiter attempt stand in round 3 and round 4 opens after them",
			rows: reviewRecipe,
			poll: checklistStatus(shedengine.StateRunning, "Rev-Fix",
				doneEntry("Build", 5),
				bouncedEntry(6), roundEntry("Rev-Review", 7, 1, "review"),
				bouncedEntry(8), roundEntry("Rev-Review", 9, 2, "review"),
				bouncedEntry(10), roundEntry("Rev-Review", 11, 3, "review"), roundEntry("Rev-Fix", 12, 3, "fixer-report"),
				awaitingEntry("Rev-Bouncer", 13),
				bouncedEntry(15), roundEntry("Rev-Review", 17, 4, "review")),
			nowMin:  19,
			arbiter: []checklistArbiter{resolvedArbiter(1, 13, 9, "resume", "")},
			want: []string{
				"[x] Build 5m", "  10:00 5m done",
				"[>] Rev 14m rounds 4",
				"  round 1",
				"    10:05 1m Rev-Bouncer bounced",
				"    10:06 1m Rev-Review done round-1-review.md",
				"  round 2",
				"    10:07 1m Rev-Bouncer bounced",
				"    10:08 1m Rev-Review done round-2-review.md",
				"  round 3",
				"    10:09 1m Rev-Bouncer bounced",
				"    10:10 1m Rev-Review done round-3-review.md",
				"    10:11 1m Rev-Fix done round-3-fixer-report.md",
				"    10:12 1m Rev-Bouncer escalated",
				"    10:13 arbiter attempt 1 resume",
				"  round 4",
				"    10:13 2m Rev-Bouncer bounced",
				"    10:15 2m Rev-Review done round-4-review.md",
				"    10:17 2m Rev-Fix running",
				"[ ] Publish", "[ ] PR-Review", "[ ] PR-Rework",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name:    "an attempt whose envelope carries hand-on with cause died renders hand-on (died) after its entry",
			rows:    recipe,
			poll:    checklistStatus(shedengine.StateRunning, "Review", doneEntry("Plan", 5)),
			nowMin:  7,
			arbiter: []checklistArbiter{resolvedArbiter(1, 5, 1, "hand-on", "died")},
			want: []string{
				"[x] Plan 5m", "  10:00 5m done", "  10:05 arbiter attempt 1 hand-on (died)",
				"[>] Review 2m", "  10:05 2m running",
				"[ ] Build",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name:   "an attempt with no envelope renders running before its deadline and no verdict from it on",
			rows:   recipe,
			poll:   checklistStatus(shedengine.StateRunning, "Review", doneEntry("Plan", 5)),
			nowMin: 7,
			arbiter: []checklistArbiter{
				pendingArbiter(1, 5, 1, 10),
				pendingArbiter(2, 6, 1, 6),
			},
			want: []string{
				"[x] Plan 5m", "  10:00 5m done",
				"  10:05 arbiter attempt 1 running", "  10:06 arbiter attempt 2 no verdict",
				"[>] Review 2m", "  10:05 2m running",
				"[ ] Build",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name:    "an attempt whose stop key exceeds the history stands at the end of the frame",
			rows:    recipe,
			poll:    checklistStatus(shedengine.StateRunning, "Review", doneEntry("Plan", 5)),
			nowMin:  7,
			arbiter: []checklistArbiter{resolvedArbiter(1, 6, 5, "resume", "")},
			want: []string{
				"[x] Plan 5m", "  10:00 5m done",
				"[>] Review 2m", "  10:05 2m running",
				"[ ] Build",
				"10:06 arbiter attempt 1 resume",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name:   "an unreadable attempt stands before the state line beside a readable one drawn in place",
			rows:   recipe,
			poll:   checklistStatus(shedengine.StateRunning, "Review", doneEntry("Plan", 5)),
			nowMin: 7,
			arbiter: []checklistArbiter{
				{dir: "attempt-2", unreadable: true},
				resolvedArbiter(1, 5, 1, "hand-on", ""),
			},
			want: []string{
				"[x] Plan 5m", "  10:00 5m done", "  10:05 arbiter attempt 1 hand-on",
				"[>] Review 2m", "  10:05 2m running",
				"[ ] Build",
				"arbiter attempt unreadable (attempt-2)",
				"loom running | now working", "waiting on nothing",
			},
		},
		{
			name:    "an unreadable recipe lists the attempt after the visit its stop key names",
			rowsErr: errors.New("no seed is recorded"),
			poll:    checklistStatus(shedengine.StateRunning, "Plan", doneEntry("Plan", 5)),
			nowMin:  7,
			arbiter: []checklistArbiter{resolvedArbiter(1, 5, 1, "resume", "")},
			want: []string{
				"recipe unreadable",
				"  10:00 5m Plan done", "  10:05 arbiter attempt 1 resume", "  10:05 2m Plan running",
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
			got := renderChecklist(checklistInput{rows: tt.rows, rowsErr: tt.rowsErr, poll: tt.poll, waitNote: tt.wait, startedAt: minuteMark(0), arbiters: tt.arbiter}, now)
			if diff := cmp.Diff(tt.want, got.Lines); diff != "" {
				t.Errorf("renderChecklist lines mismatch (-want +got):\n%s", diff)
			}
		})
	}

	t.Run("readChecklistArbiters reads each attempt directory on its own", func(t *testing.T) {
		t.Parallel()
		arbiterDir := t.TempDir()
		attempt := func(number int) loomengine.ArbiterAttempt {
			return loomengine.ArbiterAttempt{
				Number:    number,
				StopKey:   loomengine.ArbiterStopKey{HistoryLength: 2},
				SpawnedAt: checklistClock,
				Deadline:  checklistClock.Add(10 * time.Minute),
			}
		}
		writeAttempt := func(name string, number int) string {
			dir := filepath.Join(arbiterDir, name)
			if err := loomengine.WriteArbiterAttempt(dir, attempt(number)); err != nil {
				t.Fatalf("WriteArbiterAttempt(%s) = %v", name, err)
			}
			return dir
		}
		withEnvelope := writeAttempt("a1", 1)
		if err := loomengine.WriteArbiterEnvelope(loomengine.ArbiterEnvelopePath(withEnvelope), loomengine.ArbiterEnvelope{Verdict: "resume"}); err != nil {
			t.Fatalf("WriteArbiterEnvelope = %v", err)
		}
		writeAttempt("a2", 2)
		garbled := writeAttempt("a3", 3)
		if err := os.WriteFile(loomengine.ArbiterEnvelopePath(garbled), []byte("{not json"), 0o644); err != nil {
			t.Fatalf("write garbled envelope: %v", err)
		}
		if err := os.Mkdir(filepath.Join(arbiterDir, "a4"), 0o755); err != nil {
			t.Fatalf("mkdir a4: %v", err)
		}

		got := readChecklistArbiters(arbiterDir)
		var lines []string
		for _, arbiter := range got {
			lines = append(lines, checklistArbiterLine(arbiter, checklistClock.Add(time.Minute)))
		}
		want := []string{
			"10:00 arbiter attempt 1 resume",
			"10:00 arbiter attempt 2 running",
			"10:00 arbiter attempt 3 verdict unreadable",
			"arbiter attempt unreadable (a4)",
		}
		if diff := cmp.Diff(want, lines); diff != "" {
			t.Errorf("readChecklistArbiters lines mismatch (-want +got):\n%s", diff)
		}
		if absent := readChecklistArbiters(filepath.Join(arbiterDir, "absent")); len(absent) != 0 {
			t.Errorf("readChecklistArbiters(absent directory) = %+v; want none", absent)
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
