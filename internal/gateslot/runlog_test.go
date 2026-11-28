// runlog_test.go covers the run record and log a tier run leaves behind.
// It also covers the summary line a site reads back from a log.

package gateslot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestWriteRunRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)

	t.Run("RecordAndLogWrittenBesideEachOther", func(t *testing.T) {
		t.Parallel()

		dir := RunLogDir(t.TempDir())
		want := RunRecord{
			Worktree: "/hub/slug", Slug: "slug", TierList: []int{0, 1}, GateKey: "parent-align", Tier: 1,
			Command: "go test ./...", RunDir: "/hub/slug/internal/x", ScopeForm: "--all", Slots: 2,
			WaitStart: now.Add(-time.Minute), SlotsHeld: now.Add(-40 * time.Second), Start: now.Add(-30 * time.Second), End: now, WallSec: 30,
			ExitCode: 1, TimedOut: false, Outcome: "failed", GuardStep: true, Carried: true,
			Discovered: 9, Passed: 7, Failed: 1, Skipped: 1,
			Launches: []PackageLaunch{{Package: "example.com/p", LaunchSeconds: 0.25, ElapsedSeconds: 1.5}},
			Failures: []TestFailure{{Package: "example.com/p", Test: "TestA/sub", FirstLine: "a_test.go:12: got 1; want 2"}, {Package: "example.com/q", Test: "TestB"}},
			CallID:   "1700000000000000000-4242", Abandoned: true,
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
		read, err := ReadRunRecords(dir)
		if err != nil || !reflect.DeepEqual(read, []RunRecord{want}) {
			t.Errorf("ReadRunRecords() = %+v, %v; want the written record, call id and abandoned mark included", read, err)
		}
		logData, err := os.ReadFile(strings.TrimSuffix(path, ".json") + ".log")
		if err != nil {
			t.Fatalf("read log: %v", err)
		}
		if string(logData) != "FAIL\n" {
			t.Errorf("log = %q; want %q", logData, "FAIL\n")
		}
	})

	for _, tc := range []struct {
		name     string
		record   RunRecord
		wantStem string
	}{
		{"BuildRecordIsNamedForTheBuild", RunRecord{Tier: 1, Build: true, ScopeForm: "build", ExitCode: 1, Outcome: "failed"}, "-build-"},
		{"BlockedTierRecordKeepsTheFailedBuildLog", RunRecord{Tier: 2, BlockedBy: "/runs/b.log", Outcome: "failed"}, "-tier2-"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path, err := WriteRunRecord(RunLogDir(t.TempDir()), tc.record, []byte("x\n"), now)
			if err != nil {
				t.Fatalf("WriteRunRecord() error = %v", err)
			}
			if !strings.Contains(filepath.Base(path), tc.wantStem) {
				t.Errorf("WriteRunRecord() path = %q; want the stem %q", path, tc.wantStem)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read record: %v", err)
			}
			var got RunRecord
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("decode record: %v", err)
			}
			if !reflect.DeepEqual(got, tc.record) {
				t.Errorf("record read back = %+v; want %+v", got, tc.record)
			}
		})
	}

	t.Run("RecordWithoutLaunchesOrFailuresReadsWithNone", func(t *testing.T) {
		t.Parallel()

		var got RunRecord
		if err := json.Unmarshal([]byte(`{"tier":0,"outcome":"passed","discovered":3}`), &got); err != nil {
			t.Fatalf("decode record: %v", err)
		}
		if got.Launches != nil || got.Failures != nil || got.Discovered != 3 || got.CallID != "" || got.Abandoned {
			t.Errorf("record read back = %+v; want no launches, no failures, no call id, no abandoned mark and the other fields kept", got)
		}
		data, err := json.Marshal(RunRecord{})
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"launches", "failures"} {
			if strings.Contains(string(data), key) {
				t.Errorf("record without %s encodes as %s; want the key omitted", key, data)
			}
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
	build := func(tier, exit int) string {
		return BuildSummaryLine(RunRecord{Tier: tier, Build: true, ScopeForm: "build", WallSec: 2.5, ExitCode: exit}, "/runs/b.log")
	}
	carried := func(tier int) string {
		return TierSummaryLine(RunRecord{Tier: tier, ScopeForm: "all", Carried: true}, "/runs/y.log")
	}
	tests := []struct {
		name      string
		log       string
		wantTier  int
		wantBuild bool
		wantOK    bool
	}{
		{"failing line after a passing one", line(0, 0) + "\n" + line(2, 1) + "\n", 2, false, true},
		{"first of two failing lines", line(0, 3) + "\n" + line(6, 1) + "\n", 0, false, true},
		{"failing line followed by a passing one", line(1, 2) + "\n" + line(2, 0) + "\n", 1, false, true},
		{"a carried line beside a failing one is read as exit 0", carried(0) + "\n" + line(1, 3) + "\n" + carried(2) + "\n", 1, false, true},
		{"a carried line alone is no failure", carried(0) + "\n", 0, false, false},
		{"no failing summary line", line(0, 0) + "\nok example.com/x\n", 0, false, false},
		{"no summary line", "FAIL\n", 0, false, false},
		{"a failing build line first", build(1, 1) + "\n", 1, true, true},
		{"a passing build line then a failing tier line", build(0, 0) + "\n" + line(2, 1) + "\n", 2, false, true},
		{"a failing build line after a failing tier line", line(1, 1) + "\n" + build(2, 1) + "\n", 1, false, true},
		{"a failing line with no wait field", "gate test: tier1 diff discovered 3 passed 2 failed 1 skipped 0 wall 1.5s exit 1 log x\n", 1, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tier, isBuild, ok := FailedTier(tt.log)
			if tier != tt.wantTier || isBuild != tt.wantBuild || ok != tt.wantOK {
				t.Errorf("FailedTier() = %d, %v, %v; want %d, %v, %v", tier, isBuild, ok, tt.wantTier, tt.wantBuild, tt.wantOK)
			}
		})
	}
}

