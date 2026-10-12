// overview_test.go pins the runs overview: renderOverview and fitOverview over built runs, and the source that reads a prime's runs from disk.

package battencli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/statuspane"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
)

// overviewNow is the instant minute offsets in the overview tests count back from.
var overviewNow = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

// minutesAgo returns the instant the given number of minutes before overviewNow.
func minutesAgo(minutes int) time.Time {
	return overviewNow.Add(-time.Duration(minutes) * time.Minute)
}

// stamp renders t as the RFC 3339 string a status entry carries.
func stamp(t time.Time) string {
	return t.Format(time.RFC3339)
}

// foundStatus is a found poll of a status in state, on producer, with the given history.
func foundStatus(state shedengine.State, producer string, history ...shedengine.HistoryEntry) statuspane.StatusPoll {
	return statuspane.StatusPoll{
		Found:  true,
		Status: shedengine.Status{State: state, CurrentProducer: producer, History: history},
	}
}

func TestRenderOverview(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		runs []overviewRun
		want []string
		// fitRows, when positive, passes want through fitOverview with this many rows and compares wantFit instead.
		fitRows int
		wantFit []string
	}{
		{
			name: "two runs are ordered by start with their six fields, oldest first",
			runs: []overviewRun{
				{
					Slug: "beta", StartedAt: minutesAgo(60), PairExists: true, Session: "beta-s",
					Batten: foundStatus(shedengine.StateBlocked, "Publish", shedengine.HistoryEntry{At: stamp(minutesAgo(10)), LastAt: stamp(minutesAgo(5))}),
				},
				{
					Slug: "alpha", StartedAt: minutesAgo(120), PairExists: true, Session: "alpha-s",
					Batten: foundStatus(shedengine.StateRunning, "Land", shedengine.HistoryEntry{At: stamp(minutesAgo(45))}),
				},
			},
			want: []string{
				"alpha  running  Land     45m  2h00m  alpha-s",
				"beta   blocked  Publish  5m   1h00m  beta-s",
			},
		},
		{
			name: "runs with no known start come after the dated ones, by slug",
			runs: []overviewRun{
				{Slug: "mid", PairExists: true, Session: "s", Batten: foundStatus(shedengine.StateRunning, "Plan")},
				{Slug: "zeta", StartedAt: minutesAgo(30), PairExists: true, Session: "s", Batten: foundStatus(shedengine.StateRunning, "Plan")},
				{Slug: "aaa", PairExists: true, Session: "s", Batten: foundStatus(shedengine.StateRunning, "Plan")},
			},
			want: []string{
				"zeta  running  Plan  30m  30m  s",
				"aaa   running  Plan  -    -    s",
				"mid   running  Plan  -    -    s",
			},
		},
		{
			name: "a run creating its worktree shows before its pair exists",
			runs: []overviewRun{
				{Slug: "new", StartedAt: minutesAgo(5), Session: "new-s", Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameWorktreeCreate)},
			},
			want: []string{"new  running  Worktree-Create  5m  5m  new-s"},
		},
		{
			name: "a seeded run with no status, or whose first status poll failed, shows as starting with no stage",
			runs: []overviewRun{{Slug: "fresh", StartedAt: minutesAgo(3), Session: "fresh-s"}},
			want: []string{"fresh  starting  -  3m  3m  fresh-s"},
		},
		{
			name: "a done run and a non-done run whose pair is gone are hidden",
			runs: []overviewRun{
				{Slug: "finished", StartedAt: minutesAgo(90), PairExists: true, Batten: foundStatus(shedengine.StateDone, "Land")},
				{Slug: "orphan", StartedAt: minutesAgo(90), Batten: foundStatus(shedengine.StateRunning, "Land")},
			},
			want: []string{"no runs in flight"},
		},
		{
			name: "a failed run with its pair shows",
			runs: []overviewRun{
				{Slug: "bad", StartedAt: minutesAgo(10), PairExists: true, Session: "bad-s", Batten: foundStatus(shedengine.StateFailed, "Publish")},
			},
			want: []string{"bad  failed  Publish  10m  10m  bad-s"},
		},
		{
			name: "a run in Run-Shed shows the child's producer and its time",
			runs: []overviewRun{
				{
					Slug: "run", StartedAt: minutesAgo(100), PairExists: true, Session: "run-s",
					Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameRunShed, shedengine.HistoryEntry{At: stamp(minutesAgo(90))}),
					Child:  foundStatus(shedengine.StateRunning, "Plan-Write", shedengine.HistoryEntry{At: stamp(minutesAgo(30))}),
				},
			},
			want: []string{"run  running  Plan-Write  30m  1h40m  run-s"},
		},
		{
			name: "a child with no history takes batten's last entry as its stage start",
			runs: []overviewRun{
				{
					Slug: "run", StartedAt: minutesAgo(100), PairExists: true, Session: "run-s",
					Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameRunShed, shedengine.HistoryEntry{At: stamp(minutesAgo(40))}),
					Child:  foundStatus(shedengine.StateRunning, "Plan-Write"),
				},
			},
			want: []string{"run  running  Plan-Write  40m  1h40m  run-s"},
		},
		{
			name: "a run in Run-Shed whose child status is not found shows batten's producer",
			runs: []overviewRun{
				{
					Slug: "run", StartedAt: minutesAgo(100), PairExists: true, Session: "run-s",
					Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameRunShed, shedengine.HistoryEntry{At: stamp(minutesAgo(20))}),
				},
			},
			want: []string{"run  running  Run-Shed  20m  1h40m  run-s"},
		},
		{
			name: "an unreadable status shows beside a normal line, and so does an unreadable child",
			runs: []overviewRun{
				{Slug: "broken", PairExists: true, Session: "s", Batten: statuspane.StatusPoll{Unreadable: true}},
				{Slug: "child", PairExists: true, Session: "s", Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameRunShed), Child: statuspane.StatusPoll{Unreadable: true}},
				{Slug: "ok-run", PairExists: true, Session: "s", Batten: foundStatus(shedengine.StateRunning, "Plan")},
			},
			want: []string{
				"broken  status unreadable",
				"child   status unreadable",
				"ok-run  running  Plan  -  -  s",
			},
		},
		{
			name: "no runs renders the one line",
			want: []string{"no runs in flight"},
		},
		{
			name:    "three lines into two rows keep the oldest line and count the rest",
			runs:    threeStartedRuns(),
			want:    []string{"a  running  Plan  3m  3m  s", "b  running  Plan  2m  2m  s", "c  running  Plan  1m  1m  s"},
			fitRows: 2,
			wantFit: []string{"a  running  Plan  3m  3m  s", "… 2 more runs not shown"},
		},
		{
			name:    "lines that fit come back unchanged",
			runs:    threeStartedRuns(),
			want:    []string{"a  running  Plan  3m  3m  s", "b  running  Plan  2m  2m  s", "c  running  Plan  1m  1m  s"},
			fitRows: 3,
			wantFit: []string{"a  running  Plan  3m  3m  s", "b  running  Plan  2m  2m  s", "c  running  Plan  1m  1m  s"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := renderOverview(tt.runs, overviewNow)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("renderOverview mismatch (-want +got):\n%s", diff)
			}
			if tt.fitRows > 0 {
				if diff := cmp.Diff(tt.wantFit, fitOverview(got, tt.fitRows)); diff != "" {
					t.Errorf("fitOverview(%d) mismatch (-want +got):\n%s", tt.fitRows, diff)
				}
			}
		})
	}
}

