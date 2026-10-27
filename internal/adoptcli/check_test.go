//go:build tier0

// check_test.go pins the adopt check's run over a temp directory: its record, its notices and its lock, with the reconcile, the gate and the orch faked.

package adoptcli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gatecli"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lock"
)

// scriptedTier is one tier a fake gate reports: it starts, then ends with record.
type scriptedTier struct {
	tier   int
	record gateslot.RunRecord
}

// checkFixture is a runCheck over a temp directory with a fake gate, recording the notices and the gate calls.
type checkFixture struct {
	dir        string
	deps       checkDeps
	notices    []string
	gateCalls  int
	reconciled int
}

// newCheckFixture returns a fixture whose gate drives tiers through the observer and returns exit.
func newCheckFixture(t *testing.T, tiers []scriptedTier, exit int) *checkFixture {
	t.Helper()
	f := &checkFixture{dir: filepath.Join(t.TempDir(), "adopt")}
	f.deps = checkDeps{
		Dir:        f.dir,
		Now:        func() time.Time { return time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC) },
		PID:        4242,
		StartTicks: "777",
		Reconcile:  func() error { f.reconciled++; return nil },
		RunGate: func(_ context.Context, _ io.Writer, observer gatecli.TierObserver) int {
			f.gateCalls++
			for _, step := range tiers {
				observer.Started(step.tier)
				observer.Ended(step.record)
			}
			return exit
		},
		Notice: func(line string) error { f.notices = append(f.notices, line); return nil },
	}
	return f
}

func passedTier(tier, passed, skipped int) gateslot.RunRecord {
	return gateslot.RunRecord{Tier: tier, Outcome: "passed", Passed: passed, Skipped: skipped, WallSec: 1.5}
}

func failedTier(tier, failed int) gateslot.RunRecord {
	return gateslot.RunRecord{Tier: tier, Outcome: "failed", ExitCode: 1, Passed: 3, Failed: failed, WallSec: 2}
}

func (f *checkFixture) recordPath() string { return filepath.Join(f.dir, checkRecordName) }
func (f *checkFixture) logPath() string    { return filepath.Join(f.dir, suiteLogName) }