// TestTierDiscovered pins that the discovered counts of a log's tier summary lines are added up.
// A build summary line counts none.
func TestTierDiscovered(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	line := func(tier, discovered, exit int) string {
		return TierSummaryLine(RunRecord{Tier: tier, ScopeForm: "--base main", Discovered: discovered, WallSec: 1.5, ExitCode: exit}, "/runs/x.log")
	}
	build := BuildSummaryLine(RunRecord{Tier: 0, Build: true, ScopeForm: "build", WallSec: 2.5}, "/runs/b.log")
	tests := []struct {
		name      string
		log       string
		want      int
		wantFound bool
	}{
		{"one tier line", line(0, 4, 0) + "\n", 4, true},
		{"a tier line that discovered none is found", line(0, 0, 0) + "\n", 0, true},
		{"tier lines add up", line(0, 2, 0) + "\n" + line(1, 3, 1) + "\n", 5, true},
		{"a line with no wait field is read", "gate test: tier1 diff discovered 3 passed 2 failed 1 skipped 0 wall 1.5s exit 1 log x\n", 3, true},
		{"a build line and other text are no tier line", build + "\nPassed!\n", 0, false},
		{"no summary line", "FAIL\n", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, found := TierDiscovered(tt.log)
			if got != tt.want || found != tt.wantFound {
				t.Errorf("TierDiscovered() = %d, %v; want %d, %v", got, found, tt.want, tt.wantFound)
			}
		})
	}
}

// TestTierSummaryLine pins that a tier's summary line shows its slot wait and its run's wall time apart.
func TestTierSummaryLine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	start := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		record RunRecord
		want   string
	}{
		{
			"WaitAndWallApart",
			RunRecord{Tier: 1, ScopeForm: "diff", Discovered: 3, Passed: 2, Failed: 1, WaitStart: start.Add(-300 * time.Second), Start: start, WallSec: 12.5, ExitCode: 1},
			"gate test: tier1 diff wait 300.0s discovered 3 passed 2 failed 1 skipped 0 wall 12.5s exit 1 log /runs/x.log",
		},
		{
			"WaitEndsWhereTheSlotsWereHeld",
			RunRecord{Tier: 1, ScopeForm: "diff", WaitStart: start.Add(-300 * time.Second), SlotsHeld: start.Add(-100 * time.Second), Start: start},
			"gate test: tier1 diff wait 200.0s discovered 0 passed 0 failed 0 skipped 0 wall 0.0s exit 0 log /runs/x.log",
		},
		{
			"NoWaitStartWaitedNone",
			RunRecord{Tier: 0, ScopeForm: "all", Carried: true, Start: start},
			"gate test: tier0 all carried wait 0.0s discovered 0 passed 0 failed 0 skipped 0 wall 0.0s exit 0 log /runs/x.log",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := TierSummaryLine(tt.record, "/runs/x.log"); got != tt.want {
				t.Errorf("TierSummaryLine() = %q; want %q", got, tt.want)
			}
		})
	}
}
