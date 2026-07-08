// gate_test.go tables converged across all three GateMode values and their
// verdict/gatePassed combinations, exercises execGateCommand against real
// trivial commands (go's own toolchain — the one binary guaranteed present
// in this repo's test environment) for the pass, fail, and not-found paths,
// and checks writeGateOutput's file shape.

package perchengine

import (
	"os"
	"path/filepath"
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
// killed and reports a could-not-run error naming the timeout.
func TestExecGateCommand_Timeout(t *testing.T) {
	dir := t.TempDir()
	// "go version" reliably finishes well inside 30s but a 1-nanosecond
	// timeout guarantees the deadline fires before the process can even be
	// scheduled, without a platform-specific long-running command.
	_, _, err := execGateCommand([]string{"go", "version"}, dir, 1*time.Nanosecond)
	if err == nil {
		t.Fatalf("execGateCommand() error = nil; want a timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("execGateCommand() error = %q; want it to name the timeout", err.Error())
	}
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
