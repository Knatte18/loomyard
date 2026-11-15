// run_sessionholder_test.go covers the run's read of the process holding its session and of the instructions it carried.

package shuttleengine

import (
	"errors"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// holderFakeEngine is a fakeEngine with the opt-in SessionHolderFinder capability, answering a fixed holder read.
type holderFakeEngine struct {
	*fakeEngine

	pid   int
	found bool
	err   error
}

func (e *holderFakeEngine) SessionHolder(string) (int, bool, error) {
	return e.pid, e.found, e.err
}

// holderFakeEngine is checked at compile time against SessionHolderFinder.
var _ SessionHolderFinder = (*holderFakeEngine)(nil)

func TestRun_SessionHolder(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	readFailure := errors.New("process table unreadable")
	tests := []struct {
		name      string
		engine    Engine
		sessionID string
		wantPID   int
		wantFound bool
		wantErr   error
	}{
		{name: "EngineNamingTheHolder", engine: &holderFakeEngine{fakeEngine: &fakeEngine{}, pid: 4242, found: true}, sessionID: "sess", wantPID: 4242, wantFound: true},
		{name: "EngineFindingNoHolder", engine: &holderFakeEngine{fakeEngine: &fakeEngine{}}, sessionID: "sess"},
		{name: "EngineFailingTheRead", engine: &holderFakeEngine{fakeEngine: &fakeEngine{}, err: readFailure}, sessionID: "sess", wantErr: readFailure},
		{name: "EngineWithoutTheCapability", engine: &fakeEngine{}, sessionID: "sess"},
		{name: "RunNamingNoSessionAsksNoEngine", engine: &holderFakeEngine{fakeEngine: &fakeEngine{}, pid: 4242, found: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := &Run{engine: tt.engine, state: RunState{SessionID: tt.sessionID}}

			pid, found, err := run.SessionHolder()

			if pid != tt.wantPID || found != tt.wantFound || !errors.Is(err, tt.wantErr) {
				t.Errorf("SessionHolder() = %d, %v, %v; want %d, %v, %v", pid, found, err, tt.wantPID, tt.wantFound, tt.wantErr)
			}
		})
	}
}

func TestRun_CarriedInstructions(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	run := &Run{state: RunState{CarriedInstructions: []string{"keep the branch", "ask before pushing"}}}

	if got, want := run.CarriedInstructions(), []string{"keep the branch", "ask before pushing"}; !slices.Equal(got, want) {
		t.Errorf("CarriedInstructions() = %v; want %v", got, want)
	}
}
