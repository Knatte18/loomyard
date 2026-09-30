// activity_test.go is a table test over composeActivity's three mechanical rules.

package shedengine

import (
	"context"
	"testing"
)

func TestComposeActivity(t *testing.T) {
	tests := []struct {
		name            string
		currentProducer string
		history         []HistoryEntry
		state           State
		errText         string
		routedTo        string
		want            Activity
	}{
		{
			name:            "routed stuck composes as a bounce",
			currentProducer: "Plan-Write",
			history:         []HistoryEntry{{Producer: "Plan-Review", Outcome: Stuck, At: "2026-08-15T09:00:00Z"}},
			state:           StateRunning,
			routedTo:        "Plan-Write",
			want:            Activity{Now: "Plan-Write", Last: "Plan-Review → bounced to Plan-Write"},
		},
		{
			name:            "no routing target keeps the outcome for a done entry",
			currentProducer: "Plan-Review",
			history:         []HistoryEntry{{Producer: "Plan-Write", Outcome: Done, At: "2026-08-15T09:00:00Z"}},
			state:           StateRunning,
			want:            Activity{Now: "Plan-Review", Last: "Plan-Write → done"},
		},
		{
			name:            "awaiting entry keeps its outcome",
			currentProducer: "Publish",
			history:         []HistoryEntry{{Producer: "Publish", Outcome: Awaiting, At: "2026-08-15T09:00:00Z"}},
			state:           StateAwaiting,
			errText:         "review",
			want:            Activity{Now: "Publish", Last: "Publish → awaiting", Wait: "review"},
		},
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
			errText:         ReasonBounceBudgetExhausted,
			want:            Activity{Now: "Plan-Write", Last: "Plan-Write → stuck", Wait: ReasonBounceBudgetExhausted},
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
			got := composeActivity(tt.currentProducer, tt.history, tt.state, tt.errText, tt.routedTo)
			if got != tt.want {
				t.Errorf("composeActivity(%q, %v, %q, %q, %q) = %+v; want %+v", tt.currentProducer, tt.history, tt.state, tt.errText, tt.routedTo, got, tt.want)
			}
		})
	}
}

func stuckProducer() *funcProducer {
	return &funcProducer{
		fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
			return Stuck, OutputPointer{Reason: "why"}, nil
		},
	}
}

func TestStep_RoutedStuckReadsAsBounce(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: fixedOutcomeProducer(Done, ""), OnDone: "B"},
		{Name: "B", Producer: stuckProducer(), OnStuck: "A"},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("B"))

	if _, err := shed.Step(context.Background()); err != nil {
		t.Fatalf("Step = _, %v", err)
	}
	got := readStatus(t, statusPath, statusLockPath)
	if want := "B → bounced to A"; got.Activity.Last != want {
		t.Errorf("Activity.Last = %q; want %q", got.Activity.Last, want)
	}
	if got.History[0].Outcome != Stuck {
		t.Errorf("History[0].Outcome = %q; want stuck (persisted vocabulary unchanged)", got.History[0].Outcome)
	}

	// A pause right after the bounce appends no history and keeps the wording.
	got.PauseRequested = true
	seedStatus(t, statusPath, statusLockPath, got)
	if _, err := shed.Step(context.Background()); err != nil {
		t.Fatalf("Step (pause) = _, %v", err)
	}
	got = readStatus(t, statusPath, statusLockPath)
	if got.State != StatePaused {
		t.Fatalf("State = %q; want paused", got.State)
	}
	if want := "B → bounced to A"; got.Activity.Last != want {
		t.Errorf("Activity.Last after pause = %q; want %q", got.Activity.Last, want)
	}
}

func TestStep_SelfBounceReadsBouncedToItself(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.Producers = []ProducerDef{{Name: "A", Producer: stuckProducer(), OnStuck: "A", MaxBounces: 1}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	if _, err := shed.Step(context.Background()); err != nil {
		t.Fatalf("Step = _, %v", err)
	}
	got := readStatus(t, statusPath, statusLockPath)
	if want := "A → bounced to A"; got.Activity.Last != want {
		t.Errorf("Activity.Last = %q; want %q", got.Activity.Last, want)
	}
}

// TestStep_HaltsReadStuckAndResumeKeepsIt covers the escalation halt (no OnStuck) and the
// budget-exhausted halt of a self-bouncing row: both read "stuck", and the step-3b resume write
// after either keeps that text.
func TestStep_HaltsReadStuckAndResumeKeepsIt(t *testing.T) {
	tests := []struct {
		name  string
		def   ProducerDef
		steps int
	}{
		{name: "escalation halt", def: ProducerDef{Name: "A"}, steps: 1},
		{name: "budget-exhausted halt of a self-bouncing row", def: ProducerDef{Name: "A", OnStuck: "A", MaxBounces: 1}, steps: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shed, statusPath, _, statusLockPath := newTestShed(t)
			var seenDuringResume Activity
			calls := 0
			tt.def.Producer = &funcProducer{
				fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
					calls++
					if calls > tt.steps {
						seenDuringResume = readStatus(t, statusPath, statusLockPath).Activity
					}
					return Stuck, OutputPointer{Reason: "why"}, nil
				},
			}
			shed.Producers = []ProducerDef{tt.def}
			seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

			for i := 0; i < tt.steps; i++ {
				if _, err := shed.Step(context.Background()); err != nil {
					t.Fatalf("Step %d = _, %v", i+1, err)
				}
			}
			got := readStatus(t, statusPath, statusLockPath)
			if got.State != StateBlocked {
				t.Fatalf("State = %q; want blocked", got.State)
			}
			if want := "A → stuck"; got.Activity.Last != want {
				t.Errorf("Activity.Last at halt = %q; want %q", got.Activity.Last, want)
			}

			if _, err := shed.Step(context.Background()); err != nil {
				t.Fatalf("resume Step = _, %v", err)
			}
			if want := "A → stuck"; seenDuringResume.Last != want {
				t.Errorf("Activity.Last after resume write = %q; want %q", seenDuringResume.Last, want)
			}
		})
	}
}
