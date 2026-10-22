// apply_test.go table-tests engineAliveIDs, the predicate that reads a pane whose engine exited to its start shell as dead.

package reedengine

import (
	"reflect"
	"testing"
	"time"
)

func TestEngineAliveIDs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		pane       LivePane
		age        time.Duration
		ageKnown   bool
		skipShell  bool
		wantAlive  bool
		wantAgeHit bool
	}{
		{name: "EngineInTheForeground", pane: LivePane{ID: "%1", CurrentCommand: "claude", StartCommand: "/bin/bash"}, age: 31 * time.Second, ageKnown: true, wantAlive: true},
		{name: "ShellOnlyPastTheGrace", pane: LivePane{ID: "%1", CurrentCommand: "bash", StartCommand: "/bin/bash"}, age: 31 * time.Second, ageKnown: true, wantAgeHit: true},
		{name: "ShellOnlyInsideTheGrace", pane: LivePane{ID: "%1", CurrentCommand: "bash", StartCommand: "/bin/bash"}, age: 29 * time.Second, ageKnown: true, wantAlive: true, wantAgeHit: true},
		{name: "ShellOnlyWithUnreadableAge", pane: LivePane{ID: "%1", CurrentCommand: "bash", StartCommand: "/bin/bash"}, wantAlive: true, wantAgeHit: true},
		{name: "EmptyStartCommand", pane: LivePane{ID: "%1", CurrentCommand: "bash"}, age: 31 * time.Second, ageKnown: true, wantAlive: true},
		{name: "EmptyCurrentCommand", pane: LivePane{ID: "%1", StartCommand: "/bin/bash"}, age: 31 * time.Second, ageKnown: true, wantAlive: true},
		{name: "LoginShellOverQuotedPin", pane: LivePane{ID: "%1", CurrentCommand: "-bash", StartCommand: `"/bin/bash"`}, age: 31 * time.Second, ageKnown: true, wantAgeHit: true},
		{name: "ShellCheckSkipped", pane: LivePane{ID: "%1", CurrentCommand: "bash", StartCommand: "/bin/bash"}, age: 31 * time.Second, ageKnown: true, skipShell: true, wantAlive: true},
		{name: "DeadPaneWhateverItsCommands", pane: LivePane{ID: "%1", Dead: true, CurrentCommand: "claude", StartCommand: "/bin/bash"}, age: 31 * time.Second, ageKnown: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ageCalled := false
			age := func(pid int) (time.Duration, bool) {
				ageCalled = true
				return tt.age, tt.ageKnown
			}
			got := engineAliveIDs([]LivePane{tt.pane}, age, !tt.skipShell)
			want := map[string]bool{}
			if tt.wantAlive {
				want[tt.pane.ID] = true
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("engineAliveIDs(%+v) = %v; want %v", tt.pane, got, want)
			}
			if ageCalled != tt.wantAgeHit {
				t.Errorf("engineAliveIDs(%+v) read the pane age = %v; want %v", tt.pane, ageCalled, tt.wantAgeHit)
			}
		})
	}
}
