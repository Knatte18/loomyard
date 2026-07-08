// gate.go implements the command-gate execution seam and the convergence
// check the round loop evaluates every round: execGateCommand is the
// production CommandRunner (card 10's type), writeGateOutput records a
// command gate's raw output for the operator and the next round's
// hydration, and converged evaluates GateMode against a round's burler
// verdict and (when the mode runs a command) its pass/fail result.

package perchengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
)

// execGateCommand is the production CommandRunner: it runs argv[0] with
// argv[1:] as arguments inside dir, killing the command if it runs longer
// than timeout, and returns its combined stdout+stderr output plus whether
// it exited zero. No shell is invoked — argv is executed directly via
// exec.CommandContext, which is portable (no per-OS shell to pick) and
// quoting-safe (no shell metacharacter interpretation). A non-zero exit is
// reported as (output, false, nil): an ordinary gate failure the loop
// branches on, never an error. err is reserved for could-not-run failures:
// the timeout killing the command before it exited, or the command failing
// to even start (not found, permission denied, ...).
func execGateCommand(argv []string, dir string, timeout time.Duration) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err == nil {
		return output, true, nil
	}

	// A context deadline killed the process before it could exit; this is a
	// could-not-run failure (the gate never got to report pass/fail), not an
	// ordinary non-zero exit.
	if ctx.Err() == context.DeadlineExceeded {
		return nil, false, fmt.Errorf("perch: gate command %v timed out after %s", argv, timeout)
	}

	// exec.ExitError means the process started and ran to completion, just
	// with a non-zero exit code — an expected, reportable gate failure.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return output, false, nil
	}

	// Any other error means the process never started at all (binary not
	// found, permission denied, ...) — a could-not-run failure the caller
	// must treat as a hard error, since there is no gate result to report.
	return nil, false, fmt.Errorf("perch: gate command %v failed to start: %w", argv, err)
}

// writeGateOutput writes path with a small header naming argv and whether
// the run passed, followed by the raw combined output. This file is what
// the next round's burler hydration and the operator read after a command
// gate runs; per the pluggable-gate decision it is written on both pass and
// fail (the record is cheap either way), even though only a FAILED gate
// file is ever fed forward into a later round's hydration.
func writeGateOutput(path string, argv []string, output []byte, exitZero bool) error {
	status := "FAIL"
	if exitZero {
		status = "PASS"
	}

	header := fmt.Sprintf("# Gate command\n\nCommand: %v\nStatus: %s\n\n", argv, status)
	content := append([]byte(header), output...)

	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("perch: write gate output %q: %w", path, err)
	}
	return nil
}

// converged reports whether a round has reached the block's convergence
// check under mode: GateLLMVerdict trusts the fresh burler verdict alone;
// GateCommand trusts gatePassed alone (the burler verdict still drives what
// the fix phase changes, but never decides convergence in this mode);
// GateBoth requires both signals to agree the round is clean. gatePassed is
// nil for a round whose mode never ran a command (GateLLMVerdict), in which
// case GateCommand/GateBoth's command check contributes false.
func converged(mode GateMode, verdict burlerengine.Verdict, gatePassed *bool) bool {
	commandPassed := gatePassed != nil && *gatePassed
	switch mode {
	case GateLLMVerdict:
		return verdict == burlerengine.VerdictApproved
	case GateCommand:
		return commandPassed
	case GateBoth:
		return verdict == burlerengine.VerdictApproved && commandPassed
	default:
		return false
	}
}
