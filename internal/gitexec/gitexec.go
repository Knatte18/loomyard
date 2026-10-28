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
// The floor error of a git below lym's minimum is the other such result, since no git command ran to produce it.
// Run's stdout is returned in every case where git actually ran, including
// alongside a *GitError; it is empty on an exec-level failure only because
// git never ran.
//
// Args passed to either form are rendered verbatim into any resulting
// *GitError, with no redaction — callers must not pass credentials in args.
// PATTERN-gitrepo-client-boundary's checked-call rule is the mechanism that
// keeps every remaining raw call site (RunGit, or a direct exec.Command)
// deliberate rather than accidental.
// A remote subcommand additionally runs under a deadline, a stall bound with one retry, a process-group kill and the low-speed environment; see doc.go.

package gitexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/toolfloor"
)

// remoteDeadline bounds one remote git command; a command still running when it expires is killed.
// It is a variable only so a test can shorten it.
var remoteDeadline = 10 * time.Minute

// stallBound bounds the wait for a remote's first answer; a remote command that has received no packet when it passes is killed and run once more.
// It is a variable only so a test can shorten it.
var stallBound = 30 * time.Second

// spawnAgainAttempts is how many times a git the host could not start for EAGAIN is tried in all.
const spawnAgainAttempts = 4

// spawnAgainFirstDelay is the wait before the first retry of such a start; each later wait doubles it.
// It is a variable only so a test can shorten it.
var spawnAgainFirstDelay = 250 * time.Millisecond

// sleepBeforeRetry waits between start retries; it is a variable only so a test can record the waits instead of sitting them out.
var sleepBeforeRetry = time.Sleep

// runCommand runs one git child to completion; it is a variable only so a test can make a start fail.
var runCommand = (*exec.Cmd).Run

// remoteWaitDelay bounds the wait on output pipes after a remote git command exits or is killed, since an orphaned helper such as git-remote-https can still hold them.
const remoteWaitDelay = 10 * time.Second

// lowSpeedEnv makes git's HTTPS transport give up on a transfer below 1000 B/s for 60 s, so a stalled connection fails well inside remoteDeadline.
var lowSpeedEnv = []string{"GIT_HTTP_LOW_SPEED_LIMIT=1000", "GIT_HTTP_LOW_SPEED_TIME=60"}

// killReporter receives the outcome of every kill of a timed-out remote git command; nil drops it.
var killReporter atomic.Pointer[func(args []string, pid int, groupErr error)]

// SetKillReporter installs the function that receives the outcome of each kill of a timed-out remote git command:
// the command's args, its pid, and the group kill's error, which is nil when the whole process group was killed.
// The process owner decides how a kill is reported by installing a reporter; a nil report restores the default, which drops the outcome.
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

// GitError reports that a git command ran and exited non-zero, or that a remote command outlived its deadline or stalled twice and was killed.
// It carries the command that was run, the directory it ran in, its exit code, its stderr and, for a killed remote command, the bound that killed it.
// Error renders the args, the exit code or bound and the stderr; Dir deliberately is not rendered, and is carried solely for a caller that wants to name the directory itself.
// The omission is the reason a caller can wrap a GitError without the result naming the same directory twice:
// at nearly every call site the wrapper already says which repo or worktree it was operating on, so rendering Dir here would duplicate it in the one part of the message an operator reads first.
// Args are rendered verbatim, with no redaction — callers must not pass credentials in args.
type GitError struct {
	Args     []string
	Dir      string
	ExitCode int
	Stderr   string

	// Timeout is the bound a remote command outlived before it was killed, the deadline or the stall bound; zero unless a bound killed git, in which case ExitCode is -1.
	Timeout time.Duration

	// Stalled reports that the remote sent no answer within the stall bound on both attempts, and Timeout is the stall bound.
	Stalled bool
}

