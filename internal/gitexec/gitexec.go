// gitexec.go — low-level git command execution.
//
// RunGit executes git commands and returns their output and exit code.

package gitexec

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Knatte18/loomyard/internal/proc"
)

// GitError reports that a git command ran and exited non-zero.
// It carries exactly what every merged failure message needs: the command
// that was run, the directory it ran in, its exit code, and its stderr.
// Args are rendered verbatim, with no redaction — callers must not pass
// credentials in args.
type GitError struct {
	Args     []string
	Dir      string
	ExitCode int
	Stderr   string
}

// Error renders "git <args>: exit <code>", followed by ": <trimmed stderr>"
// only when the trimmed stderr is non-empty.
// Each arg is rendered bare, except an arg that is empty or contains a
// space, tab, or newline, which is rendered %q-quoted so the boundary
// between adjacent args stays legible.
func (e *GitError) Error() string {
	rendered := make([]string, len(e.Args))
	for i, arg := range e.Args {
		rendered[i] = renderArg(arg)
	}

	msg := fmt.Sprintf("git %s: exit %d", strings.Join(rendered, " "), e.ExitCode)

	if stderr := strings.TrimSpace(e.Stderr); stderr != "" {
		msg += ": " + stderr
	}

	return msg
}

// renderArg renders a single git argument for GitError.Error, quoting it
// with %q when it is empty or contains whitespace that would otherwise blur
// the boundary with an adjacent argument.
func renderArg(arg string) string {
	if arg == "" || strings.ContainsAny(arg, " \t\n") {
		return fmt.Sprintf("%q", arg)
	}
	return arg
}

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
