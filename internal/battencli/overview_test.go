// overview_test.go pins the runs overview: renderOverview over built runs, and the source that reads a prime's runs from disk.

package battencli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/statuspane"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// overviewNow is the instant minute offsets in the overview tests count back from.
var overviewNow = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

// orchThresholds are the known thresholds the orch line tests draw against.
var orchThresholds = overviewThresholds{Soft: 300000, Hard: 400000, Known: true}

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
		name       string
		orch       orchStatePoll
		thresholds overviewThresholds
		runs       []overviewRun
		want       []string
	}{
		{
			name: "two runs are ordered by start with their six fields, oldest first",
			runs: []overviewRun{
				{
					Slug: "beta", StartedAt: minutesAgo(60), PairExists: true, Session: "beta-s",
					Batten: foundStatus(shedengine.StateBlocked, "Publish", shedengine.HistoryEntry{At: stamp(minutesAgo(10)), LastAt: stamp(minutesAgo(5))}),
				},
				{
					Slug: "alpha", StartedAt: minutesAgo(120), PairExists: true, Session: "alpha-s", Recipe: shedrun.RecipeBatten,
					Batten: foundStatus(shedengine.StateRunning, "Land", shedengine.HistoryEntry{At: stamp(minutesAgo(45))}),
				},
			},
			want: []string{
				"alpha  running  7. Land  45m  2h00m  alpha-s",
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
				{Slug: "new", StartedAt: minutesAgo(5), Session: "new-s", Recipe: shedrun.RecipeBatten, Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameWorktreeCreate)},
			},
			want: []string{"new  running  1. Worktree-Create  5m  5m  new-s"},
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
			name: "a run in Run-Shed shows the child's producer, numbered under Run-Shed's number, and its time",
			runs: []overviewRun{
				{
					Slug: "run", StartedAt: minutesAgo(100), PairExists: true, Session: "run-s", Recipe: shedrun.RecipeBatten, ChildRecipe: shedrun.RecipeLoom,
					Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameRunShed, shedengine.HistoryEntry{At: stamp(minutesAgo(90))}),
					Child:  foundStatus(shedengine.StateRunning, "Plan-Write", shedengine.HistoryEntry{At: stamp(minutesAgo(30))}),
				},
			},
			want: []string{"run  running  6.5 Plan-Write  30m  1h40m  run-s"},
		},
		{
			name: "a child row in a review segment takes its segment's number",
			runs: []overviewRun{
				{
					Slug: "run", StartedAt: minutesAgo(100), PairExists: true, Session: "run-s", Recipe: shedrun.RecipeBatten, ChildRecipe: shedrun.RecipeLoom,
					Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameRunShed, shedengine.HistoryEntry{At: stamp(minutesAgo(90))}),
					Child:  foundStatus(shedengine.StateRunning, "Plan-Burler-Review", shedengine.HistoryEntry{At: stamp(minutesAgo(30))}),
				},
			},
			want: []string{"run  running  6.6 Plan-Burler-Review  30m  1h40m  run-s"},
		},
		{
			name: "a child row shows alone when the child recipe is unknown or does not hold it, and a run with an unreadable seed shows the name alone",
			runs: []overviewRun{
				{
					Slug: "a-unknown", StartedAt: minutesAgo(100), PairExists: true, Session: "s", Recipe: shedrun.RecipeBatten,
					Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameRunShed, shedengine.HistoryEntry{At: stamp(minutesAgo(90))}),
					Child:  foundStatus(shedengine.StateRunning, "Plan-Write", shedengine.HistoryEntry{At: stamp(minutesAgo(30))}),
				},
				{
					Slug: "b-missing", StartedAt: minutesAgo(99), PairExists: true, Session: "s", Recipe: shedrun.RecipeBatten, ChildRecipe: shedrun.RecipeLoom,
					Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameRunShed, shedengine.HistoryEntry{At: stamp(minutesAgo(90))}),
					Child:  foundStatus(shedengine.StateRunning, "Elsewhere", shedengine.HistoryEntry{At: stamp(minutesAgo(30))}),
				},
				{Slug: "c-unreadable-seed", StartedAt: minutesAgo(98), PairExists: true, Session: "s", Batten: foundStatus(shedengine.StateRunning, "Land")},
			},
			want: []string{
				"a-unknown          running  Plan-Write  30m    1h40m  s",
				"b-missing          running  Elsewhere   30m    1h39m  s",
				"c-unreadable-seed  running  Land        1h38m  1h38m  s",
			},
		},
		{
			name: "a child with no history takes batten's last entry as its stage start",
			runs: []overviewRun{
				{
					Slug: "run", StartedAt: minutesAgo(100), PairExists: true, Session: "run-s", Recipe: shedrun.RecipeBatten, ChildRecipe: shedrun.RecipeLoom,
					Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameRunShed, shedengine.HistoryEntry{At: stamp(minutesAgo(40))}),
					Child:  foundStatus(shedengine.StateRunning, "Plan-Write"),
				},
			},
			want: []string{"run  running  6.5 Plan-Write  40m  1h40m  run-s"},
		},
		{
			name: "a run in Run-Shed whose child status is not found shows batten's producer",
			runs: []overviewRun{
				{
					Slug: "run", StartedAt: minutesAgo(100), PairExists: true, Session: "run-s", Recipe: shedrun.RecipeBatten,
					Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameRunShed, shedengine.HistoryEntry{At: stamp(minutesAgo(20))}),
				},
			},
			want: []string{"run  running  6. Run-Shed  20m  1h40m  run-s"},
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
			name: "a running run a live pause covers shows paused with the minutes since the pause",
			runs: []overviewRun{
				{Slug: "a", StartedAt: minutesAgo(30), PairExists: true, Session: "s", PausedAt: minutesAgo(7), Batten: foundStatus(shedengine.StateRunning, "Plan")},
			},
			want: []string{"a  paused 7m  Plan  30m  30m  s"},
		},
		{
			name: "a batten-paused run a live pause covers shows the minutes, and an uncovered one the bare state",
			runs: []overviewRun{
				{Slug: "covered", StartedAt: minutesAgo(30), PairExists: true, Session: "s", PausedAt: minutesAgo(12), Batten: foundStatus(shedengine.StatePaused, "Plan")},
				{Slug: "bare", StartedAt: minutesAgo(20), PairExists: true, Session: "s", Batten: foundStatus(shedengine.StatePaused, "Plan")},
			},
			want: []string{
				"covered  paused 12m  Plan  30m  30m  s",
				"bare     paused      Plan  20m  20m  s",
			},
		},
		{
			name: "a starting run a live pause covers shows paused with the minutes and stage -",
			runs: []overviewRun{{Slug: "fresh", StartedAt: minutesAgo(3), Session: "fresh-s", PausedAt: minutesAgo(2)}},
			want: []string{"fresh  paused 2m  -  3m  3m  fresh-s"},
		},
		{
			name: "a blocked, an awaiting and a failed run keep their own state when a pause covers them",
			runs: []overviewRun{
				{Slug: "b", StartedAt: minutesAgo(30), PairExists: true, Session: "s", PausedAt: minutesAgo(5), Batten: foundStatus(shedengine.StateBlocked, "Plan")},
				{Slug: "w", StartedAt: minutesAgo(20), PairExists: true, Session: "s", PausedAt: minutesAgo(5), Batten: foundStatus(shedengine.StateAwaiting, "Plan")},
				{Slug: "f", StartedAt: minutesAgo(10), PairExists: true, Session: "s", PausedAt: minutesAgo(5), Batten: foundStatus(shedengine.StateFailed, "Plan")},
			},
			want: []string{
				"b  blocked   Plan  30m  30m  s",
				"w  awaiting  Plan  20m  20m  s",
				"f  failed    Plan  10m  10m  s",
			},
		},
		{
			name: "a pause that began after the poll's clock shows paused 0m",
			runs: []overviewRun{
				{Slug: "a", StartedAt: minutesAgo(30), PairExists: true, Session: "s", PausedAt: overviewNow.Add(5 * time.Minute), Batten: foundStatus(shedengine.StateRunning, "Plan")},
			},
			want: []string{"a  paused 0m  Plan  30m  30m  s"},
		},
		{
			name: "no runs renders the one line",
			want: []string{"no runs in flight"},
		},
		{
			name:       "a found idle orch state draws its line above the runs, tokens rounded down to thousands",
			orch:       orchStatePoll{Found: true, State: orchengine.State{Phase: orchengine.PhaseIdle, LastContextTokens: 212999, LastContextKnown: true, CycleCount: 3}},
			thresholds: orchThresholds,
			runs: []overviewRun{
				{Slug: "beta", StartedAt: minutesAgo(60), PairExists: true, Session: "beta-s", Batten: foundStatus(shedengine.StateRunning, "Plan")},
				{Slug: "alpha", StartedAt: minutesAgo(120), PairExists: true, Session: "alpha-s", Batten: foundStatus(shedengine.StateRunning, "Land")},
			},
			want: []string{
				"orch  212k / soft 300k / hard 400k  idle  cycles 3",
				"alpha  running  Land  2h00m  2h00m  alpha-s",
				"beta   running  Plan  1h00m  1h00m  beta-s",
			},
		},
		{
			name:       "an unknown reading draws - for the tokens and the phase as persisted",
			orch:       orchStatePoll{Found: true, State: orchengine.State{Phase: orchengine.PhaseCompacting}},
			thresholds: orchThresholds,
			want:       []string{"orch  - / soft 300k / hard 400k  compacting  cycles 0", "no runs in flight"},
		},
		{
			name:       "999 tokens draw plain",
			orch:       orchStatePoll{Found: true, State: orchengine.State{Phase: orchengine.PhaseIdle, LastContextTokens: 999, LastContextKnown: true}},
			thresholds: orchThresholds,
			want:       []string{"orch  999 / soft 300k / hard 400k  idle  cycles 0", "no runs in flight"},
		},
		{
			name:       "1000 tokens draw as 1k",
			orch:       orchStatePoll{Found: true, State: orchengine.State{Phase: orchengine.PhaseIdle, LastContextTokens: 1000, LastContextKnown: true}},
			thresholds: orchThresholds,
			want:       []string{"orch  1k / soft 300k / hard 400k  idle  cycles 0", "no runs in flight"},
		},
		{
			name:       "an exited watcher ends the line with watcher down",
			orch:       orchStatePoll{Found: true, State: orchengine.State{Phase: orchengine.PhaseIdle, LastContextTokens: 212500, LastContextKnown: true, CycleCount: 3, WatcherExit: "bye"}},
			thresholds: orchThresholds,
			want:       []string{"orch  212k / soft 300k / hard 400k  idle  cycles 3  watcher down", "no runs in flight"},
		},
		{
			name: "unknown thresholds draw -",
			orch: orchStatePoll{Found: true, State: orchengine.State{Phase: orchengine.PhaseIdle, LastContextTokens: 212500, LastContextKnown: true, CycleCount: 3}},
			want: []string{"orch  212k / soft - / hard -  idle  cycles 3", "no runs in flight"},
		},
		{
			name:       "an unreadable orch state draws the broken line above the runs",
			orch:       orchStatePoll{Unreadable: true},
			thresholds: orchThresholds,
			runs: []overviewRun{
				{Slug: "a", StartedAt: minutesAgo(30), PairExists: true, Session: "s", Batten: foundStatus(shedengine.StateRunning, "Plan")},
			},
			want: []string{"orch  state unreadable", "a  running  Plan  30m  30m  s"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := renderOverview(tt.orch, tt.thresholds, tt.runs, overviewNow)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("renderOverview mismatch (-want +got):\n%s", diff)
			}
		})
	}
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

	// A seed with an unknown key decodes and is listed like any other; its `-` columns come from the seed lacking started_at.
	futureKeySeed := shedrun.SeedFile(prime, "future-key")
	writeOverviewFile(t, futureKeySeed, []byte(`{"recipe":"batten","driver":"go","unknown":1}`), minutesAgo(100))
	writeOverviewStatus(t, shedrun.StatusFile(prime, "future-key"), shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan"}, minutesAgo(100))
	if err := os.MkdirAll(fabricengine.WorktreePath(prime, "future-key"), 0o755); err != nil {
		t.Fatal(err)
	}

	pauseRecordPath := filepath.Join(t.TempDir(), "pause.json")
	source := newOverviewSource(prime, pauseRecordPath, overviewThresholds{}, func() time.Time { return overviewNow })
	source.childStatus = func(string) (string, error) { return childStatusPath, nil }
	childPair := locationkit.Location(t.TempDir(), "run-a", ".")
	source.childPair = func(string) (*lyxcwd.Location, error) { return childPair, nil }
	if err := shedrun.WriteSeed(childPair, shedrun.SelfRunID, shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo}); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"run-a       running  6.5 Plan-Write  2h00m  2h00m  run-a",
		"run-b       running  Plan            1h00m  1h00m  run-b",
		"future-key  running  Plan            -      -      future-key",
	}
	if diff := cmp.Diff(want, source.frame().Lines); diff != "" {
		t.Fatalf("first frame mismatch (-want +got):\n%s", diff)
	}

	// A seed is write-once, so run-a keeps its child number from the cached recipe once the seed file is gone.
	if err := os.Remove(shedrun.SeedFile(childPair, shedrun.SelfRunID)); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, source.frame().Lines); diff != "" {
		t.Fatalf("frame after the child seed is removed mismatch (-want +got):\n%s", diff)
	}

	// The child's file goes and run-b's status turns undecodable: run-a falls back to batten's producer, and run-b keeps its line for one frame.
	if err := os.Remove(childStatusPath); err != nil {
		t.Fatal(err)
	}
	writeOverviewFile(t, runBStatus, []byte("{not json"), minutesAgo(40))
	want = []string{
		"run-a       running  6. Run-Shed  2h00m  2h00m  run-a",
		"run-b       running  Plan         1h00m  1h00m  run-b",
		"future-key  running  Plan         -      -      future-key",
	}
	if diff := cmp.Diff(want, source.frame().Lines); diff != "" {
		t.Errorf("second frame mismatch (-want +got):\n%s", diff)
	}

	want[1] = "run-b       status unreadable"
	if diff := cmp.Diff(want, source.frame().Lines); diff != "" {
		t.Errorf("third frame mismatch (-want +got):\n%s", diff)
	}

	// run-b's status is decodable again, at a modification time other than the failed write's.
	writeOverviewStatus(t, runBStatus, shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan"}, minutesAgo(35))

	// A slug entry for run-a and a hub entry: the older entry covers run-a, the hub entry covers the rest.
	record, err := json.Marshal(pauserecord.Record{Live: []pauserecord.Entry{
		{Scope: "run-a", PausedAt: minutesAgo(10)},
		{Scope: pauserecord.ScopeHub, PausedAt: minutesAgo(4)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	writeOverviewFile(t, pauseRecordPath, record, minutesAgo(1))
	want = []string{
		"run-a       paused 10m  6. Run-Shed  2h00m  2h00m  run-a",
		"run-b       paused 4m   Plan         1h00m  1h00m  run-b",
		"future-key  paused 4m   Plan         -      -      future-key",
	}
	if diff := cmp.Diff(want, source.frame().Lines); diff != "" {
		t.Errorf("fourth frame mismatch (-want +got):\n%s", diff)
	}

	// A record that does not decode never blanks the pane.
	writeOverviewFile(t, pauseRecordPath, []byte("{not json"), minutesAgo(1))
	want = []string{
		"run-a       running  6. Run-Shed  2h00m  2h00m  run-a",
		"run-b       running  Plan         1h00m  1h00m  run-b",
		"future-key  running  Plan         -      -      future-key",
	}
	if diff := cmp.Diff(want, source.frame().Lines); diff != "" {
		t.Errorf("fifth frame mismatch (-want +got):\n%s", diff)
	}

	// A child pair with no seed, and a pair that cannot be resolved, each leave the child row alone.
	hub2 := t.TempDir()
	prime2 := locationkit.Location(hub2, "prime", ".")
	for _, slug := range []string{"run-c", "run-d"} {
		if err := shedrun.WriteSeed(prime2, slug, shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo, StartedAt: stamp(minutesAgo(120))}); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(fabricengine.WorktreePath(prime2, slug), 0o755); err != nil {
			t.Fatal(err)
		}
		writeOverviewStatus(t, shedrun.StatusFile(prime2, slug), shedengine.Status{State: shedengine.StateRunning, CurrentProducer: battenrecipe.NameRunShed}, minutesAgo(100))
	}
	writeOverviewStatus(t, childStatusPath, shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan-Write"}, minutesAgo(100))
	seedless := locationkit.Location(t.TempDir(), "run-c", ".")
	source2 := newOverviewSource(prime2, pauseRecordPath, overviewThresholds{}, func() time.Time { return overviewNow })
	source2.childStatus = func(string) (string, error) { return childStatusPath, nil }
	source2.childPair = func(slug string) (*lyxcwd.Location, error) {
		if slug == "run-d" {
			return nil, errors.New("no pair")
		}
		return seedless, nil
	}
	want = []string{
		"run-c  running  Plan-Write  2h00m  2h00m  run-c",
		"run-d  running  Plan-Write  2h00m  2h00m  run-d",
	}
	if diff := cmp.Diff(want, source2.frame().Lines); diff != "" {
		t.Errorf("seedless child frame mismatch (-want +got):\n%s", diff)
	}
}

func TestOverviewSource_DrawsTheOrchLine(t *testing.T) {
	t.Parallel()

	prime := locationkit.Location(t.TempDir(), "prime", ".")
	orchPaths := orchcli.PrimePaths(prime)
	source := newOverviewSource(prime, filepath.Join(t.TempDir(), "pause.json"), orchThresholds, func() time.Time { return overviewNow })

	writeState := func(state orchengine.State, modTime time.Time) {
		t.Helper()
		data, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		writeOverviewFile(t, orchPaths.StatePath, data, modTime)
	}
	idle := orchengine.State{Phase: orchengine.PhaseIdle, LastContextTokens: 212500, LastContextKnown: true, CycleCount: 3}
	idleLine := "orch  212k / soft 300k / hard 400k  idle  cycles 3"
	exited := idle
	exited.WatcherExit = "bye"
	exitedLine := idleLine + "  watcher down"
	noRuns := "no runs in flight"

	check := func(frame string, want ...string) {
		t.Helper()
		if diff := cmp.Diff(want, source.frame().Lines); diff != "" {
			t.Errorf("%s mismatch (-want +got):\n%s", frame, diff)
		}
	}

	check("frame 1, no state file", noRuns)
	if _, err := os.Stat(orchPaths.Dir); !os.IsNotExist(err) {
		t.Errorf("polling an absent state created %s (stat err %v)", orchPaths.Dir, err)
	}

	writeOverviewFile(t, orchPaths.StatePath, []byte("{not json"), minutesAgo(50))
	check("frame 2, first failure with no good state", noRuns)

	writeState(idle, minutesAgo(40))
	check("frame 3, good state", idleLine, noRuns)

	writeState(orchengine.State{Phase: orchengine.PhaseIdle, LastContextTokens: 100000, LastContextKnown: true}, minutesAgo(40))
	check("frame 4, unchanged modification time is not re-read", idleLine, noRuns)

	writeOverviewFile(t, orchPaths.StatePath, []byte("{not json"), minutesAgo(30))
	check("frame 5, first failure keeps the last good line", idleLine, noRuns)
	check("frame 6, repeat of the failed modification time", "orch  state unreadable", noRuns)

	writeState(exited, minutesAgo(20))
	check("frame 7, watcher down", exitedLine, noRuns)

	writeOverviewFile(t, orchPaths.StatePath, []byte("{not json"), minutesAgo(10))
	check("frame 8, a good decode cleared the failure count", exitedLine, noRuns)

	if _, err := os.Stat(orchPaths.StateLockPath); !os.IsNotExist(err) {
		t.Errorf("polling took or created the state lock (stat err %v)", err)
	}

	if err := os.RemoveAll(orchPaths.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orchPaths.Dir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	check("frame 10, a stat failure with no modification time is a second failure", "orch  state unreadable", noRuns)

	if err := os.Remove(orchPaths.Dir); err != nil {
		t.Fatal(err)
	}
	writeState(idle, minutesAgo(5))
	runsDir := filepath.Dir(filepath.Dir(shedrun.SeedFile(prime, "x")))
	if err := os.MkdirAll(filepath.Dir(runsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runsDir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	check("frame 11, the orch line draws above runs unreadable", idleLine, "runs unreadable")

	if err := os.Remove(orchPaths.StatePath); err != nil {
		t.Fatal(err)
	}
	check("frame 12, an absent state draws no orch line", "runs unreadable")
}

// TestLoadOverviewThresholds does not call t.Parallel: logcapture redirects the process-global logger.
func TestLoadOverviewThresholds(t *testing.T) {
	tests := []struct {
		name string
		// config is the orch.yaml content; empty writes none.
		config   string
		want     overviewThresholds
		wantWarn bool
	}{
		{name: "no orch.yaml resolves the template thresholds", want: overviewThresholds{Soft: 300000, Hard: 400000, Known: true}},
		{name: "an invalid orch.yaml returns the unknown thresholds and warns", config: "cycle_mode: bogus\n", want: overviewThresholds{}, wantWarn: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.Capture(t)
			prime := locationkit.Location(t.TempDir(), "prime", ".")
			if tt.config != "" {
				writeOverviewFile(t, configengine.ConfigFile(prime.AnchorPath(), "orch"), []byte(tt.config), overviewNow)
			}

			got := loadOverviewThresholds(prime)
			if got != tt.want {
				t.Errorf("loadOverviewThresholds = %+v, want %+v", got, tt.want)
			}
			if warned := strings.Contains(logs.String(), "WARN"); warned != tt.wantWarn {
				t.Errorf("warned = %v, want %v; log:\n%s", warned, tt.wantWarn, logs.String())
			}
		})
	}
}