func TestRunCheck(t *testing.T) {
	t.Parallel()

	t.Run("two green tiers end green and post a line per tier and one end notice", func(t *testing.T) {
		t.Parallel()
		f := newCheckFixture(t, []scriptedTier{{0, passedTier(0, 10, 1)}, {1, passedTier(1, 4, 0)}}, 0)

		record, ran, err := runCheck(context.Background(), f.deps)
		if err != nil || !ran {
			t.Fatalf("runCheck() = ran %v, error %v; want it to run", ran, err)
		}
		if record.State != checkGreen || record.Running != nil || record.FailedTier != nil || record.EndedAt.IsZero() || record.PID != 4242 || record.StartTicks != "777" || record.Gate != "adopt" {
			t.Errorf("record = %+v; want green, nothing in flight, no failed tier, an end time and this process's identity", record)
		}
		if want := []CheckTier{{0, checkGreen, 10, 0, 1, 1.5}, {1, checkGreen, 4, 0, 0, 1.5}}; !slices.Equal(record.Tiers, want) {
			t.Errorf("tiers = %+v; want %+v", record.Tiers, want)
		}
		wantNotices := []string{
			"adopt check: tier0 green, passed 10 failed 0 skipped 1, 1.5s",
			"adopt check: tier1 green, passed 4 failed 0 skipped 0, 1.5s",
			"adopt check: green, tiers 0, 1; log " + f.logPath(),
		}
		if !slices.Equal(f.notices, wantNotices) {
			t.Errorf("notices = %q; want %q", f.notices, wantNotices)
		}
		if onDisk, found, err := ReadCheck(f.recordPath()); err != nil || !found || onDisk.State != checkGreen || len(onDisk.Tiers) != 2 {
			t.Errorf("ReadCheck() = %+v, %v, %v; want the green record on disk", onDisk, found, err)
		}
	})

	t.Run("a failing tier ends the check failed naming the tier, the log and the rerun", func(t *testing.T) {
		t.Parallel()
		f := newCheckFixture(t, []scriptedTier{{0, passedTier(0, 10, 0)}, {1, failedTier(1, 2)}}, 1)

		record, _, err := runCheck(context.Background(), f.deps)
		if err != nil {
			t.Fatalf("runCheck() error = %v", err)
		}
		if record.State != checkFailed || record.FailedTier == nil || *record.FailedTier != 1 || record.Error != "" {
			t.Errorf("record = %+v; want failed at tier 1 with no error text", record)
		}
		end := f.notices[len(f.notices)-1]
		for _, want := range []string{"tier1", f.logPath(), "lym gate test --gate adopt"} {
			if !strings.Contains(end, want) {
				t.Errorf("end notice = %q; want it to hold %q", end, want)
			}
		}
		if len(f.notices) != 3 {
			t.Errorf("notices = %q; want two tier lines and one end notice", f.notices)
		}
	})

	t.Run("a notice that fails changes nothing in the record", func(t *testing.T) {
		t.Parallel()
		f := newCheckFixture(t, []scriptedTier{{0, passedTier(0, 1, 0)}, {1, passedTier(1, 1, 0)}}, 0)
		f.deps.Notice = func(string) error { return errors.New("no orch") }

		record, ran, err := runCheck(context.Background(), f.deps)
		if err != nil || !ran || record.State != checkGreen || len(record.Tiers) != 2 {
			t.Errorf("runCheck() = %+v, ran %v, error %v; want a green record holding both tiers", record, ran, err)
		}
	})

	t.Run("a failing reconcile ends the check failed, runs no gate and posts one notice", func(t *testing.T) {
		t.Parallel()
		f := newCheckFixture(t, nil, 0)
		f.deps.Reconcile = func() error { return errors.New("config invalid") }

		record, ran, err := runCheck(context.Background(), f.deps)
		if err != nil || !ran {
			t.Fatalf("runCheck() = ran %v, error %v; want it to run", ran, err)
		}
		if record.State != checkFailed || !strings.Contains(record.Error, "config invalid") || !strings.Contains(record.Error, "lym fabric reconcile") || !strings.Contains(record.Error, "lym gate test --gate adopt") {
			t.Errorf("record = %+v; want failed with the reconcile's error and the way forward", record)
		}
		if f.gateCalls != 0 || len(f.notices) != 1 || !strings.Contains(f.notices[0], "config invalid") {
			t.Errorf("gate calls = %d, notices = %q; want no gate and the one notice naming the error", f.gateCalls, f.notices)
		}
	})

	t.Run("a check over an existing green record and a full log writes a fresh record and empties the log first", func(t *testing.T) {
		t.Parallel()
		f := newCheckFixture(t, nil, 0)
		if err := os.MkdirAll(f.dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f.recordPath(), []byte(`{"pid": 1, "state": "green", "gate": "adopt", "tiers": [{"tier": 9, "outcome": "green"}]}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f.logPath(), []byte("the previous run\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var logAtStart int64 = -1
		f.deps.RunGate = func(context.Context, io.Writer, gatecli.TierObserver) int {
			if info, err := os.Stat(f.logPath()); err == nil {
				logAtStart = info.Size()
			}
			return 0
		}

		record, _, err := runCheck(context.Background(), f.deps)
		if err != nil {
			t.Fatalf("runCheck() error = %v", err)
		}
		if logAtStart != 0 {
			t.Errorf("log size when the gate started = %d; want 0, truncated by the check", logAtStart)
		}
		if record.PID != 4242 || len(record.Tiers) != 0 {
			t.Errorf("record = %+v; want a fresh record of this process with no earlier tier", record)
		}
	})

	t.Run("ReadCheck on a directory that does not exist reports false and creates nothing", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "absent")

		if _, found, err := ReadCheck(filepath.Join(dir, checkRecordName)); found || err != nil {
			t.Errorf("ReadCheck() = found %v, error %v; want false and no error", found, err)
		}
		if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Stat(%s) error = %v; want the directory still absent", dir, err)
		}
	})

	t.Run("a held check lock reports the check did not run and touches nothing", func(t *testing.T) {
		t.Parallel()
		f := newCheckFixture(t, nil, 0)
		if err := os.MkdirAll(f.dir, 0o755); err != nil {
			t.Fatal(err)
		}
		recordBefore, logBefore := []byte(`{"pid": 1, "state": "running", "gate": "adopt"}`), []byte("the running check's output\n")
		if err := os.WriteFile(f.recordPath(), recordBefore, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f.logPath(), logBefore, 0o644); err != nil {
			t.Fatal(err)
		}
		held, err := lock.AcquireWriteLock(filepath.Join(f.dir, checkLockName))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = held.Release() })

		_, ran, err := runCheck(context.Background(), f.deps)
		if err != nil || ran {
			t.Fatalf("runCheck() = ran %v, error %v; want it not to run, without an error", ran, err)
		}
		recordAfter, _ := os.ReadFile(f.recordPath())
		logAfter, _ := os.ReadFile(f.logPath())
		if string(recordAfter) != string(recordBefore) || string(logAfter) != string(logBefore) || f.gateCalls != 0 || f.reconciled != 0 || len(f.notices) != 0 {
			t.Errorf("record %q, log %q, gate calls %d, reconciles %d, notices %q; want everything untouched", recordAfter, logAfter, f.gateCalls, f.reconciled, f.notices)
		}
	})

	t.Run("the record reads running with the tier in flight while the gate runs it", func(t *testing.T) {
		t.Parallel()
		f := newCheckFixture(t, nil, 0)
		var seen CheckRecord
		var found bool
		f.deps.RunGate = func(_ context.Context, _ io.Writer, observer gatecli.TierObserver) int {
			observer.Started(3)
			seen, found, _ = ReadCheck(f.recordPath())
			observer.Ended(passedTier(3, 1, 0))
			return 0
		}

		if _, _, err := runCheck(context.Background(), f.deps); err != nil {
			t.Fatalf("runCheck() error = %v", err)
		}
		if !found || seen.State != checkRunning || seen.Running == nil || *seen.Running != 3 {
			t.Errorf("record read mid-run = %+v, found %v; want running with tier 3 in flight", seen, found)
		}
	})

	t.Run("a gate that exits 1 with no tier ended ends failed naming the exit code and the log", func(t *testing.T) {
		t.Parallel()
		f := newCheckFixture(t, nil, 1)

		record, _, err := runCheck(context.Background(), f.deps)
		if err != nil {
			t.Fatalf("runCheck() error = %v", err)
		}
		if record.State != checkFailed || record.FailedTier != nil || !strings.Contains(record.Error, "exited 1") {
			t.Errorf("record = %+v; want failed with no failed tier and an error naming exit 1", record)
		}
		if len(f.notices) != 1 || !strings.Contains(f.notices[0], "exited 1") || !strings.Contains(f.notices[0], f.logPath()) {
			t.Errorf("notices = %q; want one end notice naming the exit code and the log", f.notices)
		}
	})

	t.Run("a gate that starts a tier and returns without ending it leaves nothing in flight", func(t *testing.T) {
		t.Parallel()
		f := newCheckFixture(t, nil, 0)
		f.deps.RunGate = func(_ context.Context, _ io.Writer, observer gatecli.TierObserver) int {
			observer.Started(0)
			return 1
		}

		record, _, err := runCheck(context.Background(), f.deps)
		if err != nil {
			t.Fatalf("runCheck() error = %v", err)
		}
		if onDisk, _, _ := ReadCheck(f.recordPath()); record.State != checkFailed || record.Running != nil || onDisk.Running != nil {
			t.Errorf("record = %+v, on disk %+v; want failed with no tier in flight", record, onDisk)
		}
	})

	t.Run("a record that does not decode reads as absent and the check writes a fresh one over it", func(t *testing.T) {
		t.Parallel()
		f := newCheckFixture(t, []scriptedTier{{0, passedTier(0, 1, 0)}}, 0)
		if err := os.MkdirAll(f.dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f.recordPath(), []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}

		if _, found, err := ReadCheck(f.recordPath()); found || err != nil {
			t.Errorf("ReadCheck() over a damaged file = found %v, error %v; want false and no error", found, err)
		}
		if _, ran, err := runCheck(context.Background(), f.deps); err != nil || !ran {
			t.Fatalf("runCheck() over a damaged record = ran %v, error %v; want it to run", ran, err)
		}
		if onDisk, found, err := ReadCheck(f.recordPath()); err != nil || !found || onDisk.State != checkGreen {
			t.Errorf("ReadCheck() after the check = %+v, %v, %v; want the fresh green record", onDisk, found, err)
		}
	})
}
