// ledger_test.go pins the ledger over a temp hub: its writes, the clear time and the reads over landings.

package pushledger_test

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

var epoch = time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)

// put writes entry at key at the time epoch plus after.
func put(t *testing.T, hub string, key pushledger.Key, after time.Duration, entry pushledger.Entry) {
	t.Helper()
	err := pushledger.Update(hub, key, epoch.Add(after), func(pushledger.Entry, bool) (pushledger.Entry, bool, error) {
		return entry, true, nil
	})
	if err != nil {
		t.Fatalf("Update(%q) = %v; want nil", key, err)
	}
}

// drop removes the entry of key at the time epoch plus after.
func drop(t *testing.T, hub string, key pushledger.Key, after time.Duration) {
	t.Helper()
	err := pushledger.Update(hub, key, epoch.Add(after), func(cur pushledger.Entry, _ bool) (pushledger.Entry, bool, error) {
		return cur, false, nil
	})
	if err != nil {
		t.Fatalf("Update(%q) = %v; want nil", key, err)
	}
}

func requireClear(t *testing.T, hub string, want time.Time) {
	t.Helper()
	got, err := pushledger.LastClear(hub)
	if err != nil || !got.Equal(want) {
		t.Errorf("LastClear() = %v, %v; want %v, nil", got, err, want)
	}
}

func TestUpdate_WritesAndRemovesTheEntry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	hub := t.TempDir()
	key := pushledger.RecordsKey("/hub/task-records")

	put(t, hub, key, 0, pushledger.Entry{Slug: "task", FirstOwed: epoch, IntervalS: 60})

	got, found, err := pushledger.Read(hub, key)
	if err != nil || !found || got.Slug != "task" || got.IntervalS != 60 || !got.FirstOwed.Equal(epoch) {
		t.Errorf("Read() = %+v, %t, %v; want the written entry", got, found, err)
	}
	listed, err := pushledger.List(hub)
	if err != nil || len(listed) != 1 || listed["task-records"].Slug != "task" {
		t.Errorf("List() = %+v, %v; want the one entry under task-records", listed, err)
	}

	drop(t, hub, key, time.Minute)

	if _, found, err := pushledger.Read(hub, key); err != nil || found {
		t.Errorf("Read() after the removal found %t, error %v; want nothing", found, err)
	}
}

func TestUpdate_KeepsTheClearTime(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	owed := pushledger.Entry{FirstOwed: epoch}
	tests := []struct {
		name string
		act  func(t *testing.T, hub string)
		// want is the clear time's offset from epoch.
		want time.Duration
	}{
		{name: "AFirstOwedEntryOnAnEmptyHubSetsIt", want: time.Minute, act: func(t *testing.T, hub string) {
			put(t, hub, "one", time.Minute, owed)
		}},
		{name: "ASecondOwedEntryLeavesIt", want: time.Minute, act: func(t *testing.T, hub string) {
			put(t, hub, "one", time.Minute, owed)
			put(t, hub, "two", 2*time.Minute, owed)
		}},
		{name: "ARemovalWhileAnotherStandsOwedLeavesIt", want: time.Minute, act: func(t *testing.T, hub string) {
			put(t, hub, "one", time.Minute, owed)
			put(t, hub, "two", 2*time.Minute, owed)
			drop(t, hub, "one", 3*time.Minute)
		}},
		{name: "TheLastOwedEntryGoingSetsIt", want: 2 * time.Minute, act: func(t *testing.T, hub string) {
			put(t, hub, "one", time.Minute, owed)
			drop(t, hub, "one", 2*time.Minute)
		}},
		{name: "ANewOwedEntryHoursAfterAClearSetsIt", want: 5 * time.Hour, act: func(t *testing.T, hub string) {
			put(t, hub, "one", time.Minute, owed)
			drop(t, hub, "one", 2*time.Minute)
			put(t, hub, "one", 5*time.Hour, owed)
		}},
		{name: "AnEntryHoldingOnlyHeldRecordsIsNotOwed", want: 2 * time.Minute, act: func(t *testing.T, hub string) {
			put(t, hub, "one", time.Minute, owed)
			put(t, hub, "one", 2*time.Minute, pushledger.Entry{HeldRecords: "abc"})
		}},
		{name: "AnEntryHoldingOnlyDroppedIsNotOwed", want: 2 * time.Minute, act: func(t *testing.T, hub string) {
			put(t, hub, "one", time.Minute, owed)
			put(t, hub, "one", 2*time.Minute, pushledger.Entry{Dropped: map[string]string{"run": "abc"}})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hub := t.TempDir()

			tt.act(t, hub)

			requireClear(t, hub, epoch.Add(tt.want))
		})
	}
}