// Error renders "git <args>: exit <code>", "git <args>: timed out after <deadline>" when the deadline killed git, or "git <args>: no answer from the remote within <stall bound>, twice" for a stall, followed by ": <trimmed stderr>" only when the trimmed stderr is non-empty.
// Each arg is rendered bare, except an arg that is empty or contains a space, tab, or newline, which is rendered %q-quoted so the boundary between adjacent args stays legible.
func (e *GitError) Error() string {
	rendered := make([]string, len(e.Args))
	for i, arg := range e.Args {
		rendered[i] = renderArg(arg)
	}

	command := strings.Join(rendered, " ")
	msg := fmt.Sprintf("git %s: exit %d", command, e.ExitCode)
	switch {
	case e.Stalled:
		msg = fmt.Sprintf("git %s: no answer from the remote within %s, twice", command, e.Timeout)
	case e.Timeout != 0:
		msg = fmt.Sprintf("git %s: timed out after %s", command, e.Timeout)
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

// floorCache holds the process's answer to the git floor check, so the version probe runs once.
type floorCache struct {
	mu   sync.Mutex
	done bool
	err  error
}

// check returns the cached answer once one exists; otherwise it runs probe.
// A probe error is returned and not cached, so the next call probes again.
// A line toolfloor parses is cached with its answer, nil or the *toolfloor.FloorError.
// An unparseable line is logged once with Warn and cached as admitted.
func (c *floorCache) check(probe func() (string, error)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		return c.err
	}
	line, err := probe()
	if err != nil {
		return err
	}
	parsed, floorErr := toolfloor.CheckGit(line)
	if !parsed {
		logger.Warn("gitexec: git version line not recognised, admitting it", "line", line)
	}
	c.done = true
	c.err = floorErr
	return c.err
}

// gitFloor is the process's git floor answer.
var gitFloor floorCache

// versionProbe reports git's version line; it is a variable so a test injects one.
var versionProbe = probeGitVersion

// probeGitVersion runs `git --version` and returns its trimmed stdout, or the exec error when git could not run.
// It spawns directly rather than through runCore, whose first step is the floor check this probe serves.
func probeGitVersion() (string, error) {
	logger.Debug("gitexec: spawn", "subcommand", "--version")
	cmd := exec.Command("git", "--version")
	cmd.Env = childEnv(nil)
	proc.HideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// CheckFloor reports whether the git on PATH is at or above lym's floor, probing once per process.
// It returns a *toolfloor.FloorError for a git below the floor, and the exec error, uncached, when git could not be run.
// It is exported so a git spawn outside the exec core checks too.
func CheckFloor() error {
	return gitFloor.check(versionProbe)
}

// runCore spawns git with args in cwd and captures its output, a remote subcommand bounded by remoteDeadline.
// RunGit and RunStdin are thin wrappers over this shared core, and Run is RunStdin with an empty stdin.
func runCore(args []string, cwd, stdin string, extraEnv []string) (stdout, stderr string, exitCode int, err error) {
	return runCoreWithin(args, cwd, stdin, extraEnv, remoteDeadline)
}

// runCoreWithin spawns git with args in cwd and captures its output.
// It first checks git's floor and returns that error unwrapped with exit code -1, before any spawn.
// A non-nil err means git could not be run at all (not found, permission denied, cwd invalid, …), that git is below the floor, or that a remote subcommand outlived deadline or stalled twice, which is a *GitError with exit code -1.
// A non-zero git exit is reported as exitCode with a nil err.
// A non-empty stdin is fed to git's standard input; an empty one leaves it unset.
//
// A remote subcommand runs under deadline and stallBound, in its own process group, with lowSpeedEnv added to the environment;
// a local one runs unbounded.
// A remote subcommand killed for a stall runs once more, since it received nothing from the remote and so changed nothing there.
// A git the host could not start for EAGAIN is started again as runSpawning says, before any of this judges the result.
// A remote subcommand first waits while a pause covers the process, through the installed network gate, if any, and its bounds start only once it runs.
// Every child gets the process environment less GIT_DIR, GIT_WORK_TREE and GIT_OPTIONAL_LOCKS, with GIT_OPTIONAL_LOCKS=0 set, and extraEnv after all of it.
func runCoreWithin(args []string, cwd, stdin string, extraEnv []string, deadline time.Duration) (stdout, stderr string, exitCode int, err error) {
	if floorErr := CheckFloor(); floorErr != nil {
		return "", "", -1, floorErr
	}
	if !remoteSubcommands[subcommand(args)] {
		stdout, stderr, exitCode, _, err = runSpawning(args, cwd, stdin, extraEnv, false, deadline)
		return stdout, stderr, exitCode, err
	}

	// The hold comes before the bounds' timers, so a command held by a pause starts its bounds only when it runs.
	_, done := pauserecord.Network(subcommand(args))
	defer done()
	stdout, stderr, exitCode, stalled, err := runSpawning(args, cwd, stdin, extraEnv, true, deadline)
	if !stalled {
		return stdout, stderr, exitCode, err
	}
	logger.Warn("gitexec: remote sent no answer, retrying once", "subcommand", subcommand(args), "dir", cwd, "stall_bound", stallBound)
	stdout, stderr, exitCode, stalled, err = runSpawning(args, cwd, stdin, extraEnv, true, deadline)
	if stalled {
		return stdout, stderr, -1, &GitError{Args: args, Dir: cwd, ExitCode: -1, Stderr: stderr, Timeout: stallBound, Stalled: true}
	}
	return stdout, stderr, exitCode, err
}

// runSpawning is runAttempt started again after a doubling wait while git could not be started for EAGAIN, up to spawnAgainAttempts tries in all.
// It returns the first attempt that is not such a failure, or the last one.
// Each EAGAIN failure is noted in proc's spawn-refusal record and each retry is logged at Warn;
// git never ran on such a failure, so a retry repeats no side effect, local or remote.
func runSpawning(args []string, cwd, stdin string, extraEnv []string, remote bool, deadline time.Duration) (stdout, stderr string, exitCode int, stalled bool, err error) {
	delay := spawnAgainFirstDelay
	for try := 1; ; try++ {
		stdout, stderr, exitCode, stalled, err = runAttempt(args, cwd, stdin, extraEnv, remote, deadline)
		if !proc.NoteSpawnAgain(err) || try == spawnAgainAttempts {
			return stdout, stderr, exitCode, stalled, err
		}
		logger.Warn("gitexec: git could not be started, retrying", "subcommand", subcommand(args), "dir", cwd, "attempt", try, "delay", delay, "error", err)
		sleepBeforeRetry(delay)
		delay *= 2
	}
}

// runAttempt spawns git once with args in cwd and captures its output, with runCoreWithin's result contract.
// A remote attempt runs under deadline, in its own process group, with lowSpeedEnv and a packet trace file added to the environment.
// When stallBound passes under deadline and the trace file is still empty, the remote has sent no answer:
// the attempt is killed and reports stalled, with a nil err and exit code -1.
func runAttempt(args []string, cwd, stdin string, extraEnv []string, remote bool, deadline time.Duration) (stdout, stderr string, exitCode int, stalled bool, err error) {
	logger.Debug("gitexec: spawn", "subcommand", subcommand(args), "dir", cwd)

	var cmd *exec.Cmd
	var ctx context.Context
	var sawStall atomic.Bool
	if remote {
		trace, err := lymtemp.CreateTemp("git-packet-trace-*")
		if err != nil {
			return "", "", -1, false, fmt.Errorf("create the git packet trace file: %w", err)
		}
		tracePath := trace.Name()
		defer os.Remove(tracePath)
		if err := trace.Close(); err != nil {
			return "", "", -1, false, fmt.Errorf("close the git packet trace file: %w", err)
		}

		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), deadline)
		defer cancel()
		cmd = exec.CommandContext(ctx, "git", args...)
		cmd.WaitDelay = remoteWaitDelay
		cmd.Env = childEnv(append(append(append([]string(nil), lowSpeedEnv...), "GIT_TRACE_PACKET="+tracePath), extraEnv...))
		proc.ConfigureGroupKill(cmd, func(pid int, groupErr error) {
			reportKill(args, pid, groupErr)
		})
		if stallBound < deadline {
			// git writes the trace's first line only once the remote's first packet arrives, so an empty trace at the bound is a remote that never answered.
			stallTimer := time.AfterFunc(stallBound, func() {
				if info, statErr := os.Stat(tracePath); statErr == nil && info.Size() == 0 {
					sawStall.Store(true)
					cancel()
				}
			})
			defer stallTimer.Stop()
		}
	} else {
		cmd = exec.Command("git", args...)
		cmd.Env = childEnv(extraEnv)
	}
	cmd.Dir = cwd
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	proc.HideWindow(cmd)

	runErr := runCommand(cmd)

	if runErr != nil && sawStall.Load() {
		return outBuf.String(), errBuf.String(), -1, true, nil
	}
	if runErr != nil && ctx != nil && ctx.Err() != nil {
		return outBuf.String(), errBuf.String(), -1, false, &GitError{Args: args, Dir: cwd, ExitCode: -1, Stderr: errBuf.String(), Timeout: deadline}
	}
	if errors.Is(runErr, exec.ErrWaitDelay) && cmd.ProcessState != nil {
		return outBuf.String(), errBuf.String(), cmd.ProcessState.ExitCode(), false, nil
	}
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		return outBuf.String(), errBuf.String(), exitErr.ExitCode(), false, nil
	} else if runErr != nil {
		return "", "", -1, false, runErr
	}

	return outBuf.String(), errBuf.String(), 0, false, nil
}

