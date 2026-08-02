// gitexec.go — low-level git command execution.
//
// RunGit executes git commands and returns their output and exit code.

package gitexec

import (
	"bytes"
	"os/exec"

	"github.com/Knatte18/loomyard/internal/proc"
)

// RunGit runs a git command and returns stdout, stderr, and exit code.
func RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	proc.HideWindow(cmd)

	err = cmd.Run()

	exitCode = 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
		err = nil
	} else if err != nil {
		return "", "", -1, err
	}

	return outBuf.String(), errBuf.String(), exitCode, err
}
