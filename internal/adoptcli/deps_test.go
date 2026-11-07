// deps_test.go covers the hub-only pause reading and the adopt check start's guard under go test.
// It also covers the start's alive skip, its liveness reading, and the adopt gate tiers it carries.

package adoptcli

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestHubOnlyPaused(t *testing.T) {
	tiergate.Require(t, 0)
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
	tiergate.Require(t, 0)
	t.Parallel()
	outPath := filepath.Join(t.TempDir(), "adopt", checkOutName)
	if pid, err := startSuite(t.TempDir(), outPath, "alpha"); err == nil {
		t.Errorf("startSuite under go test = %d, nil; want an error", pid)
	}
	if _, err := os.Stat(filepath.Dir(outPath)); !os.IsNotExist(err) {
		t.Errorf("startSuite under go test left the output directory behind: stat error %v", err)
	}
}

func TestEnsureCheck(t *testing.T) {
	tiergate.Require(t, 0)
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
		// wantRunning says the check was found running rather than started.
		wantRunning bool
	}{
		{name: "NoRecordSpawnsOnce", wantPID: 900, wantSpawn: 1},
		{name: "RunningAliveSpawnsNone", record: &runningRecord, alive: true, wantPID: 77, wantRunning: true},
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
			want := suiteStart{PID: tt.wantPID, Log: filepath.Join(dir, suiteLogName), Record: filepath.Join(dir, checkRecordName), AlreadyRunning: tt.wantRunning}
			if tt.wantErr {
				want = suiteStart{}
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("ensureCheck() = %+v; want %+v", got, want)
			}
		})
	}
}

func TestAdoptGateTiers(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const declaredTiers = "slots: 2\ntiers:\n  tier0: {command: x, timeout_min: 5, counter: vstest}\n  tier1: {command: y, timeout_min: 5, counter: vstest}\n"
	tests := []struct {
		name string
		gate string
		want []int
	}{
		{name: "AdoptKeyReturnsItsTiers", gate: declaredTiers + "gates:\n  adopt: {tiers: [0, 1], scope: all}\n", want: []int{0, 1}},
		{name: "AdoptKeyNamingAnUndeclaredTierReturnsNone", gate: declaredTiers + "gates:\n  adopt: {tiers: [5], scope: all}\n"},
		{name: "ConfigThatDoesNotReadReturnsNone", gate: "slots: 0\n"},
		{name: "AbsentConfigReturnsNone"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			prime := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "prime"}
			if tt.gate != "" {
				configDir := filepath.Join(prime.AnchorPath(), "_lym", "config")
				if err := os.MkdirAll(configDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(configDir, "gate.yaml"), []byte(tt.gate), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			if got := adoptGateTiers(prime); !slices.Equal(got, tt.want) {
				t.Errorf("adoptGateTiers() = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestStartCheckWithTiers(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	prime := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "prime"}
	configDir := filepath.Join(prime.AnchorPath(), "_lym", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gate := "slots: 2\ntiers:\n  tier0: {command: x, timeout_min: 5, counter: vstest}\n  tier1: {command: y, timeout_min: 5, counter: vstest}\ngates:\n  adopt: {tiers: [0, 1], scope: all}\n"
	if err := os.WriteFile(filepath.Join(configDir, "gate.yaml"), []byte(gate), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "adopt", "b")
	running := CheckRecord{Subject: "b", PID: 77, State: checkRunning, Gate: checkGateKey}
	if err := state.WriteJSON(filepath.Join(dir, checkRecordName), filepath.Join(dir, checkRecordName+".lock"), running); err != nil {
		t.Fatalf("write the record: %v", err)
	}

	t.Run("RunningCheckIsFoundWithTheGateTiers", func(t *testing.T) {
		t.Parallel()
		got, err := startCheckWithTiers(prime, dir, func() error { return nil }, func(CheckRecord) bool { return true }, func() (int, error) {
			t.Error("startCheckWithTiers spawned a check over a running one")
			return 0, nil
		})

		if err != nil || got.PID != 77 || !got.AlreadyRunning || !slices.Equal(got.Tiers, []int{0, 1}) {
			t.Errorf("startCheckWithTiers() = %+v, %v; want the running check 77 with tiers [0 1]", got, err)
		}
	})

	t.Run("WorktreeThatDoesNotResolveSpawnsNothingAndNamesBeamStart", func(t *testing.T) {
		t.Parallel()
		resolveErr := errors.New("the worktree of b does not resolve (no such directory); way forward: run lym beam start b, then re-run lym adopt b")
		got, err := startCheckWithTiers(prime, dir, func() error { return resolveErr }, func(CheckRecord) bool { return false }, func() (int, error) {
			t.Error("startCheckWithTiers spawned a check for a worktree that does not resolve")
			return 0, nil
		})

		if !errors.Is(err, resolveErr) || !strings.Contains(err.Error(), "lym beam start b") || !reflect.DeepEqual(got, suiteStart{}) {
			t.Errorf("startCheckWithTiers() = %+v, %v; want no start and the resolve error naming lym beam start b", got, err)
		}
	})
}

func TestCheckAlive(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	self := os.Getpid()
	ticks, _ := proc.StartTime(self)
	tests := []struct {
		name   string
		record CheckRecord
		want   bool
		// needsTicks marks a row that needs a readable start time, which only Linux gives,
		// so no other platform generates a subtest for it.
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
