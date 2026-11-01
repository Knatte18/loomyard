// land_test.go pins the past-Land test and the outcome it tells apart.

package battenrecipe_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLandOutcomeOf(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	landDone := []shedengine.HistoryEntry{
		{Producer: battenrecipe.NameRunShed, Outcome: shedengine.Done},
		{Producer: battenrecipe.NameLand, Outcome: shedengine.Done},
	}
	landStuck := []shedengine.HistoryEntry{
		{Producer: battenrecipe.NameRunShed, Outcome: shedengine.Done},
		{Producer: battenrecipe.NameLand, Outcome: shedengine.Stuck},
	}
	tests := []struct {
		name     string
		status   shedengine.Status
		wantPast bool
		want     battenrecipe.LandOutcome
	}{
		{"RunningAtRunShed", shedengine.Status{State: shedengine.StateRunning, CurrentProducer: battenrecipe.NameRunShed}, false, battenrecipe.LandOutcomeAhead},
		{"BlockedAtLand", shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: battenrecipe.NameLand, History: landStuck}, false, battenrecipe.LandOutcomeAhead},
		{"RunningAtTeardownAfterLand", shedengine.Status{State: shedengine.StateRunning, CurrentProducer: battenrecipe.NameWorktreeTeardown, History: landDone}, true, battenrecipe.LandOutcomeLanded},
		{"BlockedAtTeardownAfterLand", shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: battenrecipe.NameWorktreeTeardown, History: landDone}, true, battenrecipe.LandOutcomeLanded},
		{"DoneAfterLand", shedengine.Status{State: shedengine.StateDone, CurrentProducer: battenrecipe.NameWorktreeTeardown, History: landDone}, true, battenrecipe.LandOutcomeLanded},
		{"DoneWithoutLandDone", shedengine.Status{State: shedengine.StateDone, CurrentProducer: battenrecipe.NameWorktreeTeardown, History: landStuck}, true, battenrecipe.LandOutcomeUnlanded},
		{"DoneAtLandWithNoHistory", shedengine.Status{State: shedengine.StateDone, CurrentProducer: battenrecipe.NameLand}, true, battenrecipe.LandOutcomeUnlanded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := battenrecipe.PastLand(tt.status.State, tt.status.CurrentProducer); got != tt.wantPast {
				t.Errorf("PastLand(%q, %q) = %v; want %v", tt.status.State, tt.status.CurrentProducer, got, tt.wantPast)
			}
			if got := battenrecipe.LandOutcomeOf(tt.status); got != tt.want {
				t.Errorf("LandOutcomeOf() = %q; want %q", got, tt.want)
			}
		})
	}
}
