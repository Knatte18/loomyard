// steprun.go — the runner behind every drift command: one slot of the gate pool, the shell, the log file and the timeout tree kill.
//
// The main capture, the side capture and the compare each describe their run as a stepRun and share this body.

package driftengine

import (
	"context"
	"errors"
	"fmt"
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

// stepRun is one drift command to run.
type stepRun struct {
	// WorktreeRoot is the task worktree a relative step dir is joined onto.
	WorktreeRoot string
	// Step is the declared command.
	Step CommandStep
	// Pool is the gate-slot pool the run takes one slot of.
	Pool *gateslot.Pool
	// WaitDir is where the run's wait record is written while it waits for a slot.
	WaitDir string
	// Site labels the slot holder and the wait record.
	Site string
	// BaselineDir and SkipFile are the paths the command's placeholders render to.
	BaselineDir string
	SkipFile    string
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
	// release gives back the slot; the caller calls it once it has recorded the outcome.
	release func()
}

// runStep waits for a slot, runs the command and returns once its whole tree is gone.
// On an error the slot is already released; on success the caller releases it through the outcome.
func runStep(ctx context.Context, run stepRun) (stepOutcome, error) {
	command, err := renderCommand(run)
	if err != nil {
		return stepOutcome{}, err
	}
	if err := os.MkdirAll(filepath.Dir(run.LogPath), 0o755); err != nil {
		return stepOutcome{}, fmt.Errorf("drift: create the log directory: %w", err)
	}

	lease, limits, err := acquireSlot(ctx, run)
	if err != nil {
		return stepOutcome{}, err
	}
	release := func() {
		if err := lease.Release(); err != nil {
			logger.Warn("drift: releasing the gate slot failed", "site", run.Site, "error", err.Error())
		}
	}
	run.OnSlot(run.Now())

	logFile, err := os.OpenFile(run.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		release()
		return stepOutcome{}, fmt.Errorf("drift: open the log %s: %w", run.LogPath, err)
	}
	defer logFile.Close()

	shellName, shellFlag := verifyrun.ShellFor(runtime.GOOS)
	cmd := exec.Command(shellName, shellFlag, command)
	cmd.Dir = run.Step.Dir
	if !filepath.IsAbs(cmd.Dir) {
		cmd.Dir = filepath.Join(run.WorktreeRoot, cmd.Dir)
	}
	cmd.Env = gateslot.HolderEnv(slices.Concat(os.Environ(), renderEnv(run.Step.Env)), lease, limits.Tier0Parallel)
	cmd.Stdout, cmd.Stderr = logFile, logFile

	logger.Info("drift: spawning a command", "site", run.Site, "dir", cmd.Dir, "command", command, "log", run.LogPath)
	tied, err := proc.StartTied(cmd)
	if err != nil {
		release()
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
		release()
		return stepOutcome{}, fmt.Errorf("drift: wait for the %s command: %w", run.Site, err)
	}
	return stepOutcome{ExitCode: exit, TimedOut: timedOut, release: release}, nil
}

// acquireSlot writes a wait record, waits for one slot with no deadline and removes the record.
func acquireSlot(ctx context.Context, run stepRun) (*gateslot.Lease, gateslot.Limits, error) {
	limits, err := run.Pool.Limits()
	if err != nil {
		return nil, gateslot.Limits{}, fmt.Errorf("drift: read the gate limits: %w", err)
	}
	waitPath, err := gateslot.WriteWait(run.WaitDir, gateslot.Wait{Site: run.Site, PID: os.Getpid(), Started: run.Now()})
	if err != nil {
		return nil, gateslot.Limits{}, err
	}
	lease, err := run.Pool.Acquire(ctx, gateslot.Holder{Worktree: run.WorktreeRoot, Site: run.Site})
	if removeErr := os.Remove(waitPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		logger.Warn("drift: removing the gate wait record failed", "path", waitPath, "error", removeErr.Error())
	}
	if err != nil {
		return nil, gateslot.Limits{}, err
	}
	return lease, limits, nil
}

// renderCommand returns the step's command with its placeholders replaced by quoted paths.
func renderCommand(run stepRun) (string, error) {
	baseline, err := verifyrun.QuoteArg(runtime.GOOS, run.BaselineDir)
	if err != nil {
		return "", fmt.Errorf("drift: quote the baseline directory: %w", err)
	}
	skip, err := verifyrun.QuoteArg(runtime.GOOS, run.SkipFile)
	if err != nil {
		return "", fmt.Errorf("drift: quote the skip file: %w", err)
	}
	return strings.NewReplacer(baselineDirPlaceholder, baseline, skipFilePlaceholder, skip).Replace(run.Step.Command), nil
}

// skipFilePlaceholder is the placeholder a command holds for the file naming the skipped tests.
const skipFilePlaceholder = "{skip_file}"

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

// exitCodeOf returns the exit code a Wait error carries, 0 for none; an error that is no exit status is returned.
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

// writeSkipFile writes the skipped tests, one per line, to path; an empty list writes an empty file.
func writeSkipFile(path string, skip []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("drift: create the skip file directory: %w", err)
	}
	var body strings.Builder
	for _, test := range skip {
		body.WriteString(test + "\n")
	}
	if err := os.WriteFile(path, []byte(body.String()), 0o644); err != nil {
		return fmt.Errorf("drift: write the skip file: %w", err)
	}
	return nil
}

// logPathFor returns the log file of a run of kind at now, under logsDir.
func logPathFor(logsDir, kind string, now time.Time) string {
	return filepath.Join(logsDir, kind+"-"+now.UTC().Format(stampLayout)+".log")
}
