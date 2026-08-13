// gitexec.go implements the package's two-shape entry-point split.
// Run is the checked default: it treats a non-zero git exit as a failure,
// returning *GitError.
// RunGit is the raw form, for the sites where a non-zero exit is an answer
// rather than a failure — it stays permanently correct there rather than
// becoming a "legacy" wrapper.
// Both are thin wrappers over one unexported exec core, runCore.
//
// An exec-level failure — git could not be run at all — is returned
// unwrapped from Run, never as a *GitError, so errors.As(err, &gitErr)
// means precisely "git ran and rejected this", not "something went wrong".
// Run's stdout is returned in every case where git actually ran, including
// alongside a *GitError; it is empty on an exec-level failure only because
// git never ran.
//
// Args passed to either form are rendered verbatim into any resulting
// *GitError, with no redaction — callers must not pass credentials in args.
// CONSTRAINTS.md's gitexec Checked-Call Invariant is the mechanism that
// keeps every remaining raw call site (RunGit, or a direct exec.Command)
// deliberate rather than accidental.

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

// runCore spawns git with args in cwd and captures its output.
// A non-nil err means git could not be run at all (not found, permission
// denied, cwd invalid, …); a non-zero git exit is reported as exitCode with
// a nil err. RunGit and Run are both thin wrappers over this shared core
// and never call each other.
func runCore(args []string, cwd string) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	proc.HideWindow(cmd)

	runErr := cmd.Run()

	if exitErr, ok := runErr.(*exec.ExitError); ok {
		return outBuf.String(), errBuf.String(), exitErr.ExitCode(), nil
	} else if runErr != nil {
		return "", "", -1, runErr
	}

	return outBuf.String(), errBuf.String(), 0, nil
}

// RunGit runs a git command and returns stdout, stderr, and exit code.
func RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error) {
	stdout, stderr, exitCode, err = runCore(args, cwd)
	if err != nil {
		return "", "", -1, err
	}
	return stdout, stderr, exitCode, nil
}

// Run executes a git command and treats a non-zero exit as a failure: it
// returns *GitError, recoverable via errors.As, whenever git ran and
// rejected the command. It returns the raw underlying error, never wrapped
// in a GitError, when git could not be run at all — that distinction is
// what makes errors.As(err, &gitErr) mean precisely "git ran and rejected
// this". stdout is returned in every case where git actually ran, including
// alongside a *GitError; it is empty on an exec-level failure only because
// git never ran.
func Run(args []string, cwd string) (string, error) {
	stdout, stderr, exitCode, err := runCore(args, cwd)
	if err != nil {
		return "", err
	}
	if exitCode != 0 {
		return stdout, &GitError{Args: args, Dir: cwd, ExitCode: exitCode, Stderr: stderr}
	}
	return stdout, nil
}
