// landings_test.go tests the landing joins, kinds and the fresh-code rule over fake parent, records and board repositories.

package reportengine

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// wantLanding is what a row expects of one landing.
type wantLanding struct {
	commit, kind, slug, tag, join, fresh string
	hand                                 bool
	incomplete                           *bool
}

// landingExpectation is what a row expects of a report's landings.
// runLandings maps a run's tag to the commit its Landing names, empty for none, and landingSteps its tag to its landing LLM step.
type landingExpectation struct {
	landings     []wantLanding
	runLandings  map[string]string
	landingSteps map[string]int
	findings     []string
	blames       []blameCall
}

func boolPtr(b bool) *bool { return &b }

var (
	finalizeDone  = []shedengine.HistoryEntry{{Producer: loomshed.NameFinalize, Outcome: shedengine.Done}}
	finalizeStuck = []shedengine.HistoryEntry{{Producer: loomshed.NameFinalize, Outcome: shedengine.Stuck}}
)

// exactRun adds a full loom run of slug tagged sha at hour, whose records history ends in a commit naming task in its trailer and whose loom history is history.
func (w *world) exactRun(slug, sha string, hour int, task string, history []shedengine.HistoryEntry) {
	w.recordsCommit("r-"+sha, hour-1, task)
	w.tagOver(slug, sha, "r-"+sha, at(hour), seedText("loom"), statusJSON(w.t, shedengine.Status{State: shedengine.StateDone, History: history}), nil)
}

