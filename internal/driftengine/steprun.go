// steprun.go runs one drift command in the gate slot its caller holds, through a shell.
// It writes the command's output to the log file, and it kills the command's tree on timeout.
//
// The drift commands describe their run as a stepRun and share this body.

package driftengine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// stampLayout is the UTC time layout that names a run's log file.
const stampLayout = "20060102T150405.000Z"

// The sites a drift command runs from label its slot holder and wait record,
// and a refused dir's way forward names the verbs for its site.
const (
	siteCapture        = "drift capture"
	siteCompare        = "drift compare"
	siteCurrentCapture = "drift current capture"
)

// stepRun is one drift command to run.
type stepRun struct {
	// WorktreeDir is the absolute worktree root a relative step dir is joined onto.
	WorktreeDir string
	// AnchorDir is the absolute anchored directory a step with no dir runs in.
	AnchorDir string
	// Name is the step's drift.yaml key, capture or compare, which a refusal of its dir names.
	Name string
	// Step is the declared command.
	Step CommandStep
	// Pool is the gate-slot pool the run takes one slot of.
	Pool *gateslot.Pool
	// WaitDir is where the run's wait record is written while it waits for a slot.
	WaitDir string
	// InheritedSlot is the lock path of the slot a gate holds around the run;
	// when it is held, the run runs in that slot instead of taking one.
	InheritedSlot string
	// Slot is the slot the run's command runs in, which the caller takes before the run and releases after it.
	Slot heldSlot
	// Site labels the slot holder and the wait record.
	Site string
	// BaselineDir is the path the command's {baseline_dir} placeholder renders to.
	BaselineDir string
	// MetaDir is the path the command's {meta_dir} placeholder renders to, created before the command starts.
	MetaDir string
	// LogPath is the file the command's stdout and stderr go to.
	LogPath string
	// Timeout bounds the command from its start, on the monotonic clock.
	Timeout time.Duration
	// OnSlot is called with the time the slot was acquired.
	OnSlot func(at time.Time)
	// OnStart is called with the command's pid and start time.
	OnStart func(pid int, at time.Time)
	// Now is the clock the callbacks' times come from.
	Now func() time.Time
}

// stepOutcome is how a drift command ended.
type stepOutcome struct {
	// ExitCode is the command's exit code, -1 when a signal ended it.
	ExitCode int
	// TimedOut is true when the timeout killed the command's tree.
	TimedOut bool
	// Dir is the absolute directory the command ran in.
	Dir string
}

// heldSlot is the gate slot a drift command runs in.
type heldSlot struct {
	// lease is the slot the command acquired; nil when it runs in an inherited one.
	lease *gateslot.Lease
	// limits are the gate limits read when the slot was taken.
	limits gateslot.Limits
	// inherited is the lock path of the slot a gate holds around the command, empty when the command acquired its own.
	inherited string
	// release gives back an acquired slot and does nothing for an inherited one.
	release func()
}

// runStep runs the command in the slot the caller holds and returns once its whole tree is gone.
// The caller takes the slot before runStep runs and releases it after runStep returns.
// This lets a capture hold its slot before it takes the capture lock.
// A step whose dir is no existing directory is refused with a stepRefusal before its command starts,
// so a shell never fails to start in a missing directory.
func runStep(ctx context.Context, run stepRun) (stepOutcome, error) {
	command, err := renderCommand(run)
	if err != nil {
		return stepOutcome{}, err
	}
	dir, err := gateslot.StatCommandDir(run.WorktreeDir, run.AnchorDir, run.Step.Dir)
	if err != nil {
		return stepOutcome{}, missingDirRefusal(run, err)
	}
	if err := os.MkdirAll(filepath.Dir(run.LogPath), 0o755); err != nil {
		return stepOutcome{}, fmt.Errorf("drift: create the log directory: %w", err)
	}
	if err := os.MkdirAll(run.MetaDir, 0o755); err != nil {
		return stepOutcome{}, fmt.Errorf("drift: create the metadata directory: %w", err)
	}
	run.OnSlot(run.Now())

	logFile, err := os.OpenFile(run.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return stepOutcome{}, fmt.Errorf("drift: open the log %s: %w", run.LogPath, err)
	}
	defer logFile.Close()

	environ := slices.Concat(os.Environ(), renderEnv(run.Step.Env))
	if run.Slot.inherited != "" {
		environ = append(environ, gateslot.InheritEnv+"="+run.Slot.inherited)
	}
	shellName, shellFlag := verifyrun.ShellFor(runtime.GOOS)
	cmd := exec.Command(shellName, shellFlag, command)
	cmd.Dir = dir
	cmd.Env = gateslot.HolderEnv(environ, run.Slot.lease, run.Slot.limits.Tier0Parallel)
	cmd.Stdout, cmd.Stderr = logFile, logFile

	logger.Info("drift: spawning a command", "site", run.Site, "dir", cmd.Dir, "command", command, "log", run.LogPath)
	tied, err := proc.StartTied(cmd)
	if err != nil {
		return stepOutcome{}, fmt.Errorf("drift: start the %s command in %s: %w", run.Site, cmd.Dir, err)
	}
	run.OnStart(cmd.Process.Pid, run.Now())

	done := make(chan error, 1)
	go func() { done <- tied.Wait() }()
	timer := time.NewTimer(run.Timeout)
	defer timer.Stop()

	var waitErr error
	timedOut := false
	select {
	case waitErr = <-done:
	case <-timer.C:
		timedOut = true
		killTree(tied, run.Site)
		waitErr = <-done
	case <-ctx.Done():
		killTree(tied, run.Site)
		waitErr = <-done
	}
	logger.Info("drift: a command ended", "site", run.Site, "timed_out", timedOut, "error", errorText(waitErr))

	exit, err := exitCodeOf(waitErr)
	if err != nil {
		return stepOutcome{}, fmt.Errorf("drift: wait for the %s command: %w", run.Site, err)
	}
	return stepOutcome{ExitCode: exit, TimedOut: timedOut, Dir: dir}, nil
}

