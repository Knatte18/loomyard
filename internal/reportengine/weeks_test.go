// weeks_test.go tests the per-week aggregates, the absent counts, the JSON envelope and the markdown rendering over fake sources.

package reportengine

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/haltnote"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

func floatPtr(f float64) *float64 { return &f }

// weekOf returns the week labelled label of report.
func weekOf(t *testing.T, report Report, label string) Week {
	t.Helper()

	for _, week := range report.Weeks {
		if week.Week == label {
			return week
		}
	}
	t.Fatalf("report has no week %s; weeks = %+v", label, report.Weeks)
	return Week{}
}

// crashNote is a halt note of class crash that no batten change matches.
func crashNote(name string) map[string]string {
	return haltNoteFiles("", name, haltnote.Note{Producer: "Land", State: "failed", Anomaly: "crash", Reason: "died"})
}

func TestCompute_Weeks(t *testing.T) {
	t.Parallel()
	running, awaiting := shedengine.StateRunning, shedengine.StateAwaiting
	awaitingAtWrite := battenStatusOf(awaiting, "", stateChange(running, awaiting, "Plan-Write", 2))

	tests := []struct {
		name  string
		setup func(w *world)
		check func(t *testing.T, report Report)
	}{
		{
			name: "the wake mean covers the runs holding every wake source, and a run lacking one is counted lacking",
			setup: func(w *world) {
				w.fullRun("a", "ta", 100, []shedengine.Status{awaitingAtWrite}, nil, mergeFiles(
					parentRoundFiles(1, true, 0, "fallback", ""),
					alignmentAttemptFiles(w.t, "a", "pr-gate", 1, "approve", "approve", "alpha")))
				w.fullRun("b", "tb", 101, []shedengine.Status{awaitingAtWrite}, nil,
					alignmentAttemptFiles(w.t, "b", "pr-gate", 1, "approve", "approve", "alpha"))
			},
			check: func(t *testing.T, report Report) {
				week := weekOf(t, report, "2026-W02")
				if week.Runs != 2 || week.WakeRuns != 1 || !reflect.DeepEqual(week.WakesPerRun, floatPtr(3)) {
					t.Errorf("runs, wake runs, mean = %d, %d, %v; want 2, 1, 3", week.Runs, week.WakeRuns, week.WakesPerRun)
				}
				wantWakes := map[string]map[string]int{
					SourceBatten:       {"awaiting at Plan-Write": 2},
					SourceParentReview: {"fallback": 1},
					SourceAlignment:    {"approve-superseding": 2},
				}
				if !reflect.DeepEqual(week.Wakes, wantWakes) {
					t.Errorf("wakes = %v; want %v", week.Wakes, wantWakes)
				}
				if week.Lacking[SourceParentReview] != 1 || report.Absent[SourceParentReview] != 1 {
					t.Errorf("parent-review lacking = %d, absent = %d; want 1 and 1", week.Lacking[SourceParentReview], report.Absent[SourceParentReview])
				}
				if week.Lacking[SourceLandReadyTasks] != 2 || report.Absent[SourceLandReadyTasks] != 2 {
					t.Errorf("land-ready-tasks lacking = %d, absent = %d; want 2 and 2", week.Lacking[SourceLandReadyTasks], report.Absent[SourceLandReadyTasks])
				}
			},
		},
		{
			name: "a week whose only run lacks the halt-note source has no stop classes, encoded as null, and counts it lacking",
			setup: func(w *world) {
				w.fullRun("a", "ta", 100, nil, nil, nil)
			},
			check: func(t *testing.T, report Report) {
				week := weekOf(t, report, "2026-W02")
				if week.StopClasses != nil || week.FailureClasses != nil || week.Lacking[SourceHaltNotes] != 1 || report.Absent[SourceHaltNotes] != 1 {
					t.Errorf("stop classes = %v, failure classes = %v, lacking = %d; want nil, nil and 1", week.StopClasses, week.FailureClasses, week.Lacking[SourceHaltNotes])
				}
				data, err := json.Marshal(week)
				if err != nil {
					t.Fatalf("marshal week: %v", err)
				}
				if !strings.Contains(string(data), `"stop_classes":null`) || !strings.Contains(string(data), `"failure_classes":null`) {
					t.Errorf("week JSON = %s; want stop_classes and failure_classes null", data)
				}
			},
		},
		{
			name: "the fix share is fresh over fresh plus aged, nil in a week with no fix, and the unmapped share likewise",
			setup: func(w *world) {
				w.taskCommit("T0", 0)
				w.parentMerge("M0", 0, "T0")
				w.parentCommit("F1", 10, "fresh fix")
				w.parentCommit("F2", 100, "aged fix")
				w.code.preImages["F1"] = map[string][]int{"a.go": {1}}
				w.code.preImages["F2"] = map[string][]int{"a.go": {1}}
				w.code.blames["M0 a.go"] = map[int]string{1: "M0"}
				w.code.blames["F1 a.go"] = map[int]string{1: "M0"}
				w.parentCommit("T3", 200, "feature")
				w.boardCommit("bd", 1, boardTask("f1", "", "bug"), boardTask("f2", "", "bug"), boardTask("ft", "", "feature"))
				w.exactRun("f1", "t1", 105, "F1", finalizeDone)
				w.exactRun("f2", "t2", 106, "F2", finalizeDone)
				w.exactRun("ft", "t3", 210, "T3", finalizeDone)
			},
			check: func(t *testing.T, report Report) {
				first := weekOf(t, report, "2026-W02")
				if !reflect.DeepEqual(first.FixShare, floatPtr(0.5)) || first.Fix != (FreshCounts{Fresh: 1, Aged: 1}) || first.UnmappedShare != nil || first.Unmapped != (FreshCounts{NoPreImage: 1}) {
					t.Errorf("W02 fix share = %v %+v, unmapped share = %v %+v; want 0.5 {1 1 0}, nil {0 0 1}", first.FixShare, first.Fix, first.UnmappedShare, first.Unmapped)
				}
				second := weekOf(t, report, "2026-W03")
				if second.FixShare != nil || second.Landings[LandingKindFeature] != 1 || second.Landings[LandingKindFix] != 0 {
					t.Errorf("W03 fix share = %v, landings = %v; want nil and one feature", second.FixShare, second.Landings)
				}
			},
		},
		{
			name: "a stop class seen in two runs of a week shows two stops and two runs, in each week it recurs",
			setup: func(w *world) {
				w.fullRun("a", "ta", 100, nil, nil, crashNote("loom-halt.md"))
				w.fullRun("b", "tb", 101, nil, nil, crashNote("loom-halt.md"))
				w.fullRun("c", "tc", 200, nil, nil, crashNote("loom-halt.md"))
				w.fullRun("d", "td", 201, nil, nil, crashNote("loom-halt.md"))
			},
			check: func(t *testing.T, report Report) {
				for _, label := range []string{"2026-W02", "2026-W03"} {
					week := weekOf(t, report, label)
					if want := []ClassCount{{Class: "crash", Stops: 2, Runs: 2}}; !reflect.DeepEqual(week.StopClasses, want) {
						t.Errorf("%s stop classes = %v; want %v", label, week.StopClasses, want)
					}
					if want := []ClassCount{{Class: "Land: died", Stops: 2, Runs: 2}}; !reflect.DeepEqual(week.FailureClasses, want) {
						t.Errorf("%s failure classes = %v; want %v", label, week.FailureClasses, want)
					}
				}
			},
		},
		{
			name: "a run and its landing on either side of a week boundary count in different weeks, and a partial run counts in its week's partial alone",
			setup: func(w *world) {
				w.fullRun("a", "ta", 167, nil, nil, nil)
				w.parentCommit("U", 168, "landing")
				w.tag("part", "tp", at(170), seedText("darn"), "")
			},
			check: func(t *testing.T, report Report) {
				first, second := weekOf(t, report, "2026-W02"), weekOf(t, report, "2026-W03")
				if first.Runs != 1 || first.Partial != 0 || first.Landings[LandingKindUnmapped] != 0 {
					t.Errorf("W02 runs, partial, unmapped = %d, %d, %d; want 1, 0, 0", first.Runs, first.Partial, first.Landings[LandingKindUnmapped])
				}
				if second.Runs != 0 || second.Partial != 1 || second.Landings[LandingKindUnmapped] != 1 {
					t.Errorf("W03 runs, partial, unmapped = %d, %d, %d; want 0, 1, 1", second.Runs, second.Partial, second.Landings[LandingKindUnmapped])
				}
			},
		},
		{
			name: "a review segment two runs hold with 2 and 4 rounds has mean 3 and one run lacking it",
			setup: func(w *world) {
				w.fullRun("a", "ta", 100, nil, nil, reviewFiles("plan", "round-1-review.md", "round-2-review.md"))
				w.fullRun("b", "tb", 101, nil, nil, reviewFiles("plan", "round-1-review.md", "round-2-review.md", "round-3-review.md", "round-4-review.md"))
				w.fullRun("c", "tc", 102, nil, nil, nil)
			},
			check: func(t *testing.T, report Report) {
				want := &SegmentWeek{Mean: floatPtr(3), Runs: 2, Lacking: 1, Outcomes: map[string]int{"accepted": 2}}
				if got := weekOf(t, report, "2026-W02").Rounds["plan"]; !reflect.DeepEqual(got, want) {
					t.Errorf("plan rounds = %+v; want %+v", got, want)
				}
			},
		},
		{
			name: "the arbiter outcomes sum over the runs holding the source, and a run lacking it counts lacking",
			setup: func(w *world) {
				w.fullRun("a", "ta", 100, nil, nil, arbiterAttemptFiles(w.t, "a", 1, arbiterAt("Publish", "failed", 3), "verdict: resume\nreason: r\n", true))
				w.fullRun("b", "tb", 101, nil, nil, nil)
			},
			check: func(t *testing.T, report Report) {
				week := weekOf(t, report, "2026-W02")
				if want := []ArbiterOutcome{{Verdict: "resume", Count: 1, Applied: 1}}; !reflect.DeepEqual(week.Arbiter, want) {
					t.Errorf("arbiter = %v; want %v", week.Arbiter, want)
				}
				if week.Lacking[SourceArbiter] != 1 || report.Absent[SourceArbiter] != 1 {
					t.Errorf("arbiter lacking = %d, absent = %d; want 1 and 1", week.Lacking[SourceArbiter], report.Absent[SourceArbiter])
				}
			},
		},
		{
			name: "LLM steps sum per source over the week's runs, a hand landing adding to the landing source",
			setup: func(w *world) {
				goto1 := shedengine.HistoryEntry{Producer: "Plan-Write", Outcome: shedengine.OutcomeGoto}
				w.fullRun("a", "ta", 100, nil, []shedengine.HistoryEntry{goto1, goto1}, nil)
				w.parentCommit("T1", 50, "land")
				w.boardCommit("bd", 1, boardTask("b", "", "chore"))
				w.exactRun("b", "tb", 101, "T1", append([]shedengine.HistoryEntry{goto1, goto1, goto1}, finalizeStuck...))
			},
			check: func(t *testing.T, report Report) {
				if want := map[string]int{"loom": 5, "landing": 1}; !reflect.DeepEqual(weekOf(t, report, "2026-W02").LLMSteps, want) {
					t.Errorf("LLM steps = %v; want %v", weekOf(t, report, "2026-W02").LLMSteps, want)
				}
			},
		},
		{
			name: "the JSON envelope carries its seven keys and an absent wake source encodes as null",
			setup: func(w *world) {
				w.fullRun("a", "ta", 100, nil, nil, nil)
			},
			check: func(t *testing.T, report Report) {
				data, err := json.Marshal(report)
				if err != nil {
					t.Fatalf("marshal report: %v", err)
				}
				var envelope map[string]json.RawMessage
				if err := json.Unmarshal(data, &envelope); err != nil {
					t.Fatalf("unmarshal report: %v", err)
				}
				var keys []string
				for key := range envelope {
					keys = append(keys, key)
				}
				slices.Sort(keys)
				if want := []string{"absent", "findings", "landings", "partial", "runs", "selection", "weeks"}; !slices.Equal(keys, want) {
					t.Errorf("envelope keys = %v; want %v", keys, want)
				}
				var runs []struct {
					Wakes map[string]json.RawMessage `json:"wakes"`
				}
				if err := json.Unmarshal(envelope["runs"], &runs); err != nil || len(runs) != 1 {
					t.Fatalf("runs = %s (%v); want one", envelope["runs"], err)
				}
				for _, source := range []string{SourceBatten, SourceParentReview, SourceAlignment, SourceLandReadyTasks} {
					if string(runs[0].Wakes[source]) != "null" {
						t.Errorf("wake source %s = %s; want null", source, runs[0].Wakes[source])
					}
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			tt.setup(w)

			report, err := Compute(w.sources(), Options{Since: at(-500), FreshDays: 3})
			if err != nil {
				t.Fatalf("Compute error = %v", err)
			}
			tt.check(t, report)
		})
	}
}

// sectionRows returns the cell rows of the table under heading, up to the next heading, whose first cell is week.
func sectionRows(markdown, heading, week string) [][]string {
	_, after, _ := strings.Cut(markdown, heading+"\n")
	section, _, _ := strings.Cut(after, "\n## ")
	var rows [][]string
	for _, line := range strings.Split(section, "\n") {
		if !strings.HasPrefix(line, "| "+week+" |") {
			continue
		}
		cells := strings.Split(strings.TrimSuffix(strings.TrimPrefix(line, "| "), " |"), " | ")
		rows = append(rows, cells)
	}
	return rows
}

func cellFloat(value *float64) string {
	if value == nil {
		return "no data"
	}
	return fmt.Sprintf("%.2f", *value)
}

func TestRenderMarkdown(t *testing.T) {
	t.Parallel()
	running, awaiting := shedengine.StateRunning, shedengine.StateAwaiting

	w := newWorld(t)
	w.fullRun("a", "ta", 100,
		[]shedengine.Status{battenStatusOf(awaiting, "", stateChange(running, awaiting, "Plan-Write", 2))}, nil,
		mergeFiles(
			parentRoundFiles(1, true, 0, "fallback", ""),
			alignmentAttemptFiles(w.t, "a", "pr-gate", 1, "approve", "approve", "alpha"),
			crashNote("loom-halt.md"),
			reviewFiles("plan", "round-1-review.md", "round-2-review.md")))
	w.fullRun("b", "tb", 200, nil, nil, reviewFiles("plan", "round-1-review.md", "round-2-review.md", "round-3-review.md", "round-4-review.md"))
	w.parentCommit("U1", 50, "first landing")
	w.parentCommit("U1b", 200, "second landing")
	w.parentCommit("U2", 210, "fix")
	w.code.preImages["U2"] = map[string][]int{"a.go": {1}}
	w.code.blames["U1b a.go"] = map[int]string{1: "U1b"}

	report, err := Compute(w.sources(), Options{Since: at(-500), FreshDays: 3})
	if err != nil {
		t.Fatalf("Compute error = %v", err)
	}
	markdown := RenderMarkdown(report)

	last := -1
	for _, heading := range []string{HeadingWakes, HeadingStops, HeadingFreshFixes, HeadingRounds} {
		index := strings.Index(markdown, "\n"+heading+"\n")
		if index <= last {
			t.Fatalf("heading %q at %d after %d; want the four headings in order\n%s", heading, index, last, markdown)
		}
		last = index
	}
	if !strings.HasPrefix(markdown, "# Convergence report\n") || strings.Index(markdown, "- since: ") > strings.Index(markdown, HeadingWakes) {
		t.Errorf("the selection does not open the document:\n%s", markdown)
	}

	for _, week := range report.Weeks {
		wakes := sectionRows(markdown, HeadingWakes, week.Week)
		wantWakes := []string{week.Week, fmt.Sprint(week.Runs), fmt.Sprint(week.WakeRuns), cellFloat(week.WakesPerRun)}
		if len(wakes) != 1 || !slices.Equal(wakes[0][:4], wantWakes) {
			t.Errorf("%s wake rows = %v; want one starting %v", week.Week, wakes, wantWakes)
		}

		fresh := sectionRows(markdown, HeadingFreshFixes, week.Week)
		wantFresh := []string{
			week.Week, fmt.Sprint(week.Landings[LandingKindFix]), fmt.Sprint(week.Landings[LandingKindFeature]), fmt.Sprint(week.Landings[LandingKindOther]), fmt.Sprint(week.Landings[LandingKindUnmapped]),
			fmt.Sprint(week.Hand), cellFloat(week.FixShare),
		}
		if len(fresh) != 1 || !slices.Equal(fresh[0][:7], wantFresh) || fresh[0][8] != cellFloat(week.UnmappedShare) {
			t.Errorf("%s fresh-fix rows = %v; want one starting %v and unmapped share %s", week.Week, fresh, wantFresh, cellFloat(week.UnmappedShare))
		}

		rounds := sectionRows(markdown, HeadingRounds, week.Week)
		wantRounds := []string{week.Week, "plan", cellFloat(week.Rounds["plan"].Mean), fmt.Sprint(week.Rounds["plan"].Runs), fmt.Sprint(week.Rounds["plan"].Lacking)}
		if !slices.ContainsFunc(rounds, func(row []string) bool { return slices.Equal(row[:5], wantRounds) }) {
			t.Errorf("%s round rows = %v; want one starting %v", week.Week, rounds, wantRounds)
		}

		stops := sectionRows(markdown, HeadingStops, week.Week)
		var wantStops [][]string
		if week.StopClasses == nil {
			wantStops = [][]string{{week.Week, "stop", "no data", "no data", "no data"}, {week.Week, "failure", "no data", "no data", "no data"}}
		} else {
			wantStops = [][]string{
				{week.Week, "stop", "awaiting", "1", "1"},
				{week.Week, "stop", "crash", "1", "1"},
				{week.Week, "failure", "Land: died", "1", "1"},
			}
		}
		if !reflect.DeepEqual(stops, wantStops) {
			t.Errorf("%s stop rows = %v; want %v", week.Week, stops, wantStops)
		}
	}

	nilShares := 0
	for _, week := range report.Weeks {
		if week.FixShare == nil {
			nilShares++
		}
	}
	if nilShares == 0 || !strings.Contains(markdown, "no data") {
		t.Errorf("no week has a nil fix share rendered as no data:\n%s", markdown)
	}
}
