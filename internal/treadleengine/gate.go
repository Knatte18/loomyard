// gate.go implements the command-gate execution seam and the convergence
// check the round loop evaluates every round: execGateCommand is the
// production CommandRunner (the seam type defined in engine.go),
// writeGateOutput records a
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

// gateWaitDelay is how long execGateCommand's Wait may keep reading the
// combined-output pipe AFTER the gate command itself has exited (or been
// killed at the deadline). Without it, Wait blocks until EVERY process
// holding the pipe exits — and real gate commands routinely leave one
// behind (go test's test binaries, build workers, a server the round's own
// fix started), which would stall the gate long past Gate.Timeout, or
// forever, defeating the timeout's bound on the loop (proven by
// experiment: a 2-second timeout observed returning after 29 seconds while
// a spawned child held the pipe).
const gateWaitDelay = 10 * time.Second

// execGateCommand is the production CommandRunner: it runs argv[0] with
// argv[1:] as arguments inside dir, killing the command if it runs longer
// than timeout, and returns its combined stdout+stderr output plus whether
// it exited zero. No shell is invoked — argv is executed directly via
// exec.CommandContext, which is portable (no per-OS shell to pick) and
// quoting-safe (no shell metacharacter interpretation). A non-zero exit is
// reported as (output, false, nil): an ordinary gate failure the loop
// branches on, never an error. A TIMEOUT is a failing gate too, not an
// error — a command that hangs is most plausibly hung BECAUSE of the
// round's own fix (a deadlocked test suite), which is exactly the artifact
// signal the gate exists to report; the partial output plus a timeout note
// is returned so the failure feeds forward like any other. err is reserved
// for could-not-start failures only (binary not found, permission denied):
// there the gate could never observe the artifact at all.
//
// The command's own lifetime is bounded by timeout; the CALL's lifetime is
// bounded by timeout plus gateWaitDelay, after which any output pipe still
// held open by a child the command spawned is abandoned (the exit status
// is already known by then). A killed command's orphaned grandchildren may
// outlive the gate — Windows offers no process-group kill here — but they
// can no longer hang the loop.
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

	// The deadline killed the process before it could exit: report it as an
	// ordinary FAILING gate carrying whatever output the command produced
	// plus a note naming the timeout, so the next round's hydration starts
	// knowing the command hung rather than what exit code it never reached.
	if ctx.Err() == context.DeadlineExceeded {
		note := fmt.Sprintf("\n(gate command timed out after %s and was killed)\n", timeout)
		return append(output, []byte(note)...), false, nil
	}

	// Wait abandoned the output pipe gateWaitDelay after the command
	// exited, because a child the command spawned still holds it open.
	// ErrWaitDelay is only returned when the command itself otherwise
	// looked successful (a failure exit surfaces as ExitError below even
	// with held pipes), so classify pass/fail from the recorded exit
	// status exactly as if the pipe had closed normally.
	if errors.Is(err, exec.ErrWaitDelay) {
		return output, cmd.ProcessState.Success(), nil
	}

	// exec.ExitError means the process started and ran to completion, just
	// with a non-zero exit code — an expected, reportable gate failure.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return output, false, nil
	}

	// Any other error means the process never started at all (binary not
	// found, permission denied, ...) — a could-not-start failure the caller
	// must treat as a hard error, since the gate never observed the
	// artifact. Un-prefixed: the loop wraps it with its own "perch: " round
	// context.
	return nil, false, fmt.Errorf("gate command %v failed to start: %w", argv, err)
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
