// gitexec.go implements the package's two-shape entry-point split.
// Run is the checked default: it treats a non-zero git exit as a failure,
// returning *GitError; RunStdin is Run with stdin fed to git.
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
// PATTERN-gitexec-checked-call is the mechanism that
// keeps every remaining raw call site (RunGit, or a direct exec.Command)
// deliberate rather than accidental.
// A remote subcommand additionally runs under a deadline, a process-group kill and the low-speed environment; see doc.go.

package gitexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
)

// remoteDeadline bounds one remote git command; a command still running when it expires is killed.
// It is a variable only so a test can shorten it.
var remoteDeadline = 10 * time.Minute

// remoteWaitDelay bounds the wait on output pipes after a remote git command exits or is killed, since an orphaned helper such as git-remote-https can still hold them.
const remoteWaitDelay = 10 * time.Second

// lowSpeedEnv makes git's HTTPS transport give up on a transfer below 1000 B/s for 60 s, so a stalled connection fails well inside remoteDeadline.
var lowSpeedEnv = []string{"GIT_HTTP_LOW_SPEED_LIMIT=1000", "GIT_HTTP_LOW_SPEED_TIME=60"}

// killReporter receives the outcome of every kill of a timed-out remote git command; nil drops it.
var killReporter atomic.Pointer[func(args []string, pid int, groupErr error)]

// SetKillReporter installs the function that receives the outcome of each kill of a timed-out remote git command:
// the command's args, its pid, and the group kill's error, which is nil when the whole process group was killed.
// gitexec cannot log, so the process owner installs a reporter that does; a nil report restores the default, which drops the outcome.
// The reporter only observes a kill after it happened and cannot change the command's result.
func SetKillReporter(report func(args []string, pid int, groupErr error)) {
	if report == nil {
		killReporter.Store(nil)
		return
	}
	killReporter.Store(&report)
}

// reportKill hands one kill's outcome to the installed reporter, if any.
func reportKill(args []string, pid int, groupErr error) {
	if report := killReporter.Load(); report != nil {
		(*report)(args, pid, groupErr)
	}
}

// GitError reports that a git command ran and exited non-zero, or that a remote command outlived its deadline and was killed.
// It carries the command that was run, the directory it ran in, its exit code, its stderr and, for a killed remote command, the deadline.
// Error renders the args, the exit code or deadline and the stderr; Dir deliberately is not rendered, and is carried solely for a caller that wants to name the directory itself.
// The omission is the reason a caller can wrap a GitError without the result naming the same directory twice:
// at nearly every call site the wrapper already says which repo or worktree it was operating on, so rendering Dir here would duplicate it in the one part of the message an operator reads first.
// Args are rendered verbatim, with no redaction — callers must not pass credentials in args.
type GitError struct {
	Args     []string
	Dir      string
	ExitCode int
	Stderr   string

	// Timeout is the deadline a remote command outlived before it was killed; zero unless the deadline killed git, in which case ExitCode is -1.
	Timeout time.Duration
}

// Error renders "git <args>: exit <code>", or "git <args>: timed out after <deadline>" when the deadline killed git, followed by ": <trimmed stderr>" only when the trimmed stderr is non-empty.
// Each arg is rendered bare, except an arg that is empty or contains a space, tab, or newline, which is rendered %q-quoted so the boundary between adjacent args stays legible.
func (e *GitError) Error() string {
	rendered := make([]string, len(e.Args))
	for i, arg := range e.Args {
		rendered[i] = renderArg(arg)
	}

	msg := fmt.Sprintf("git %s: exit %d", strings.Join(rendered, " "), e.ExitCode)
	if e.Timeout != 0 {
		msg = fmt.Sprintf("git %s: timed out after %s", strings.Join(rendered, " "), e.Timeout)
	}

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
// A non-nil err means git could not be run at all (not found, permission denied, cwd invalid, …), or that a remote subcommand outlived remoteDeadline, which is a *GitError with exit code -1.
// A non-zero git exit is reported as exitCode with a nil err.
// RunGit and RunStdin are thin wrappers over this shared core, and Run is RunStdin with an empty stdin.
// A non-empty stdin is fed to git's standard input; an empty one leaves it unset.
//
// A remote subcommand runs under remoteDeadline, in its own process group, with lowSpeedEnv added to the environment;
// a local one runs unbounded with the inherited environment.
func runCore(args []string, cwd, stdin string) (stdout, stderr string, exitCode int, err error) {
	var cmd *exec.Cmd
	var ctx context.Context
	deadline := remoteDeadline
	if remoteSubcommands[subcommand(args)] {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), deadline)
		defer cancel()
		cmd = exec.CommandContext(ctx, "git", args...)
		cmd.WaitDelay = remoteWaitDelay
		cmd.Env = append(os.Environ(), lowSpeedEnv...)
		proc.ConfigureGroupKill(cmd, func(pid int, groupErr error) {
			reportKill(args, pid, groupErr)
		})
	} else {
		cmd = exec.Command("git", args...)
	}
	cmd.Dir = cwd
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	proc.HideWindow(cmd)

	runErr := cmd.Run()

	if runErr != nil && ctx != nil && ctx.Err() != nil {
		return outBuf.String(), errBuf.String(), -1, &GitError{Args: args, Dir: cwd, ExitCode: -1, Stderr: errBuf.String(), Timeout: deadline}
	}
	if errors.Is(runErr, exec.ErrWaitDelay) && cmd.ProcessState != nil {
		return outBuf.String(), errBuf.String(), cmd.ProcessState.ExitCode(), nil
	}
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		return outBuf.String(), errBuf.String(), exitErr.ExitCode(), nil
	} else if runErr != nil {
		return "", "", -1, runErr
	}

	return outBuf.String(), errBuf.String(), 0, nil
}

// RunGit runs a git command and returns stdout, stderr, and exit code.
// A remote command that outlives its deadline is returned as a *GitError with a non-zero Timeout, though git did not exit on its own.
func RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error) {
	stdout, stderr, exitCode, err = runCore(args, cwd, "")
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
// A remote command that outlives its deadline is returned as a *GitError with a non-zero Timeout, though git did not exit on its own.
func Run(args []string, cwd string) (string, error) {
	return RunStdin(args, cwd, "")
}

// RunStdin is Run with stdin fed to git's standard input, for a command such as `cat-file --batch-check` that reads its operands there.
// It reports a non-zero exit, an exec-level failure and a remote deadline exactly as Run does, and an empty stdin behaves as Run.
func RunStdin(args []string, cwd, stdin string) (string, error) {
	stdout, stderr, exitCode, err := runCore(args, cwd, stdin)
	if err != nil {
		return "", err
	}
	if exitCode != 0 {
		return stdout, &GitError{Args: args, Dir: cwd, ExitCode: exitCode, Stderr: stderr}
	}
	return stdout, nil
}
