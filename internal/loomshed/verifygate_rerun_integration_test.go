//go:build !windows

// verifygate_rerun_integration_test.go drives the darn form of NewVerifyGate past a failed run.
// A stand-in `lym` on PATH records each call, over a real scratch repo and real shell processes.

package loomshed

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestVerifyGate_DarnFormRunsTheFailedTestsAloneFirst pins the arrivals after a failed darn-form run.
// After a run whose report names failed tests, the next arrival runs those tests alone first.
// Tests that fail again end it with their findings, and the full command does not run.
// Tests that pass alone hand it to the full command, and a run that names no failed test keeps nothing.
// The steps share one gate closure and run in order.
// It is not parallel: t.Setenv puts the stand-in `lym` first on PATH, which is process-global state.
func TestVerifyGate_DarnFormRunsTheFailedTestsAloneFirst(t *testing.T) {
	tiergate.Require(t, 1)

	worktree, verifyDir, bin := t.TempDir(), t.TempDir(), t.TempDir()
	gitkit.Git(t, worktree, "init", "-b", "main")
	gitkit.Git(t, worktree, "config", "user.email", "test@test.com")
	gitkit.Git(t, worktree, "config", "user.name", "Test")
	gitkit.CommitFile(t, worktree, "a.txt", "a\n", "init")
	base := gitkit.Git(t, worktree, "rev-parse", "HEAD")

	write := func(t *testing.T, path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	lines := func(t *testing.T, path string) []string {
		t.Helper()
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			t.Fatal(err)
		}
		return strings.Split(strings.TrimSpace(string(data)), "\n")
	}

	calls, aloneReport := filepath.Join(bin, "calls.log"), filepath.Join(bin, "alone-report.txt")
	write(t, filepath.Join(bin, "lym"), "#!/bin/sh\necho \"$*\" >> '"+calls+"'\nif [ -f '"+aloneReport+"' ]; then cat '"+aloneReport+"'; exit 1; fi\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	namesTestA := gateslot.FailureReportHeader + "\ntier1 (exit 1), log /runs/a.log:\n  example.com/m/a TestA/sub:\n    a_test.go:3: boom\n" +
		gateslot.FailedRunsPrefix + "tier1 (exit 1); the exit code is tier1's\n"
	fullRuns, fullReport := filepath.Join(bin, "full.log"), filepath.Join(bin, "full-report.txt")
	fullExit := "1"
	scoped := func() (string, bool, error) {
		return "echo full >> '" + fullRuns + "'; cat '" + fullReport + "'; exit " + fullExit, true, nil
	}
	told := func() (string, error) { return "lym gate test --wait --gate darn-write --stop-on-fail", nil }
	gate := NewVerifyGate(worktree, verifyDir, "Darn-Write gate", told, func() (string, error) { return base, nil }, scoped, nil, VerifyBudget{})
	wantAlone := "gate test --wait --stop-on-fail --tier 1 example.com/m/a -- -run ^(TestA)$"

	if !t.Run("a failed full run names its failed tests and runs nothing alone", func(t *testing.T) {
		write(t, fullReport, namesTestA)
		got, err := gate()
		if err != nil || got.Passed || !strings.Contains(got.Findings, "TestA") {
			t.Fatalf("gate() = %+v, %v; want a failure naming TestA", got, err)
		}
		if got := lines(t, calls); got != nil {
			t.Errorf("lym calls = %q; want none on the first arrival", got)
		}
	}) {
		return
	}

	if !t.Run("tests that fail again alone end the arrival with their findings and leave the full command unrun", func(t *testing.T) {
		write(t, aloneReport, namesTestA)
		got, err := gate()
		if err != nil || got.Passed || !strings.HasPrefix(got.Findings, failedAloneNote) {
			t.Fatalf("gate() = %+v, %v; want a failure opening with the note on the tests run alone", got, err)
		}
		if got := lines(t, calls); len(got) != 1 || got[0] != wantAlone {
			t.Errorf("lym calls = %q; want the one call %q", got, wantAlone)
		}
		if runs := len(lines(t, fullRuns)); runs != 1 {
			t.Errorf("the full command ran %d times; want once, on the first arrival", runs)
		}
	}) {
		return
	}

	if !t.Run("tests that pass alone hand the arrival to the full command", func(t *testing.T) {
		if err := os.Remove(aloneReport); err != nil {
			t.Fatal(err)
		}
		fullExit = "0"
		got, err := gate()
		if err != nil || !got.Passed {
			t.Fatalf("gate() = %+v, %v; want a pass", got, err)
		}
		if got := lines(t, calls); len(got) != 2 || got[1] != wantAlone {
			t.Errorf("lym calls = %q; want a second call %q", got, wantAlone)
		}
		if runs := len(lines(t, fullRuns)); runs != 2 {
			t.Errorf("the full command ran %d times; want twice", runs)
		}
	}) {
		return
	}

	outsideAnyTest := gateslot.FailureReportHeader + "\ntier0 (exit 1), log /runs/a.log:\n  example.com/m/a, failed outside any test:\n    a.go:3: undefined: x\n"

	if !t.Run("a failed run that names no failed test keeps nothing, so the next arrival runs the full command at once", func(t *testing.T) {
		write(t, fullReport, outsideAnyTest)
		fullExit = "2"
		if got, err := gate(); err != nil || got.Passed {
			t.Fatalf("gate() = %+v, %v; want a failure", got, err)
		}
		fullExit = "3"
		if got, err := gate(); err != nil || got.Passed {
			t.Fatalf("gate() = %+v, %v; want a failure", got, err)
		}
		if got := lines(t, calls); len(got) != 2 {
			t.Errorf("lym calls = %q; want no further call", got)
		}
		if runs := len(lines(t, fullRuns)); runs != 4 {
			t.Errorf("the full command ran %d times; want four", runs)
		}
	}) {
		return
	}

	// keepTestA commits a new file, so no pass recorded on an earlier tree skips a run.
	// It then fails the full command naming TestA.
	// The next arrival then runs TestA alone first.
	commits := 0
	keepTestA := func(t *testing.T) {
		t.Helper()
		commits++
		gitkit.CommitFile(t, worktree, fmt.Sprintf("kept-%d.txt", commits), "x\n", "a new tree")
		write(t, fullReport, namesTestA)
		fullExit = "1"
		if got, err := gate(); err != nil || got.Passed {
			t.Fatalf("gate() = %+v, %v; want a failure naming TestA", got, err)
		}
	}

	for _, tt := range []struct{ name, aloneReport string }{
		{"times out", gateslot.TierTimeoutLine("tier1", time.Minute) + "\n" + namesTestA},
		{"crashes its test runner", gateslot.RunnerCrashLine("tier1 (exit 134, SIGABRT)") + "\n" + namesTestA},
		{"fails naming no failed test", outsideAnyTest},
	} {
		if !t.Run("a rerun of the failed tests alone that "+tt.name+" hands the arrival to the full command and keeps nothing", func(t *testing.T) {
			keepTestA(t)
			callsBefore, fullsBefore := len(lines(t, calls)), len(lines(t, fullRuns))
			write(t, aloneReport, tt.aloneReport)
			t.Cleanup(func() { _ = os.Remove(aloneReport) })
			write(t, fullReport, outsideAnyTest)
			fullExit = "2"

			if got, err := gate(); err != nil || got.Passed || strings.HasPrefix(got.Findings, failedAloneNote) {
				t.Fatalf("gate() = %+v, %v; want a failure from the full command", got, err)
			}
			if got := len(lines(t, calls)); got != callsBefore+1 {
				t.Errorf("lym calls = %d; want the one rerun on top of the %d before", got, callsBefore)
			}
			if got := len(lines(t, fullRuns)); got != fullsBefore+1 {
				t.Errorf("the full command ran %d times; want once on top of the %d before", got, fullsBefore)
			}

			if got, err := gate(); err != nil || got.Passed {
				t.Fatalf("gate() = %+v, %v; want a failure", got, err)
			}
			if got := len(lines(t, calls)); got != callsBefore+1 {
				t.Errorf("lym calls = %d; want no rerun on the next arrival", got)
			}
			if got := len(lines(t, fullRuns)); got != fullsBefore+2 {
				t.Errorf("the full command ran %d times; want the next arrival to run it at once", got)
			}
		}) {
			return
		}
	}

	t.Run("a rerun that finds the tree dirty ends the arrival with the dirty paths and keeps the tests for the next", func(t *testing.T) {
		keepTestA(t)
		callsBefore, fullsBefore := len(lines(t, calls)), len(lines(t, fullRuns))
		dirty := filepath.Join(worktree, "dirty.txt")
		write(t, dirty, "x\n")

		got, err := gate()
		if err != nil || got.Passed || !strings.Contains(got.Findings, "uncommitted changes") || !strings.Contains(got.Findings, "dirty.txt") {
			t.Fatalf("gate() = %+v, %v; want a failure naming the uncommitted dirty.txt", got, err)
		}
		if gotCalls, gotFulls := len(lines(t, calls)), len(lines(t, fullRuns)); gotCalls != callsBefore || gotFulls != fullsBefore {
			t.Errorf("lym calls %d and full runs %d; want %d and %d, since a dirty tree runs nothing", gotCalls, gotFulls, callsBefore, fullsBefore)
		}

		if err := os.Remove(dirty); err != nil {
			t.Fatal(err)
		}
		fullExit = "0"
		if got, err := gate(); err != nil || !got.Passed {
			t.Fatalf("gate() = %+v, %v; want a pass", got, err)
		}
		if got := len(lines(t, calls)); got != callsBefore+1 {
			t.Errorf("lym calls = %d; want the kept tests run alone once the tree is clean", got)
		}
	})
}
