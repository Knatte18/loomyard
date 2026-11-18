// accountswitch_test.go covers Runner.SwitchAccountOnLimit: how it reaches the engine of a run found by its strand GUID.

package shuttleengine

import (
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// switchAccountEngine is a fakeEngine that implements AccountSwitcher with a scripted answer.
// It records the run directory and turn end each ask carried.
type switchAccountEngine struct {
	*fakeEngine

	relaunch bool
	mu       sync.Mutex
	runDirs  []string
	turnEnds []Event
}

// SwitchAccount records the ask and answers the scripted relaunch.
func (e *switchAccountEngine) SwitchAccount(runDir string, turnEnd Event) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.runDirs = append(e.runDirs, runDir)
	e.turnEnds = append(e.turnEnds, turnEnd)
	return e.relaunch, nil
}

// TestRunner_SwitchAccountOnLimit pins that the runner asks the engine the run's record names, with the run's directory.
func TestRunner_SwitchAccountOnLimit(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// engine is the engine of the set the run starts on.
		engine string
		// guid is the strand GUID the switch is asked for.
		guid         string
		want         bool
		wantErr      bool
		wantAskedRun bool
	}{
		{name: "AKnownGUIDReachesTheEngineWithTheRunDirectory", engine: "switcher", guid: "strand-1", want: true, wantAskedRun: true},
		{name: "AnEngineWithoutTheCapabilityAnswersFalse", engine: "plain", guid: "strand-1"},
		{name: "AnUnknownGUIDIsAnError", engine: "switcher", guid: "nope", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			switcher := &switchAccountEngine{fakeEngine: &fakeEngine{}, relaunch: true}
			plain := &fakeEngine{}
			engines := NewEngineSet("switcher", map[string]Engine{"switcher": switcher, "plain": plain})
			reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}, CaptureQueue: []string{"IDLE"}}
			readyStart(reed, switcher.fakeEngine)
			readyStart(reed, plain)
			fx := newFixture(t, reed, switcher, withEngines(engines), withConfig(fastConfig), withSeparateRunDir())
			if _, err := fx.Runner.Start(Spec{Prompt: "x", OutputFiles: []string{"out.md"}, ModelChoice: ModelChoice{Engine: tt.engine}}); err != nil {
				t.Fatalf("Start(engine %q) error: %v", tt.engine, err)
			}
			_, runDir, err := FindRun(fx.Runner.cfg, fx.Anchor, "strand-1")
			if err != nil {
				t.Fatalf("FindRun(strand-1): %v", err)
			}

			turnEnd := Event{Kind: EventStop, Raw: []byte(`{"transcript_path":"t.jsonl"}`)}
			got, err := fx.Runner.SwitchAccountOnLimit(tt.guid, turnEnd)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SwitchAccountOnLimit(%q) error = %v; want error %v", tt.guid, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("SwitchAccountOnLimit(%q) = %v; want %v", tt.guid, got, tt.want)
			}
			asked := len(switcher.runDirs) > 0
			if asked != tt.wantAskedRun {
				t.Fatalf("the engine was asked = %v; want %v", asked, tt.wantAskedRun)
			}
			if asked && (switcher.runDirs[0] != runDir || string(switcher.turnEnds[0].Raw) != string(turnEnd.Raw)) {
				t.Errorf("the engine was asked with run directory %q and turn end %q; want %q and %q", switcher.runDirs[0], switcher.turnEnds[0].Raw, runDir, turnEnd.Raw)
			}
		})
	}
}