func TestOwedTipsAndHeldRecordsCommit(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	hub := t.TempDir()
	landing := func(run, tip, records string, after time.Duration) pushledger.Landing {
		return pushledger.Landing{Run: run, Tip: tip, RecordsCommit: records, ParentBranch: "trunk", At: epoch.Add(after)}
	}
	put(t, hub, pushledger.LandingKey, 0, pushledger.Entry{Landings: []pushledger.Landing{
		landing("first", "tip1", "records1", time.Minute),
		landing("second", "tip2", "records2", 2*time.Minute),
		landing("first", "tip3", "records3", 3*time.Minute),
	}})
	put(t, hub, "gone-records", 0, pushledger.Entry{Slug: "gone", HeldRecords: "held"})

	tips, err := pushledger.OwedTips(hub, pushledger.LandingKey)
	if want := []string{"tip1", "tip2", "tip3"}; err != nil || !slices.Equal(tips, want) {
		t.Errorf("OwedTips() = %q, %v; want %q, nil", tips, err, want)
	}

	tests := []struct {
		slug     string
		want     string
		wantHeld bool
	}{
		{slug: "first", want: "records1", wantHeld: true},
		{slug: "second", want: "records2", wantHeld: true},
		{slug: "gone", want: "held", wantHeld: true},
		{slug: "unknown"},
	}
	for _, tt := range tests {
		got, held, err := pushledger.HeldRecordsCommit(hub, tt.slug)
		if err != nil || got != tt.want || held != tt.wantHeld {
			t.Errorf("HeldRecordsCommit(%q) = %q, %t, %v; want %q, %t, nil", tt.slug, got, held, err, tt.want, tt.wantHeld)
		}
	}
}

func TestRead_AcceptsAnUnknownKey(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	hub := t.TempDir()
	put(t, hub, pushledger.BoardKey, 0, pushledger.Entry{Slug: "task"})
	path := pushledger.EntryPath(hub, pushledger.BoardKey)
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v; want nil", path, err)
	}
	extended := strings.Replace(string(written), "{", `{"added_later": true,`, 1)
	if err := os.WriteFile(path, []byte(extended), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}

	got, found, err := pushledger.Read(hub, pushledger.BoardKey)

	if err != nil || !found || got.Slug != "task" {
		t.Errorf("Read() = %+v, %t, %v; want the entry", got, found, err)
	}
}

func TestEmptyHub_ReadsAsAnEmptyLedger(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	hub := t.TempDir()

	listed, err := pushledger.List(hub)
	if err != nil || len(listed) != 0 {
		t.Errorf("List() = %+v, %v; want none", listed, err)
	}
	if _, found, err := pushledger.Read(hub, pushledger.BoardKey); err != nil || found {
		t.Errorf("Read() found %t, error %v; want nothing", found, err)
	}
	if _, held, err := pushledger.HeldRecordsCommit(hub, "task"); err != nil || held {
		t.Errorf("HeldRecordsCommit() held %t, error %v; want nothing", held, err)
	}
	requireClear(t, hub, time.Time{})
}