func TestCompute_Landings(t *testing.T) {
	t.Parallel()
	opts := Options{Since: at(-500), FreshDays: 3}

	tests := []struct {
		name  string
		setup func(w *world) landingExpectation
	}{
		{
			name: "a fix whose modified lines came from a landing two days earlier is fresh",
			setup: func(w *world) landingExpectation {
				w.parentCommit("L0", 0, "earlier")
				w.parentCommit("F", 48, "fix")
				w.code.preImages["F"] = map[string][]int{"a.go": {1, 2, 3, 4}}
				w.code.blames["L0 a.go"] = map[int]string{1: "L0", 2: "L0", 3: "L0", 4: "L0"}
				return landingExpectation{
					landings: []wantLanding{{commit: "L0", kind: "unmapped", fresh: "no-pre-image"}, {commit: "F", kind: "unmapped", fresh: "fresh"}},
					blames:   []blameCall{{rev: "L0", path: "a.go", lines: []int{1, 2, 3, 4}}},
				}
			},
		},
		{
			name: "a fix whose lines came from a landing ten days earlier is aged",
			setup: func(w *world) landingExpectation {
				w.parentCommit("L0", 0, "earlier")
				w.parentCommit("F", 240, "fix")
				w.code.preImages["F"] = map[string][]int{"a.go": {1, 2}}
				w.code.blames["L0 a.go"] = map[int]string{1: "L0", 2: "L0"}
				return landingExpectation{
					landings: []wantLanding{{commit: "L0", kind: "unmapped", fresh: "no-pre-image"}, {commit: "F", kind: "unmapped", fresh: "aged"}},
					blames:   []blameCall{{rev: "L0", path: "a.go", lines: []int{1, 2}}},
				}
			},
		},
		{
			name: "a fix with exactly half its lines from a recent landing is aged",
			setup: func(w *world) landingExpectation {
				w.parentCommit("L0", 0, "old")
				w.parentCommit("L1", 230, "recent")
				w.parentCommit("F", 240, "fix")
				w.code.preImages["F"] = map[string][]int{"a.go": {1, 2, 3, 4}}
				w.code.blames["L1 a.go"] = map[int]string{1: "L1", 2: "L1", 3: "L0", 4: "L0"}
				return landingExpectation{
					landings: []wantLanding{
						{commit: "L0", kind: "unmapped", fresh: "no-pre-image"},
						{commit: "L1", kind: "unmapped", fresh: "no-pre-image"},
						{commit: "F", kind: "unmapped", fresh: "aged"},
					},
					blames: []blameCall{{rev: "L1", path: "a.go", lines: []int{1, 2, 3, 4}}},
				}
			},
		},
		{
			name: "a fix that only adds lines, and one touching only a test file, a docs file and a markdown file, have no pre-image",
			setup: func(w *world) landingExpectation {
				w.parentCommit("F1", 10, "adds")
				w.parentCommit("F2", 20, "tests and docs")
				w.code.preImages["F2"] = map[string][]int{"a_test.go": {1}, "docs/x.go": {1}, "README.md": {2}}
				return landingExpectation{landings: []wantLanding{
					{commit: "F1", kind: "unmapped", fresh: "no-pre-image"},
					{commit: "F2", kind: "unmapped", fresh: "no-pre-image"},
				}}
			},
		},
		{
			name: "a fix's lines blamed on a task-branch commit belong to the fast-forward landing that brought it",
			setup: func(w *world) landingExpectation {
				w.parentCommit("A1", 5, "branch one")
				w.parentCommit("A2", 6, "branch two")
				w.parentCommit("A3", 82, "branch task commit")
				w.parentCommit("FX", 100, "fix")
				w.code.preImages["FX"] = map[string][]int{"a.go": {1}}
				w.code.blames["A3 a.go"] = map[int]string{1: "A2"}
				w.boardCommit("bd", 1, boardTask("ffs", "", "feature"), boardTask("fx", "", "bug"))
				w.exactRun("ffs", "tffs", 120, "A3", finalizeDone)
				w.exactRun("fx", "tfx", 121, "FX", finalizeDone)
				return landingExpectation{
					landings: []wantLanding{
						{commit: "A3", kind: "feature", slug: "ffs", tag: tagName("ffs", "tffs"), join: "exact"},
						{commit: "FX", kind: "fix", slug: "fx", tag: tagName("fx", "tfx"), join: "exact", fresh: "fresh"},
					},
					runLandings: map[string]string{tagName("ffs", "tffs"): "A3", tagName("fx", "tfx"): "FX"},
					blames:      []blameCall{{rev: "A3", path: "a.go", lines: []int{1}}},
				}
			},
		},
		{
			name: "a full run whose loom history holds Finalize without a done outcome is a hand landing with one landing LLM step",
			setup: func(w *world) landingExpectation {
				w.parentCommit("T1", 50, "land")
				w.boardCommit("bd", 1, boardTask("hand", "", "chore"))
				w.exactRun("hand", "th", 100, "T1", finalizeStuck)
				return landingExpectation{
					landings:     []wantLanding{{commit: "T1", kind: "other", slug: "hand", tag: tagName("hand", "th"), join: "exact", hand: true}},
					runLandings:  map[string]string{tagName("hand", "th"): "T1"},
					landingSteps: map[string]int{tagName("hand", "th"): 1},
				}
			},
		},
		{
			name: "a partial tag with no Finalize entry joined exactly is a hand landing carrying its tag and adds no run",
			setup: func(w *world) landingExpectation {
				w.parentCommit("T1", 50, "land")
				w.boardCommit("bd", 1, boardTask("partial", "", "chore"))
				w.recordsCommit("r-tp", 99, "T1")
				w.tagOver("partial", "tp", "r-tp", at(100), seedText("loom"), statusText(w.t, "Plan-Write"), nil)
				return landingExpectation{landings: []wantLanding{{commit: "T1", kind: "other", slug: "partial", tag: tagName("partial", "tp"), join: "exact", hand: true}}}
			},
		},
		{
			name: "a pair not yet torn down, on a slug with no archive tag, maps through its checkpoint and board done commit and adds no run",
			setup: func(w *world) landingExpectation {
				w.boardCommit("bd1", 10, boardTask("pending", "", "feature"))
				w.checkpoint("cp1", 30, "pending")
				w.parentCommit("L1", 35, "landed")
				w.boardCommit("bd2", 40, boardTask("pending", "done", "feature"))
				return landingExpectation{landings: []wantLanding{{commit: "L1", kind: "feature", slug: "pending", join: "time"}}}
			},
		},
		{
			name: "a slug with two runs maps each landing to its own run, and a run with two checkpoints opens its window at the newer",
			setup: func(w *world) landingExpectation {
				w.boardCommit("bd1", 5, boardTask("s", "", "feature"))
				w.checkpoint("cpA", 20, "s")
				w.parentCommit("P1", 25, "before the newer checkpoint")
				w.checkpoint("cpB", 30, "s")
				w.parentCommit("P2", 35, "first landing")
				w.fullRun("s", "ts1", 100, nil, nil, nil)
				w.boardCommit("bd2", 60, boardTask("s", "done", "feature"))
				w.checkpoint("cpC", 130, "s")
				w.parentCommit("P3", 140, "second landing")
				w.boardCommit("bd3", 150, boardTask("s", "done", "feature"))
				w.fullRun("s", "ts2", 200, nil, nil, nil)
				return landingExpectation{
					landings: []wantLanding{
						{commit: "P1", kind: "unmapped", fresh: "no-pre-image"},
						{commit: "P2", kind: "feature", slug: "s", tag: tagName("s", "ts1"), join: "time"},
						{commit: "P3", kind: "feature", slug: "s", tag: tagName("s", "ts2"), join: "time"},
					},
					runLandings: map[string]string{tagName("s", "ts1"): "P2", tagName("s", "ts2"): "P3"},
				}
			},
		},
		{
			name: "a run with no checkpoint in its interval, and a run whose entry was never marked done after its checkpoint, get no time join",
			setup: func(w *world) landingExpectation {
				w.checkpoint("cp1", 80, "notdone")
				w.parentCommit("Q1", 90, "unclaimed")
				w.boardCommit("bd1", 85, boardTask("notdone", "", "feature"))
				w.fullRun("nocp", "t1", 100, nil, nil, nil)
				w.fullRun("notdone", "t2", 110, nil, nil, nil)
				return landingExpectation{
					landings:    []wantLanding{{commit: "Q1", kind: "unmapped", fresh: "no-pre-image"}},
					runLandings: map[string]string{tagName("nocp", "t1"): "", tagName("notdone", "t2"): ""},
				}
			},
		},
		{
			name: "a fast-forwarded task branch of three commits is one landing and no unmapped commit, and nothing is blamed",
			setup: func(w *world) landingExpectation {
				w.parentCommit("A1", 20, "one")
				w.parentCommit("A2", 21, "two")
				w.parentCommit("A3", 22, "three")
				w.boardCommit("bd", 1, boardTask("branch", "", "feature"))
				w.exactRun("branch", "tb", 100, "A3", finalizeDone)
				return landingExpectation{
					landings:    []wantLanding{{commit: "A3", kind: "feature", slug: "branch", tag: tagName("branch", "tb"), join: "exact"}},
					runLandings: map[string]string{tagName("branch", "tb"): "A3"},
				}
			},
		},
		{
			name: "a run whose Finalize is done but whose batten record never reaches Land done is landing-incomplete, and one that does is not",
			setup: func(w *world) landingExpectation {
				w.parentCommit("T1", 60, "one")
				w.parentCommit("T2", 61, "two")
				w.boardCommit("bd", 1, boardTask("short", "", "chore"), boardTask("whole", "", "chore"))
				w.battenRecord("b-short", 50, "short", battenStatusOf(shedengine.StateDone, ""))
				w.battenRecord("b-whole", 51, "whole", battenStatusOf(shedengine.StateDone, "",
					shedengine.HistoryEntry{Producer: battenrecipe.NameLand, Outcome: shedengine.Done}))
				w.exactRun("short", "ts", 100, "T1", finalizeDone)
				w.exactRun("whole", "tw", 101, "T2", finalizeDone)
				return landingExpectation{
					landings: []wantLanding{
						{commit: "T1", kind: "other", slug: "short", tag: tagName("short", "ts"), join: "exact", incomplete: boolPtr(true)},
						{commit: "T2", kind: "other", slug: "whole", tag: tagName("whole", "tw"), join: "exact", incomplete: boolPtr(false)},
					},
					runLandings: map[string]string{tagName("short", "ts"): "T1", tagName("whole", "tw"): "T2"},
				}
			},
		},
		{
			name: "a task commit's trailer maps by merge parent three trailer-free commits below the tag",
			setup: func(w *world) landingExpectation {
				w.parentCommit("X", 10, "unrelated")
				w.taskCommit("T", 15)
				w.parentMerge("MG", 20, "T")
				w.boardCommit("bd", 1, boardTask("merged", "", "chore"))
				w.recordsCommit("r-trailer", 50, "T")
				for _, sha := range []string{"r-a", "r-b", "r-c"} {
					w.recordsCommit(sha, 60, "")
				}
				w.tagOver("merged", "tm", "r-c", at(100), seedText("loom"), statusJSON(w.t, shedengine.Status{History: finalizeDone}), nil)
				return landingExpectation{
					landings: []wantLanding{
						{commit: "X", kind: "unmapped", fresh: "no-pre-image"},
						{commit: "MG", kind: "other", slug: "merged", tag: tagName("merged", "tm"), join: "exact"},
					},
					runLandings: map[string]string{tagName("merged", "tm"): "MG"},
				}
			},
		},
		{
			name: "a task commit's trailer maps by tree to a squash commit, past more trailer-free commits than the first page reads",
			setup: func(w *world) landingExpectation {
				w.taskCommit("T", 15)
				w.code.setTree("T", "tree-shared")
				w.parentCommit("SQ", 20, "squash")
				w.code.setTree("SQ", "tree-shared")
				w.boardCommit("bd", 1, boardTask("squashed", "", "chore"))
				w.recordsCommit("r-trailer", 50, "T")
				last := "r-trailer"
				for i := 0; i < 70; i++ {
					last = "r-" + strings.Repeat("x", i+1)
					w.recordsCommit(last, 60, "")
				}
				w.tagOver("squashed", "tq", last, at(100), seedText("loom"), statusJSON(w.t, shedengine.Status{History: finalizeDone}), nil)
				return landingExpectation{
					landings:    []wantLanding{{commit: "SQ", kind: "other", slug: "squashed", tag: tagName("squashed", "tq"), join: "exact"}},
					runLandings: map[string]string{tagName("squashed", "tq"): "SQ"},
				}
			},
		},
		{
			name: "a commit another tagged run's exact join claimed is not taken by a time join",
			setup: func(w *world) landingExpectation {
				w.boardCommit("bd1", 1, boardTask("x", "", "chore"), boardTask("y", "", "chore"))
				w.checkpoint("cp", 40, "y")
				w.parentCommit("E", 50, "claimed by x")
				w.parentCommit("F", 55, "in y's window")
				w.boardCommit("bd2", 70, boardTask("y", "done", "chore"))
				w.exactRun("x", "tx", 100, "E", finalizeDone)
				w.fullRun("y", "ty", 110, nil, nil, nil)
				return landingExpectation{
					landings: []wantLanding{
						{commit: "E", kind: "other", slug: "x", tag: tagName("x", "tx"), join: "exact"},
						{commit: "F", kind: "other", slug: "y", tag: tagName("y", "ty"), join: "time"},
					},
					runLandings: map[string]string{tagName("x", "tx"): "E", tagName("y", "ty"): "F"},
				}
			},
		},
		{
			name: "two runs whose exact joins claim one commit are both unmapped and the clash is a finding",
			setup: func(w *world) landingExpectation {
				w.parentCommit("E", 50, "claimed twice")
				w.exactRun("x", "tx", 100, "E", finalizeDone)
				w.exactRun("y", "ty", 101, "E", finalizeDone)
				return landingExpectation{
					landings:    []wantLanding{{commit: "E", kind: "unmapped", fresh: "no-pre-image"}},
					runLandings: map[string]string{tagName("x", "tx"): "", tagName("y", "ty"): ""},
					findings:    []string{tagName("x", "tx")},
				}
			},
		},
		{
			name: "labels bug, enhancement and chore give fix, feature and other, read from the last board commit before the landing even after a later commit pruned the entry",
			setup: func(w *world) landingExpectation {
				w.boardCommit("bd1", 5, boardTask("b1", "", "bug"), boardTask("b2", "", "enhancement"), boardTask("b3", "", "chore"))
				w.parentCommit("T1", 50, "one")
				w.parentCommit("T2", 51, "two")
				w.parentCommit("T3", 52, "three")
				w.boardCommit("bd2", 90)
				w.exactRun("b1", "t1", 100, "T1", finalizeDone)
				w.exactRun("b2", "t2", 101, "T2", finalizeDone)
				w.exactRun("b3", "t3", 102, "T3", finalizeDone)
				return landingExpectation{landings: []wantLanding{
					{commit: "T1", kind: "fix", slug: "b1", tag: tagName("b1", "t1"), join: "exact", fresh: "no-pre-image"},
					{commit: "T2", kind: "feature", slug: "b2", tag: tagName("b2", "t2"), join: "exact"},
					{commit: "T3", kind: "other", slug: "b3", tag: tagName("b3", "t3"), join: "exact"},
				}}
			},
		},
		{
			name: "an exact-joined landing whose slug no board commit holds is kind other with a finding naming the slug and is not blamed",
			setup: func(w *world) landingExpectation {
				w.parentCommit("T1", 50, "ghost landing")
				w.exactRun("ghost", "tg", 100, "T1", finalizeDone)
				return landingExpectation{
					landings: []wantLanding{{commit: "T1", kind: "other", slug: "ghost", tag: tagName("ghost", "tg"), join: "exact"}},
					findings: []string{"ghost"},
				}
			},
		},
		{
			name: "a landing committed before the since date is left out",
			setup: func(w *world) landingExpectation {
				w.parentCommit("E", -520, "before since")
				w.parentCommit("F", 10, "after since")
				return landingExpectation{landings: []wantLanding{{commit: "F", kind: "unmapped", fresh: "no-pre-image"}}}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newWorld(t)
			want := tt.setup(w)

			report, err := Compute(w.sources(), opts)
			if err != nil {
				t.Fatalf("Compute error = %v", err)
			}

			var got []wantLanding
			for _, landing := range report.Landings {
				got = append(got, wantLanding{
					commit: landing.Commit, kind: landing.Kind, slug: landing.Slug, tag: landing.Tag, join: landing.Join,
					fresh: landing.Fresh, hand: landing.Hand, incomplete: landing.LandIncomplete,
				})
			}
			if !reflect.DeepEqual(got, want.landings) {
				t.Errorf("landings = %+v\nwant %+v", got, want.landings)
			}

			for _, run := range report.Runs {
				wantCommit, listed := want.runLandings[run.Tag]
				if listed {
					gotCommit := ""
					if run.Landing != nil {
						gotCommit = run.Landing.Commit
					}
					if gotCommit != wantCommit {
						t.Errorf("run %s landing = %q; want %q", run.Tag, gotCommit, wantCommit)
					}
				}
				if steps := run.LLMSteps[LLMStepsLanding]; steps != want.landingSteps[run.Tag] {
					t.Errorf("run %s landing LLM steps = %d; want %d", run.Tag, steps, want.landingSteps[run.Tag])
				}
			}
			if len(report.Runs) != len(want.runLandings) && want.runLandings != nil {
				t.Errorf("runs = %d; want %d", len(report.Runs), len(want.runLandings))
			}

			if len(report.Findings) != len(want.findings) {
				t.Fatalf("findings = %+v; want %d", report.Findings, len(want.findings))
			}
			for i, text := range want.findings {
				if finding := report.Findings[i]; finding.Tag != text && !strings.Contains(finding.Error, text) {
					t.Errorf("finding %d = %+v; want it to name %q", i, finding, text)
				}
			}

			if len(w.code.blameCalls) != len(want.blames) || (len(want.blames) > 0 && !reflect.DeepEqual(w.code.blameCalls, want.blames)) {
				t.Errorf("blame calls = %+v; want %+v", w.code.blameCalls, want.blames)
			}
		})
	}
}