// threeStartedRuns is three running runs, a the oldest and c the newest.
func threeStartedRuns() []overviewRun {
	run := func(slug string, started int) overviewRun {
		return overviewRun{Slug: slug, StartedAt: minutesAgo(started), PairExists: true, Session: "s", Batten: foundStatus(shedengine.StateRunning, "Plan")}
	}
	return []overviewRun{run("c", 1), run("a", 3), run("b", 2)}
}

// writeOverviewFile writes data at path with the given modification time, creating its directory.
func writeOverviewFile(t *testing.T, path string, data []byte, modTime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}

// writeOverviewStatus writes status as the status file at path.
func writeOverviewStatus(t *testing.T, path string, status shedengine.Status, modTime time.Time) {
	t.Helper()
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	writeOverviewFile(t, path, data, modTime)
}

func TestOverviewSource_ReadsThePrimesRunsFromDisk(t *testing.T) {
	t.Parallel()

	hub := t.TempDir()
	prime := locationkit.Location(hub, "prime", ".")
	childStatusPath := filepath.Join(t.TempDir(), "child-status.json")

	seedBatten := func(slug, startedAt string) {
		t.Helper()
		if err := shedrun.WriteSeed(prime, slug, shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo, StartedAt: startedAt}); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(fabricengine.WorktreePath(prime, slug), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	seedBatten("run-a", stamp(minutesAgo(120)))
	writeOverviewStatus(t, shedrun.StatusFile(prime, "run-a"), shedengine.Status{State: shedengine.StateRunning, CurrentProducer: battenrecipe.NameRunShed}, minutesAgo(100))
	writeOverviewStatus(t, childStatusPath, shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan-Write"}, minutesAgo(100))

	seedBatten("run-b", stamp(minutesAgo(60)))
	runBStatus := shedrun.StatusFile(prime, "run-b")
	writeOverviewStatus(t, runBStatus, shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan"}, minutesAgo(50))

	// A seed of another recipe is not a batten run.
	if err := shedrun.WriteSeed(prime, "other", shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo}); err != nil {
		t.Fatal(err)
	}

	// A seed with an unknown key is listed but fails the strict decode.
	strictSeed := shedrun.SeedFile(prime, "strict")
	writeOverviewFile(t, strictSeed, []byte(`{"recipe":"batten","driver":"go","unknown":1}`), minutesAgo(100))
	writeOverviewStatus(t, shedrun.StatusFile(prime, "strict"), shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan"}, minutesAgo(100))
	if err := os.MkdirAll(fabricengine.WorktreePath(prime, "strict"), 0o755); err != nil {
		t.Fatal(err)
	}

	source := newOverviewSource(prime, func() time.Time { return overviewNow })
	source.childStatus = func(string) (string, error) { return childStatusPath, nil }

	want := []string{
		"run-a   running  Plan-Write  2h00m  2h00m  run-a",
		"run-b   running  Plan        1h00m  1h00m  run-b",
		"strict  running  Plan        -      -      strict",
	}
	if diff := cmp.Diff(want, source.frame().Lines); diff != "" {
		t.Fatalf("first frame mismatch (-want +got):\n%s", diff)
	}

	// The child's file goes and run-b's status turns undecodable: run-a falls back to batten's producer, and run-b keeps its line for one frame.
	if err := os.Remove(childStatusPath); err != nil {
		t.Fatal(err)
	}
	writeOverviewFile(t, runBStatus, []byte("{not json"), minutesAgo(40))
	want = []string{
		"run-a   running  Run-Shed  2h00m  2h00m  run-a",
		"run-b   running  Plan      1h00m  1h00m  run-b",
		"strict  running  Plan      -      -      strict",
	}
	if diff := cmp.Diff(want, source.frame().Lines); diff != "" {
		t.Errorf("second frame mismatch (-want +got):\n%s", diff)
	}

	want[1] = "run-b   status unreadable"
	if diff := cmp.Diff(want, source.frame().Lines); diff != "" {
		t.Errorf("third frame mismatch (-want +got):\n%s", diff)
	}
}
