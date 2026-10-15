// status_test.go covers the JSON round-trip TDD candidate for Status: an in-memory marshal/unmarshal round-trip and State.valid's enum gate.

package shedengine

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
		PauseBefore:     "Plan-Write",
		PauseAfter:      "Preflight",
		PendingBy:       "orch",
		Activity: Activity{
			Now:  "Plan-Write",
			Last: "Preflight → done",
			Wait: "",
		},
		History: []HistoryEntry{
			{Producer: "Preflight", Outcome: Done, Output: "", At: "2026-08-15T09:00:00Z", By: "driver"},
			{Producer: "Plan-Write", Outcome: Stuck, Output: "_lyx/plan.md", At: "2026-08-15T09:05:00Z", GateSpans: []GateSpan{
				{Gate: "plan-parse", Kind: GateSpanKindGate, Start: "2026-08-15T09:04:00.25Z", End: "2026-08-15T09:04:01.5Z"},
				{Gate: "plan-parse", Kind: GateSpanKindRework, Start: "2026-08-15T09:04:02Z", End: "2026-08-15T09:04:30Z"},
			}},
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
	if got.PauseBefore != want.PauseBefore || got.PauseAfter != want.PauseAfter {
		t.Errorf("PauseBefore, PauseAfter = %q, %q; want %q, %q", got.PauseBefore, got.PauseAfter, want.PauseBefore, want.PauseAfter)
	}
	empty, err := json.Marshal(Status{})
	if err != nil {
		t.Fatalf("json.Marshal(Status{}) = %v", err)
	}
	if strings.Contains(string(empty), "pause_before") || strings.Contains(string(empty), "pause_after") {
		t.Errorf("empty Status marshals as %s; want pause_before and pause_after omitted", empty)
	}
	if got.PendingBy != want.PendingBy {
		t.Errorf("PendingBy = %q; want %q", got.PendingBy, want.PendingBy)
	}
	// A record without the caller-label keys decodes with empty values and re-encodes without them.
	legacy, err := ParseStatusTolerant([]byte(`{"history":[{"producer":"A","outcome":"done","output":"","at":"t"}]}`))
	if err != nil {
		t.Fatalf("ParseStatusTolerant(record without caller labels) = %v", err)
	}
	reencoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("json.Marshal(legacy) = %v", err)
	}
	if legacy.PendingBy != "" || legacy.History[0].By != "" || strings.Contains(string(reencoded), `"by"`) || strings.Contains(string(reencoded), "pending_by") {
		t.Errorf("record without caller labels = %+v re-encoded as %s; want empty values and no by or pending_by key", legacy, reencoded)
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

	withBogus := append(bytes.TrimSuffix(data, []byte("}")), []byte(`,"bogus":1}`)...)
	tolerant, err := ParseStatusTolerant(withBogus)
	if err != nil {
		t.Fatalf("ParseStatusTolerant(with bogus key) = %v", err)
	}
	if tolerant.CurrentProducer != want.CurrentProducer || tolerant.State != want.State || !reflect.DeepEqual(tolerant.History, want.History) {
		t.Errorf("ParseStatusTolerant() = %+v; want the round-tripped producer, state and history", tolerant)
	}
	if _, err := ParseStatusTolerant([]byte("{")); err == nil {
		t.Error("ParseStatusTolerant({) error = nil; want a decode error")
	}

	// A span object carrying a key this binary does not know decodes both ways, keeping the span's known fields.
	spanWithUnknownKey := []byte(`{"history":[{"producer":"Plan-Write","outcome":"stuck","output":"","at":"t","gate_spans":[{"gate":"g","kind":"held","start":"s","end":"e","future":1}]}]}`)
	wantSpan := []GateSpan{{Gate: "g", Kind: GateSpanKindHeld, Start: "s", End: "e"}}
	var strict Status
	if err := json.Unmarshal(spanWithUnknownKey, &strict); err != nil {
		t.Fatalf("json.Unmarshal(span with unknown key) = %v", err)
	}
	tolerantSpan, err := ParseStatusTolerant(spanWithUnknownKey)
	if err != nil {
		t.Fatalf("ParseStatusTolerant(span with unknown key) = %v", err)
	}
	if !reflect.DeepEqual(strict.History[0].GateSpans, wantSpan) || !reflect.DeepEqual(tolerantSpan.History[0].GateSpans, wantSpan) {
		t.Errorf("GateSpans = %+v and %+v; want %+v", strict.History[0].GateSpans, tolerantSpan.History[0].GateSpans, wantSpan)
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

// TestRecordResumer pins which statuses the waiter's marking touches: halted runs with no label pending, and nothing else.
func TestRecordResumer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		seed        *Status
		by          string
		wantPending string
	}{
		{name: "blocked is marked", seed: pendingSeed(StateBlocked, "A", ""), by: "orch", wantPending: "orch"},
		{name: "failed is marked", seed: pendingSeed(StateFailed, "A", ""), by: "orch", wantPending: "orch"},
		{name: "awaiting is marked", seed: pendingSeed(StateAwaiting, "A", ""), by: "orch", wantPending: "orch"},
		{name: "running is left unmarked", seed: pendingSeed(StateRunning, "A", ""), by: "orch"},
		{name: "paused is left unmarked", seed: pendingSeed(StatePaused, "A", ""), by: "orch"},
		{name: "the first label stands", seed: pendingSeed(StateBlocked, "A", "orch"), by: "driver", wantPending: "orch"},
		{name: "an empty label marks nothing", seed: pendingSeed(StateBlocked, "A", ""), by: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shed, statusPath, _, statusLockPath := newTestShed(t)
			seedStatus(t, statusPath, statusLockPath, *tt.seed)
			if err := RecordResumer(statusPath, statusLockPath, tt.by); err != nil {
				t.Fatalf("RecordResumer(...) = %v; want nil", err)
			}
			if got := readStatus(t, shed.StatusPath, statusLockPath); got.PendingBy != tt.wantPending {
				t.Errorf("PendingBy = %q; want %q", got.PendingBy, tt.wantPending)
			}
		})
	}

	t.Run("an absent status file returns nil and creates none", func(t *testing.T) {
		t.Parallel()
		_, statusPath, _, statusLockPath := newTestShed(t)
		if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
			t.Fatalf("create status lock parent: %v", err)
		}
		if err := RecordResumer(statusPath, statusLockPath, "orch"); err != nil {
			t.Fatalf("RecordResumer(absent) = %v; want nil", err)
		}
		if _, err := os.Stat(statusPath); !os.IsNotExist(err) {
			t.Errorf("os.Stat(status) = %v; want not-exist", err)
		}
	})

	t.Run("an undecodable status file returns an error", func(t *testing.T) {
		t.Parallel()
		_, statusPath, _, statusLockPath := newTestShed(t)
		if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
			t.Fatalf("create status lock parent: %v", err)
		}
		if err := os.WriteFile(statusPath, []byte("{"), 0o644); err != nil {
			t.Fatalf("write corrupt status: %v", err)
		}
		if err := RecordResumer(statusPath, statusLockPath, "orch"); err == nil {
			t.Error("RecordResumer(undecodable) = nil; want a read error")
		}
	})
}