// outputTailLines is how many of a finished command's last non-empty output lines a failure's reason quotes.
const outputTailLines = 3

// outputTailMaxRunes bounds the quoted output, so a reason stays one readable line; a longer tail keeps its end.
const outputTailMaxRunes = 400

// outputTailReadBytes bounds how much of the end of a command's log commandReport reads.
const outputTailReadBytes = 64 * 1024

// readLogTail returns the last outputTailReadBytes bytes of the file at path, or all of it when it is shorter.
// A cut file starts at a line boundary when its read window holds one,
// and a line the window begins inside is dropped.
func readLogTail(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	// A cut file is read from one byte before its window,
	// so the byte shows whether the window begins on a line start.
	offset := max(0, info.Size()-outputTailReadBytes-1)
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		startsLine := data[0] == '\n'
		data = data[1:]
		if newline := bytes.IndexByte(data, '\n'); !startsLine && newline >= 0 {
			data = data[newline+1:]
		}
	}
	return data, nil
}

// commandReport returns the clause that ends a failed command's reason.
// The clause names the directory the command ran in, and quotes the last non-empty lines of its log.
// When the log holds no such lines, it says the command printed nothing.
// A tail over the output bound is cut from its start, so the last line, which usually states the cause, stays.
// A log that cannot be read is named with its error,
// so the clause still says where the command ran.
func commandReport(dir, logPath string) string {
	data, err := readLogTail(logPath)
	if err != nil {
		return fmt.Sprintf("; it ran in %s and its log could not be read: %v", dir, err)
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	if len(lines) == 0 {
		return "; it ran in " + dir + " and printed nothing"
	}
	tail := lines[max(0, len(lines)-outputTailLines):]
	return "; it ran in " + dir + " and its last output was: " + boundOneLineKeepingEnd(strings.Join(tail, " / "), outputTailMaxRunes)
}

// foldOneLine folds text onto one line as runes.
func foldOneLine(text string) []rune {
	return []rune(strings.Join(strings.Fields(text), " "))
}

// boundOneLine folds text onto one line and cuts it to maxRunes from its end, marking a cut with a trailing ellipsis.
func boundOneLine(text string, maxRunes int) string {
	runes := foldOneLine(text)
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes-1]) + "…"
}

// boundOneLineKeepingEnd folds text onto one line and cuts it to maxRunes from its start.
// A cut is marked with a leading ellipsis.
func boundOneLineKeepingEnd(text string, maxRunes int) string {
	runes := foldOneLine(text)
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return "…" + string(runes[len(runes)-(maxRunes-1):])
}

// stepRefusal is the error of a step refused before its command started, as opposed to an I/O failure while running it.
type stepRefusal struct {
	cause error
}

// Error returns the refusal's text, which names its way forward.
func (e *stepRefusal) Error() string { return e.cause.Error() }

// Unwrap returns the failure the refusal wraps.
func (e *stepRefusal) Unwrap() error { return e.cause }

// missingDirRefusal is the refusal of a step whose dir is no existing directory.
// It names the config key and the resolved path.
// Its way forward names the config write that fixes the key from the prime, where hub-wide config is written.
// Then it names the verbs that run the step again from its site.
func missingDirRefusal(run stepRun, cause error) error {
	again := "lym loom resume"
	if run.Site == siteCapture {
		again = "lym drift capture and lym loom resume"
	}
	return &stepRefusal{cause: fmt.Errorf("drift: the %s command cannot start: %s.yaml key %q: %w; way forward: lym config drift --set %s.dir=<dir> from the prime, then %s", run.Site, configModule, run.Name+".dir", cause, run.Name, again)}
}

