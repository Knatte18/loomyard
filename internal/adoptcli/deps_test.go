//go:build tier0

// deps_test.go covers the hub-only pause reading that makes a pair under a slug-scope pause read live, the adopt check start's guard under go test, its alive skip and its liveness reading.

package adoptcli

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/state"
)

func TestHubOnlyPaused(t *testing.T) {
	t.Parallel()

	writeRecord := func(t *testing.T, live ...pauserecord.Entry) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), pauserecord.RecordFileName)
		if _, err := pauserecord.Update(path, time.Unix(0, 0), func(record *pauserecord.Record) error {
			record.Live = live
			return nil
		}); err != nil {
			t.Fatalf("write the pause record: %v", err)
		}
		return path
	}

	t.Run("HubEntryCovers", func(t *testing.T) {
		t.Parallel()
		covered, err := hubOnlyPaused(writeRecord(t, pauserecord.Entry{Scope: pauserecord.ScopeHub, Caller: "operator"}))("alpha")
		if err != nil || !covered {
			t.Errorf("hubOnlyPaused(alpha) = %v, %v; want true under a hub entry", covered, err)
		}
	})

	t.Run("SlugEntryDoesNotCover", func(t *testing.T) {
		t.Parallel()
		covered, err := hubOnlyPaused(writeRecord(t, pauserecord.Entry{Scope: "alpha", Caller: pauserecord.OwnerCaller}))("alpha")
		if err != nil || covered {
			t.Errorf("hubOnlyPaused(alpha) = %v, %v; want false under its own slug entry", covered, err)
		}
	})

	t.Run("UnreadableRecordIsReturned", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), pauserecord.RecordFileName)
		if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
			t.Fatalf("write the broken record: %v", err)
		}
		if _, err := hubOnlyPaused(path)("alpha"); err == nil {
			t.Errorf("hubOnlyPaused over an undecodable record returned no error")
		}
	})
}

func TestStartSuiteStartsNothingUnderTest(t *testing.T) {
	t.Parallel()
	outPath := filepath.Join(t.TempDir(), "adopt", checkOutName)
	if pid, err := startSuite(t.TempDir(), outPath); err == nil {
		t.Errorf("startSuite under go test = %d, nil; want an error", pid)
	}
	if _, err := os.Stat(filepath.Dir(outPath)); !os.IsNotExist(err) {
		t.Errorf("startSuite under go test left the output directory behind: stat error %v", err)
	}
}

func TestEnsureCheck(t *testing.T) {
	t.Parallel()

	runningRecord := CheckRecord{PID: 77, State: checkRunning, Gate: checkGateKey}
	tests := []struct {
		name      string
		record    *CheckRecord
		alive     bool
		spawnErr  error
		wantPID   int
		wantSpawn int
		wantErr   bool
	}{
		{name: "NoRecordSpawnsOnce", wantPID: 900, wantSpawn: 1},
		{name: "RunningAliveSpawnsNone", record: &runningRecord, alive: true, wantPID: 77},
		{name: "RunningDeadSpawnsOnce", record: &runningRecord, wantPID: 900, wantSpawn: 1},
		{name: "GreenRecordSpawnsOnce", record: &CheckRecord{PID: 77, State: checkGreen}, alive: true, wantPID: 900, wantSpawn: 1},
		{name: "FailedSpawnReturnsItsError", spawnErr: errors.New("exec failed"), wantSpawn: 1, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "adopt")
			if tt.record != nil {
				if err := state.WriteJSON(filepath.Join(dir, checkRecordName), filepath.Join(dir, checkRecordName+".lock"), *tt.record); err != nil {
					t.Fatalf("write the record: %v", err)
				}
			}
			spawns := 0
			got, err := ensureCheck(dir, func(CheckRecord) bool { return tt.alive }, func() (int, error) {
				spawns++
				return 900, tt.spawnErr
			})

			if (err != nil) != tt.wantErr || spawns != tt.wantSpawn {
				t.Fatalf("ensureCheck() error = %v, spawns = %d; want error %v and %d spawns", err, spawns, tt.wantErr, tt.wantSpawn)
			}
			want := suiteStart{PID: tt.wantPID, Log: filepath.Join(dir, suiteLogName), Record: filepath.Join(dir, checkRecordName)}
			if tt.wantErr {
				want = suiteStart{}
			}
			if got != want {
				t.Errorf("ensureCheck() = %+v; want %+v", got, want)
			}
		})
	}
}

func TestCheckAlive(t *testing.T) {
	t.Parallel()

	self := os.Getpid()
	ticks, _ := proc.StartTime(self)
	tests := []struct {
		name   string
		record CheckRecord
		want   bool
		// needsTicks marks a row that needs a readable start time, which only Linux gives, so no other platform generates a subtest for it.
		needsTicks bool
	}{
		{name: "OwnPidAndTicksIsAlive", record: CheckRecord{PID: self, StartTicks: ticks}, want: true},
		{name: "ReusedPidWithOtherTicksIsDead", record: CheckRecord{PID: self, StartTicks: ticks + "1"}, needsTicks: true},
		{name: "PidAloneWithNoTicksIsAlive", record: CheckRecord{PID: self}, want: true},
		{name: "PidNoProcessHoldsIsDead", record: CheckRecord{PID: math.MaxInt32 - 1, StartTicks: ticks}},
	}
	for _, tt := range tests {
		if tt.needsTicks && runtime.GOOS != "linux" {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := checkAlive(tt.record); got != tt.want {
				t.Errorf("checkAlive(%+v) = %v; want %v", tt.record, got, tt.want)
			}
		})
	}
}
