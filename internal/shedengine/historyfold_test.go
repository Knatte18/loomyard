// historyfold_test.go covers appendOrFold: consecutive budget-exempt Stucks with the same producer, output, and gate attempts fold into one entry,
// and every other shape appends.

package shedengine

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/state"
)

func exemptStuck(producer, output, at string) HistoryEntry {
	return HistoryEntry{Producer: producer, Outcome: Stuck, Output: output, At: at, BudgetExempt: true}
}

func intPtr(v int) *int { return &v }

// cloneHistory deep-copies history, including GateAttempts pointees.
func cloneHistory(h []HistoryEntry) []HistoryEntry {
	out := make([]HistoryEntry, len(h))
	copy(out, h)
	for i := range out {
		if out[i].GateAttempts != nil {
			out[i].GateAttempts = intPtr(*out[i].GateAttempts)
		}
	}
	return out
}

func TestAppendOrFold_Folds(t *testing.T) {
	t.Parallel()
	withGate := func(e HistoryEntry, n int) HistoryEntry {
		e.GateAttempts = intPtr(n)
		return e
	}
	tests := []struct {
		name        string
		entries     []HistoryEntry
		wantRepeats int
		wantLastAt  string
	}{
		{"consecutive exempt stucks fold into the first entry",
			[]HistoryEntry{exemptStuck("Wait", "x", "t1"), exemptStuck("Wait", "x", "t2"), exemptStuck("Wait", "x", "t3")}, 2, "t3"},
		{"equal gate attempts fold",
			[]HistoryEntry{withGate(exemptStuck("Wait", "x", "t1"), 2), withGate(exemptStuck("Wait", "x", "t2"), 2)}, 1, "t2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var h []HistoryEntry
			for _, e := range tt.entries {
				h = appendOrFold(h, e)
			}
			if len(h) != 1 {
				t.Fatalf("len = %d; want 1", len(h))
			}
			if h[0].At != "t1" || h[0].Repeats != tt.wantRepeats || h[0].LastAt != tt.wantLastAt {
				t.Errorf("entry = %+v; want At t1, Repeats %d, LastAt %s", h[0], tt.wantRepeats, tt.wantLastAt)
			}
		})
	}
}

func TestAppendOrFold_Appends(t *testing.T) {
	gated := func(n int) HistoryEntry {
		e := exemptStuck("Wait", "x", "t1")
		e.GateAttempts = intPtr(n)
		return e
	}
	counted := exemptStuck("Wait", "x", "t1")
	counted.BudgetExempt = false
	tests := []struct {
		name    string
		history []HistoryEntry
		entry   HistoryEntry
	}{
		{"empty history", nil, exemptStuck("Wait", "x", "t2")},
		{"counted last", []HistoryEntry{counted}, exemptStuck("Wait", "x", "t2")},
		{"counted entry", []HistoryEntry{exemptStuck("Wait", "x", "t1")}, func() HistoryEntry {
			e := exemptStuck("Wait", "x", "t2")
			e.BudgetExempt = false
			return e
		}()},
		{"done last", []HistoryEntry{{Producer: "Wait", Outcome: Done, At: "t1"}}, exemptStuck("Wait", "x", "t2")},
		{"awaiting last", []HistoryEntry{{Producer: "Wait", Outcome: Awaiting, At: "t1"}}, exemptStuck("Wait", "x", "t2")},
		{"different producer", []HistoryEntry{exemptStuck("Other", "x", "t1")}, exemptStuck("Wait", "x", "t2")},
		{"different output", []HistoryEntry{exemptStuck("Wait", "x", "t1")}, exemptStuck("Wait", "y", "t2")},
		{"gate nil vs set", []HistoryEntry{exemptStuck("Wait", "x", "t1")}, gated(1)},
		{"gate set vs nil", []HistoryEntry{gated(1)}, exemptStuck("Wait", "x", "t2")},
		{"gate differs", []HistoryEntry{gated(1)}, gated(2)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendOrFold(tt.history, tt.entry)
			if len(got) != len(tt.history)+1 {
				t.Fatalf("len = %d; want %d", len(got), len(tt.history)+1)
			}
			last := got[len(got)-1]
			if last.Repeats != 0 || last.LastAt != "" {
				t.Errorf("appended entry = %+v; want zero Repeats and empty LastAt", last)
			}
		})
	}
}

