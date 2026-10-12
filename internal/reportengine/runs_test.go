// runs_test.go tests run selection and the batten span over a fake records repository.

package reportengine

import (
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// wantRun is what a row expects of one full run: its tag and ISO week, whether it holds the batten source and the history length of each batten snapshot, oldest first.
type wantRun struct {
	tag       string
	week      string
	batten    bool
	snapshots []int
}

// expectation is what a row expects of a report; a finding is compared by tag and file.
type expectation struct {
	runs     []wantRun
	partial  []string
	findings []Finding
}

func tagName(slug, sha string) string {
	return fabricengine.ArchiveTagPrefix + slug + "/" + sha
}

func TestCompute_SelectsRuns(t *testing.T) {
	t.Parallel()
	create, runShed, land := battenrecipe.NameWorktreeCreate, battenrecipe.NameRunShed, battenrecipe.NameLand

	tests := []struct {
		name  string
		since time.Time
		setup func(w *world) expectation
	}{
		{
			name:  "no archive tags yield an empty report",
			setup: func(w *world) expectation { return expectation{} },
		},
		{
			name: "a tag with the prefix and no slug and sha segments is a finding and no run",
			setup: func(w *world) expectation {
				w.tags = append(w.tags, gitrepo.TagRef{Name: "archive/x", Commit: "unread", Committed: at(1)})
				return expectation{findings: []Finding{{Tag: "archive/x"}}}
			},
		},
		{
			name: "a slug with two tags yields two runs, each with the batten snapshots committed before its tag",
			setup: func(w *world) expectation {
				w.battenStatus("b1", 0, "reused", create)
				w.battenStatus("b2", 1, "reused", create, runShed)
				w.finishedRun("reused", "t1", 2)
				w.battenStatus("b3", 3, "reused", create)
				w.battenStatus("b4", 4, "reused", create, runShed, land)
				w.finishedRun("reused", "t2", 5)
				return expectation{runs: []wantRun{
					{tag: tagName("reused", "t1"), week: "2026-W02", batten: true, snapshots: []int{1, 2}},
					{tag: tagName("reused", "t2"), week: "2026-W02", batten: true, snapshots: []int{1, 3}},
				}}
			},
		},
		{
			name: "a run batten never seeded reports the batten source absent and takes no snapshots of the earlier run",
			setup: func(w *world) expectation {
				w.battenStatus("b1", 0, "reused", create)
				w.battenStatus("b2", 1, "reused", create, runShed)
				w.finishedRun("reused", "t1", 2)
				w.finishedRun("reused", "t2", 5)
				return expectation{runs: []wantRun{
					{tag: tagName("reused", "t1"), week: "2026-W02", batten: true, snapshots: []int{1, 2}},
					{tag: tagName("reused", "t2"), week: "2026-W02"},
				}}
			},
		},
		{
			name: "a status a later commit deleted is read from the last commit that held it",
			setup: func(w *world) expectation {
				w.battenStatus("b1", 0, "gone", create)
				w.battenStatus("b2", 1, "gone", create, runShed)
				w.battenDeleted("b3", 2, "gone")
				w.finishedRun("gone", "t1", 3)
				return expectation{runs: []wantRun{{tag: tagName("gone", "t1"), week: "2026-W02", batten: true, snapshots: []int{1, 2}}}}
			},
		},
		{
			name: "a loom run that never finalized and a run of another recipe are partial and in no run",
			setup: func(w *world) expectation {
				w.tag("parked", "t1", at(1), seedText("loom"), statusText(w.t, "Plan-Write", "Discussion-Write"))
				w.tag("darned", "t2", at(2), seedText("darn"), "")
				return expectation{partial: []string{tagName("parked", "t1"), tagName("darned", "t2")}}
			},
		},
		{
			name: "a status that does not decode is a finding and the tag is partial",
			setup: func(w *world) expectation {
				w.tag("broken", "t1", at(1), seedText("loom"), "{not json")
				return expectation{
					partial:  []string{tagName("broken", "t1")},
					findings: []Finding{{Tag: tagName("broken", "t1"), File: recordPaths(w.t, "broken").Status}},
				}
			},
		},
		{
			name: "a seed and a status carrying unknown keys read and the run is full",
			setup: func(w *world) expectation {
				w.tag("newer", "t1", at(1),
					`{"recipe":"loom","driver":"go","added_later":1}`,
					`{"state":"done","added_later":true,"history":[{"producer":"Finalize","outcome":"done","output":"","at":"","added_later":2}]}`)
				return expectation{runs: []wantRun{{tag: tagName("newer", "t1"), week: "2026-W02"}}}
			},
		},
		{
			name:  "a tag committed before the since date is in neither runs nor partial runs",
			since: at(100),
			setup: func(w *world) expectation {
				w.finishedRun("early", "t1", 3)
				w.tag("early-partial", "t2", at(4), seedText("darn"), "")
				return expectation{}
			},
		},
		{
			name: "ISO weeks start on Monday in UTC",
			setup: func(w *world) expectation {
				sundayEnd := time.Date(2026, 1, 11, 23, 59, 59, 0, time.UTC)
				mondayStart := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)
				mondayEarlyEast := time.Date(2026, 1, 19, 0, 30, 0, 0, time.FixedZone("east", 2*60*60))
				for i, when := range []time.Time{sundayEnd, mondayStart, mondayEarlyEast} {
					slug := []string{"sunday", "monday", "east"}[i]
					w.tag(slug, "t"+slug, when, seedText("loom"), statusText(w.t, "Finalize"))
				}
				return expectation{runs: []wantRun{
					{tag: tagName("sunday", "tsunday"), week: "2026-W02"},
					{tag: tagName("monday", "tmonday"), week: "2026-W03"},
					{tag: tagName("east", "teast"), week: "2026-W03"},
				}}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			want := tt.setup(w)

			report, err := Compute(w.sources(), Options{Since: tt.since})
			if err != nil {
				t.Fatalf("Compute error = %v", err)
			}

			if len(report.Runs) != len(want.runs) {
				t.Fatalf("runs = %+v; want %d", report.Runs, len(want.runs))
			}
			for i, wantRun := range want.runs {
				run := report.Runs[i]
				var snapshots []int
				for _, snapshot := range run.battenSnapshots {
					snapshots = append(snapshots, len(snapshot.History))
				}
				if run.Tag != wantRun.tag || run.Week != wantRun.week || run.Sources[SourceBatten] != wantRun.batten || !slices.Equal(snapshots, wantRun.snapshots) {
					t.Errorf("run %d = (%s, %s, batten %v, snapshots %v); want (%s, %s, batten %v, snapshots %v)",
						i, run.Tag, run.Week, run.Sources[SourceBatten], snapshots, wantRun.tag, wantRun.week, wantRun.batten, wantRun.snapshots)
				}
			}

			var partial []string
			for _, p := range report.Partial {
				partial = append(partial, p.Tag)
				if p.Reason == "" {
					t.Errorf("partial run %s has no reason", p.Tag)
				}
			}
			if !slices.Equal(partial, want.partial) {
				t.Errorf("partial runs = %v; want %v", partial, want.partial)
			}

			if len(report.Findings) != len(want.findings) {
				t.Fatalf("findings = %+v; want %d", report.Findings, len(want.findings))
			}
			for i, wantFinding := range want.findings {
				if got := report.Findings[i]; got.Tag != wantFinding.Tag || got.File != wantFinding.File || got.Error == "" {
					t.Errorf("finding %d = %+v; want tag %q, file %q and an error", i, got, wantFinding.Tag, wantFinding.File)
				}
			}
		})
	}
}
