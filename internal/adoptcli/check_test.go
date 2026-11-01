// check_test.go pins the adopt check's run over a temp directory: its record, its notices and its lock, with the reconcile, the gate and the orch faked.

package adoptcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gatecli"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// scriptedTier is one tier a fake gate reports: it starts, then ends with record.
type scriptedTier struct {
	tier   int
	record gateslot.RunRecord
}

// checkFixture is a runCheck over a temp directory with a fake gate, recording the notices, the status bar marks and the gate calls.
type checkFixture struct {
	dir  string
	deps checkDeps
	// ownedBy and ownerErr are what the fixture's owner read answers.
	ownedBy    string
	ownerErr   error
	notices    []string
	marks      []checkMark
	gateCalls  int
	reconciled int
	clockReads int
}

// tick is the fixture's clock: its nth read, counting from zero, returns the time checkFixtureAfter(n), so every read differs.
func (f *checkFixture) tick() time.Time {
	now := checkFixtureAfter(f.clockReads)
	f.clockReads++
	return now
}

// checkFixtureAfter returns the time a fixture's clock reads at its nth read.
func checkFixtureAfter(read int) time.Time {
	return checkFixtureNow.Add(time.Duration(read) * time.Second)
}

// newCheckFixture returns a fixture whose gate drives tiers through the observer and returns exit.
func newCheckFixture(t *testing.T, tiers []scriptedTier, exit int) *checkFixture {
	t.Helper()
	f := &checkFixture{dir: filepath.Join(t.TempDir(), "alpha")}
	f.deps = checkDeps{
		Subject:        "alpha",
		Dir:            f.dir,
		Now:            f.tick,
		PID:            4242,
		StartTicks:     "777",
		Reconcile:      func() error { f.reconciled++; return nil },
		OwnedElsewhere: func() (string, error) { return f.ownedBy, f.ownerErr },
		RunGate: func(_ context.Context, _ io.Writer, observer gatecli.TierObserver) int {
			f.gateCalls++
			for _, step := range tiers {
				observer.Started(step.tier)
				observer.Ended(step.record)
			}
			return exit
		},
		Notice: func(line string) error { f.notices = append(f.notices, line); return nil },
		Mark:   func(mark checkMark) error { f.marks = append(f.marks, mark); return nil },
	}
	return f
}

// checkFixtureNow is the time of a fixture clock's first read, which the check's start takes.
var checkFixtureNow = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

func passedTier(tier, passed, skipped int) gateslot.RunRecord {
	return gateslot.RunRecord{Tier: tier, Outcome: "passed", Passed: passed, Skipped: skipped, WallSec: 1.5}
}

// failedTier returns a failed tier record whose counter named failures; Failed counts them.
func failedTier(tier int, failures ...gateslot.TestFailure) gateslot.RunRecord {
	return gateslot.RunRecord{Tier: tier, Outcome: "failed", ExitCode: 1, Passed: 3, Failed: len(failures), WallSec: 2, Failures: failures}
}

func (f *checkFixture) recordPath() string { return filepath.Join(f.dir, checkRecordName) }
func (f *checkFixture) logPath() string    { return filepath.Join(f.dir, suiteLogName) }

