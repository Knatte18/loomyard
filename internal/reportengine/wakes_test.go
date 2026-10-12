// wakes_test.go tests the wake sources, automatic steps, alignment outcomes, stops and failure classes of a run over a fake records repository.

package reportengine

import (
	"encoding/json"
	"path"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/haltnote"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// wakeSlug is the slug of the one run each wakes row builds.
const wakeSlug = "r"

// figures are the wake and stop figures of one run.
type figures struct {
	wakes     map[string]*WakeSource
	automatic map[string]int
	paused    *int
	pings     *int
	alignment []AlignmentOutcome
	stops     []Stop
	failures  map[string]int
}

func figuresOf(run RunRow) figures {
	return figures{run.Wakes, run.Automatic, run.Paused, run.ParentReviewPings, run.Alignment, run.Stops, run.Failures}
}

func (f figures) String() string {
	data, _ := json.Marshal(map[string]any{
		"wakes": f.wakes, "automatic": f.automatic, "paused": f.paused, "pings": f.pings,
		"alignment": f.alignment, "stops": f.stops, "failures": f.failures,
	})
	return string(data)
}

// sourceOf is a wake source holding kinds, its total the sum of their counts.
func sourceOf(kinds map[string]int) *WakeSource {
	source := &WakeSource{Kinds: kinds}
	for _, count := range kinds {
		source.Total += count
	}
	return source
}

// wakesOf is the Wakes map of a run holding the given sources; land-ready-tasks is always absent.
func wakesOf(batten, parentReview, alignment *WakeSource) map[string]*WakeSource {
	return map[string]*WakeSource{SourceBatten: batten, SourceParentReview: parentReview, SourceAlignment: alignment, SourceLandReadyTasks: nil}
}

func intPtr(n int) *int { return &n }

// wakeRun adds the one full run of slug wakeSlug that a wakes row builds.
func (w *world) wakeRun(snapshots []shedengine.Status, entries []shedengine.HistoryEntry, extra map[string]string) {
	w.fullRun(wakeSlug, "t1", 100, snapshots, entries, extra)
}

func frictionDirRel() string { return filepath.ToSlash(loomengine.LoomFrictionDirRel()) }

func TestCompute_WakesAndStops(t *testing.T) {
	t.Parallel()
	running, awaiting, blocked, failed := shedengine.StateRunning, shedengine.StateAwaiting, shedengine.StateBlocked, shedengine.StateFailed
	archiveDir := filepath.Base(frictionDirRel()) + "-20261012T101500Z"

	type row struct {
		name     string
		setup    func(w *world)
		want     figures
		findings []Finding
	}
	tests := []row{
		{
			name: "a Bouncer halt whose segment holds an escalation file is a review-escalation wake",
			setup: func(w *world) {
				w.wakeRun(
					[]shedengine.Status{battenStatusOf(awaiting, "", stateChange(running, awaiting, "Plan-Bouncer", 2))},
					nil,
					reviewFiles("plan", "round-1-review.md", "round-1-escalation.md"))
			},
			want: figures{wakes: wakesOf(sourceOf(map[string]int{"review-escalation": 1}), nil, nil), automatic: map[string]int{}, paused: intPtr(0)},
		},
		{
			name: "a Bouncer halt with no escalation file in its segment is an awaiting wake, and a run lacking the parent-review directory reports that source absent beside its alignment source",
			setup: func(w *world) {
				w.wakeRun(
					[]shedengine.Status{battenStatusOf(awaiting, "", stateChange(running, awaiting, "Plan-Bouncer", 2))},
					nil,
					mergeFiles(reviewFiles("plan", "round-1-review.md"),
						map[string]string{recordPaths(w.t, wakeSlug).Alignment + "/pr-gate/attempt-1/attempt.json": `{"point":"pr-gate","token":"x","applied":""}`}))
			},
			want: figures{
				wakes:     wakesOf(sourceOf(map[string]int{"awaiting at Plan-Bouncer": 1}), nil, sourceOf(map[string]int{})),
				automatic: map[string]int{},
				paused:    intPtr(0),
				alignment: []AlignmentOutcome{{Point: "pr-gate", Verdict: "none", Count: 1}},
			},
		},
		{
			name: "an escalation file in the archived sibling of the segment counts while the live segment holds none",
			setup: func(w *world) {
				w.wakeRun(
					[]shedengine.Status{battenStatusOf(awaiting, "", stateChange(running, awaiting, "Plan-Bouncer", 2))},
					nil,
					mergeFiles(reviewFiles("plan", "round-1-review.md"), reviewFiles("plan-20261012T101500Z", "round-1-review.md", "round-1-escalation.md")))
			},
			want: figures{wakes: wakesOf(sourceOf(map[string]int{"review-escalation": 1}), nil, nil), automatic: map[string]int{}, paused: intPtr(0)},
		},
		{
			name: "the gate halt, a Publish verify retry and a Land failure episode show one wake each of pr-gate and failed at Land and one automatic step",
			setup: func(w *world) {
				opening := battenStatusOf(running, "",
					stateChange(running, blocked, "Publish", 4),
					stateChange(running, awaiting, "PR-Gate", 10))
				episode := opening
				episode.State = failed
				episode.CurrentProducer = "Land"
				episode.History = append(append([]shedengine.HistoryEntry{}, opening.History...),
					shedengine.HistoryEntry{Producer: "Land", Outcome: shedengine.Done, At: at(12).Format(time.RFC3339)})
				w.wakeRun(
					[]shedengine.Status{opening, episode},
					[]shedengine.HistoryEntry{{Producer: "Publish", Outcome: shedengine.OutcomeGoto, At: at(5).Format(time.RFC3339)}},
					nil)
			},
			want: figures{
				wakes:     wakesOf(sourceOf(map[string]int{"pr-gate": 1, "failed at Land": 1}), nil, nil),
				automatic: map[string]int{"verify-retry": 1},
				paused:    intPtr(0),
			},
		},
		{
			name: "a change into paused is counted as paused and is no wake",
			setup: func(w *world) {
				w.wakeRun([]shedengine.Status{battenStatusOf(running, "", stateChange(running, shedengine.StatePaused, "Plan-Write", 2))}, nil, nil)
			},
			want: figures{wakes: wakesOf(sourceOf(map[string]int{}), nil, nil), automatic: map[string]int{}, paused: intPtr(1)},
		},
		{
			name: "a round settled by an applied alignment verdict is no wake, a delivered fallback is one, its pings add none and an undelivered round is none",
			setup: func(w *world) {
				w.wakeRun(nil, nil, mergeFiles(
					parentRoundFiles(1, true, 0, "applied", ""),
					parentRoundFiles(2, true, 2, "fallback", ""),
					parentRoundFiles(3, false, 0, "", "")))
			},
			want: figures{wakes: wakesOf(nil, sourceOf(map[string]int{"fallback": 1}), nil), pings: intPtr(2)},
		},
		{
			name: "an applied approve naming superseded entries is one alignment wake, and each point and verdict shows its own outcome",
			setup: func(w *world) {
				w.wakeRun(nil, nil, mergeFiles(
					alignmentAttemptFiles(w.t, wakeSlug, "parent-review", 1, "approve", "approve", "alpha", "beta"),
					alignmentAttemptFiles(w.t, wakeSlug, "pr-gate", 1, "reject", "reject"),
					alignmentAttemptFiles(w.t, wakeSlug, "pr-gate", 2, "", "escalate"),
					alignmentAttemptFiles(w.t, wakeSlug, "pr-gate", 3, "", "")))
			},
			want: figures{
				wakes: wakesOf(nil, nil, sourceOf(map[string]int{"approve-superseding": 1})),
				alignment: []AlignmentOutcome{
					{Point: "parent-review", Verdict: "approve", Count: 1, Applied: 1},
					{Point: "pr-gate", Verdict: "escalate", Count: 1},
					{Point: "pr-gate", Verdict: "none", Count: 1},
					{Point: "pr-gate", Verdict: "reject", Count: 1, Applied: 1},
				},
			},
		},
		{
			name: "an awaiting stop is classed awaiting and a failed change with its note is one stop with a failure class",
			setup: func(w *world) {
				w.wakeRun(
					[]shedengine.Status{battenStatusOf(failed, "Publish",
						stateChange(running, awaiting, "PR-Gate", 2),
						stateChange(running, failed, "Publish", 3))},
					nil,
					haltNoteFiles("", "loom-halt.md", haltnote.Note{
						Producer: "Publish", State: "failed", Anomaly: "status-missing",
						Reason: "publish_verify failed after merging parent branch: conflict in 3 files",
					}))
			},
			want: figures{
				wakes:     wakesOf(sourceOf(map[string]int{"pr-gate": 1, "failed at Publish": 1}), nil, nil),
				automatic: map[string]int{},
				paused:    intPtr(0),
				stops: []Stop{
					{Class: "awaiting", State: "awaiting", Producer: "PR-Gate", Change: true},
					{Class: "status-missing", State: "failed", Producer: "Publish", Failure: "Publish: publish_verify failed after merging parent branch", Change: true, Note: path.Join(frictionDirRel(), "loom-halt.md")},
				},
				failures: map[string]int{"Publish: publish_verify failed after merging parent branch": 1},
			},
		},
		{
			name: "a blocked stop takes its class from the note's anomaly, with the note's state and producer",
			setup: func(w *world) {
				w.wakeRun(
					[]shedengine.Status{battenStatusOf(blocked, "Plan-Write", stateChange(running, blocked, "Plan-Write", 2))},
					nil,
					haltNoteFiles("", "loom-halt.md", haltnote.Note{Producer: "Plan-Write", State: "blocked", Anomaly: "watchdog", Reason: "no output for 30 minutes"}))
			},
			want: figures{
				wakes:     wakesOf(sourceOf(map[string]int{"blocked at Plan-Write": 1}), nil, nil),
				automatic: map[string]int{},
				paused:    intPtr(0),
				stops:     []Stop{{Class: "watchdog", State: "blocked", Producer: "Plan-Write", Failure: "Plan-Write: no output for <n> minutes", Change: true, Note: path.Join(frictionDirRel(), "loom-halt.md")}},
				failures:  map[string]int{"Plan-Write: no output for <n> minutes": 1},
			},
		},
		{
			name: "halts differing only in a path, a number or a quoted string share a failure class, as do halts sharing the text before the first colon, semicolon or parenthesis",
			setup: func(w *world) {
				note := func(reason string) haltnote.Note {
					return haltnote.Note{Producer: "Plan-Write", State: "failed", Anomaly: "stall", Reason: reason}
				}
				w.wakeRun(nil, nil, mergeFiles(
					haltNoteFiles("", "loom-halt.md", note(`read "a.txt" at /x/y/z line 12: no such file`)),
					haltNoteFiles("", "loom-halt-2.md", note(`read "b.txt" at /p/q line 7: permission denied`)),
					haltNoteFiles("", "loom-halt-3.md", note("review timed out; round 3")),
					haltNoteFiles("", "loom-halt-4.md", note("review timed out (limit 5)"))))
			},
			want: figures{
				stops: []Stop{
					{Class: "stall", State: "failed", Producer: "Plan-Write", Failure: "Plan-Write: read <str> at <path> line <n>", Note: path.Join(frictionDirRel(), "loom-halt.md")},
					{Class: "stall", State: "failed", Producer: "Plan-Write", Failure: "Plan-Write: read <str> at <path> line <n>", Note: path.Join(frictionDirRel(), "loom-halt-2.md")},
					{Class: "stall", State: "failed", Producer: "Plan-Write", Failure: "Plan-Write: review timed out", Note: path.Join(frictionDirRel(), "loom-halt-3.md")},
					{Class: "stall", State: "failed", Producer: "Plan-Write", Failure: "Plan-Write: review timed out", Note: path.Join(frictionDirRel(), "loom-halt-4.md")},
				},
				failures: map[string]int{"Plan-Write: read <str> at <path> line <n>": 2, "Plan-Write: review timed out": 2},
			},
		},
		{
			name: "a failed change with no note in a run holding notes is a no-note stop, and a note left after matching is a stop of its own",
			setup: func(w *world) {
				w.wakeRun(
					[]shedengine.Status{battenStatusOf(blocked, "Plan-Write",
						stateChange(running, failed, "Publish", 2),
						stateChange(running, blocked, "Plan-Write", 3))},
					nil,
					mergeFiles(
						haltNoteFiles("", "loom-halt.md", haltnote.Note{Producer: "Plan-Write", State: "blocked", Anomaly: "watchdog", Reason: "idle"}),
						haltNoteFiles("", "loom-halt-2.md", haltnote.Note{Producer: "Land", State: "failed", Anomaly: "crash", Reason: "died"})))
			},
			want: figures{
				wakes:     wakesOf(sourceOf(map[string]int{"failed at Publish": 1, "blocked at Plan-Write": 1}), nil, nil),
				automatic: map[string]int{},
				paused:    intPtr(0),
				stops: []Stop{
					{Class: "no-note", State: "failed", Producer: "Publish", Change: true},
					{Class: "watchdog", State: "blocked", Producer: "Plan-Write", Failure: "Plan-Write: idle", Change: true, Note: path.Join(frictionDirRel(), "loom-halt.md")},
					{Class: "crash", State: "failed", Producer: "Land", Failure: "Land: died", Note: path.Join(frictionDirRel(), "loom-halt-2.md")},
				},
				failures: map[string]int{"Plan-Write: idle": 1, "Land: died": 1},
			},
		},
		{
			name: "a run whose only notes sit in an archive sibling reports them",
			setup: func(w *world) {
				w.wakeRun(nil, nil, haltNoteFiles(archiveDir, "loom-halt.md", haltnote.Note{Producer: "Land", State: "failed", Anomaly: "crash", Reason: "died"}))
			},
			want: figures{
				stops:    []Stop{{Class: "crash", State: "failed", Producer: "Land", Failure: "Land: died", Note: path.Join(filepath.ToSlash(loomengine.LoomDurableDirRel()), archiveDir, "loom-halt.md")}},
				failures: map[string]int{"Land: died": 1},
			},
		},
		{
			name: "a halt note haltnote refuses is a finding naming the tag and the note, and the run's other stops stand",
			setup: func(w *world) {
				w.wakeRun(nil, nil, mergeFiles(
					map[string]string{path.Join(frictionDirRel(), "loom-halt.md"): "not a halt note"},
					haltNoteFiles("", "loom-halt-2.md", haltnote.Note{Producer: "Land", State: "failed", Anomaly: "crash", Reason: "died"})))
			},
			want: figures{
				stops:    []Stop{{Class: "crash", State: "failed", Producer: "Land", Failure: "Land: died", Note: path.Join(frictionDirRel(), "loom-halt-2.md")}},
				failures: map[string]int{"Land: died": 1},
			},
			findings: []Finding{{Tag: tagName(wakeSlug, "t1"), File: path.Join(frictionDirRel(), "loom-halt.md")}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			tt.setup(w)
			if tt.want.wakes == nil {
				tt.want.wakes = wakesOf(nil, nil, nil)
			}

			report, err := Compute(w.sources(), Options{})
			if err != nil {
				t.Fatalf("Compute error = %v", err)
			}

			if len(report.Runs) != 1 {
				t.Fatalf("runs = %d; want 1 (partial %+v, findings %+v)", len(report.Runs), report.Partial, report.Findings)
			}
			if got := figuresOf(report.Runs[0]); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("figures =\n%s\nwant\n%s", got, tt.want)
			}
			if len(report.Findings) != len(tt.findings) {
				t.Fatalf("findings = %+v; want %d", report.Findings, len(tt.findings))
			}
			for i, want := range tt.findings {
				if got := report.Findings[i]; got.Tag != want.Tag || got.File != want.File || got.Error == "" {
					t.Errorf("finding %d = %+v; want tag %q, file %q and an error", i, got, want.Tag, want.File)
				}
			}
		})
	}
}
