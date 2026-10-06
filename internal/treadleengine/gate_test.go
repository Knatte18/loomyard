// gate_test.go tables converged across all three GateMode values and their verdict/gatePassed
// combinations, exercises execGateCommand against real trivial commands (go's own toolchain — the
// one binary guaranteed present in this repo's test environment) for the pass, fail, timeout, and
// not-found paths, and checks writeGateOutput's file shape.
// The one real-time execGateCommand case (a lingering child holding the output pipe past
// gateWaitDelay) lives in gate_lingering_test.go under the integration build tag instead — see the
// PATTERN-test-speed.

package treadleengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func boolPtr(b bool) *bool {
	return &b
}

// TestConverged tables every GateMode against every verdict/gatePassed combination.
func TestConverged(t *testing.T) {
	tests := []struct {
		name       string
		mode       GateMode
		verdict    Verdict
		gatePassed *bool
		want       bool
	}{
		{"llm-verdict approved converges regardless of nil gatePassed", GateLLMVerdict, VerdictApproved, nil, true},
		{"llm-verdict blocking never converges", GateLLMVerdict, VerdictBlocking, nil, false},
		{"command mode ignores an approved verdict with failing command", GateCommand, VerdictApproved, boolPtr(false), false},
		{"command mode converges on a passing command despite blocking verdict", GateCommand, VerdictBlocking, boolPtr(true), true},
		{"command mode with nil gatePassed never converges", GateCommand, VerdictApproved, nil, false},
		{"both requires approved and passing", GateBoth, VerdictApproved, boolPtr(true), true},
		{"both fails when verdict is blocking despite a passing command", GateBoth, VerdictBlocking, boolPtr(true), false},
		{"both fails when command fails despite an approved verdict", GateBoth, VerdictApproved, boolPtr(false), false},
		{"both fails when gatePassed is nil", GateBoth, VerdictApproved, nil, false},
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

// TestExecGateCommand table-drives execGateCommand over a zero-exit command (success), a non-zero
// exit (failure, not an error), a command that cannot run (an error) and a timed-out command (a
// failing gate carrying a timeout note, not an infrastructure error).
// "go version" reliably finishes well inside 30s but a 1-nanosecond timeout guarantees the deadline
// fires before the process can even be scheduled, without a platform-specific long-running command.
func TestExecGateCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		argv         []string
		timeout      time.Duration
		wantErr      bool
		wantExitZero bool
		wantOutput   string
	}{
		{name: "zero exit", argv: []string{"go", "version"}, timeout: 30 * time.Second, wantExitZero: true, wantOutput: "go version"},
		{name: "non-zero exit", argv: []string{"go", "bogus-subcommand"}, timeout: 30 * time.Second, wantOutput: "unknown command"},
		{name: "not found", argv: []string{"treadle-gate-command-does-not-exist-xyz"}, timeout: 30 * time.Second, wantErr: true},
		{name: "timeout", argv: []string{"go", "version"}, timeout: time.Nanosecond, wantOutput: "timed out after"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			output, exitZero, err := execGateCommand(tt.argv, t.TempDir(), tt.timeout)

			if (err != nil) != tt.wantErr {
				t.Fatalf("execGateCommand() error = %v; want error: %v", err, tt.wantErr)
			}
			if exitZero != tt.wantExitZero {
				t.Errorf("execGateCommand() exitZero = %v; want %v", exitZero, tt.wantExitZero)
			}
			if !strings.Contains(string(output), tt.wantOutput) {
				t.Errorf("execGateCommand() output = %q; want it to contain %q", output, tt.wantOutput)
			}
		})
	}
}

// TestWriteGateOutput proves the written file's format.
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

			if err := writeGateOutput("gate", path, argv, output, tt.exitZero); err != nil {
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
