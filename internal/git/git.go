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
	hideProcWindow(cmd) // no console window flash on Windows

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()

	// Extract exit code from ProcessState
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	} else {
		exitCode = -1
	}

	// Only return err for execution failures, not for non-zero exit codes
	if err != nil && cmd.ProcessState == nil {
		return "", "", -1, err
	}
	return outBuf.String(), errBuf.String(), exitCode, nil
}
