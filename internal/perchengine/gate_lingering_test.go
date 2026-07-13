//go:build integration

// gate_lingering_test.go holds the one perchengine test that spawns real
// cmd/ping child processes and, by design, sits in the production
// gateWaitDelay (10s) pipe-abandon grace window — real-time cost that
// violates the offline Tier 1 loop's premise (see the Test Tier Purity
// Invariant in CONSTRAINTS.md), so it is tagged integration rather than
// running on every plain `go test`. It previously evaded the tierpurity
// guard because it spawns via the production execGateCommand wrapper rather
// than a banned token (gitexec.RunGit, exec.Command, lyxtest.Copy) the guard
// greps for.

package perchengine

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestExecGateCommand_LingeringChildDoesNotHangPastWaitDelay proves the
// gate call's lifetime is bounded even when the command leaves a child
// holding the combined-output pipe — the exact shape of real gate commands
// (go test's test binaries, build workers, a server the round's own fix
// started). Without cmd.WaitDelay, Wait blocks until every pipe holder
// exits: the pass case would stall for the child's full ~19s lifetime with
// the deadline never firing at all, and the timeout case would blow far
// past its 2s deadline the same way — both observed before the fix. The
// child outliving the assertions is reaped by the OS on its own (a plain
// ping); the test only asserts the GATE returned without waiting for it.
// Windows-only: the child-spawning idiom (cmd's start /b) has no portable
// equivalent, and this repo's substrate is Windows.
func TestExecGateCommand_LingeringChildDoesNotHangPastWaitDelay(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("uses cmd.exe's start /b to spawn a pipe-holding child")
	}

	// Both subtests spend most of their time inside gateWaitDelay; run them
	// in parallel so the suite pays for the grace window once, not twice.
	t.Run("command exits zero, child still holds the pipe", func(t *testing.T) {
		t.Parallel()
		start := time.Now()
		// The parent exits 0 immediately; the started ping (~19s) inherits
		// and holds the output pipe. The command dir is the SYSTEM temp dir,
		// not t.TempDir(): the lingering child keeps the dir as its cwd past
		// the test's end, which would fail t.TempDir's RemoveAll cleanup.
		_, exitZero, err := execGateCommand(
			[]string{"cmd", "/c", "start /b ping -n 20 127.0.0.1"},
			os.TempDir(), 30*time.Second)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("execGateCommand() error = %v; want nil", err)
		}
		if !exitZero {
			t.Errorf("execGateCommand() exitZero = false; want true — the command itself exited zero")
		}
		if elapsed > 15*time.Second {
			t.Errorf("execGateCommand() returned after %s; want within the ~10s pipe-abandon grace, not the child's ~19s lifetime", elapsed.Round(time.Millisecond))
		}
	})

	t.Run("command killed at the deadline, child still holds the pipe", func(t *testing.T) {
		t.Parallel()
		start := time.Now()
		// The parent runs a ~19s foreground ping (killed at the 2s
		// deadline) after starting a background ping that keeps the pipe
		// open past the kill. System temp dir for the same cwd-outliving
		// reason as the pass-case subtest above.
		output, exitZero, err := execGateCommand(
			[]string{"cmd", "/c", "start /b ping -n 20 127.0.0.1 & ping -n 20 127.0.0.1 > nul"},
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
		if elapsed > 15*time.Second {
			t.Errorf("execGateCommand() returned after %s; want within deadline + the ~10s pipe-abandon grace, not the child's ~19s lifetime", elapsed.Round(time.Millisecond))
		}
	})
}
