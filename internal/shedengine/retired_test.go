// retired_test.go covers a step's move of a run off a row the recipe's Retired map names.

package shedengine

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// retiredMoveCall is one recorded call of Shed.RetiredMove.
type retiredMoveCall struct {
	from, to string
	resumed  bool
}

func TestStep_RetiredRowMovesAndPauses(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const wayForward = `"lym batten run some-slug"`
	moveCause := errors.New("the tracked ready record is unreadable")

	tests := []struct {
		name       string
		current    string
		state      State
		moveErr    error
		wantMoves  []retiredMoveCall
		wantErr    bool
		wantACalls int
	}{
		{
			name:      "blocked status moves resumed and pauses",
			current:   "Old",
			state:     StateBlocked,
			wantMoves: []retiredMoveCall{{from: "Old", to: "B", resumed: true}},
		},
		{
			name:      "running status moves not resumed and pauses",
			current:   "Old",
			state:     StateRunning,
			wantMoves: []retiredMoveCall{{from: "Old", to: "B", resumed: false}},
		},
		{
			name:      "a move error persists nothing and names the cause and way forward",
			current:   "Old",
			state:     StateBlocked,
			moveErr:   moveCause,
			wantMoves: []retiredMoveCall{{from: "Old", to: "B", resumed: true}},
			wantErr:   true,
		},
		{
			name:       "a row of the recipe never consults the retired map",
			current:    "A",
			state:      StateRunning,
			wantACalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shed, a, b := gotoShed(t)
			shed.Retired = map[string]string{"Old": "B", "A": "B"}
			shed.RetiredWayForward = wayForward
			var moves []retiredMoveCall
			shed.RetiredMove = func(from, to string, resumed bool) error {
				moves = append(moves, retiredMoveCall{from: from, to: to, resumed: resumed})
				return tt.moveErr
			}
			seed := commonSeed(tt.current)
			seed.State = tt.state
			seedStatus(t, shed.StatusPath, shed.StatusLockPath, seed)
			before := readStatus(t, shed.StatusPath, shed.StatusLockPath)

			res, err := shed.Step(context.Background())

			if !slices.Equal(moves, tt.wantMoves) {
				t.Errorf("RetiredMove calls = %v; want %v", moves, tt.wantMoves)
			}
			if a.calls != tt.wantACalls || b.calls != 0 {
				t.Errorf("calls A, B = %d, %d; want %d, 0", a.calls, b.calls, tt.wantACalls)
			}
			after := readStatus(t, shed.StatusPath, shed.StatusLockPath)

			if tt.wantErr {
				if err == nil {
					t.Fatal("Step(...) = nil error; want the move's failure")
				}
				for _, want := range []string{"Old", moveCause.Error(), wayForward} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				}
				if !reflect.DeepEqual(before, after) {
					t.Errorf("status file changed on a failed move")
				}
				return
			}
			if err != nil {
				t.Fatalf("Step(...) = _, %v; want nil error", err)
			}
			if len(tt.wantMoves) == 0 {
				if res.Producer != "A" {
					t.Errorf("Step Producer = %q; want A stepped", res.Producer)
				}
				return
			}

			wantReason := "the row Old is retired from the recipe, so the run moved to B and paused; way forward: " + wayForward
			if after.CurrentProducer != "B" || after.State != StatePaused || after.PauseReason != wantReason {
				t.Errorf("CurrentProducer, State, PauseReason = %q, %q, %q; want B, paused, %q", after.CurrentProducer, after.State, after.PauseReason, wantReason)
			}
			if res.Next != "B" || res.State != StatePaused || res.Reason != wantReason || res.Producer != "" {
				t.Errorf("Step = %+v; want Next B, paused, Reason %q, no producer called", res, wantReason)
			}
		})
	}
}
