// gate_test.go tables converged across all three GateMode values and their
// verdict/gatePassed combinations, exercises execGateCommand against real
// trivial commands (go's own toolchain — the one binary guaranteed present
// in this repo's test environment) for the pass, fail, and not-found paths,
// and checks writeGateOutput's file shape.

package perchengine

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
)

func boolPtr(b bool) *bool {
	return &b
}

// TestConverged tables every GateMode against every verdict/gatePassed
// combination the loop can hand it, including the nil-gatePassed case a
// GateLLMVerdict round always produces.
func TestConverged(t *testing.T) {
	tests := []struct {
		name       string
		mode       GateMode
		verdict    burlerengine.Verdict
		gatePassed *bool
		want       bool
	}{
		{"llm-verdict approved converges regardless of nil gatePassed", GateLLMVerdict, burlerengine.VerdictApproved, nil, true},
		{"llm-verdict blocking never converges", GateLLMVerdict, burlerengine.VerdictBlocking, nil, false},
		{"command mode ignores an approved verdict with failing command", GateCommand, burlerengine.VerdictApproved, boolPtr(false), false},
		{"command mode converges on a passing command despite blocking verdict", GateCommand, burlerengine.VerdictBlocking, boolPtr(true), true},
		{"command mode with nil gatePassed never converges", GateCommand, burlerengine.VerdictApproved, nil, false},
		{"both requires approved and passing", GateBoth, burlerengine.VerdictApproved, boolPtr(true), true},
		{"both fails when verdict is blocking despite a passing command", GateBoth, burlerengine.VerdictBlocking, boolPtr(true), false},
		{"both fails when command fails despite an approved verdict", GateBoth, burlerengine.VerdictApproved, boolPtr(false), false},
		{"both fails when gatePassed is nil", GateBoth, burlerengine.VerdictApproved, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := converged(tt.mode, tt.verdict, tt.gatePassed)
			if got != tt.want {
				t.Errorf("converged(%q, %q, %v) = %v; want %v", tt.mode, tt.verdict, tt.gatePassed, got, tt.want)
			}
		})
	}
}

// TestExecGateCommand_Pass proves a zero-exit command reports exitZero
// true, a nil error, and non-empty combined output.
func TestExecGateCommand_Pass(t *testing.T) {
	dir := t.TempDir()
	output, exitZero, err := execGateCommand([]string{"go", "version"}, dir, 30*time.Second)
	if err != nil {
		t.Fatalf("execGateCommand() error = %v; want nil", err)
	}
	if !exitZero {
		t.Errorf("execGateCommand() exitZero = false; want true")
	}
	if len(output) == 0 {
		t.Errorf("execGateCommand() output is empty; want go version's banner")
	}
}

// TestExecGateCommand_Fail proves a non-zero-exit command reports exitZero
// false with a nil error (a normal gate failure, not a could-not-run
// failure) and still carries the command's output.
func TestExecGateCommand_Fail(t *testing.T) {
	dir := t.TempDir()
	output, exitZero, err := execGateCommand([]string{"go", "bogus-subcommand"}, dir, 30*time.Second)
	if err != nil {
		t.Fatalf("execGateCommand() error = %v; want nil for a non-zero exit", err)
	}
	if exitZero {
		t.Errorf("execGateCommand() exitZero = true; want false")
	}
	if len(output) == 0 {
		t.Errorf("execGateCommand() output is empty; want go's unknown-subcommand message")
	}
}

// TestExecGateCommand_NotFound proves a command that cannot even start
// (unknown binary) reports a non-nil error, distinguishing a could-not-run
// failure from an ordinary non-zero exit.
func TestExecGateCommand_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, exitZero, err := execGateCommand([]string{"perch-gate-command-does-not-exist-xyz"}, dir, 30*time.Second)
	if err == nil {
		t.Fatalf("execGateCommand() error = nil; want a could-not-run error")
	}
	if exitZero {
		t.Errorf("execGateCommand() exitZero = true; want false")
	}
}

// TestExecGateCommand_Timeout proves a command that outlives timeout is
// killed and reported as an ORDINARY failing gate — never an error — whose
// output carries a note naming the timeout, so the failure feeds forward
// into the next round's hydration like any other failing gate.
func TestExecGateCommand_Timeout(t *testing.T) {
	dir := t.TempDir()
	// "go version" reliably finishes well inside 30s but a 1-nanosecond
	// timeout guarantees the deadline fires before the process can even be
	// scheduled, without a platform-specific long-running command.
	output, exitZero, err := execGateCommand([]string{"go", "version"}, dir, 1*time.Nanosecond)
	if err != nil {
		t.Fatalf("execGateCommand() error = %v; want nil — a timeout is a failing gate, not an infrastructure error", err)
	}
	if exitZero {
		t.Errorf("execGateCommand() exitZero = true; want false")
	}
	if !strings.Contains(string(output), "timed out after") {
		t.Errorf("execGateCommand() output = %q; want it to carry the timeout note", output)
	}
}

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

// TestWriteGateOutput proves the written file's header carries the argv and
// pass/fail status, followed by the raw output.
func TestWriteGateOutput(t *testing.T) {
	tests := []struct {
		name       string
		exitZero   bool
		wantStatus string
	}{
		{"pass", true, "PASS"},
		{"fail", false, "FAIL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "round-1-gate.md")
			argv := []string{"make", "test"}
			output := []byte("some command output\n")

			if err := writeGateOutput(path, argv, output, tt.exitZero); err != nil {
				t.Fatalf("writeGateOutput() error = %v; want nil", err)
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile(%q) = %v; want nil", path, err)
			}
			gotStr := string(got)
			if !strings.Contains(gotStr, tt.wantStatus) {
				t.Errorf("writeGateOutput() content = %q; want it to contain status %q", gotStr, tt.wantStatus)
			}
			if !strings.Contains(gotStr, "make test") {
				t.Errorf("writeGateOutput() content = %q; want it to name the argv", gotStr)
			}
			if !strings.Contains(gotStr, "some command output") {
				t.Errorf("writeGateOutput() content = %q; want it to carry the raw output", gotStr)
			}
		})
	}
}
