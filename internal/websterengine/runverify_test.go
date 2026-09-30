//go:build integration

// runverify_test.go exercises runVerifyCapture's outcomes -- a non-zero exit (a failed
// verify, which is expected), a spawn failure (a genuine error), and output capture with the optional
// log file -- asserting that the two teardown paths log differently.
// It carries the integration tag because it spawns real processes and reuses the package's hermetic
// TestMain (testmain_test.go) for free.

package websterengine

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
)

// runverifyCapture redirects logger output into a buffer for the duration of one test at Info
// verbosity, restoring both the output sink and the default verbosity via t.Cleanup.
func runverifyCapture(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	logger.SetVerbosity(1)
	t.Cleanup(func() {
		logger.SetOutput(os.Stderr)
		logger.SetVerbosity(0)
	})
	return &buf
}

// TestRunVerifyCapture covers the non-zero-exit path (expect Passed false, nil error and a captured
// INFO teardown line carrying exitCode), the spawn-failure path (expect a non-nil error and a
// captured WARN line carrying cause), and output capture with and without a log path.
func TestRunVerifyCapture(t *testing.T) {
	t.Run("NonZeroExit", func(t *testing.T) {
		buf := runverifyCapture(t)

		run, err := runVerifyCapture("exit 1", t.TempDir(), "")
		if err != nil {
			t.Fatalf("runVerifyCapture(exit 1) error = %v; want nil", err)
		}
		if run.Passed {
			t.Fatalf("runVerifyCapture(exit 1) Passed = true; want false")
		}

		out := buf.String()
		if !strings.Contains(out, "INFO") {
			t.Errorf("runVerifyCapture(exit 1) output = %q; want an INFO line", out)
		}
		if !strings.Contains(out, "exitCode") {
			t.Errorf("runVerifyCapture(exit 1) output = %q; want an exitCode key", out)
		}
		if strings.Contains(out, "WARN") {
			t.Errorf("runVerifyCapture(exit 1) output = %q; want no WARN line", out)
		}
	})

	t.Run("SpawnFailure", func(t *testing.T) {
		buf := runverifyCapture(t)

		// Clearing PATH makes the shell binary itself unresolvable, so cmd.Run()
		// fails to start the process at all -- a genuine spawn failure, distinct
		// from the shell itself running and reporting a missing command via a
		// non-zero exit (which is the NonZeroExit subtest's *exec.ExitError case).
		t.Setenv("PATH", "")

		_, err := runVerifyCapture("does-not-exist-binary-lyx-test", t.TempDir(), "")
		if err == nil {
			t.Fatalf("runVerifyCapture(missing binary) error = nil; want non-nil")
		}

		out := buf.String()
		if !strings.Contains(out, "WARN") {
			t.Errorf("runVerifyCapture(missing binary) output = %q; want a WARN line", out)
		}
		if !strings.Contains(out, "cause") {
			t.Errorf("runVerifyCapture(missing binary) output = %q; want a cause key", out)
		}
	})

	t.Run("FailingCommandCapturesOutputAndLog", func(t *testing.T) {
		runverifyCapture(t)
		logPath := filepath.Join(t.TempDir(), "nested", "verify.log")

		run, err := runVerifyCapture("echo to-stdout; echo to-stderr 1>&2; exit 1", t.TempDir(), logPath)
		if err != nil {
			t.Fatalf("runVerifyCapture error = %v; want nil", err)
		}
		if run.Passed {
			t.Fatalf("runVerifyCapture Passed = true; want false")
		}
		for _, want := range []string{"to-stdout", "to-stderr"} {
			if !strings.Contains(run.Output, want) {
				t.Errorf("Output = %q; want it to contain %q", run.Output, want)
			}
		}

		logged, readErr := os.ReadFile(logPath)
		if readErr != nil {
			t.Fatalf("read log %s: %v", logPath, readErr)
		}
		if string(logged) != run.Output {
			t.Errorf("log file = %q; want the same bytes as Output %q", logged, run.Output)
		}
	})

	t.Run("PassingCommandEmptyLogPathWritesNoFile", func(t *testing.T) {
		runverifyCapture(t)
		worktree := t.TempDir()

		run, err := runVerifyCapture("echo ok", worktree, "")
		if err != nil {
			t.Fatalf("runVerifyCapture error = %v; want nil", err)
		}
		if !run.Passed {
			t.Fatalf("runVerifyCapture Passed = false; want true")
		}
		entries, readErr := os.ReadDir(worktree)
		if readErr != nil {
			t.Fatalf("read dir: %v", readErr)
		}
		if len(entries) != 0 {
			t.Errorf("worktree entries = %d; want no file written for an empty logPath", len(entries))
		}
	})
}