func TestRunCheck(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("two green tiers end green and post a line per tier and one end notice", func(t *testing.T) {
		t.Parallel()
		f := newCheckFixture(t, []scriptedTier{{0, passedTier(0, 10, 1)}, {1, passedTier(1, 4, 0)}}, 0)

		record, ran, err := runCheck(context.Background(), f.deps)
		if err != nil || !ran {
			t.Fatalf("runCheck() = ran %v, error %v; want it to run", ran, err)
		}
		if record.State != checkGreen || record.Running != nil || record.FailedTier != nil || record.EndedAt.IsZero() || record.PID != 4242 || record.StartTicks != "777" || record.Gate != "adopt" || record.Subject != "alpha" {
			t.Errorf("record = %+v; want green, nothing in flight, no failed tier, an end time and this process's identity", record)
		}
		if want := []CheckTier{{0, checkGreen, 10, 0, 1, 1.5, nil}, {1, checkGreen, 4, 0, 0, 1.5, nil}}; !reflect.DeepEqual(record.Tiers, want) {
			t.Errorf("tiers = %+v; want %+v", record.Tiers, want)
		}
		wantNotices := []string{
			"adopt check of alpha: tier0 green, passed 10 failed 0 skipped 1, 1.5s",
			"adopt check of alpha: tier1 green, passed 4 failed 0 skipped 0, 1.5s",
			"adopt check of alpha: green, tiers 0, 1; log " + f.logPath(),
		}
		if !slices.Equal(f.notices, wantNotices) {
			t.Errorf("notices = %q; want %q", f.notices, wantNotices)
		}
		if onDisk, found, err := ReadCheck(f.recordPath()); err != nil || !found || onDisk.State != checkGreen || len(onDisk.Tiers) != 2 {
			t.Errorf("ReadCheck() = %+v, %v, %v; want the green record on disk", onDisk, found, err)
		}
	})

	t.Run("a failing tier ends the check failed naming the tier, its failed tests, the log and the rerun", func(t *testing.T) {
		t.Parallel()
		failures := []gateslot.TestFailure{
			{Package: "example.com/a", Test: "TestA/sub", FirstLine: "a_test.go:12: got 1; want 2"},
			{Package: "example.com/b", Test: "TestB"},
		}
		f := newCheckFixture(t, []scriptedTier{{0, passedTier(0, 10, 0)}, {1, failedTier(1, failures...)}}, 1)

		record, _, err := runCheck(context.Background(), f.deps)
		if err != nil {
			t.Fatalf("runCheck() error = %v", err)
		}
		if record.State != checkFailed || record.FailedTier == nil || *record.FailedTier != 1 || record.Error != "" {
			t.Errorf("record = %+v; want failed at tier 1 with no error text", record)
		}
		wantTierNotice := "adopt check of alpha: tier1 failed, passed 3 failed 2 skipped 0, 2.0s\n" +
			"example.com/a TestA/sub: a_test.go:12: got 1; want 2\n" +
			"example.com/b TestB\n" +
			"log " + f.logPath()
		if len(f.notices) < 2 || f.notices[1] != wantTierNotice {
			t.Errorf("notices = %q; want the tier1 notice %q", f.notices, wantTierNotice)
		}
		if onDisk, _, err := ReadCheck(f.recordPath()); err != nil || len(onDisk.Tiers) != 2 || !slices.Equal(onDisk.Tiers[1].Failures, failures) {
			t.Errorf("ReadCheck() = %+v, %v; want tier1's failures on disk", onDisk, err)
		}
		end := f.notices[len(f.notices)-1]
		for _, want := range []string{"tier1", f.logPath()} {
			if !strings.Contains(end, want) {
				t.Errorf("end notice = %q; want it to hold %q", end, want)
			}
		}
		for _, unwanted := range []string{"in the prime", "lym gate test --gate adopt"} {
			if strings.Contains(end, unwanted) {
				t.Errorf("end notice = %q; want it to hold no %q", end, unwanted)
			}
		}
		if len(f.notices) != 3 {
			t.Errorf("notices = %q; want two tier lines and one end notice", f.notices)
		}
	})

	t.Run("a failing tier's notice lists failed tests up to the cap and counts the rest", func(t *testing.T) {
		t.Parallel()
		failures := make([]gateslot.TestFailure, maxListedFailures+3)
		for i := range failures {
			failures[i] = gateslot.TestFailure{Package: "example.com/p", Test: fmt.Sprintf("Test%d", i), FirstLine: "boom"}
		}
		f := newCheckFixture(t, []scriptedTier{{0, failedTier(0, failures...)}}, 1)

		if _, _, err := runCheck(context.Background(), f.deps); err != nil {
			t.Fatalf("runCheck() error = %v", err)
		}
		lines := strings.Split(f.notices[0], "\n")
		if want := maxListedFailures + 3; len(lines) != want {
			t.Fatalf("tier notice = %q; want %d lines: the summary, %d tests, the remainder and the log", f.notices[0], want, maxListedFailures)
		}
		if got, want := lines[maxListedFailures], "example.com/p Test9: boom"; got != want {
			t.Errorf("last listed test = %q; want %q", got, want)
		}
		if got, want := lines[len(lines)-2:], []string{"and 3 more", "log " + f.logPath()}; !slices.Equal(got, want) {
			t.Errorf("notice tail = %q; want %q", got, want)
		}
	})

	t.Run("a failing tier with no recorded failures has a notice of the summary line and the log", func(t *testing.T) {
		t.Parallel()
		f := newCheckFixture(t, []scriptedTier{{0, failedTier(0)}}, 1)

		if _, _, err := runCheck(context.Background(), f.deps); err != nil {
			t.Fatalf("runCheck() error = %v", err)
		}
		want := "adopt check of alpha: tier0 failed, passed 3 failed 0 skipped 0, 2.0s\nlog " + f.logPath()
		if len(f.notices) == 0 || f.notices[0] != want {
			t.Errorf("notices = %q; want the tier0 notice %q", f.notices, want)
		}
	})

	t.Run("a notice or mark that fails changes nothing in the record", func(t *testing.T) {
		t.Parallel()
		f := newCheckFixture(t, []scriptedTier{{0, passedTier(0, 1, 0)}, {1, passedTier(1, 1, 0)}}, 0)
		f.deps.Notice = func(string) error { return errors.New("no orch") }
		f.deps.Mark = func(checkMark) error { return errors.New("no reed session") }

		record, ran, err := runCheck(context.Background(), f.deps)
		if err != nil || !ran || record.State != checkGreen || len(record.Tiers) != 2 {
			t.Errorf("runCheck() = %+v, ran %v, error %v; want a green record holding both tiers", record, ran, err)
		}
	})

	t.Run("a failing reconcile ends the check failed, runs no gate and posts one notice", func(t *testing.T) {
		t.Parallel()
		f := newCheckFixture(t, nil, 0)
		f.deps.Reconcile = func() error { return errors.New("config invalid") }
		endCalls := 0
		f.deps.Green = func() []string { endCalls++; return nil }
		f.deps.Failed = func(CheckRecord) []string { endCalls++; return nil }

		record, ran, err := runCheck(context.Background(), f.deps)
		if err != nil || !ran {
			t.Fatalf("runCheck() = ran %v, error %v; want it to run", ran, err)
		}
		if record.State != checkFailed || !strings.Contains(record.Error, "config invalid") || !strings.Contains(record.Error, "lym fabric reconcile") || !strings.Contains(record.Error, "lym adopt alpha") || strings.Contains(record.Error, "lym gate test --gate adopt") {
			t.Errorf("record = %+v; want failed with the reconcile's error and the re-run of lym adopt alpha", record)
		}
		if endCalls != 0 {
			t.Errorf("Green and Failed calls = %d; want none", endCalls)
		}
		if f.gateCalls != 0 || len(f.notices) != 1 || !strings.Contains(f.notices[0], "config invalid") {
			t.Errorf("gate calls = %d, notices = %q; want no gate and the one notice naming the error", f.gateCalls, f.notices)
		}
	})

	t.Run("a subject owned elsewhere posts the notice naming its adopt, writes no record and runs no gate", func(t *testing.T) {
		t.Parallel()
		f := newCheckFixture(t, nil, 0)
		f.ownedBy = "otherbox"

		_, ran, err := runCheck(context.Background(), f.deps)
		want := "adopt check of alpha: alpha is owned by otherbox, so no check ran; way forward: run lym adopt alpha on this machine"
		if ran || err == nil || err.Error() != want {
			t.Errorf("runCheck() = ran %v, error %v; want it not to run, with the error %q", ran, err, want)
		}
		if !slices.Equal(f.notices, []string{want}) || f.gateCalls != 0 || f.reconciled != 0 {
			t.Errorf("notices = %q, gate calls = %d, reconciles = %d; want the one notice and nothing run", f.notices, f.gateCalls, f.reconciled)
		}
		if _, err := os.Stat(f.dir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Stat(%s) error = %v; want no directory or record written", f.dir, err)
		}
	})

	t.Run("an owner read that fails writes nothing and runs no gate", func(t *testing.T) {
		t.Parallel()
		f := newCheckFixture(t, nil, 0)
		f.ownerErr = errors.New("board unreadable")

		_, ran, err := runCheck(context.Background(), f.deps)
		if ran || err == nil || !strings.Contains(err.Error(), "board unreadable") || !strings.Contains(err.Error(), "lym adopt alpha") {
			t.Errorf("runCheck() = ran %v, error %v; want it not to run, with an error naming the cause and lym adopt alpha", ran, err)
		}
		if _, statErr := os.Stat(f.dir); !errors.Is(statErr, os.ErrNotExist) || f.gateCalls != 0 || len(f.notices) != 0 {
			t.Errorf("Stat(%s) error = %v, gate calls = %d, notices = %q; want nothing written, run or posted", f.dir, statErr, f.gateCalls, f.notices)
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
		failedCalls := 0
		f.deps.Failed = func(CheckRecord) []string { failedCalls++; return nil }

		record, _, err := runCheck(context.Background(), f.deps)
		if err != nil {
			t.Fatalf("runCheck() error = %v", err)
		}
		if record.State != checkFailed || record.FailedTier != nil || !strings.Contains(record.Error, "exited 1") {
			t.Errorf("record = %+v; want failed with no failed tier and an error naming exit 1", record)
		}
		if len(f.notices) != 1 || !strings.Contains(f.notices[0], "exited 1") || !strings.Contains(f.notices[0], f.logPath()) || !strings.Contains(f.notices[0], "lym adopt alpha") {
			t.Errorf("notices = %q; want one end notice naming the exit code, the log and lym adopt alpha", f.notices)
		}
		if strings.Contains(f.notices[0], "in the prime") || strings.Contains(f.notices[0], "lym gate test --gate adopt") || failedCalls != 0 {
			t.Errorf("notice = %q, Failed calls = %d; want no worktree named and no fix loop", f.notices[0], failedCalls)
		}
	})

	t.Run("a failed tier calls Failed once after the failed record is written and posts its lines after the failed notice", func(t *testing.T) {
		t.Parallel()
		f := newCheckFixture(t, []scriptedTier{{0, failedTier(0, gateslot.TestFailure{Package: "example.com/a", Test: "TestA"})}}, 1)
		calls := 0
		var seen CheckRecord
		var onDisk CheckRecord
		f.deps.Failed = func(record CheckRecord) []string {
			calls++
			seen = record
			onDisk, _, _ = ReadCheck(f.recordPath())
			return []string{"filed the fix task adopt-fix-alpha-t1"}
		}

		if _, _, err := runCheck(context.Background(), f.deps); err != nil {
			t.Fatalf("runCheck() error = %v", err)
		}
		if calls != 1 || seen.State != checkFailed || seen.FailedTier == nil || onDisk.State != checkFailed || onDisk.EndedAt.IsZero() {
			t.Errorf("Failed calls = %d, record = %+v, on disk = %+v; want one call with the failed record written", calls, seen, onDisk)
		}
		wantEnd := []string{"adopt check of alpha: failed, tier0 failed; log " + f.logPath(), "filed the fix task adopt-fix-alpha-t1"}
		if got := f.notices[len(f.notices)-2:]; !slices.Equal(got, wantEnd) {
			t.Errorf("last notices = %q; want %q", got, wantEnd)
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

	t.Run("a green end calls Green once after the green record is written and posts its lines after the green notice", func(t *testing.T) {
		t.Parallel()
		f := newCheckFixture(t, []scriptedTier{{0, passedTier(0, 1, 0)}}, 0)
		calls := 0
		var onDiskAtCall CheckRecord
		f.deps.Green = func() []string {
			calls++
			onDiskAtCall, _, _ = ReadCheck(f.recordPath())
			return []string{"the hold of t1 was not lifted", "the dobby was not brought up"}
		}

		if _, _, err := runCheck(context.Background(), f.deps); err != nil {
			t.Fatalf("runCheck() error = %v", err)
		}
		if calls != 1 || onDiskAtCall.State != checkGreen || onDiskAtCall.EndedAt.IsZero() {
			t.Errorf("Green calls = %d, record at the call = %+v; want one call after the green record was written", calls, onDiskAtCall)
		}
		want := []string{
			"adopt check of alpha: tier0 green, passed 1 failed 0 skipped 0, 1.5s",
			"adopt check of alpha: green, tiers 0; log " + f.logPath(),
			"the hold of t1 was not lifted",
			"the dobby was not brought up",
		}
		if !slices.Equal(f.notices, want) {
			t.Errorf("notices = %q; want %q", f.notices, want)
		}
	})

	t.Run("a failed end and an end before any tier never call Green, and a check leaves the holds and tries files as they were", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			tiers []scriptedTier
			exit  int
		}{
			{"GreenEnd", []scriptedTier{{0, passedTier(0, 1, 0)}}, 0},
			{"FailedTier", []scriptedTier{{0, failedTier(0)}}, 1},
			{"GateExitsBeforeAnyTier", nil, 1},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newCheckFixture(t, tt.tiers, tt.exit)
				if err := appendHolds(f.dir, []string{"t1", "t2"}); err != nil {
					t.Fatal(err)
				}
				if err := writeTries(f.dir, 2); err != nil {
					t.Fatal(err)
				}
				holdsBefore, _ := os.ReadFile(filepath.Join(f.dir, holdsFileName))
				triesBefore, _ := os.ReadFile(filepath.Join(f.dir, triesFileName))
				calls := 0
				f.deps.Green = func() []string { calls++; return nil }

				if _, ran, err := runCheck(context.Background(), f.deps); err != nil || !ran {
					t.Fatalf("runCheck() = ran %v, error %v; want it to run", ran, err)
				}
				if wantCalls := map[bool]int{true: 1, false: 0}[tt.exit == 0]; calls != wantCalls {
					t.Errorf("Green calls = %d; want %d", calls, wantCalls)
				}
				holdsAfter, _ := os.ReadFile(filepath.Join(f.dir, holdsFileName))
				triesAfter, _ := os.ReadFile(filepath.Join(f.dir, triesFileName))
				if string(holdsAfter) != string(holdsBefore) || string(triesAfter) != string(triesBefore) {
					t.Errorf("holds %q -> %q, tries %q -> %q; want both files byte for byte as they were", holdsBefore, holdsAfter, triesBefore, triesAfter)
				}
			})
		}
	})

	t.Run("a subject owned elsewhere and a failing reconcile never call Green", func(t *testing.T) {
		t.Parallel()
		owned := newCheckFixture(t, nil, 0)
		owned.ownedBy = "otherbox"
		reconcile := newCheckFixture(t, nil, 0)
		reconcile.deps.Reconcile = func() error { return errors.New("config invalid") }
		for _, f := range []*checkFixture{owned, reconcile} {
			calls := 0
			f.deps.Green = func() []string { calls++; return nil }
			_, _, _ = runCheck(context.Background(), f.deps)
			if calls != 0 {
				t.Errorf("Green calls = %d; want none", calls)
			}
		}
	})
}

