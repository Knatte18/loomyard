// gate.go implements the command-gate execution seam and the convergence
// check the round loop evaluates every round: execGateCommand is the
// production CommandRunner (the seam type defined in engine.go),
// writeGateOutput records a command gate's raw output for the operator and
// the next round's hydration, and converged evaluates GateMode against a
// round's runner verdict and (when the mode runs a command) its pass/fail
// result.

package treadleengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// gateWaitDelay bounds Wait's read after the command exits.
const gateWaitDelay = 10 * time.Second

// execGateCommand is the production CommandRunner. No shell is invoked.
// A timeout returns partial output with a timeout note.
func execGateCommand(argv []string, dir string, timeout time.Duration) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.WaitDelay = gateWaitDelay

	output, err := cmd.CombinedOutput()
	if err == nil {
		return output, true, nil
	}

	if ctx.Err() == context.DeadlineExceeded {
		note := fmt.Sprintf("\n(gate command timed out after %s and was killed)\n", timeout)
		return append(output, []byte(note)...), false, nil
	}

	if errors.Is(err, exec.ErrWaitDelay) {
		return output, cmd.ProcessState.Success(), nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return output, false, nil
	}

	return nil, false, fmt.Errorf("gate command %v failed to start: %w", argv, err)
}

// writeGateOutput writes path with a header and the combined output.
func writeGateOutput(name string, path string, argv []string, output []byte, exitZero bool) error {
	status := "FAIL"
	if exitZero {
		status = "PASS"
	}

	header := fmt.Sprintf("# Gate command\n\nCommand: %v\nStatus: %s\n\n", argv, status)
	content := append([]byte(header), output...)

	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("%s: write gate output %q: %w", name, path, err)
	}
	return nil
}

// converged reports whether a round has reached the block's convergence check.
func converged(mode GateMode, verdict Verdict, gatePassed *bool) bool {
	commandPassed := gatePassed != nil && *gatePassed
	switch mode {
	case GateLLMVerdict:
		return verdict == VerdictApproved
	case GateCommand:
		return commandPassed
	case GateBoth:
		return verdict == VerdictApproved && commandPassed
	default:
		return false
	}
}
