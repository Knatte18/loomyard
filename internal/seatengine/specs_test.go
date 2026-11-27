// specs_test.go covers the specs a table's seats are composed with ahead of a start.

package seatengine

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSpecs_MatchTheStartedSpecs pins that Specs returns each seat's spec as the engine starts it.
// The lead's hold deferral is the one difference.
func TestSpecs_MatchTheStartedSpecs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	shuttle := newFakeShuttle(t)
	engine, geom := engineFixture(t, shuttle)
	table := engineTable(geom.WorktreeRoot, 2)

	want, err := Specs(geom, table)
	if err != nil {
		t.Fatalf("Specs() error = %v", err)
	}
	ended := runAsync(engine, table)
	shuttle.awaitStarts(len(table.Seats))
	shuttle.handle(role("")).release(shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil)
	if end := await(t, ended); end.err != nil {
		t.Fatalf("Run() error = %v", end.err)
	}

	for _, seat := range table.Seats {
		started := shuttle.spec(role(seat.Name))
		if seat.Name == leadName && started.HoldDeferral == nil {
			t.Error("the lead was started with no hold deferral")
		}
		started.HoldDeferral = nil
		composed := want[seat.Name]
		composed.HoldDeferral = nil
		if !reflect.DeepEqual(composed, started) {
			t.Errorf("Specs()[%q] = %+v; want the started spec %+v", seat.Name, composed, started)
		}
	}
}
