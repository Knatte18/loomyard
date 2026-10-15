// stopkey_test.go covers StopKeyOf's reading of a status.

package sheddriver

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

func TestStopKeyOf(t *testing.T) {
	t.Parallel()
	st := shedengine.Status{CurrentProducer: "Plan-Review", State: shedengine.StateBlocked, History: []shedengine.HistoryEntry{{}, {}}}
	want := StopKey{CurrentProducer: "Plan-Review", HistoryLength: 2, State: "blocked"}
	if got := StopKeyOf(st); got != want {
		t.Errorf("StopKeyOf(...) = %+v; want %+v", got, want)
	}
}
