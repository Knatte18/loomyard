// looparm_test.go pins the hook loom arms on the loop's step watchdog: what it stops and what it tells the interrupted cause.

package loomcli

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func TestWatchdogKillNote_StopsTheKilledStepsRunsAndNamesTheRest(t *testing.T) {
	tests := []struct {
		name     string
		report   shuttleengine.StepStopReport
		stopErr  error
		wantNote string
	}{
		{name: "nothing live leaves an empty note"},
		{
			name:     "stopped and untied runs are both named",
			report:   shuttleengine.StepStopReport{Stopped: []string{"a:1:plan", "a:1:review"}, Untied: []string{"a:1:driver"}},
			wantNote: "stopped shuttle runs: a:1:plan, a:1:review; left live runs naming no step: a:1:driver",
		},
		{
			name:     "a run whose stop failed names its error",
			report:   shuttleengine.StepStopReport{Failed: []shuttleengine.StepStopFailure{{Strand: "a:1:plan", Error: "strand busy"}}},
			wantNote: "could not stop the run a:1:plan: strand busy",
		},
		{
			name:     "a failing stop renders its error",
			stopErr:  errors.New("probe reed: no server"),
			wantNote: "could not stop the step's runs: probe reed: no server",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ties []shuttleengine.StepTie
			stop := func(tie shuttleengine.StepTie) (shuttleengine.StepStopReport, error) {
				ties = append(ties, tie)
				return tt.report, tt.stopErr
			}

			note := watchdogKillNote(stop, "/steps")(shedverbs.WatchdogKill{TraceID: "trace-9"})

			if want := []shuttleengine.StepTie{{StepsDir: "/steps", TraceID: "trace-9"}}; !reflect.DeepEqual(ties, want) {
				t.Errorf("stop ties = %+v; want %+v", ties, want)
			}
			if note != tt.wantNote {
				t.Errorf("note = %q; want %q", note, tt.wantNote)
			}
		})
	}
}