// noOptionalLocksEnv stops a read-only git command, such as `git status`, from taking the index lock to write back a refreshed stat cache.
// Such a probe runs under no lym lock, so its index lock would otherwise fail a concurrent `git add` with "index.lock: File exists".
const noOptionalLocksEnv = "GIT_OPTIONAL_LOCKS=0"

// childEnv returns the process environment without GIT_DIR, GIT_WORK_TREE and GIT_OPTIONAL_LOCKS, followed by noOptionalLocksEnv and extra.
// Dropping GIT_DIR and GIT_WORK_TREE makes a git child follow the directory it is run in, so one command never reads one repository and writes another.
func childEnv(extra []string) []string {
	environ := os.Environ()
	env := make([]string, 0, len(environ)+1+len(extra))
	for _, entry := range environ {
		if strings.HasPrefix(entry, "GIT_DIR=") || strings.HasPrefix(entry, "GIT_WORK_TREE=") || strings.HasPrefix(entry, "GIT_OPTIONAL_LOCKS=") {
			continue
		}
		env = append(env, entry)
	}
	env = append(env, noOptionalLocksEnv)
	return append(env, extra...)
}

// RunGit runs a git command and returns stdout, stderr, and exit code.
// A git below lym's floor returns the *toolfloor.FloorError with exit code -1, which is no *GitError.
// A remote command that outlives its deadline or stalls twice is returned as a *GitError with a non-zero Timeout, though git did not exit on its own.
func RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error) {
	stdout, stderr, exitCode, err = runCore(args, cwd, "", nil)
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
// A git below lym's floor returns the *toolfloor.FloorError, which is no *GitError, since no git command ran.
// A remote command that outlives its deadline or stalls twice is returned as a *GitError with a non-zero Timeout, though git did not exit on its own.
func Run(args []string, cwd string) (string, error) {
	return RunStdin(args, cwd, "")
}

// RunWithin is Run with a remote command bounded by deadline instead of the package-wide bound, for a caller that must give up on an unreachable remote sooner.
// A local command ignores deadline, and a deadline at or under the stall bound leaves the stall check unarmed.
func RunWithin(args []string, cwd string, deadline time.Duration) (string, error) {
	stdout, stderr, exitCode, err := runCoreWithin(args, cwd, "", nil, deadline)
	if err != nil {
		return "", err
	}
	if exitCode != 0 {
		return stdout, &GitError{Args: args, Dir: cwd, ExitCode: exitCode, Stderr: stderr}
	}
	return stdout, nil
}

// RunStdin is Run with stdin fed to git's standard input, for a command such as `cat-file --batch-check` that reads its operands there.
// It reports a non-zero exit, an exec-level failure, the floor error, a remote deadline and a stall exactly as Run does, and an empty stdin behaves as Run.
func RunStdin(args []string, cwd, stdin string) (string, error) {
	return runWithStdinAndEnv(args, cwd, stdin, nil)
}

// RunWithEnv is Run with extraEnv, entries of the form NAME=value, appended to the child's environment after the variables gitexec sets.
// It is the one way to give a local git child a variable such as GIT_INDEX_FILE, and reports every failure exactly as Run does.
func RunWithEnv(args []string, cwd string, extraEnv []string) (string, error) {
	return runWithStdinAndEnv(args, cwd, "", extraEnv)
}

// runWithStdinAndEnv is the body of RunStdin and RunWithEnv: a non-zero exit is a *GitError beside the stdout git wrote.
func runWithStdinAndEnv(args []string, cwd, stdin string, extraEnv []string) (string, error) {
	stdout, stderr, exitCode, err := runCore(args, cwd, stdin, extraEnv)
	if err != nil {
		return "", err
	}
	if exitCode != 0 {
		return stdout, &GitError{Args: args, Dir: cwd, ExitCode: exitCode, Stderr: stderr}
	}
	return stdout, nil
}
