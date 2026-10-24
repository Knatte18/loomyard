//go:build tier0

// runlog_test.go covers the run record and log a tier run leaves behind, and the summary line a site reads back from a log.

package gateslot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestWriteRunRecord(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)

	t.Run("RecordAndLogWrittenBesideEachOther", func(t *testing.T) {
		t.Parallel()

		dir := RunLogDir(t.TempDir())
		want := RunRecord{
			Worktree: "/hub/slug", Slug: "slug", TierList: []int{0, 1}, GateKey: "merge-in", Tier: 1,
			Command: "go test ./...", RunDir: "/hub/slug/internal/x", ScopeForm: "--all", Slots: 2,
			WaitStart: now.Add(-time.Minute), Start: now.Add(-30 * time.Second), End: now, WallSec: 30,
			ExitCode: 1, TimedOut: false, Outcome: "failed", GuardStep: true, Carried: true,
			Discovered: 9, Passed: 7, Failed: 1, Skipped: 1,
			Launches: []PackageLaunch{{Package: "example.com/p", LaunchSeconds: 0.25, ElapsedSeconds: 1.5}},
		}
		path, err := WriteRunRecord(dir, want, []byte("FAIL\n"), now)
		if err != nil {
			t.Fatalf("WriteRunRecord() error = %v", err)
		}
		if filepath.Dir(path) != dir || !strings.HasSuffix(path, ".json") || !strings.Contains(filepath.Base(path), "-tier1-") {
			t.Errorf("WriteRunRecord() path = %q; want a .json named for tier1 in %q", path, dir)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read record: %v", err)
		}
		var got RunRecord
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("decode record: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("record read back = %+v; want %+v", got, want)
		}
		logData, err := os.ReadFile(strings.TrimSuffix(path, ".json") + ".log")
		if err != nil {
			t.Fatalf("read log: %v", err)
		}
		if string(logData) != "FAIL\n" {
			t.Errorf("log = %q; want %q", logData, "FAIL\n")
		}
	})

	t.Run("RecordWithoutLaunchesReadsWithNone", func(t *testing.T) {
		t.Parallel()

		var got RunRecord
		if err := json.Unmarshal([]byte(`{"tier":0,"outcome":"passed","discovered":3}`), &got); err != nil {
			t.Fatalf("decode record: %v", err)
		}
		if got.Launches != nil || got.Discovered != 3 {
			t.Errorf("record read back = %+v; want no launches and the other fields kept", got)
		}
		data, err := json.Marshal(RunRecord{})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "launches") {
			t.Errorf("record without launches encodes as %s; want the key omitted", data)
		}
	})

	t.Run("PrunesOnlyFilesOlderThanFourteenDays", func(t *testing.T) {
		t.Parallel()

		dir := RunLogDir(t.TempDir())
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		files := map[string]time.Duration{
			"old.json": 15 * 24 * time.Hour, "old.log": 15 * 24 * time.Hour,
			"recent.json": 13 * 24 * time.Hour, "recent.log": 13 * 24 * time.Hour,
		}
		for name, age := range files {
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(path, now.Add(-age), now.Add(-age)); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := WriteRunRecord(dir, RunRecord{Tier: 0}, nil, now); err != nil {
			t.Fatalf("WriteRunRecord() error = %v", err)
		}
		for name, age := range files {
			_, err := os.Stat(filepath.Join(dir, name))
			wantGone := age > 14*24*time.Hour
			if gone := os.IsNotExist(err); gone != wantGone {
				t.Errorf("%s removed = %v; want %v", name, gone, wantGone)
			}
		}
	})

	t.Run("SummaryLineRoundTripsThroughFailedTier", func(t *testing.T) {
		t.Parallel()

		testFailedTier(t)
	})
}

func testFailedTier(t *testing.T) {
	t.Helper()

	line := func(tier, exit int) string {
		return TierSummaryLine(RunRecord{Tier: tier, ScopeForm: "--base main", Discovered: 4, Passed: 4, WallSec: 1.5, ExitCode: exit}, "/runs/x.log")
	}
	carried := func(tier int) string {
		return TierSummaryLine(RunRecord{Tier: tier, ScopeForm: "all", Carried: true}, "/runs/y.log")
	}
	tests := []struct {
		name     string
		log      string
		wantTier int
		wantOK   bool
	}{
		{"last failing line after a passing one", line(0, 0) + "\n" + line(2, 1) + "\n", 2, true},
		{"failing line followed by a passing one", line(1, 2) + "\n" + line(2, 0) + "\n", 1, true},
		{"a carried line beside a failing one is read as exit 0", carried(0) + "\n" + line(1, 3) + "\n" + carried(2) + "\n", 1, true},
		{"a carried line alone is no failure", carried(0) + "\n", 0, false},
		{"no failing summary line", line(0, 0) + "\nok example.com/x\n", 0, false},
		{"no summary line", "FAIL\n", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tier, ok := FailedTier(tt.log)
			if tier != tt.wantTier || ok != tt.wantOK {
				t.Errorf("FailedTier() = %d, %v; want %d, %v", tier, ok, tt.wantTier, tt.wantOK)
			}
		})
	}
}