// TestRunCheckMarks pins the status bar marks of a check run, in order: its start, each tier in flight and ended, and its end.
// The clock reads once for the check's start, then once as each tier starts, so tier n starts at read n+1 and a mark timed from the check's start instead fails.
func TestRunCheckMarks(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	timed := func(tier int, outcome string, wall float64) scriptedTier {
		return scriptedTier{tier, gateslot.RunRecord{Tier: tier, Outcome: outcome, WallSec: wall}}
	}
	starting := checkMark{State: checkRunning, Text: "adopt check of alpha: starting", Start: checkFixtureNow}

	tests := []struct {
		name      string
		tiers     []scriptedTier
		exit      int
		reconcile error
		want      []checkMark
	}{
		{
			name:  "GreenShowsEachTierInFlightThenEveryTierWithItsTime",
			tiers: []scriptedTier{timed(0, "passed", 40), timed(1, "passed", 104)},
			want: []checkMark{
				starting,
				{State: checkRunning, Text: "adopt check of alpha: tier0", Start: checkFixtureAfter(1)},
				{State: checkRunning, Text: "adopt check of alpha: tier0 green 40s"},
				{State: checkRunning, Text: "adopt check of alpha: tier0 green 40s · tier1", Start: checkFixtureAfter(2)},
				{State: checkRunning, Text: "adopt check of alpha: tier0 green 40s · tier1 green 104s"},
				{State: checkGreen, Text: "adopt check of alpha green: tier0 40s · tier1 104s"},
			},
		},
		{
			name:  "FailedEndsWithEveryTierAndItsOutcome",
			tiers: []scriptedTier{timed(0, "passed", 40), timed(1, "failed", 104)},
			exit:  1,
			want: []checkMark{
				starting,
				{State: checkRunning, Text: "adopt check of alpha: tier0", Start: checkFixtureAfter(1)},
				{State: checkRunning, Text: "adopt check of alpha: tier0 green 40s"},
				{State: checkRunning, Text: "adopt check of alpha: tier0 green 40s · tier1", Start: checkFixtureAfter(2)},
				{State: checkRunning, Text: "adopt check of alpha: tier0 green 40s · tier1 failed 104s"},
				{State: checkFailed, Text: "adopt check of alpha failed: tier0 green 40s · tier1 failed 104s"},
			},
		},
		{
			name:      "FailedBeforeAnyTierNamesBeamStatus",
			reconcile: errors.New("config invalid"),
			want: []checkMark{
				starting,
				{State: checkFailed, Text: "adopt check of alpha failed before any tier ended; lym beam status shows why"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newCheckFixture(t, tt.tiers, tt.exit)
			if tt.reconcile != nil {
				f.deps.Reconcile = func() error { return tt.reconcile }
			}

			if _, _, err := runCheck(context.Background(), f.deps); err != nil {
				t.Fatalf("runCheck() error = %v", err)
			}
			if !reflect.DeepEqual(f.marks, tt.want) {
				t.Errorf("marks = %+v; want %+v", f.marks, tt.want)
			}
		})
	}
}

// TestPrimeMark pins the prime's mark over a temp adopt directory: the check's own mark, joined with the running checks of the other subjects.
func TestPrimeMark(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	own := checkMark{State: checkRunning, Text: "adopt check of alpha: tier0", Start: checkFixtureNow}
	ownGreen := checkMark{State: checkGreen, Text: "adopt check of alpha green: tier0 40s"}

	tests := []struct {
		name string
		own  checkMark
		// others maps a subject to the record its directory holds.
		others map[string]CheckRecord
		want   checkMark
	}{
		{
			name: "OwnMarkAlone",
			own:  own,
			want: own,
		},
		{
			name:   "RunningOtherSubjectsAreJoinedAfterItSortedWithStateRunning",
			own:    ownGreen,
			others: map[string]CheckRecord{"zeta": {Subject: "zeta", State: checkRunning}, "root": {Subject: "root", State: checkRunning}, "beta": {Subject: "beta", State: checkGreen}},
			want:   checkMark{State: checkRunning, Text: "adopt check of alpha green: tier0 40s | adopt check of root: starting | adopt check of zeta: starting"},
		},
		{
			name:   "FailedOtherSubjectIsNotJoined",
			own:    ownGreen,
			others: map[string]CheckRecord{"beta": {Subject: "beta", State: checkFailed}},
			want:   ownGreen,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			adoptDir := t.TempDir()
			for subject, record := range tt.others {
				if err := writeCheck(filepath.Join(adoptDir, subject, checkRecordName), record); err != nil {
					t.Fatalf("write the record of %s: %v", subject, err)
				}
			}
			if err := writeCheck(filepath.Join(adoptDir, "alpha", checkRecordName), CheckRecord{Subject: "alpha", State: checkRunning}); err != nil {
				t.Fatalf("write the record of alpha: %v", err)
			}

			if got := primeMark(adoptDir, "alpha", tt.own); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("primeMark() = %+v; want %+v", got, tt.want)
			}
		})
	}
}