// acquireSlot returns the slot the run's command runs in.
// A run whose InheritedSlot the pool reports as held takes that slot and acquires none.
// A command inside a gate is such a run.
// Otherwise it writes a wait record, waits for one slot with no deadline and removes the record.
// The caller releases the returned slot once the command ended.
func acquireSlot(ctx context.Context, run stepRun) (heldSlot, error) {
	limits, err := run.Pool.Limits()
	if err != nil {
		return heldSlot{}, fmt.Errorf("drift: read the gate limits: %w", err)
	}
	if run.InheritedSlot != "" && run.Pool.Inherited(run.InheritedSlot) {
		return heldSlot{limits: limits, inherited: run.InheritedSlot, release: func() {}}, nil
	}
	waitPath, err := gateslot.WriteWait(run.WaitDir, gateslot.Wait{Site: run.Site, PID: os.Getpid(), Started: run.Now()})
	if err != nil {
		return heldSlot{}, err
	}
	lease, err := run.Pool.Acquire(ctx, gateslot.Holder{Worktree: run.AnchorDir, Site: run.Site})
	if removeErr := os.Remove(waitPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		logger.Warn("drift: removing the gate wait record failed", "path", waitPath, "error", removeErr.Error())
	}
	if err != nil {
		return heldSlot{}, err
	}
	release := func() {
		if err := lease.Release(); err != nil {
			logger.Warn("drift: releasing the gate slot failed", "site", run.Site, "error", err.Error())
		}
	}
	return heldSlot{lease: lease, limits: limits, release: release}, nil
}

// renderCommand returns the step's command with {baseline_dir} and {meta_dir} replaced by their quoted directories.
// A command holding the {skip_file} placeholder is refused, since nothing fills it.
func renderCommand(run stepRun) (string, error) {
	if err := refuseSkipFilePlaceholder(run.Site, run.Name, run.Step.Command); err != nil {
		return "", err
	}
	baseline, err := verifyrun.QuoteArg(runtime.GOOS, run.BaselineDir)
	if err != nil {
		return "", fmt.Errorf("drift: quote the baseline directory: %w", err)
	}
	meta, err := verifyrun.QuoteArg(runtime.GOOS, run.MetaDir)
	if err != nil {
		return "", fmt.Errorf("drift: quote the metadata directory: %w", err)
	}
	return strings.NewReplacer(baselineDirPlaceholder, baseline, metaDirPlaceholder, meta).Replace(run.Step.Command), nil
}

// skipFilePlaceholder is the {skip_file} placeholder, which drift does not render, so a command holding it is refused.
const skipFilePlaceholder = "{skip_file}"

// refuseSkipFilePlaceholder returns the stepRefusal of a command at the site that holds the {skip_file} placeholder.
// The refusal names the reconcile and the edit of the step that clear it.
// Both are hub-wide writes, refused from a task or beam session,
// so the way forward hands them to the operator or the hub orch.
// It returns nil for a command holding none.
func refuseSkipFilePlaceholder(site, step, command string) error {
	if !strings.Contains(command, skipFilePlaceholder) {
		return nil
	}
	return &stepRefusal{cause: fmt.Errorf("drift: the %s command holds %s, which drift no longer renders; way forward: the operator or the hub orch runs lym config reconcile --layer default --apply from the hub's prime worktree, which removes an -e NAME=%s argument, and removes any other use of it from %s with lym config drift --set, since a hub-wide config write is refused from any other session, then lym loom resume", site, skipFilePlaceholder, skipFilePlaceholder, step)}
}

// renderEnv returns env as KEY=VALUE entries in key order.
func renderEnv(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	entries := make([]string, len(keys))
	for i, key := range keys {
		entries[i] = key + "=" + env[key]
	}
	return entries
}

// killTree kills the command's whole tree and logs a failure.
func killTree(tied *proc.Tied, site string) {
	if err := tied.Kill(); err != nil {
		logger.Warn("drift: killing a command's process tree failed", "site", site, "error", err.Error())
	}
}

// exitCodeOf returns the exit code a Wait error carries, 0 for a nil error.
// An error that carries no exit status is returned unchanged.
func exitCodeOf(waitErr error) (int, error) {
	if waitErr == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 0, waitErr
}

// errorText returns err's text, empty for nil.
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// logPathFor returns the log file of a run of kind at now, under logsDir.
func logPathFor(logsDir, kind string, now time.Time) string {
	return filepath.Join(logsDir, kind+"-"+now.UTC().Format(stampLayout)+".log")
}
