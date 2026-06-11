// git.go — low-level git command execution.
//
// RunGit executes git commands and returns their output and exit code.

package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// RunGit runs a git command and returns stdout, stderr, and exit code
func RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	hideProcWindow(cmd) // no console window flash on Windows

	err = cmd.Run()

	// Extract exit code from error, defaulting to 0 on success
	exitCode = 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
		err = nil // Non-zero exit is not an error condition
	} else if err != nil {
		// Non-ExitError failures return empty buffers and -1 exit code
		return "", "", -1, err
	}

	return outBuf.String(), errBuf.String(), exitCode, err
}

// FindRoot returns the root directory of a git repository.
//
// It calls git rev-parse --show-toplevel from the given cwd.
// Returns the repository root on success, or an empty string and an error on failure.
// If git command fails to start, the error is propagated.
// If git exits with non-zero code, an error including stderr is returned.
func FindRoot(cwd string) (string, error) {
	stdout, stderr, exitCode, err := RunGit([]string{"rev-parse", "--show-toplevel"}, cwd)
	if err != nil {
		// Process failed to start
		return "", err
	}

	if exitCode != 0 {
		// Git exited with error (e.g., not a git repo)
		return "", fmt.Errorf("git rev-parse failed: %s", stderr)
	}

	// Success
	return strings.TrimSpace(stdout), nil
}
