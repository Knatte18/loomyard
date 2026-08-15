// status_test.go covers the JSON round-trip TDD candidate for Status: an in-memory marshal/unmarshal
// round-trip, a full write-then-read cycle through internal/state against a real file under
// t.TempDir(), and State.valid's enum gate.

package shedengine

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/state"
)

// semanticJSONEqual compares two json.RawMessage payloads by semantic equality: unmarshal both
// sides into any and reflect.DeepEqual the result, never a raw byte compare.
// Persistence goes through json.MarshalIndent, which re-indents an embedded json.RawMessage, so a
// payload written with different whitespace survives semantically but not byte-for-byte.
func semanticJSONEqual(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		t.Fatalf("unmarshal a: %v", err)
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		t.Fatalf("unmarshal b: %v", err)
	}
	return reflect.DeepEqual(av, bv)
}

func TestStatus_JSONRoundTrip(t *testing.T) {
	want := Status{
		CurrentProducer: "Plan-Write",
		State:           StateRunning,
		Error:           "",
		PauseRequested:  true,
		Activity: Activity{
			Now:  "Plan-Write",
			Last: "Preflight → done",
			Wait: "",
		},
		History: []HistoryEntry{
			{Producer: "Preflight", Outcome: Done, Output: "", At: "2026-08-15T09:00:00Z"},
			{Producer: "Plan-Write", Outcome: Stuck, Output: "_lyx/plan.md", At: "2026-08-15T09:05:00Z"},
		},
		Product: json.RawMessage(`{"foo":"bar","nested":{"n":1}}`),
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("json.Marshal(want) = %v", err)
	}

	var got Status
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal(data, &got) = %v", err)
	}

	if got.CurrentProducer != want.CurrentProducer {
		t.Errorf("CurrentProducer = %q; want %q", got.CurrentProducer, want.CurrentProducer)
	}
	if got.State != want.State {
		t.Errorf("State = %q; want %q", got.State, want.State)
	}
	if got.Error != want.Error {
		t.Errorf("Error = %q; want %q", got.Error, want.Error)
	}
	if got.PauseRequested != want.PauseRequested {
		t.Errorf("PauseRequested = %v; want %v", got.PauseRequested, want.PauseRequested)
	}
	if got.Activity != want.Activity {
		t.Errorf("Activity = %+v; want %+v", got.Activity, want.Activity)
	}
	if !reflect.DeepEqual(got.History, want.History) {
		t.Errorf("History = %+v; want %+v", got.History, want.History)
	}
	if !semanticJSONEqual(t, got.Product, want.Product) {
		t.Errorf("Product = %s; want (semantically) %s", got.Product, want.Product)
	}
}

func TestStatus_WriteReadCycleThroughState(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	lockPath := filepath.Join(dir, "status.json.lock")

	want := Status{
		CurrentProducer: "Finalize",
		State:           StateDone,
		Error:           "",
		PauseRequested:  false,
		Activity: Activity{
			Now:  "Finalize",
			Last: "Finalize → done",
			Wait: "",
		},
		History: []HistoryEntry{
			{Producer: "Finalize", Outcome: Done, Output: "", At: "2026-08-15T10:00:00Z"},
		},
		Product: json.RawMessage(`{"bounces_used":2}`),
	}

	if err := state.WriteJSON(statusPath, lockPath, want); err != nil {
		t.Fatalf("state.WriteJSON(...) = %v", err)
	}

	got, found, err := state.ReadJSONStrict[Status](statusPath, lockPath)
	if err != nil {
		t.Fatalf("state.ReadJSONStrict(...) = _, _, %v", err)
	}
	if !found {
		t.Fatalf("state.ReadJSONStrict(...) found = false; want true")
	}

	if got.CurrentProducer != want.CurrentProducer {
		t.Errorf("CurrentProducer = %q; want %q", got.CurrentProducer, want.CurrentProducer)
	}
	if got.State != want.State {
		t.Errorf("State = %q; want %q", got.State, want.State)
	}
	if !semanticJSONEqual(t, got.Product, want.Product) {
		t.Errorf("Product = %s; want (semantically) %s", got.Product, want.Product)
	}
}

func TestState_Valid(t *testing.T) {
	tests := []struct {
		name string
		s    State
		want bool
	}{
		{"running", StateRunning, true},
		{"paused", StatePaused, true},
		{"done", StateDone, true},
		{"blocked", StateBlocked, true},
		{"failed", StateFailed, true},
		{"typo", State("Runnning"), false},
		{"empty", State(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.valid(); got != tt.want {
				t.Errorf("State(%q).valid() = %v; want %v", string(tt.s), got, tt.want)
			}
		})
	}
}
