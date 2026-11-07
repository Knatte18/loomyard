// gate.go holds the command-gate execution and the convergence check the round loop evaluates every round.
// execGateCommand runs a gate command.
// writeGateOutput records its raw output for the operator and the next round's hydration.
// converged decides whether a round has converged under its GateMode.

package treadleengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// gateWaitDelay bounds Wait's read after the command exits.
const gateWaitDelay = 10 * time.Second

// execGateCommand runs argv inside dir and kills it after timeout.
// No shell is invoked.
// A timeout returns the partial output with a timeout note, and reports a failed gate rather than an error.
func execGateCommand(argv []string, dir string, timeout time.Duration) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.WaitDelay = gateWaitDelay

	logger.Info("treadleengine: spawning gate command", "argv", argv, "dir", dir, "timeout", timeout)
	start := time.Now()
	output, err := cmd.CombinedOutput()
	if err == nil {
		logger.Info("treadleengine: gate command exited", "argv", argv, "dir", dir, "exitZero", true, "durationMs", time.Since(start).Milliseconds())
		return output, true, nil
	}

	if ctx.Err() == context.DeadlineExceeded {
		logger.Info("treadleengine: gate command exited", "argv", argv, "dir", dir, "exitZero", false, "durationMs", time.Since(start).Milliseconds(), "timedOut", true)
		note := fmt.Sprintf("\n(gate command timed out after %s and was killed)\n", timeout)
		return append(output, []byte(note)...), false, nil
	}

	if errors.Is(err, exec.ErrWaitDelay) {
		exitZero := cmd.ProcessState.Success()
		logger.Info("treadleengine: gate command exited", "argv", argv, "dir", dir, "exitZero", exitZero, "durationMs", time.Since(start).Milliseconds())
		return output, exitZero, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		logger.Info("treadleengine: gate command exited", "argv", argv, "dir", dir, "exitZero", false, "durationMs", time.Since(start).Milliseconds())
		return output, false, nil
	}

	logger.Warn("treadleengine: gate command failed to start", "argv", argv, "dir", dir, "durationMs", time.Since(start).Milliseconds(), "cause", err)
	return nil, false, fmt.Errorf("gate command %v failed to start: %w", argv, err)
}

// writeGateOutput writes the command's combined output to path.
// The file starts with a header that names the command and its PASS or FAIL status.
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

// converged reports whether a round has converged under mode, given its runner verdict and gate result.
// GateLLMVerdict needs an approved verdict, GateCommand needs a passing command, and GateBoth needs both.
// A nil gatePassed counts as a failed command, and an unknown mode never converges.
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
