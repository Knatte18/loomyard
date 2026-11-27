// syncstatus_test.go pins the sync field over ledgers written through Update.

package pushledger_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestSyncStatus(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	cleared := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		entries []pushledger.Entry
		want    string
	}{
		{"EmptyLedger", nil, "sync: 0m · 60s"},
		{"TwoOwedShowTheLongestInterval", []pushledger.Entry{
			{FirstOwed: cleared, IntervalS: 60},
			{FirstOwed: cleared, IntervalS: 240},
		}, "sync: 7m · 240s"},
		// A refused target's entry stands as any owed one does.
		{"ARefusedTargetsEntryKeepsCounting", []pushledger.Entry{{FirstOwed: cleared, IntervalS: 60}}, "sync: 7m · 60s"},
		{"HeldRecordsOnly", []pushledger.Entry{{HeldRecords: "abc"}}, "sync: 0m · 60s"},
		{"DroppedOnly", []pushledger.Entry{{Dropped: map[string]string{"run": "abc"}}}, "sync: 0m · 60s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hub := t.TempDir()
			for i, entry := range tt.entries {
				key := pushledger.Key(string(rune('a' + i)))
				err := pushledger.Update(hub, key, cleared, func(pushledger.Entry, bool) (pushledger.Entry, bool, error) {
					return entry, true, nil
				})
				if err != nil {
					t.Fatalf("Update(%s) = %v; want nil", key, err)
				}
			}

			got, err := pushledger.SyncStatus(hub, cleared.Add(7*time.Minute), time.Minute)

			if err != nil || got != tt.want {
				t.Errorf("SyncStatus() = %q, %v; want %q, nil", got, err, tt.want)
			}
		})
	}

	t.Run("AnEntryThatDoesNotDecodeIsAnError", func(t *testing.T) {
		t.Parallel()
		hub := t.TempDir()
		path := pushledger.EntryPath(hub, pushledger.BoardKey)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir the ledger: %v", err)
		}
		if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
			t.Fatalf("write the entry: %v", err)
		}

		if got, err := pushledger.SyncStatus(hub, cleared, time.Minute); err == nil {
			t.Errorf("SyncStatus() = %q, nil; want an error", got)
		}
	})
}