//testtiming:keep pins that a fold ends at a done or awaiting entry and the next stuck appends fresh, which its covering test does not
func TestAppendOrFold_FoldThenDoneThenStuckAppendsFresh(t *testing.T) {
	for _, mid := range []Outcome{Done, Awaiting} {
		h := []HistoryEntry{exemptStuck("Wait", "x", "t1")}
		h = appendOrFold(h, exemptStuck("Wait", "x", "t2"))
		h = appendOrFold(h, HistoryEntry{Producer: "Wait", Outcome: mid, At: "t3"})
		h = appendOrFold(h, exemptStuck("Wait", "x", "t4"))
		if len(h) != 3 {
			t.Fatalf("%s: len = %d; want 3", mid, len(h))
		}
		if h[2].Repeats != 0 || h[2].LastAt != "" {
			t.Errorf("%s: fresh entry = %+v; want no Repeats/LastAt", mid, h[2])
		}
		if h[0].Repeats != 1 {
			t.Errorf("%s: folded entry Repeats = %d; want 1", mid, h[0].Repeats)
		}
	}
}

//testtiming:keep pins that appendOrFold never mutates or aliases its input, which its covering test does not
func TestAppendOrFold_InputUnchanged(t *testing.T) {
	gated := exemptStuck("Wait", "x", "t1")
	gated.GateAttempts = intPtr(1)
	for name, entry := range map[string]HistoryEntry{
		"fold":   func() HistoryEntry { e := exemptStuck("Wait", "x", "t2"); e.GateAttempts = intPtr(1); return e }(),
		"append": {Producer: "Wait", Outcome: Done, At: "t2"},
	} {
		h := []HistoryEntry{{Producer: "A", Outcome: Done, At: "t0"}, gated}
		before := cloneHistory(h)
		got := appendOrFold(h, entry)
		if !reflect.DeepEqual(h, before) {
			t.Errorf("%s: input = %+v; want unchanged %+v", name, h, before)
		}
		if len(h) > 0 && len(got) > 0 && &got[0] == &h[0] {
			t.Errorf("%s: result shares backing array with input", name)
		}
	}
}

//testtiming:keep pins that a folded history counts the same stuck episode as the unfolded calls, which its covering tests do not
func TestEpisodeStuckCount_FoldedEqualsUnfolded(t *testing.T) {
	calls := []HistoryEntry{
		{Producer: "Wait", Outcome: Stuck, At: "t0"},
		exemptStuck("Wait", "x", "t1"),
		exemptStuck("Wait", "x", "t2"),
		exemptStuck("Wait", "x", "t3"),
		{Producer: "Wait", Outcome: Stuck, At: "t4"},
		{Producer: "Wait", Outcome: Done, At: "t5"},
		{Producer: "Wait", Outcome: Stuck, At: "t6"},
		exemptStuck("Wait", "x", "t7"),
		exemptStuck("Wait", "x", "t8"),
	}
	var folded []HistoryEntry
	for _, c := range calls {
		folded = appendOrFold(folded, c)
	}
	if len(folded) >= len(calls) {
		t.Fatalf("folded len = %d; want fewer than %d", len(folded), len(calls))
	}
	wait := ProducerDef{Name: "Wait"}
	if got, want := episodeStuckCount(folded, wait, nil), episodeStuckCount(calls, wait, nil); got != want {
		t.Errorf("folded count = %d; want %d", got, want)
	}
}

//testtiming:keep pins the repeats and last_at JSON omission and round-trip, which its covering test does not
func TestHistoryEntry_FoldFieldsOmittedWhenZero(t *testing.T) {
	b, err := json.Marshal(HistoryEntry{Producer: "Wait", Outcome: Stuck, At: "2026-01-01T00:00:00Z"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(b), "repeats") || strings.Contains(string(b), "last_at") {
		t.Errorf("marshalled %s carries repeats/last_at; want both omitted", b)
	}

	dir := t.TempDir()
	statusPath, lockPath := filepath.Join(dir, "status.json"), filepath.Join(dir, "status.json.lock")
	folded := exemptStuck("Wait", "x", "2026-01-01T00:00:00Z")
	folded.Repeats, folded.LastAt = 3, "2026-01-01T00:05:00Z"
	if err := state.WriteJSON(statusPath, lockPath, Status{History: []HistoryEntry{{Producer: "Wait", Outcome: Stuck, At: "2026-01-01T00:00:00Z"}, folded}}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	got := readStatus(t, statusPath, lockPath)
	if len(got.History) != 2 || got.History[0].Repeats != 0 || got.History[1].Repeats != 3 || got.History[1].LastAt != "2026-01-01T00:05:00Z" {
		t.Errorf("History = %+v; want round-trip of repeats/last_at", got.History)
	}
}
