// hibernatedaemon_test.go pins how the anchor verb reads the hub's hibernate request.
// It covers which phases count as ended, and when a request holds a running carrier.

package fabriccli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/hibernatecli"
	"github.com/Knatte18/loomyard/internal/hibernateengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestHibernateEndedPhases_AreTheEnginesTerminalPhases(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	terminal := hibernateengine.TerminalPhases()
	ended := fabriccli.HibernateEndedPhasesForTest()

	for _, phase := range terminal {
		if !slices.Contains(ended, string(phase)) {
			t.Errorf("ended phases = %v; want the engine's terminal phase %q in them", ended, phase)
		}
	}
	for _, name := range ended {
		if !slices.Contains(terminal, hibernateengine.Phase(name)) {
			t.Errorf("ended phase %q is not a terminal phase of the engine %v", name, terminal)
		}
	}
}

func TestHibernateDaemonLive(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	selfPID := os.Getpid()
	selfStart, _ := proc.StartTime(selfPID)
	tests := []struct {
		name  string
		write bool
		req   hibernateengine.Request
		want  bool
	}{
		{"NoRequestFile", false, hibernateengine.Request{}, false},
		{"WaitingRequestNamingALiveCarrier", true, hibernateengine.Request{DaemonPID: selfPID, DaemonStart: selfStart, Phase: hibernateengine.PhaseWaiting}, true},
		{"PendingRequestNamingADeadCarrier", true, hibernateengine.Request{DaemonPID: selfPID, DaemonStart: "not-the-start-time", Phase: hibernateengine.PhaseWaiting}, false},
		{"PendingRequestNamingNoCarrier", true, hibernateengine.Request{DaemonPID: 0, Phase: hibernateengine.PhaseWaiting}, false},
		{"DoneRequestStillNamingALiveCarrier", true, hibernateengine.Request{DaemonPID: selfPID, DaemonStart: selfStart, Phase: hibernateengine.PhaseDone}, false},
		{"FailedRequestStillNamingALiveCarrier", true, hibernateengine.Request{DaemonPID: selfPID, DaemonStart: selfStart, Phase: hibernateengine.PhaseFailed}, false},
		{"CancelledRequestStillNamingALiveCarrier", true, hibernateengine.Request{DaemonPID: selfPID, DaemonStart: selfStart, Phase: hibernateengine.PhaseCancelled}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hub := t.TempDir()
			if tt.write {
				path := hibernatecli.RequestPath(&lymcwd.Location{HubPath: hub})
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				data, err := json.Marshal(tt.req)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0o644); err != nil {
					t.Fatal(err)
				}
			}

			got, err := fabriccli.HibernateDaemonLiveForTest(hub)

			if err != nil || got != tt.want {
				t.Errorf("hibernateDaemonLive(%+v) = %v, %v; want %v, nil", tt.req, got, err, tt.want)
			}
		})
	}
}
