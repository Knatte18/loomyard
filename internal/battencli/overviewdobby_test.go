// overviewdobby_test.go pins the dobby's lines.
// It covers DobbyLines over built views and cells, and the window frame that draws them above the orchs.

package battencli

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestDobbyLines(t *testing.T) {
	tiergate.Require(t, 0)
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
					{Slug: "flying", PassInFlight: true, LandNotice: true},
					{Slug: "failing", LastPassExit: minutesAgo(12), FullPassOwed: true, FailingPassCount: 2, FailingPassError: "pre-run refused\nmore detail"},
				},
				Waiting: []dobbyengine.WaitingTask{
					{Slug: "ready"},
					{Slug: "deps", Dependencies: []string{"x", "y"}, Paused: true},
					{Slug: "refused", Refusal: "dirty source", NextTry: overviewNow.Add(4 * time.Minute)},
					{Slug: "owned", OwnedElsewhere: true},
					{Slug: "capped", AtMaxRuns: true},
					{Slug: "done", StartRefusal: "already completed\nmore detail", Refusal: "already completed\nmore detail"},
					{Slug: "retry", Refusal: "dirty source"},
					{Slug: "behind", FrontFirst: "fix"},
				},
			},
			cells: map[string]RunCells{
				"flying":  {State: "running", Stage: "6.5 Plan-Write", InStage: "30m"},
				"failing": {State: "blocked", Stage: "7. Land", InStage: "1h12m"},
			},
			want: []string{
				"dobby prime: alive, max_runs 2",
				"  flying   running  6.5 Plan-Write  30m    pass in flight, land_notice",
				"  failing  blocked  7. Land         1h12m  last pass 12m ago, full pass owed, failing 2× in a row: pre-run refused",
				"  ready    ready to start",
				"  deps     waits on dependencies x, y; a pause",
				"  refused  waits on a refused start (dirty source), retry in 4m",
				"  owned    waits on another machine's ownership",
				"  capped   waits on max_runs 2",
				"  done     waits on a start batten refuses: already completed",
				"  retry    ready to start, last refused start: dirty source",
				"  behind   waits on front task fix runs first",
			},
		},
		{
			name:  "ADeadDobbyNamesTheWayForwardAndARunWithNoPassSaysSo",
			view:  dobbyengine.View{MaxRuns: 3, Runs: []dobbyengine.ViewRun{{Slug: "r"}}},
			cells: map[string]RunCells{"r": {State: "paused 4m", Stage: "6. Run-Shed", InStage: "-"}},
			want: []string{
				"dobby prime: not running; way forward: lym beam start",
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
			name: "ARunThatOwesAPushOrACodeTipEndsItsPassCellWithWhatItOwes",
			view: dobbyengine.View{MaxRuns: 3, Runs: []dobbyengine.ViewRun{{Slug: "owing"}, {Slug: "tip"}}},
			cells: map[string]RunCells{
				"owing": {State: "halted", Stage: "7. Land", InStage: "3m", Owes: "push owed: origin unreachable"},
				"tip":   {State: "running", Stage: "6. Run-Shed", InStage: "1h12m", Owes: "code tip 0123456789ab missing on origin, owed by oslo"},
			},
			want: []string{
				"dobby prime: not running; way forward: lym beam start",
				"  owing  halted   7. Land      3m     no pass yet, push owed: origin unreachable",
				"  tip    running  6. Run-Shed  1h12m  no pass yet, code tip 0123456789ab missing on origin, owed by oslo",
			},
		},
		{
			name: "AReadyBeamHoldsAWaitingTaskAndNamesTheCancel",
			view: dobbyengine.View{Alive: true, MaxRuns: 2, Waiting: []dobbyengine.WaitingTask{{Slug: "held", ReadyHold: true}}},
			want: []string{
				"dobby prime: alive, max_runs 2",
				"  no runs driven",
				"  held  waits on the beam is flagged ready: lym batten cancel held --remove",
			},
		},
		{
			name: "AKeptPairShowsWhenItsCancelFinishedAndNamesTheRemoval",
			view: dobbyengine.View{Alive: true, MaxRuns: 3, Kept: []dobbyengine.KeptPair{{Slug: "k", Since: minutesAgo(12)}}},
			want: []string{
				"dobby prime: alive, max_runs 3",
				"  no runs driven",
				"  k  kept pair, cancelled 12m ago; way forward: lym fabric remove k",
			},
		},
		{
			name: "AKeptPairWithNoFinishTimeSaysItsAgeIsUnknown",
			view: dobbyengine.View{Alive: true, MaxRuns: 3, Kept: []dobbyengine.KeptPair{{Slug: "k"}}},
			want: []string{
				"dobby prime: alive, max_runs 3",
				"  no runs driven",
				"  k  kept pair, age unknown; way forward: lym fabric remove k",
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
	tiergate.Require(t, 0)
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
	tiergate.Require(t, 0)
	t.Parallel()
	prime := &lymcwd.Location{WorktreeName: "main"}
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
