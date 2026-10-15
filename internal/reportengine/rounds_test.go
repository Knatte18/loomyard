// rounds_test.go tests the repair split, review rounds and parent decisions of a run over a fake records repository.

package reportengine

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/haltnote"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// roundFigures are the repair and review figures of one run.
type roundFigures struct {
	arbiter   []ArbiterOutcome
	llm       map[string]int
	drive     *int
	segments  map[string]*SegmentRounds
	decisions map[string]int
}

func roundFiguresOf(run RunRow) roundFigures {
	return roundFigures{run.Arbiter, run.LLMSteps, run.DriveReports, run.Segments, run.Decisions}
}

// arbiterAt is an arbiter attempt that stopped at producer in state, spawned at hour.
func arbiterAt(producer, state string, hour int) loomengine.ArbiterAttempt {
	return loomengine.ArbiterAttempt{StopKey: sheddriver.StopKey{CurrentProducer: producer, State: state}, SpawnedAt: at(hour)}
}

func TestCompute_RepairsAndRounds(t *testing.T) {
	t.Parallel()
	running, blocked, failed := shedengine.StateRunning, shedengine.StateBlocked, shedengine.StateFailed
	hourAt := func(hour int) string { return at(hour).Format(time.RFC3339) }

	tests := []struct {
		name     string
		setup    func(w *world) map[string]roundFigures
		findings []Finding
	}{
		{
			name: "a resume backed by an applied arbiter verdict is an arbiter repair and no LLM step, and an attempt not applied shows as such",
			setup: func(w *world) map[string]roundFigures {
				w.fullRun("a", "ta", 100,
					[]shedengine.Status{battenStatusOf(running, "",
						stateChange(running, failed, "Publish", 2),
						stateChange(failed, running, "Publish", 4))},
					nil,
					mergeFiles(
						arbiterAttemptFiles(w.t, "a", 1, arbiterAt("Publish", "failed", 3), "verdict: resume\nreason: r\n", true),
						arbiterAttemptFiles(w.t, "a", 2, arbiterAt("Publish", "failed", 5), "verdict: hand-on\nreason: r\nquestion: q\n", false)))
				return map[string]roundFigures{"a": {
					arbiter:  []ArbiterOutcome{{Verdict: "hand-on", Count: 1}, {Verdict: "resume", Count: 1, Applied: 1}},
					llm:      map[string]int{"loom": 0, "batten": 0, "landing": 0},
					segments: map[string]*SegmentRounds{},
				}}
			},
		},
		{
			name: "a goto, a reroute and a resume after a halt are each an LLM step, and the goto of a verify retry and the resume it ends in are none",
			setup: func(w *world) map[string]roundFigures {
				w.fullRun("a", "ta", 100,
					[]shedengine.Status{battenStatusOf(running, "",
						stateChange(running, blocked, "Publish", 4),
						stateChange(blocked, running, "Publish", 6),
						stateChange(running, failed, "Plan-Write", 7),
						stateChange(failed, running, "Plan-Write", 8))},
					[]shedengine.HistoryEntry{
						{Producer: "Publish", Outcome: shedengine.OutcomeGoto, At: hourAt(5)},
						{Producer: "Plan-Write", Outcome: shedengine.OutcomeGoto, At: hourAt(30)},
						{Producer: "Plan-Review", Outcome: shedengine.OutcomeReroute, At: hourAt(31)},
					}, nil)
				return map[string]roundFigures{"a": {llm: map[string]int{"loom": 2, "batten": 1, "landing": 0}, segments: map[string]*SegmentRounds{}}}
			},
		},
		{
			name: "an applied arbiter verdict carrying a goto accounts for the first goto to that row after its spawn",
			setup: func(w *world) map[string]roundFigures {
				w.fullRun("a", "ta", 100, nil,
					[]shedengine.HistoryEntry{
						{Producer: "Plan-Write", Outcome: shedengine.OutcomeGoto, At: hourAt(2)},
						{Producer: "Plan-Write", Outcome: shedengine.OutcomeGoto, At: hourAt(30)},
					},
					arbiterAttemptFiles(w.t, "a", 1, arbiterAt("Publish", "failed", 10), "verdict: resume\nreason: r\ngoto: Plan-Write\n", true))
				return map[string]roundFigures{"a": {
					arbiter:  []ArbiterOutcome{{Verdict: "resume", Count: 1, Applied: 1}},
					llm:      map[string]int{"loom": 1, "landing": 0},
					segments: map[string]*SegmentRounds{},
				}}
			},
		},
		{
			name: "decisions are counted per author, drive reports per file, and a run without the drive-reports directory reports them absent",
			setup: func(w *world) map[string]roundFigures {
				w.fullRun("a", "ta", 100, nil, nil, mergeFiles(
					decisionRecordFiles(addedDecision("alice", "one"), addedDecision("alice", "two"), addedDecision("bob", "three")),
					driveReportFiles(w.t, "a", 2)))
				w.fullRun("b", "tb", 101, nil, nil, nil)
				return map[string]roundFigures{
					"a": {llm: map[string]int{"loom": 0, "landing": 0}, drive: intPtr(2), segments: map[string]*SegmentRounds{}, decisions: map[string]int{"alice": 2, "bob": 1}},
					"b": {llm: map[string]int{"loom": 0, "landing": 0}, segments: map[string]*SegmentRounds{}},
				}
			},
		},
		{
			name: "a malformed added-decision heading is a finding naming the tag and the record, and the run's other counts stand",
			setup: func(w *world) map[string]roundFigures {
				w.fullRun("a", "ta", 100, nil, nil, mergeFiles(
					decisionRecordFiles("### Added after Discussion (broken"),
					driveReportFiles(w.t, "a", 1)))
				return map[string]roundFigures{"a": {llm: map[string]int{"loom": 0, "landing": 0}, drive: intPtr(1), segments: map[string]*SegmentRounds{}}}
			},
			findings: []Finding{{Tag: tagName("a", "ta"), File: filepath.ToSlash(loomengine.DiscussionDecisionRecordRel())}},
		},
		{
			name: "a segment's outcome comes from its last round, only a round with a review file counts, and a run lacking a segment another holds reports it absent",
			setup: func(w *world) map[string]roundFigures {
				w.fullRun("a", "ta", 100, nil, nil, mergeFiles(
					reviewFiles("alpha", "round-1-review.md", "round-2-review.md", "round-2-escalation.md"),
					reviewFiles("beta", "round-1-review.md", "round-1-circling-decision.md", "round-1-escalation.md"),
					reviewFiles("gamma", "round-1-review.md"),
					reviewFiles("delta", "round-1-review.md", "round-2-focus.md")))
				w.fullRun("b", "tb", 101, nil, nil, reviewFiles("alpha", "round-1-review.md"))
				return map[string]roundFigures{
					"a": {llm: map[string]int{"loom": 0, "landing": 0}, segments: map[string]*SegmentRounds{
						"alpha": {Rounds: 2, Outcome: "escalated"},
						"beta":  {Rounds: 1, Outcome: "circling-decided"},
						"gamma": {Rounds: 1, Outcome: "accepted"},
						"delta": {Rounds: 1, Outcome: "accepted"},
					}},
					"b": {llm: map[string]int{"loom": 0, "landing": 0}, segments: map[string]*SegmentRounds{
						"alpha": {Rounds: 1, Outcome: "accepted"},
						"beta":  nil,
						"gamma": nil,
						"delta": nil,
					}},
				}
			},
		},
		{
			name: "an archived generation of a segment adds its rounds to the segment and is no segment of its own",
			setup: func(w *world) map[string]roundFigures {
				w.fullRun("a", "ta", 100, nil, nil, mergeFiles(
					reviewFiles("plan", "round-1-review.md"),
					reviewFiles("plan-20261012T101500Z", "round-1-review.md", "round-2-review.md")))
				return map[string]roundFigures{"a": {llm: map[string]int{"loom": 0, "landing": 0}, segments: map[string]*SegmentRounds{"plan": {Rounds: 3, Outcome: "accepted"}}}}
			},
		},
		{
			name: "a parent-review round takes its outcome from its verdict record, the last round's, and none when it has no verdict",
			setup: func(w *world) map[string]roundFigures {
				w.fullRun("a", "ta", 100, nil, nil, mergeFiles(
					parentRoundFiles(1, true, 0, "", "reject"),
					parentRoundFiles(2, true, 0, "", "approve")))
				w.fullRun("b", "tb", 101, nil, nil, parentRoundFiles(1, true, 0, "", ""))
				return map[string]roundFigures{
					"a": {llm: map[string]int{"loom": 0, "landing": 0}, segments: map[string]*SegmentRounds{"parent-review": {Rounds: 2, Outcome: "approve"}}},
					"b": {llm: map[string]int{"loom": 0, "landing": 0}, segments: map[string]*SegmentRounds{"parent-review": {Rounds: 1, Outcome: "none"}}},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			want := tt.setup(w)

			report, err := Compute(w.sources(), Options{})
			if err != nil {
				t.Fatalf("Compute error = %v", err)
			}

			if len(report.Runs) != len(want) {
				t.Fatalf("runs = %d; want %d (partial %+v, findings %+v)", len(report.Runs), len(want), report.Partial, report.Findings)
			}
			for _, run := range report.Runs {
				if got := roundFiguresOf(run); !reflect.DeepEqual(got, want[run.Slug]) {
					t.Errorf("run %s figures = %+v\nwant %+v", run.Slug, got, want[run.Slug])
				}
			}
			if len(report.Findings) != len(tt.findings) {
				t.Fatalf("findings = %+v; want %d", report.Findings, len(tt.findings))
			}
			for i, wantFinding := range tt.findings {
				if got := report.Findings[i]; got.Tag != wantFinding.Tag || got.File != wantFinding.File || got.Error == "" {
					t.Errorf("finding %d = %+v; want tag %q, file %q and an error", i, got, wantFinding.Tag, wantFinding.File)
				}
			}
		})
	}
}

func TestCompute_FullRunCounts(t *testing.T) {
	t.Parallel()
	running, awaiting, failed := shedengine.StateRunning, shedengine.StateAwaiting, shedengine.StateFailed

	w := newWorld(t)
	w.fullRun("full", "tfull", 100,
		[]shedengine.Status{battenStatusOf(awaiting, "",
			stateChange(running, awaiting, "Plan-Bouncer", 2),
			stateChange(running, failed, "Publish", 3),
			stateChange(failed, running, "Publish", 4),
			stateChange(running, awaiting, "PR-Gate", 5))},
		[]shedengine.HistoryEntry{
			{Producer: "Plan-Write", Outcome: shedengine.OutcomeGoto, At: at(30).Format(time.RFC3339)},
			{Producer: "Plan-Review", Outcome: shedengine.OutcomeGoto, At: at(31).Format(time.RFC3339)},
		},
		mergeFiles(
			arbiterAttemptFiles(t, "full", 1, arbiterAt("Publish", "failed", 3), "verdict: resume\nreason: r\ngoto: Plan-Write\n", true),
			driveReportFiles(t, "full", 2),
			alignmentAttemptFiles(t, "full", "pr-gate", 1, "approve", "approve", "alpha"),
			parentRoundFiles(1, true, 1, "fallback", "approve"),
			reviewFiles("plan", "round-1-review.md", "round-1-escalation.md"),
			decisionRecordFiles(addedDecision("alice", "one")),
			haltNoteFiles("", "loom-halt.md", haltnote.Note{Producer: "Publish", State: "failed", Anomaly: "crash", Reason: "died"})))

	report, err := Compute(w.sources(), Options{})
	if err != nil {
		t.Fatalf("Compute error = %v", err)
	}
	if len(report.Runs) != 1 || len(report.Findings) != 0 {
		t.Fatalf("runs = %d, findings = %+v; want 1 run and no finding", len(report.Runs), report.Findings)
	}
	run := report.Runs[0]

	wakeTotals := map[string]int{}
	for source, wakes := range run.Wakes {
		if wakes != nil {
			wakeTotals[source] = wakes.Total
		}
	}
	got := map[string]any{
		"wakes":      wakeTotals,
		"automatic":  run.Automatic,
		"paused":     *run.Paused,
		"pings":      *run.ParentReviewPings,
		"alignment":  run.Alignment,
		"stops":      len(run.Stops),
		"failures":   run.Failures,
		"arbiter":    run.Arbiter,
		"llm":        run.LLMSteps,
		"drive":      *run.DriveReports,
		"segments":   run.Segments,
		"decisions":  run.Decisions,
		"findingCnt": len(report.Findings),
	}
	want := map[string]any{
		"wakes":      map[string]int{SourceBatten: 3, SourceParentReview: 1, SourceAlignment: 1},
		"automatic":  map[string]int{},
		"paused":     0,
		"pings":      1,
		"alignment":  []AlignmentOutcome{{Point: "pr-gate", Verdict: "approve", Count: 1, Applied: 1}},
		"stops":      3,
		"failures":   map[string]int{"Publish: died": 1},
		"arbiter":    []ArbiterOutcome{{Verdict: "resume", Count: 1, Applied: 1}},
		"llm":        map[string]int{"loom": 1, "batten": 0, "landing": 0},
		"drive":      2,
		"segments":   map[string]*SegmentRounds{"plan": {Rounds: 1, Outcome: "escalated"}, "parent-review": {Rounds: 1, Outcome: "approve"}},
		"decisions":  map[string]int{"alice": 1},
		"findingCnt": 0,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("counts =\n%+v\nwant\n%+v", got, want)
	}
}
