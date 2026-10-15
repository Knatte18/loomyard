// settle_test.go — untagged Tier-1 tests for the Go driver's settle records, over a temp file.

package sheddriver

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// TestSettleRecords pins the append-and-read round trip in order, an absent file reading nil, an unknown key staying readable, a torn last line skipped, and the verify-retry cap read.
func TestSettleRecords(t *testing.T) {
	t.Parallel()
	first := SettleRecord{
		StopKey: StopKey{CurrentProducer: "Plan-Write", HistoryLength: 2, State: "blocked"},
		Settler: SettlerGoRule,
		Action:  SettleRestep,
		Outcome: "ok",
		At:      time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	second := SettleRecord{
		StopKey: StopKey{CurrentProducer: "Publish", HistoryLength: 5, State: "blocked"},
		Settler: SettlerGoRule,
		Action:  SettleVerifyRetry,
		Outcome: "spent",
		At:      time.Date(2026, 1, 2, 3, 5, 5, 0, time.UTC),
		Build:   "abc123",
		Target:  "Publish",
	}

	t.Run("two appends read back in order, and an absent file reads nil", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "friction", "go-settle.jsonl")
		if records, err := ReadSettleRecords(path); records != nil || err != nil {
			t.Fatalf("ReadSettleRecords(absent) = %v, %v; want nil, nil", records, err)
		}
		for _, record := range []SettleRecord{first, second} {
			if err := AppendSettleRecord(path, record); err != nil {
				t.Fatalf("AppendSettleRecord() = %v; want nil", err)
			}
		}
		got, err := ReadSettleRecords(path)
		if err != nil {
			t.Fatalf("ReadSettleRecords() = _, %v; want nil", err)
		}
		if want := []SettleRecord{first, second}; !reflect.DeepEqual(got, want) {
			t.Errorf("ReadSettleRecords() = %+v; want %+v", got, want)
		}
	})

	t.Run("an unknown key stays readable and a torn last line is skipped", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "go-settle.jsonl")
		if err := AppendSettleRecord(path, first); err != nil {
			t.Fatalf("AppendSettleRecord() = %v; want nil", err)
		}
		file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		const newer = `{"stop_key":{"current_producer":"B","history_length":1,"state":"failed"},"settler":"tackler","action":"spawn","outcome":"ok","at":"2026-01-02T03:04:05Z","from_a_newer_binary":true}`
		if _, err := file.WriteString(newer + "\n" + `{"stop_key":{"current_prod`); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}

		got, err := ReadSettleRecords(path)
		if err != nil {
			t.Fatalf("ReadSettleRecords() = _, %v; want nil", err)
		}
		if len(got) != 2 || got[0] != first || got[1].Settler != SettlerTackler || got[1].Action != SettleSpawn {
			t.Errorf("ReadSettleRecords() = %+v; want the first record, then the newer-binary record, and not the torn line", got)
		}
	})

	t.Run("the verify retry is spent only by a verify-retry record", func(t *testing.T) {
		t.Parallel()
		if GoVerifyRetrySpent([]SettleRecord{first}) {
			t.Error("GoVerifyRetrySpent(restep only) = true; want false")
		}
		if !GoVerifyRetrySpent([]SettleRecord{first, second}) {
			t.Error("GoVerifyRetrySpent(with verify-retry) = false; want true")
		}
	})
}
