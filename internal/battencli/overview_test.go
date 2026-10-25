//go:build tier0

// overview_test.go pins a beam's window: the cells of a run over built polls, DobbyRunCells reading runs from disk, the orch lines and which orchs a window shows.

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
	"github.com/Knatte18/loomyard/internal/dobbyengine"
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

func TestOverviewRunCells(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  overviewRun
		want RunCells
	}{
		{
			name: "a batten stage is numbered from the run's recipe and timed from its last entry",
			run:  overviewRun{StartedAt: minutesAgo(120), Recipe: shedrun.RecipeBatten, Batten: foundStatus(shedengine.StateRunning, "Land", shedengine.HistoryEntry{At: stamp(minutesAgo(45))})},
			want: RunCells{State: "running", Stage: "7. Land", InStage: "45m"},
		},
		{
			name: "an entry's last_at wins over its at",
			run:  overviewRun{StartedAt: minutesAgo(60), Batten: foundStatus(shedengine.StateBlocked, "Publish", shedengine.HistoryEntry{At: stamp(minutesAgo(10)), LastAt: stamp(minutesAgo(5))})},
			want: RunCells{State: "blocked", Stage: "Publish", InStage: "5m"},
		},
		{
			name: "a run creating its worktree shows its numbered stage",
			run:  overviewRun{StartedAt: minutesAgo(5), Recipe: shedrun.RecipeBatten, Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameWorktreeCreate)},
			want: RunCells{State: "running", Stage: "1. Worktree-Create", InStage: "5m"},
		},
		{
			name: "a seeded run with no status, or whose first status poll failed, shows as starting with no stage",
			run:  overviewRun{StartedAt: minutesAgo(3)},
			want: RunCells{State: "starting", Stage: "-", InStage: "3m"},
		},
		{
			name: "a run with no known start and no history has no time in stage",
			run:  overviewRun{Batten: foundStatus(shedengine.StateFailed, "Plan")},
			want: RunCells{State: "failed", Stage: "Plan", InStage: "-"},
		},
		{
			name: "a run in Run-Shed shows the child's producer, numbered under Run-Shed's number, and its time",
			run: overviewRun{
				StartedAt: minutesAgo(100), Recipe: shedrun.RecipeBatten, ChildRecipe: shedrun.RecipeLoom,
				Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameRunShed, shedengine.HistoryEntry{At: stamp(minutesAgo(90))}),
				Child:  foundStatus(shedengine.StateRunning, "Plan-Write", shedengine.HistoryEntry{At: stamp(minutesAgo(30))}),
			},
			want: RunCells{State: "running", Stage: "6.6 Plan-Write", InStage: "30m"},
		},
		{
			name: "a child row in a review segment takes its segment's number",
			run: overviewRun{
				StartedAt: minutesAgo(100), Recipe: shedrun.RecipeBatten, ChildRecipe: shedrun.RecipeLoom,
				Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameRunShed, shedengine.HistoryEntry{At: stamp(minutesAgo(90))}),
				Child:  foundStatus(shedengine.StateRunning, "Plan-Burler-Review", shedengine.HistoryEntry{At: stamp(minutesAgo(30))}),
			},
			want: RunCells{State: "running", Stage: "6.7 Plan-Burler-Review", InStage: "30m"},
		},
		{
			name: "a child row shows alone when the child recipe is unknown",
			run: overviewRun{
				StartedAt: minutesAgo(100), Recipe: shedrun.RecipeBatten,
				Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameRunShed, shedengine.HistoryEntry{At: stamp(minutesAgo(90))}),
				Child:  foundStatus(shedengine.StateRunning, "Plan-Write", shedengine.HistoryEntry{At: stamp(minutesAgo(30))}),
			},
			want: RunCells{State: "running", Stage: "Plan-Write", InStage: "30m"},
		},
		{
			name: "a child row shows alone when the child recipe does not hold it",
			run: overviewRun{
				StartedAt: minutesAgo(99), Recipe: shedrun.RecipeBatten, ChildRecipe: shedrun.RecipeLoom,
				Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameRunShed, shedengine.HistoryEntry{At: stamp(minutesAgo(90))}),
				Child:  foundStatus(shedengine.StateRunning, "Elsewhere", shedengine.HistoryEntry{At: stamp(minutesAgo(30))}),
			},
			want: RunCells{State: "running", Stage: "Elsewhere", InStage: "30m"},
		},
		{
			name: "a run with an unreadable seed shows its stage's name alone, timed from its start",
			run:  overviewRun{StartedAt: minutesAgo(98), Batten: foundStatus(shedengine.StateRunning, "Land")},
			want: RunCells{State: "running", Stage: "Land", InStage: "1h38m"},
		},
		{
			name: "a child with no history takes batten's last entry as its stage start",
			run: overviewRun{
				StartedAt: minutesAgo(100), Recipe: shedrun.RecipeBatten, ChildRecipe: shedrun.RecipeLoom,
				Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameRunShed, shedengine.HistoryEntry{At: stamp(minutesAgo(40))}),
				Child:  foundStatus(shedengine.StateRunning, "Plan-Write"),
			},
			want: RunCells{State: "running", Stage: "6.6 Plan-Write", InStage: "40m"},
		},
		{
			name: "a run in Run-Shed whose child status is not found shows batten's producer",
			run: overviewRun{
				StartedAt: minutesAgo(100), Recipe: shedrun.RecipeBatten,
				Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameRunShed, shedengine.HistoryEntry{At: stamp(minutesAgo(20))}),
			},
			want: RunCells{State: "running", Stage: "6. Run-Shed", InStage: "20m"},
		},
		{
			name: "an unreadable status is unreadable",
			run:  overviewRun{Batten: statuspane.StatusPoll{Unreadable: true}},
			want: RunCells{Unreadable: true},
		},
		{
			name: "an unreadable child of a run in Run-Shed is unreadable",
			run:  overviewRun{Batten: foundStatus(shedengine.StateRunning, battenrecipe.NameRunShed), Child: statuspane.StatusPoll{Unreadable: true}},
			want: RunCells{Unreadable: true},
		},
		{
			name: "a running run a live pause covers shows paused with the minutes since the pause",
			run:  overviewRun{StartedAt: minutesAgo(30), PausedAt: minutesAgo(7), Batten: foundStatus(shedengine.StateRunning, "Plan")},
			want: RunCells{State: "paused 7m", Stage: "Plan", InStage: "30m"},
		},
		{
			name: "a batten-paused run a live pause covers shows the minutes",
			run:  overviewRun{StartedAt: minutesAgo(30), PausedAt: minutesAgo(12), Batten: foundStatus(shedengine.StatePaused, "Plan")},
			want: RunCells{State: "paused 12m", Stage: "Plan", InStage: "30m"},
		},
		{
			name: "a batten-paused run no pause covers shows the bare state",
			run:  overviewRun{StartedAt: minutesAgo(20), Batten: foundStatus(shedengine.StatePaused, "Plan")},
			want: RunCells{State: "paused", Stage: "Plan", InStage: "20m"},
		},
		{
			name: "a starting run a live pause covers shows paused with the minutes and stage -",
			run:  overviewRun{StartedAt: minutesAgo(3), PausedAt: minutesAgo(2)},
			want: RunCells{State: "paused 2m", Stage: "-", InStage: "3m"},
		},
		{
			name: "a blocked run keeps its own state when a pause covers it",
			run:  overviewRun{StartedAt: minutesAgo(30), PausedAt: minutesAgo(5), Batten: foundStatus(shedengine.StateBlocked, "Plan")},
			want: RunCells{State: "blocked", Stage: "Plan", InStage: "30m"},
		},
		{
			name: "an awaiting run keeps its own state when a pause covers it",
			run:  overviewRun{StartedAt: minutesAgo(20), PausedAt: minutesAgo(5), Batten: foundStatus(shedengine.StateAwaiting, "Plan")},
			want: RunCells{State: "awaiting", Stage: "Plan", InStage: "20m"},
		},
		{
			name: "a failed run keeps its own state when a pause covers it",
			run:  overviewRun{StartedAt: minutesAgo(10), PausedAt: minutesAgo(5), Batten: foundStatus(shedengine.StateFailed, "Plan")},
			want: RunCells{State: "failed", Stage: "Plan", InStage: "10m"},
		},
		{
			name: "a pause that began after the poll's clock shows paused 0m",
			run:  overviewRun{StartedAt: minutesAgo(30), PausedAt: overviewNow.Add(5 * time.Minute), Batten: foundStatus(shedengine.StateRunning, "Plan")},
			want: RunCells{State: "paused 0m", Stage: "Plan", InStage: "30m"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := overviewRunCells(tt.run, overviewNow); got != tt.want {
				t.Errorf("overviewRunCells() = %+v; want %+v", got, tt.want)
			}
		})
	}
}

func TestOverviewLabeledOrchLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		label      string
		orch       orchStatePoll
		thresholds overviewThresholds
		want       string
		wantShown  bool
	}{
		{
			name:       "a found idle state draws its line, tokens rounded down to thousands",
			orch:       orchStatePoll{Found: true, State: orchengine.State{Phase: orchengine.PhaseIdle, LastContextTokens: 212999, LastContextKnown: true, CycleCount: 3}},
			thresholds: orchThresholds,
			want:       "orch  212k / soft 300k / hard 400k  idle  cycles 3",
			wantShown:  true,
		},
		{
			name:       "a beam's line carries its label",
			label:      "b",
			orch:       orchStatePoll{Found: true, State: orchengine.State{Phase: orchengine.PhaseIdle, LastContextTokens: 212999, LastContextKnown: true, CycleCount: 3}},
			thresholds: orchThresholds,
			want:       "orch b  212k / soft 300k / hard 400k  idle  cycles 3",
			wantShown:  true,
		},
		{
			name:       "an unknown reading draws - for the tokens and the phase as persisted",
			orch:       orchStatePoll{Found: true, State: orchengine.State{Phase: orchengine.PhaseCompacting}},
			thresholds: orchThresholds,
			want:       "orch  - / soft 300k / hard 400k  compacting  cycles 0",
			wantShown:  true,
		},
		{
			name:       "999 tokens draw plain",
			orch:       orchStatePoll{Found: true, State: orchengine.State{Phase: orchengine.PhaseIdle, LastContextTokens: 999, LastContextKnown: true}},
			thresholds: orchThresholds,
			want:       "orch  999 / soft 300k / hard 400k  idle  cycles 0",
			wantShown:  true,
		},
		{
			name:       "1000 tokens draw as 1k",
			orch:       orchStatePoll{Found: true, State: orchengine.State{Phase: orchengine.PhaseIdle, LastContextTokens: 1000, LastContextKnown: true}},
			thresholds: orchThresholds,
			want:       "orch  1k / soft 300k / hard 400k  idle  cycles 0",
			wantShown:  true,
		},
		{
			name:       "an exited watcher ends the line with watcher down",
			orch:       orchStatePoll{Found: true, State: orchengine.State{Phase: orchengine.PhaseIdle, LastContextTokens: 212500, LastContextKnown: true, CycleCount: 3, WatcherExit: "bye"}},
			thresholds: orchThresholds,
			want:       "orch  212k / soft 300k / hard 400k  idle  cycles 3  watcher down",
			wantShown:  true,
		},
		{
			name:      "unknown thresholds draw -",
			orch:      orchStatePoll{Found: true, State: orchengine.State{Phase: orchengine.PhaseIdle, LastContextTokens: 212500, LastContextKnown: true, CycleCount: 3}},
			want:      "orch  212k / soft - / hard -  idle  cycles 3",
			wantShown: true,
		},
		{
			name:       "an unreadable state draws the broken line",
			orch:       orchStatePoll{Unreadable: true},
			thresholds: orchThresholds,
			want:       "orch  state unreadable",
			wantShown:  true,
		},
		{
			name:       "no state file draws no line",
			thresholds: orchThresholds,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, shown := overviewLabeledOrchLine(tt.label, tt.orch, tt.thresholds)
			if got != tt.want || shown != tt.wantShown {
				t.Errorf("overviewLabeledOrchLine(%q) = (%q, %v); want (%q, %v)", tt.label, got, shown, tt.want, tt.wantShown)
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

// viewRuns returns the ViewRuns of slugs, which is all DobbyRunCells.Of reads of them.
func viewRuns(slugs ...string) []dobbyengine.ViewRun {
	runs := make([]dobbyengine.ViewRun, len(slugs))
	for i, slug := range slugs {
		runs[i] = dobbyengine.ViewRun{Slug: slug}
	}
	return runs
}

func TestDobbyRunCells_ReadsTheRunsFromDisk(t *testing.T) {
	t.Parallel()

	hub := t.TempDir()
	prime := locationkit.Location(hub, "prime", ".")
	childStatusPath := filepath.Join(t.TempDir(), "child-status.json")

	seedBatten := func(slug, startedAt string) {
		t.Helper()
		if err := shedrun.WriteSeed(prime, slug, shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo, StartedAt: startedAt}); err != nil {
			t.Fatal(err)
		}
	}

	seedBatten("run-a", stamp(minutesAgo(120)))
	writeOverviewStatus(t, shedrun.StatusFile(prime, "run-a"), shedengine.Status{State: shedengine.StateRunning, CurrentProducer: battenrecipe.NameRunShed}, minutesAgo(100))
	writeOverviewStatus(t, childStatusPath, shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan-Write"}, minutesAgo(100))

	seedBatten("run-b", stamp(minutesAgo(60)))
	runBStatus := shedrun.StatusFile(prime, "run-b")
	writeOverviewStatus(t, runBStatus, shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan"}, minutesAgo(50))

	// A seed with an unknown key decodes like any other; its `-` time comes from the seed lacking started_at.
	writeOverviewFile(t, shedrun.SeedFile(prime, "future-key"), []byte(`{"recipe":"batten","driver":"go","unknown":1}`), minutesAgo(100))
	writeOverviewStatus(t, shedrun.StatusFile(prime, "future-key"), shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan"}, minutesAgo(100))

	pauseRecordPath := filepath.Join(t.TempDir(), "pause.json")
	cells := NewDobbyRunCells(prime, pauseRecordPath)
	cells.childStatus = func(string) (string, error) { return childStatusPath, nil }
	childPair := locationkit.Location(t.TempDir(), "run-a", ".")
	cells.childPair = func(string) (*lyxcwd.Location, error) { return childPair, nil }
	if err := shedrun.WriteSeed(childPair, shedrun.SelfRunID, shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo}); err != nil {
		t.Fatal(err)
	}
	runs := viewRuns("run-a", "run-b", "future-key")

	check := func(read string, want map[string]RunCells) {
		t.Helper()
		if diff := cmp.Diff(want, cells.Of(runs, overviewNow)); diff != "" {
			t.Errorf("%s mismatch (-want +got):\n%s", read, diff)
		}
	}

	want := map[string]RunCells{
		"run-a":      {State: "running", Stage: "6.6 Plan-Write", InStage: "2h00m"},
		"run-b":      {State: "running", Stage: "Plan", InStage: "1h00m"},
		"future-key": {State: "running", Stage: "Plan", InStage: "-"},
	}
	check("first read", want)

	// A seed is write-once, so run-a keeps its child number from the cached recipe once the seed file is gone.
	if err := os.Remove(shedrun.SeedFile(childPair, shedrun.SelfRunID)); err != nil {
		t.Fatal(err)
	}
	check("read after the child seed is removed", want)

	// The child's file goes and run-b's status turns undecodable: run-a falls back to batten's producer, and run-b keeps its cells for one read.
	if err := os.Remove(childStatusPath); err != nil {
		t.Fatal(err)
	}
	writeOverviewFile(t, runBStatus, []byte("{not json"), minutesAgo(40))
	want["run-a"] = RunCells{State: "running", Stage: "6. Run-Shed", InStage: "2h00m"}
	check("second read", want)

	want["run-b"] = RunCells{Unreadable: true}
	check("third read", want)

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
	check("fourth read", map[string]RunCells{
		"run-a":      {State: "paused 10m", Stage: "6. Run-Shed", InStage: "2h00m"},
		"run-b":      {State: "paused 4m", Stage: "Plan", InStage: "1h00m"},
		"future-key": {State: "paused 4m", Stage: "Plan", InStage: "-"},
	})

	// A record that does not decode covers nothing.
	writeOverviewFile(t, pauseRecordPath, []byte("{not json"), minutesAgo(1))
	check("fifth read", map[string]RunCells{
		"run-a":      {State: "running", Stage: "6. Run-Shed", InStage: "2h00m"},
		"run-b":      {State: "running", Stage: "Plan", InStage: "1h00m"},
		"future-key": {State: "running", Stage: "Plan", InStage: "-"},
	})

	// A child pair with no seed, and a pair that cannot be resolved, each leave the child row alone.
	prime2 := locationkit.Location(t.TempDir(), "prime", ".")
	for _, slug := range []string{"run-c", "run-d"} {
		if err := shedrun.WriteSeed(prime2, slug, shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo, StartedAt: stamp(minutesAgo(120))}); err != nil {
			t.Fatal(err)
		}
		writeOverviewStatus(t, shedrun.StatusFile(prime2, slug), shedengine.Status{State: shedengine.StateRunning, CurrentProducer: battenrecipe.NameRunShed}, minutesAgo(100))
	}
	writeOverviewStatus(t, childStatusPath, shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan-Write"}, minutesAgo(100))
	seedless := locationkit.Location(t.TempDir(), "run-c", ".")
	cells2 := NewDobbyRunCells(prime2, pauseRecordPath)
	cells2.childStatus = func(string) (string, error) { return childStatusPath, nil }
	cells2.childPair = func(slug string) (*lyxcwd.Location, error) {
		if slug == "run-d" {
			return nil, errors.New("no pair")
		}
		return seedless, nil
	}
	wantSeedless := map[string]RunCells{
		"run-c": {State: "running", Stage: "Plan-Write", InStage: "2h00m"},
		"run-d": {State: "running", Stage: "Plan-Write", InStage: "2h00m"},
	}
	if diff := cmp.Diff(wantSeedless, cells2.Of(viewRuns("run-c", "run-d"), overviewNow)); diff != "" {
		t.Errorf("seedless child read mismatch (-want +got):\n%s", diff)
	}
}

func TestOverviewSource_DrawsTheOrchLine(t *testing.T) {
	t.Parallel()

	prime := locationkit.Location(t.TempDir(), "prime", ".")
	orchPaths := orchcli.PrimePaths(prime)
	source := newOverviewSource(prime, prime, orchThresholds, func() time.Time { return overviewNow })

	writeState := func(path string, state orchengine.State, modTime time.Time) {
		t.Helper()
		data, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		writeOverviewFile(t, path, data, modTime)
	}
	idle := orchengine.State{Phase: orchengine.PhaseIdle, LastContextTokens: 212500, LastContextKnown: true, CycleCount: 3}
	idleLine := "orch  212k / soft 300k / hard 400k  idle  cycles 3"
	exited := idle
	exited.WatcherExit = "bye"
	exitedLine := idleLine + "  watcher down"

	check := func(source *overviewSource, frame string, want ...string) {
		t.Helper()
		if diff := cmp.Diff(want, source.frame().Lines); diff != "" {
			t.Errorf("%s mismatch (-want +got):\n%s", frame, diff)
		}
	}

	check(source, "frame 1, no state file")
	if _, err := os.Stat(orchPaths.Dir); !os.IsNotExist(err) {
		t.Errorf("polling an absent state created %s (stat err %v)", orchPaths.Dir, err)
	}

	writeOverviewFile(t, orchPaths.StatePath, []byte("{not json"), minutesAgo(50))
	check(source, "frame 2, first failure with no good state")

	writeState(orchPaths.StatePath, idle, minutesAgo(40))
	check(source, "frame 3, good state", idleLine)

	writeState(orchPaths.StatePath, orchengine.State{Phase: orchengine.PhaseIdle, LastContextTokens: 100000, LastContextKnown: true}, minutesAgo(40))
	check(source, "frame 4, unchanged modification time is not re-read", idleLine)

	writeOverviewFile(t, orchPaths.StatePath, []byte("{not json"), minutesAgo(30))
	check(source, "frame 5, first failure keeps the last good line", idleLine)
	check(source, "frame 6, repeat of the failed modification time", "orch  state unreadable")

	writeState(orchPaths.StatePath, exited, minutesAgo(20))
	check(source, "frame 7, watcher down", exitedLine)

	writeOverviewFile(t, orchPaths.StatePath, []byte("{not json"), minutesAgo(10))
	check(source, "frame 8, a good decode cleared the failure count", exitedLine)

	if _, err := os.Stat(orchPaths.StateLockPath); !os.IsNotExist(err) {
		t.Errorf("polling took or created the state lock (stat err %v)", err)
	}

	if err := os.RemoveAll(orchPaths.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orchPaths.Dir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	check(source, "frame 9, a stat failure with no modification time is a second failure", "orch  state unreadable")

	if err := os.Remove(orchPaths.Dir); err != nil {
		t.Fatal(err)
	}
	beam := locationkit.Location(t.TempDir(), "beam-b", ".")
	beamPaths := orchcli.PrimePaths(beam)
	source.orchs = func() ([]orchengine.Paths, []string, error) {
		return []orchengine.Paths{orchPaths, beamPaths}, []string{"", "b"}, nil
	}
	writeState(orchPaths.StatePath, idle, minutesAgo(5))
	writeState(beamPaths.StatePath, exited, minutesAgo(5))
	beamExitedLine := "orch b  212k / soft 300k / hard 400k  idle  cycles 3  watcher down"
	check(source, "frame 10, every listed orch's line, labelled", idleLine, beamExitedLine)

	writeOverviewFile(t, beamPaths.StatePath, []byte("{not json"), minutesAgo(1))
	check(source, "frame 11, a beam's first failure keeps its last good line", idleLine, beamExitedLine)
	check(source, "frame 12, a beam's repeated failure marks its own line only", idleLine, "orch b  state unreadable")

	source.orchs = func() ([]orchengine.Paths, []string, error) {
		return nil, nil, errors.New("listing failed")
	}
	check(source, "frame 13, an orch list that fails falls back to the window's own orch", idleLine)

	// A beam's window over a hub whose worktrees do not list falls back to the beam's own orch, labelled with its slug.
	writeState(beamPaths.StatePath, idle, minutesAgo(1))
	beamSource := newOverviewSource(prime, beam, orchThresholds, func() time.Time { return overviewNow })
	check(beamSource, "a beam's fallback frame", "orch beam-b  212k / soft 300k / hard 400k  idle  cycles 3")
}

func TestOrchsAtOrBelow(t *testing.T) {
	t.Parallel()

	parents := map[string]string{"a": "", "a-child": "a", "a-grandchild": "a-child", "b": "", "loop-x": "loop-y", "loop-y": "loop-x"}
	parentOf := func(slug string) (string, error) {
		parent, ok := parents[slug]
		if !ok {
			return "", errors.New("no origin record")
		}
		return parent, nil
	}
	var locations []*lyxcwd.Location
	for _, name := range []string{"main", "a", "a-child", "a-grandchild", "b", "broken", "loop-x", "loop-y"} {
		locations = append(locations, &lyxcwd.Location{WorktreeName: name})
	}

	tests := []struct {
		name     string
		worktree string
		want     []string
	}{
		{"ThePrimesWindowShowsEveryOrch", "main", []string{"main", "a", "a-child", "a-grandchild", "b", "broken", "loop-x", "loop-y"}},
		{"ABeamsWindowShowsItsOwnAndEveryBeamBelowIt", "a", []string{"a", "a-child", "a-grandchild"}},
		{"ANestedBeamsWindowLeavesOutItsParent", "a-child", []string{"a-child", "a-grandchild"}},
		{"ABeamWithNoBeamBelowShowsItsOwnAlone", "b", []string{"b"}},
		{"ACyclicParentChainIsLeftOut", "a", []string{"a", "a-child", "a-grandchild"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got []string
			for _, location := range orchsAtOrBelow("main", tt.worktree, locations, parentOf) {
				got = append(got, location.WorktreeName)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("orchsAtOrBelow(%q) mismatch (-want +got):\n%s", tt.worktree, diff)
			}
		})
	}
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
		{name: "no orch.yaml resolves the template thresholds", want: overviewThresholds{Soft: 150000, Hard: 200000, Known: true}},
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
