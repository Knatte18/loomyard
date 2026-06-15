// git.go — low-level git command execution.
//
// RunGit executes git commands and returns their output and exit code.

package git

import (
	"bytes"
	"os/exec"
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

