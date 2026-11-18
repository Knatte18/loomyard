// pauseconfirm_test.go pins PauseAndConfirm called with no request file, as the hub stop calls it:
// which entry it owns, the caller label that entry records, and the reason an unconfirmed pause names.

package hibernateengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/tenderengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestPauseAndConfirm(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const caller = "beam-stop"
	busy := []statusReading{{sessions: []tenderengine.SessionStatus{sessionReading("a", "busy")}}}
	tests := []struct {
		name string
		// standing adds a live hub entry of another caller before the pause.
		standing     bool
		readings     []statusReading
		wantOwned    bool
		wantOutcome  Confirmation
		wantWithheld string
		wantCallers  []string
	}{
		{
			name: "an idle hub confirms and owns the entry the pause added under the caller's label", wantOwned: true,
			wantOutcome: ConfirmationReached, wantCallers: []string{caller},
		},
		{
			name: "a hub entry already standing covers the caller, which owns none and leaves it live", standing: true,
			wantOutcome: ConfirmationReached, wantCallers: []string{"EXTERNAL"},
		},
		{
			name: "a session busy through the whole wait times out naming it and lifts nothing", readings: busy, wantOwned: true,
			wantOutcome: ConfirmationTimedOut, wantWithheld: "a reads busy", wantCallers: []string{caller},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newFakeWorld(t)
			if tt.standing {
				w.addHubEntry("EXTERNAL")
			}
			w.readings = tt.readings

			got, err := PauseAndConfirm(w.deps(), PauseConfirmRequest{Caller: caller, Requester: requesterExternal})

			if err != nil {
				t.Fatalf("PauseAndConfirm() error = %v; want none", err)
			}
			if got.Outcome != tt.wantOutcome || got.Withheld != tt.wantWithheld {
				t.Errorf("PauseAndConfirm() = %+v; want outcome %d withheld %q", got, tt.wantOutcome, tt.wantWithheld)
			}
			if (got.Owned != nil) != tt.wantOwned {
				t.Errorf("PauseAndConfirm() owned = %+v; want owned %v", got.Owned, tt.wantOwned)
			}
			if tt.wantOwned && (got.Owned.Scope != pauserecord.ScopeHub || got.Owned.Caller != caller) {
				t.Errorf("PauseAndConfirm() owned = %+v; want the hub entry under %q", got.Owned, caller)
			}
			var callers []string
			for _, live := range w.liveEntries() {
				callers = append(callers, live.Caller)
			}
			if len(callers) != len(tt.wantCallers) || (len(callers) == 1 && callers[0] != tt.wantCallers[0]) {
				t.Errorf("live entry callers = %v; want %v", callers, tt.wantCallers)
			}
		})
	}
}
