// record_test.go — tier 0 unit tests for the Sampler-Session record and the entry stamp.

package samplerengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRecord covers an absent file reading not found; an update on an absent file;
// a second update keeping the fields it does not touch; and a file with an unknown key.
func TestRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("absent file reads not found", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		got, found, err := ReadRecord(filepath.Join(dir, "sampler.json"), filepath.Join(dir, "sampler.json.lock"))
		if err != nil || found || got != (Record{}) {
			t.Errorf("ReadRecord() = %+v, %v, %v; want the zero record, false, nil", got, found, err)
		}
	})

	t.Run("updates write the mutated record and keep untouched fields", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path, lockPath := filepath.Join(dir, "scratch", "sampler.json"), filepath.Join(dir, "scratch", "sampler.json.lock")

		if err := UpdateRecord(path, lockPath, func(rec *Record) { rec.StrandGUID, rec.SessionID = "guid-1", "session-1" }); err != nil {
			t.Fatalf("UpdateRecord() = %v; want nil", err)
		}
		want := Record{StrandGUID: "guid-1", SessionID: "session-1"}
		if got, found, err := ReadRecord(path, lockPath); err != nil || !found || got != want {
			t.Errorf("ReadRecord() = %+v, %v, %v; want %+v, true, nil", got, found, err, want)
		}

		if err := UpdateRecord(path, lockPath, func(rec *Record) { rec.DoneStamp = 2 }); err != nil {
			t.Fatalf("second UpdateRecord() = %v; want nil", err)
		}
		want.DoneStamp = 2
		if got, _, err := ReadRecord(path, lockPath); err != nil || got != want {
			t.Errorf("ReadRecord() after second update = %+v, %v; want %+v, nil", got, err, want)
		}
	})

	t.Run("a file with an unknown key reads without error", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "sampler.json")
		if err := os.WriteFile(path, []byte(`{"strand_guid":"g","from_a_newer_binary":true}`), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v; want nil", err)
		}
		got, found, err := ReadRecord(path, filepath.Join(dir, "sampler.json.lock"))
		if err != nil || !found || got.StrandGUID != "g" {
			t.Errorf("ReadRecord() = %+v, %v, %v; want the record with strand_guid g, true, nil", got, found, err)
		}
	})
}

// TestEntryStamp covers which history entries count as the run entering the row, and which leave the stamp unchanged.
func TestEntryStamp(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const row = "Sampler-Session"
	predecessors := []string{"Record-Write"}
	entry := func(producer string, outcome shedengine.Outcome) shedengine.HistoryEntry {
		return shedengine.HistoryEntry{Producer: producer, Outcome: outcome}
	}
	recordWrite := entry("Record-Write", shedengine.Done)
	tests := []struct {
		name    string
		history []shedengine.HistoryEntry
		want    int
	}{
		{"empty history", nil, 0},
		{"predecessor done", []shedengine.HistoryEntry{recordWrite}, 1},
		{"a halt of the row leaves it", []shedengine.HistoryEntry{recordWrite, entry(row, shedengine.Stuck)}, 1},
		{"a goto right after the row's own halt is no entry", []shedengine.HistoryEntry{recordWrite, entry(row, shedengine.Stuck), entry(row, shedengine.OutcomeGoto)}, 1},
		{"a goto back after the row done and another row stuck", []shedengine.HistoryEntry{recordWrite, entry(row, shedengine.Done), entry("Describe", shedengine.Stuck), entry(row, shedengine.OutcomeGoto)}, 2},
		{"a goto back after a goto away", []shedengine.HistoryEntry{recordWrite, entry(row, shedengine.Stuck), entry("Describe", shedengine.OutcomeGoto), entry(row, shedengine.OutcomeGoto)}, 2},
		{"a goto right after the row done", []shedengine.HistoryEntry{recordWrite, entry(row, shedengine.Done), entry(row, shedengine.OutcomeGoto)}, 2},
		{"a goto after a failure with no history entry is no entry", []shedengine.HistoryEntry{recordWrite, entry(row, shedengine.OutcomeGoto)}, 1},
		{"a folded predecessor done counts each call", []shedengine.HistoryEntry{{Producer: "Record-Write", Outcome: shedengine.Done, Repeats: 1}}, 2},
		{"a skipped predecessor enters", []shedengine.HistoryEntry{entry("Record-Write", shedengine.OutcomeSkipped)}, 1},
		{"a stuck entry routed to another row leaves the row", []shedengine.HistoryEntry{recordWrite, {Producer: row, Outcome: shedengine.Stuck, RoutedTo: "Describe"}, entry(row, shedengine.OutcomeGoto)}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := EntryStamp(tt.history, row, predecessors); got != tt.want {
				t.Errorf("EntryStamp() = %d; want %d", got, tt.want)
			}
		})
	}
}
