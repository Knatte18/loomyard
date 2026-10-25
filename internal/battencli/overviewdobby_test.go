//go:build tier0

// overviewdobby_test.go pins the dobby's lines: DobbyLines over built views, and the overview frame that draws them below the runs.

package battencli

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
)

func TestDobbyLines(t *testing.T) {
	t.Parallel()
	runShed := overviewStage(shedrun.RecipeBatten, battenrecipe.NameRunShed)
	land := overviewStage(shedrun.RecipeBatten, battenrecipe.NameLand)
	pad := func(stage string) string { return padRight(stage, max(len([]rune(runShed)), len([]rune(land)))) }

	tests := []struct {
		name string
		view dobbyengine.View
		want []string
	}{
		{
			name: "AnAliveDobbyShowsItsRunsAndWhyEachTaskWaits",
			view: dobbyengine.View{
				Alive: true, MaxRuns: 2,
				Runs: []dobbyengine.ViewRun{
					{Slug: "flying", State: "running", Producer: battenrecipe.NameRunShed, PassInFlight: true},
					{Slug: "failing", State: "blocked", Producer: battenrecipe.NameLand, LastPassExit: minutesAgo(12), FullPassOwed: true, FailingPassCount: 2, FailingPassError: "pre-run refused\nmore detail"},
				},
				Waiting: []dobbyengine.WaitingTask{
					{Slug: "ready"},
					{Slug: "deps", Dependencies: []string{"x", "y"}, Paused: true},
					{Slug: "refused", Refusal: "dirty source", NextTry: overviewNow.Add(4 * time.Minute)},
					{Slug: "owned", OwnedElsewhere: true},
					{Slug: "capped", AtMaxRuns: true},
				},
			},
			want: []string{
				"dobby prime: alive, max_runs 2",
				"  flying   running  " + pad(runShed) + "  pass in flight",
				"  failing  blocked  " + pad(land) + "  last pass 12m ago, full pass owed, failing 2× in a row: pre-run refused",
				"  ready    ready to start",
				"  deps     waits on dependencies x, y; a pause",
				"  refused  waits on a refused start (dirty source), retry in 4m",
				"  owned    waits on another machine's ownership",
				"  capped   waits on max_runs 2",
			},
		},
		{
			name: "ADeadDobbyNamesTheWayForwardAndARunWithNoPassSaysSo",
			view: dobbyengine.View{MaxRuns: 3, Runs: []dobbyengine.ViewRun{{Slug: "r", State: "running", Producer: battenrecipe.NameRunShed}}},
			want: []string{
				"dobby prime: not running; way forward: lyx beam start",
				"  r  running  " + runShed + "  no pass yet",
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
			if diff := cmp.Diff(tt.want, DobbyLines(dobbyengine.PrimeLabel, tt.view, overviewNow)); diff != "" {
				t.Errorf("DobbyLines() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestOverviewSource_DrawsTheDobbyBelowTheRuns(t *testing.T) {
	t.Parallel()
	prime := locationkit.Location(t.TempDir(), "prime", ".")
	source := newOverviewSource(prime, filepath.Join(t.TempDir(), "pause.json"), overviewThresholds{}, func() time.Time { return overviewNow })
	var drawnAt time.Time
	source.dobby = func(now time.Time) []string {
		drawnAt = now
		return []string{"dobby prime: alive, max_runs 3", "  no runs driven"}
	}

	want := []string{overviewNoRuns, "", "dobby prime: alive, max_runs 3", "  no runs driven"}
	if diff := cmp.Diff(want, source.frame().Lines); diff != "" {
		t.Errorf("frame() mismatch (-want +got):\n%s", diff)
	}
	if !drawnAt.Equal(overviewNow) {
		t.Errorf("the dobby was drawn at %v; want the frame's time %v", drawnAt, overviewNow)
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
