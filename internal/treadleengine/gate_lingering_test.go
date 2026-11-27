//go:build windows

// gate_lingering_test.go holds the one treadleengine test that spawns real cmd/ping child processes.
// The test sits in the production gateWaitDelay (10s) pipe-abandon grace window, which costs real time.
// It is tagged integration rather than run on every plain `go test`:
// the offline Tier 1 loop cannot afford that cost (see PATTERN-test-speed).

package treadleengine

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// The gate call's lifetime is bounded when a child holds the output pipe.
// Windows-only: the child-spawning idiom (cmd's start /b) has no portable equivalent.
func TestExecGateCommand_LingeringChildDoesNotHangPastWaitDelay(t *testing.T) {
	tiergate.Require(t, 1)
	// Both subtests spend most of their time inside gateWaitDelay.
	// They run in parallel, so the suite pays for the grace window once, not twice.
	t.Run("command exits zero, child still holds the pipe", func(t *testing.T) {
		t.Parallel()
		start := time.Now()
		// The parent exits 0 at once, but the started ping (about two minutes) inherits and holds the output pipe.
		// It outlasts the test's one-minute bound, so a loaded machine cannot pass a call that waits on it.
		// The command dir is the system temp dir, not t.TempDir():
		// the lingering child keeps that dir as its cwd past the test's end, and t.TempDir's RemoveAll cleanup would fail.
		_, exitZero, err := execGateCommand(
			[]string{"cmd", "/c", "start /b ping -n 120 127.0.0.1"},
			os.TempDir(), 30*time.Second)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("execGateCommand() error = %v; want nil", err)
		}
		if !exitZero {
			t.Errorf("execGateCommand() exitZero = false; want true — the command itself exited zero")
		}
		if elapsed > time.Minute {
			t.Errorf("execGateCommand() returned after %s; want within the ~10s pipe-abandon grace, not the child's two-minute lifetime", elapsed.Round(time.Millisecond))
		}
	})

	t.Run("command killed at the deadline, child still holds the pipe", func(t *testing.T) {
		t.Parallel()
		start := time.Now()
		// The parent runs a two-minute foreground ping, killed at the 2s deadline.
		// It starts a background ping first, which keeps the pipe open past the kill and past the test's one-minute bound.
		// The system temp dir is used because the lingering child keeps it as its cwd past the test's end.
		output, exitZero, err := execGateCommand(
			[]string{"cmd", "/c", "start /b ping -n 120 127.0.0.1 & ping -n 120 127.0.0.1 > nul"},
			os.TempDir(), 2*time.Second)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("execGateCommand() error = %v; want nil — a timeout is a failing gate, not an infrastructure error", err)
		}
		if exitZero {
			t.Errorf("execGateCommand() exitZero = true; want false")
		}
		if !strings.Contains(string(output), "timed out after") {
			t.Errorf("execGateCommand() output = %q; want it to carry the timeout note", output)
		}
		if elapsed > time.Minute {
			t.Errorf("execGateCommand() returned after %s; want within deadline + the ~10s pipe-abandon grace, not the child's two-minute lifetime", elapsed.Round(time.Millisecond))
		}
	})
}
