//go:build tier0

// idle_test.go covers the idle rule's reading of agent states against a bound.

package parentlock

import (
	"testing"
	"time"
)

type idleReading struct {
	agents []AgentState
	read   bool
	awake  time.Duration
	want   bool
}

func TestIdleWatchObserve(t *testing.T) {
	t.Parallel()

	waiting := []AgentState{{Name: "a", Waiting: true}}
	busyBeside := []AgentState{{Name: "a", Waiting: true}, {Name: "b", Busy: true}}
	active := []AgentState{{Name: "a"}}

	for _, tc := range []struct {
		name     string
		readings []idleReading
	}{
		{"waiting past the bound with nothing busy", []idleReading{
			{waiting, true, 0, false},
			{waiting, true, 10 * time.Minute, true},
		}},
		{"waiting below the bound", []idleReading{
			{waiting, true, 0, false},
			{waiting, true, 9 * time.Minute, false},
		}},
		{"a busy agent beside the waiting one", []idleReading{
			{waiting, true, 0, false},
			{busyBeside, true, 10 * time.Minute, false},
		}},
		{"an unreadable state clears the first-seen times", []idleReading{
			{waiting, true, 0, false},
			{nil, false, 10 * time.Minute, false},
			{waiting, true, 11 * time.Minute, false},
		}},
		{"an agent that stops waiting starts its bound afresh", []idleReading{
			{waiting, true, 0, false},
			{active, true, 5 * time.Minute, false},
			{waiting, true, 6 * time.Minute, false},
			{waiting, true, 12 * time.Minute, false},
			{waiting, true, 16 * time.Minute, true},
		}},
	} {
		watch := IdleWatch{Bound: 10 * time.Minute}
		for i, reading := range tc.readings {
			if got := watch.Observe(reading.agents, reading.read, reading.awake); got != reading.want {
				t.Fatalf("%s: reading %d at %s = %v; want %v", tc.name, i, reading.awake, got, reading.want)
			}
		}
	}
}

// TestIdleWatch_RestoreKeepsTheWait pins that a watch saved and restored keeps its earlier sightings, and that a restore with no readings starts every wait afresh.
func TestIdleWatch_RestoreKeepsTheWait(t *testing.T) {
	t.Parallel()

	const bound = 10 * time.Minute
	waiting := []AgentState{{Name: "a", Waiting: true}}
	first := 10 * time.Second

	saved := IdleWatch{Bound: bound}
	if saved.Observe(waiting, true, first) {
		t.Fatalf("Observe at %s = true; want a first sighting not idle", first)
	}

	tests := []struct {
		name    string
		waiting map[string]time.Duration
		want    bool
	}{
		{"restored readings keep the wait", saved.Waiting(), true},
		{"a restore with no readings starts afresh", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			restored := RestoreIdleWatch(bound, tt.waiting)
			if got := restored.Observe(waiting, true, first+bound); got != tt.want {
				t.Errorf("Observe at %s after the restore = %v; want %v", first+bound, got, tt.want)
			}
		})
	}
}
