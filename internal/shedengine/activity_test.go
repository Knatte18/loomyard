// activity_test.go is a table test over composeActivity's three mechanical rules.

package shedengine

import "testing"

func TestComposeActivity(t *testing.T) {
	tests := []struct {
		name            string
		currentProducer string
		history         []HistoryEntry
		state           State
		errText         string
		want            Activity
	}{
		{
			name:            "empty history yields empty Last",
			currentProducer: "Preflight",
			history:         nil,
			state:           StateRunning,
			errText:         "",
			want:            Activity{Now: "Preflight", Last: "", Wait: ""},
		},
		{
			name:            "multi-entry history composes from the last entry only",
			currentProducer: "Plan-Review",
			history: []HistoryEntry{
				{Producer: "Preflight", Outcome: Done, Output: "", At: "2026-08-15T09:00:00Z"},
				{Producer: "Plan-Write", Outcome: Done, Output: "", At: "2026-08-15T09:05:00Z"},
			},
			state:   StateRunning,
			errText: "",
			want:    Activity{Now: "Plan-Review", Last: "Plan-Write → done", Wait: ""},
		},
		{
			name:            "Wait populated for StateBlocked",
			currentProducer: "Plan-Write",
			history:         []HistoryEntry{{Producer: "Plan-Write", Outcome: Stuck, Output: "", At: "2026-08-15T09:00:00Z"}},
			state:           StateBlocked,
			errText:         "bounce budget exhausted",
			want:            Activity{Now: "Plan-Write", Last: "Plan-Write → stuck", Wait: "bounce budget exhausted"},
		},
		{
			name:            "Wait populated for StateFailed",
			currentProducer: "Plan-Write",
			history:         nil,
			state:           StateFailed,
			errText:         "engine error",
			want:            Activity{Now: "Plan-Write", Last: "", Wait: "engine error"},
		},
		{
			name:            "Wait empty for StateRunning even with non-empty errText",
			currentProducer: "Plan-Write",
			history:         nil,
			state:           StateRunning,
			errText:         "should not surface",
			want:            Activity{Now: "Plan-Write", Last: "", Wait: ""},
		},
		{
			name:            "Wait empty for StatePaused even with non-empty errText",
			currentProducer: "Plan-Write",
			history:         nil,
			state:           StatePaused,
			errText:         "should not surface",
			want:            Activity{Now: "Plan-Write", Last: "", Wait: ""},
		},
		{
			name:            "Wait empty for StateDone even with non-empty errText",
			currentProducer: "Finalize",
			history:         nil,
			state:           StateDone,
			errText:         "should not surface",
			want:            Activity{Now: "Finalize", Last: "", Wait: ""},
		},
		{
			name:            "Now echoes currentProducer including when empty",
			currentProducer: "",
			history:         nil,
			state:           StateRunning,
			errText:         "",
			want:            Activity{Now: "", Last: "", Wait: ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := composeActivity(tt.currentProducer, tt.history, tt.state, tt.errText)
			if got != tt.want {
				t.Errorf("composeActivity(%q, %v, %q, %q) = %+v; want %+v", tt.currentProducer, tt.history, tt.state, tt.errText, got, tt.want)
			}
		})
	}
}
