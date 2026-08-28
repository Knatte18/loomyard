//go:build integration

// runverify_test.go exercises runVerifyCommand's two teardown outcomes -- a non-zero exit (a failed
// verify, which is expected) and a spawn failure (a genuine error) -- asserting that they log
// differently, per card 6 of the spawn-site-log-lines batch.
// It carries the integration tag because it spawns real processes and reuses the package's hermetic
// TestMain (testmain_test.go) for free.

package websterengine

import (
	"bytes"
	"os"
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

// TestRunVerifyCommand covers the non-zero-exit path (expect false, nil and a captured INFO
// teardown line carrying exitCode) and the spawn-failure path (expect a non-nil error and a
// captured WARN line carrying cause), asserting the two paths log differently.
func TestRunVerifyCommand(t *testing.T) {
	t.Run("NonZeroExit", func(t *testing.T) {
		buf := runverifyCapture(t)

		passed, err := runVerifyCommand("exit 1", t.TempDir())
		if err != nil {
			t.Fatalf("runVerifyCommand(exit 1) error = %v; want nil", err)
		}
		if passed {
			t.Fatalf("runVerifyCommand(exit 1) passed = true; want false")
		}

		out := buf.String()
		if !strings.Contains(out, "INFO") {
			t.Errorf("runVerifyCommand(exit 1) output = %q; want an INFO line", out)
		}
		if !strings.Contains(out, "exitCode") {
			t.Errorf("runVerifyCommand(exit 1) output = %q; want an exitCode key", out)
		}
		if strings.Contains(out, "WARN") {
			t.Errorf("runVerifyCommand(exit 1) output = %q; want no WARN line", out)
		}
	})

	t.Run("SpawnFailure", func(t *testing.T) {
		buf := runverifyCapture(t)

		// Clearing PATH makes the shell binary itself unresolvable, so cmd.Run()
		// fails to start the process at all -- a genuine spawn failure, distinct
		// from the shell itself running and reporting a missing command via a
		// non-zero exit (which is the NonZeroExit subtest's *exec.ExitError case).
		t.Setenv("PATH", "")

		_, err := runVerifyCommand("does-not-exist-binary-lyx-test", t.TempDir())
		if err == nil {
			t.Fatalf("runVerifyCommand(missing binary) error = nil; want non-nil")
		}

		out := buf.String()
		if !strings.Contains(out, "WARN") {
			t.Errorf("runVerifyCommand(missing binary) output = %q; want a WARN line", out)
		}
		if !strings.Contains(out, "cause") {
			t.Errorf("runVerifyCommand(missing binary) output = %q; want a cause key", out)
		}
	})
}
