// sessionreads_test.go covers the liveness and registration reads of one kept session's strand.

package reedengine_test

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSessionReads asserts the liveness read finds a strand live only while it is live and not retiring.
// The registrar registers the pane process of a strand that has one, and refuses a strand that is absent or has none.
// A failing table read passes through.
func TestSessionReads(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	strand := func(guid string, live, retiring bool, panePID int) reedengine.StrandStatus {
		return reedengine.StrandStatus{GUID: guid, Live: live, Retiring: retiring, PanePID: panePID}
	}
	statusErr := errors.New("reed down")
	registerErr := errors.New("disk full")

	tests := []struct {
		name         string
		strands      []reedengine.StrandStatus
		statusErr    error
		registerErr  error
		wantLive     bool
		wantLiveErr  error
		wantPID      int
		wantRegister bool
		wantRegErr   error
	}{
		{name: "LiveStrandIsLiveAndRegistersItsPanePID", strands: []reedengine.StrandStatus{strand("g", true, false, 4242)}, wantLive: true, wantPID: 4242, wantRegister: true},
		{name: "DeadStrandIsNotLiveAndHasNoPaneToRegister", strands: []reedengine.StrandStatus{strand("g", false, false, 0)}},
		{name: "RetiringStrandIsNotLive", strands: []reedengine.StrandStatus{strand("g", true, true, 4242)}, wantPID: 4242, wantRegister: true},
		{name: "AbsentStrandIsNotLive", strands: []reedengine.StrandStatus{strand("other", true, false, 1)}},
		{name: "TableReadFailureIsPassedThrough", statusErr: statusErr, wantLiveErr: statusErr, wantRegErr: statusErr},
		{name: "RegisterFailureIsPassedThrough", strands: []reedengine.StrandStatus{strand("g", true, false, 4242)}, registerErr: registerErr, wantLive: true, wantPID: 4242, wantRegister: true, wantRegErr: registerErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status := func() (reedengine.StatusResult, error) {
				return reedengine.StatusResult{Strands: tt.strands}, tt.statusErr
			}
			live, err := reedengine.SessionLiveness(status)("g")
			if live != tt.wantLive || !errors.Is(err, tt.wantLiveErr) {
				t.Errorf("SessionLiveness()(%q) = %v, %v; want %v, %v", "g", live, err, tt.wantLive, tt.wantLiveErr)
			}

			gotPID := 0
			err = reedengine.SessionRegistrar(status, func(pid int) error {
				gotPID = pid
				return tt.registerErr
			})("g")
			if gotPID != tt.wantPID {
				t.Errorf("SessionRegistrar() registered pid %d; want %d", gotPID, tt.wantPID)
			}
			wantErr := tt.wantRegErr != nil || !tt.wantRegister
			if (err != nil) != wantErr || (tt.wantRegErr != nil && !errors.Is(err, tt.wantRegErr)) {
				t.Errorf("SessionRegistrar()(%q) error = %v; want error %v (%v)", "g", err, wantErr, tt.wantRegErr)
			}
		})
	}
}
