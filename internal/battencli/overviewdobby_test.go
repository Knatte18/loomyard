//go:build tier0

// overviewdobby_test.go pins the dobby's lines: DobbyLines over built views and cells, and the window frame that draws them above the orchs.

package battencli

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
)

func TestDobbyLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		view  dobbyengine.View
		cells map[string]RunCells
		want  []string
	}{
		{
			name: "AnAliveDobbyShowsItsRunsWithTheirCellsAndWhyEachTaskWaits",
			view: dobbyengine.View{
				Alive: true, MaxRuns: 2,
				Runs: []dobbyengine.ViewRun{
					{Slug: "flying", PassInFlight: true},
					{Slug: "failing", LastPassExit: minutesAgo(12), FullPassOwed: true, FailingPassCount: 2, FailingPassError: "pre-run refused\nmore detail"},
				},
				Waiting: []dobbyengine.WaitingTask{
					{Slug: "ready"},
					{Slug: "deps", Dependencies: []string{"x", "y"}, Paused: true},
					{Slug: "refused", Refusal: "dirty source", NextTry: overviewNow.Add(4 * time.Minute)},
					{Slug: "owned", OwnedElsewhere: true},
					{Slug: "capped", AtMaxRuns: true},
				},
			},
			cells: map[string]RunCells{
				"flying":  {State: "running", Stage: "6.6 Plan-Write", InStage: "30m"},
				"failing": {State: "blocked", Stage: "7. Land", InStage: "1h12m"},
			},
			want: []string{
				"dobby prime: alive, max_runs 2",
				"  flying   running  6.6 Plan-Write  30m    pass in flight",
				"  failing  blocked  7. Land         1h12m  last pass 12m ago, full pass owed, failing 2× in a row: pre-run refused",
				"  ready    ready to start",
				"  deps     waits on dependencies x, y; a pause",
				"  refused  waits on a refused start (dirty source), retry in 4m",
				"  owned    waits on another machine's ownership",
				"  capped   waits on max_runs 2",
			},
		},
		{
			name:  "ADeadDobbyNamesTheWayForwardAndARunWithNoPassSaysSo",
			view:  dobbyengine.View{MaxRuns: 3, Runs: []dobbyengine.ViewRun{{Slug: "r"}}},
			cells: map[string]RunCells{"r": {State: "paused 4m", Stage: "6. Run-Shed", InStage: "-"}},
			want: []string{
				"dobby prime: not running; way forward: lyx beam start",
				"  r  paused 4m  6. Run-Shed  -  no pass yet",
			},
		},
		{
			name: "AnUnreadableRunTakesNoPartInTheColumnWidths",
			view: dobbyengine.View{Alive: true, MaxRuns: 3, Runs: []dobbyengine.ViewRun{{Slug: "broken"}, {Slug: "ok"}}},
			cells: map[string]RunCells{
				"broken": {Unreadable: true},
				"ok":     {State: "running", Stage: "Plan", InStage: "5m"},
			},
			want: []string{
				"dobby prime: alive, max_runs 3",
				"  broken  status unreadable  no pass yet",
				"  ok      running  Plan  5m  no pass yet",
			},
		},
		{
			name: "NoRunsSaysSo",
			view: dobbyengine.View{Alive: true, MaxRuns: 3},
			want: []string{"dobby prime: alive, max_runs 3", "  no runs driven"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(tt.want, DobbyLines(dobbyengine.PrimeLabel, tt.view, tt.cells, overviewNow)); diff != "" {
				t.Errorf("DobbyLines() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestOverviewSource_DrawsTheDobbyAboveTheOrchs(t *testing.T) {
	t.Parallel()
	prime := locationkit.Location(t.TempDir(), "prime", ".")
	source := newOverviewSource(prime, prime, orchThresholds, func() time.Time { return overviewNow })
	var drawnAt time.Time
	source.dobby = func(now time.Time) []string {
		drawnAt = now
		return []string{"dobby prime: alive, max_runs 3", "  no runs driven"}
	}

	want := []string{"dobby prime: alive, max_runs 3", "  no runs driven"}
	if diff := cmp.Diff(want, source.frame().Lines); diff != "" {
		t.Errorf("frame() with no orch state mismatch (-want +got):\n%s", diff)
	}
	if !drawnAt.Equal(overviewNow) {
		t.Errorf("the dobby was drawn at %v; want the frame's time %v", drawnAt, overviewNow)
	}

	data, err := json.Marshal(orchengine.State{Phase: orchengine.PhaseIdle, LastContextTokens: 212500, LastContextKnown: true, CycleCount: 3})
	if err != nil {
		t.Fatal(err)
	}
	writeOverviewFile(t, orchcli.PrimePaths(prime).StatePath, data, minutesAgo(5))
	want = append(want, "", "orch  212k / soft 300k / hard 400k  idle  cycles 3")
	if diff := cmp.Diff(want, source.frame().Lines); diff != "" {
		t.Errorf("frame() with an orch state mismatch (-want +got):\n%s", diff)
	}
}

func TestWorktreeHasDobby(t *testing.T) {
	t.Parallel()
	prime := &lyxcwd.Location{WorktreeName: "main"}
	beams := func() ([]dobbyengine.BeamPair, error) {
		return []dobbyengine.BeamPair{{Slug: "beam-a"}}, nil
	}
	failing := func() ([]dobbyengine.BeamPair, error) { return nil, errors.New("origin unreadable") }

	tests := []struct {
		name     string
		worktree string
		beams    func() ([]dobbyengine.BeamPair, error)
		want     bool
		wantErr  bool
	}{
		{"Prime", "main", failing, true, false},
		{"Beam", "beam-a", beams, true, false},
		{"TaskPair", "task-b", beams, false, false},
		{"BeamsUnreadable", "beam-a", failing, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := worktreeHasDobby(prime, tt.worktree, tt.beams)
			if (err != nil) != tt.wantErr {
				t.Fatalf("worktreeHasDobby(%q) error = %v; want error %v", tt.worktree, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("worktreeHasDobby(%q) = %v; want %v", tt.worktree, got, tt.want)
			}
		})
	}
}
