// runner.go runs the KWin scripts through qdbus6, one at a time per user, and reads their reports from the journal.

package kwinplace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/standalonestate"
	"github.com/Knatte18/loomyard/internal/state"
)

// The files the runner keeps in the per-user state directory.
const (
	scriptLockFileName   = "kwinscript.lock"
	loadedScriptFileName = "kwinscript.loaded"
	queryCacheFileName   = "kwinquery.json"
	queryCacheLockName   = "kwinquery.lock"
)

// What a script run sends to KWin and reads from the journal.
const (
	pluginNamePrefix = "lym-kwinplace-"
	kwinService      = "org.kde.KWin"
	scriptingObject  = "/Scripting"
	kwinJournalUnit  = "plasma-kwin_wayland.service"
)

// The production bounds of a script run.
const (
	defaultCallTimeout    = 5 * time.Second
	defaultScriptLockWait = 30 * time.Second
	defaultJournalWait    = 5 * time.Second
	defaultJournalPoll    = 250 * time.Millisecond
	defaultUnloadWait     = 2 * time.Second
	defaultUnloadPoll     = 250 * time.Millisecond
)

// ErrScriptTimeout reports that a script's end marker did not reach the journal in time.
var ErrScriptTimeout = errors.New("kwinplace: no end marker within the bound")

var errScriptLockBusy = errors.New("kwinplace: another script run holds the script lock")

// LeftoverScriptError reports a script an earlier run left in KWin that was not seen to unload.
// No script is loaded while it stays.
type LeftoverScriptError struct {
	// Plugin is the plugin name the script is loaded under.
	Plugin string
	// Err is why the script's load state could not be read, nil when KWin answered that it is loaded.
	Err error
}

// Error names the plugin, what is known of its state and how to unload it by hand.
func (e *LeftoverScriptError) Error() string {
	state := "is still loaded"
	if e.Err != nil {
		state = fmt.Sprintf("may still be loaded, as its load state could not be read (%v)", e.Err)
	}
	return fmt.Sprintf("kwinplace: KWin script %s %s; run `%s %s %s unloadScript %s`, then the next placement runs again",
		e.Plugin, state, qdbusProgram, kwinService, scriptingObject, e.Plugin)
}

// Unwrap returns why the load state could not be read.
func (e *LeftoverScriptError) Unwrap() error {
	return e.Err
}

// Runner runs KWin scripts and reads the screen setup.
// Runners sharing a state directory, in one process or many, run their scripts one at a time.
type Runner struct {
	stateDir string
	exec     func(ctx context.Context, name string, args ...string) ([]byte, error)
	now      func() time.Time
	sleep    func(time.Duration)
	newRunID func() string

	callTimeout    time.Duration
	scriptLockWait time.Duration
	journalWait    time.Duration
	journalPoll    time.Duration
	unloadWait     time.Duration
	unloadPoll     time.Duration
}

// NewRunner returns the runner that reaches this user's KWin session.
func NewRunner() (*Runner, error) {
	stateDir, err := standalonestate.UserStateDir()
	if err != nil {
		return nil, fmt.Errorf("kwinplace: state directory: %w", err)
	}
	return &Runner{
		stateDir:       stateDir,
		exec:           execLogged,
		now:            time.Now,
		sleep:          time.Sleep,
		newRunID:       randomRunID,
		callTimeout:    defaultCallTimeout,
		scriptLockWait: defaultScriptLockWait,
		journalWait:    defaultJournalWait,
		journalPoll:    defaultJournalPoll,
		unloadWait:     defaultUnloadWait,
		unloadPoll:     defaultUnloadPoll,
	}, nil
}

func execLogged(ctx context.Context, name string, args ...string) ([]byte, error) {
	logger.Debug("kwinplace: spawning", "program", name, "args", args)
	output, err := exec.CommandContext(ctx, name, args...).Output()
	logger.Debug("kwinplace: ended", "program", name, "error", err)
	return output, err
}

func randomRunID() string {
	var id [6]byte
	// rand.Read of crypto/rand never returns an error.
	_, _ = rand.Read(id[:])
	return hex.EncodeToString(id[:])
}

// queryCache is the last query's report, kept so one query serves every process waiting for the script lock.
type queryCache struct {
	TilingOutput string    `json:"tilingOutput"`
	EndedAt      time.Time `json:"endedAt"`
	Snapshot     Snapshot  `json:"snapshot"`
}

// Query returns a snapshot of KWin's state with the tiles of the output named tilingOutput, and when its report ended.
// A report for the same output that another run ended after `after` is returned with no script loaded.
// It waits at most within for the script lock.
// It returns ErrNoTilingOutput when KWin has no output of that name.
func (r *Runner) Query(ctx context.Context, tilingOutput string, after time.Time, within time.Duration) (Snapshot, time.Time, error) {
	scriptLock, err := r.lockScripts(ctx, within)
	if err != nil {
		return Snapshot{}, time.Time{}, err
	}
	defer scriptLock.Release()

	cachePath := filepath.Join(r.stateDir, queryCacheFileName)
	cacheLockPath := filepath.Join(r.stateDir, queryCacheLockName)
	cached, found, err := state.ReadJSON[queryCache](cachePath, cacheLockPath)
	if err != nil {
		logger.Warn("kwinplace: unreadable query report, querying again", "path", cachePath, "error", err)
	} else if found && cached.TilingOutput == tilingOutput && cached.EndedAt.After(after) {
		return cached.Snapshot, cached.EndedAt, nil
	}

	lines, err := r.runLocked(ctx, func(runID string) string { return QueryScript(runID, tilingOutput) })
	if err != nil {
		return Snapshot{}, time.Time{}, err
	}
	snapshot, err := ParseQueryReport(lines)
	if err != nil {
		return Snapshot{}, time.Time{}, err
	}
	ended := r.now()
	if err := state.WriteJSON(cachePath, cacheLockPath, queryCache{TilingOutput: tilingOutput, EndedAt: ended, Snapshot: snapshot}); err != nil {
		logger.Warn("kwinplace: query report not shared", "path", cachePath, "error", err)
	}
	return snapshot, ended, nil
}

// Apply carries out order in KWin and returns what the script reports.
func (r *Runner) Apply(ctx context.Context, order ApplyOrder) (ApplyReport, error) {
	lines, err := r.runScript(ctx, func(runID string) string { return ApplyScript(runID, order) }, r.scriptLockWait)
	if err != nil {
		return ApplyReport{}, err
	}
	return ParseApplyReport(lines)
}

// ReadTilingOutput returns the name of the screen KDE ranks first.
func (r *Runner) ReadTilingOutput(ctx context.Context) (string, error) {
	output, err := r.call(ctx, kscreenProgram, "-j")
	if err != nil {
		return "", err
	}
	return TilingOutput(output)
}

// runScript loads, runs and unloads one script under the script lock and returns the lines of its report.
// It waits at most within for the lock.
func (r *Runner) runScript(ctx context.Context, script func(runID string) string, within time.Duration) ([]string, error) {
	scriptLock, err := r.lockScripts(ctx, within)
	if err != nil {
		return nil, err
	}
	defer scriptLock.Release()
	return r.runLocked(ctx, script)
}

func (r *Runner) lockScripts(ctx context.Context, within time.Duration) (*lock.FileLock, error) {
	if err := os.MkdirAll(r.stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("kwinplace: state directory: %w", err)
	}
	scriptLock, taken, err := acquireWithin(ctx, filepath.Join(r.stateDir, scriptLockFileName), min(within, r.scriptLockWait), lock.AcquireWriteLock)
	if err != nil {
		return nil, fmt.Errorf("kwinplace: script lock: %w", err)
	}
	if !taken {
		return nil, errScriptLockBusy
	}
	return scriptLock, nil
}

// runLocked is a script run for a caller that holds the script lock.
// A script is loaded only once the one an earlier run recorded as loaded has left KWin.
func (r *Runner) runLocked(ctx context.Context, script func(runID string) string) ([]string, error) {
	loadedPath := filepath.Join(r.stateDir, loadedScriptFileName)
	leftover, err := os.ReadFile(loadedPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("kwinplace: read the loaded script's name: %w", err)
	}
	if name := strings.TrimSpace(string(leftover)); name != "" {
		if err := r.unload(ctx, name); err != nil {
			return nil, err
		}
	}

	runID := r.newRunID()
	plugin := pluginNamePrefix + runID
	file, err := writeScriptFile(script(runID))
	if err != nil {
		return nil, err
	}
	defer os.Remove(file)
	if err := fsx.AtomicWriteBytes(loadedPath, []byte(plugin)); err != nil {
		return nil, fmt.Errorf("kwinplace: record the loaded script's name: %w", err)
	}

	loadedAt := r.now()
	// A load that fails or times out may still have loaded the script, so every exit from here unloads it.
	defer r.unload(ctx, plugin)
	answer, err := r.call(ctx, qdbusProgram, kwinService, scriptingObject, "loadScript", file, plugin)
	if err != nil {
		return nil, err
	}
	id, err := strconv.Atoi(strings.TrimSpace(string(answer)))
	if err != nil || id < 0 {
		return nil, fmt.Errorf("kwinplace: KWin refused to load script %s: answer %q", plugin, strings.TrimSpace(string(answer)))
	}
	if _, err := r.call(ctx, qdbusProgram, kwinService, scriptingObject+"/Script"+strconv.Itoa(id), "run"); err != nil {
		return nil, err
	}
	return r.readReport(ctx, runID, loadedAt)
}

func writeScriptFile(script string) (string, error) {
	file, err := lymtemp.CreateTemp("kwinplace-*.js")
	if err != nil {
		return "", fmt.Errorf("kwinplace: script file: %w", err)
	}
	_, writeErr := file.WriteString(script)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return "", errors.Join(fmt.Errorf("kwinplace: script file: %w", err), os.Remove(file.Name()))
	}
	return file.Name(), nil
}

func (r *Runner) readReport(ctx context.Context, runID string, since time.Time) ([]string, error) {
	deadline := r.now().Add(r.journalWait)
	for {
		journal, err := r.call(ctx, journalctlProgram, "--user", "-u", kwinJournalUnit, "--since", "@"+strconv.FormatInt(since.Unix(), 10), "-o", "cat")
		if err != nil {
			return nil, err
		}
		if lines, ended := ReportLines(journal, runID); ended {
			return lines, nil
		}
		if !r.now().Before(deadline) {
			return nil, ErrScriptTimeout
		}
		r.sleep(r.journalPoll)
	}
}

// unload asks KWin to drop the plugin and returns nil once it has left.
// A plugin not seen to leave within the bound is a *LeftoverScriptError.
// The record of the loaded script is emptied once it has left.
// A cancelled ctx does not stop it, so a run ended by its caller leaves no script it can remove.
func (r *Runner) unload(ctx context.Context, plugin string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.unloadWait+r.callTimeout)
	defer cancel()
	if _, err := r.call(ctx, qdbusProgram, kwinService, scriptingObject, "unloadScript", plugin); err != nil {
		logger.Warn("kwinplace: unload failed", "plugin", plugin, "error", err)
	}
	deadline := r.now().Add(r.unloadWait)
	for {
		answer, err := r.call(ctx, qdbusProgram, kwinService, scriptingObject, "isScriptLoaded", plugin)
		if err == nil && strings.TrimSpace(string(answer)) == "false" {
			if err := fsx.AtomicWriteBytes(filepath.Join(r.stateDir, loadedScriptFileName), nil); err != nil {
				logger.Warn("kwinplace: loaded script's name not cleared", "plugin", plugin, "error", err)
			}
			return nil
		}
		if !r.now().Before(deadline) {
			logger.Warn("kwinplace: script not seen to unload", "plugin", plugin, "error", err)
			return &LeftoverScriptError{Plugin: plugin, Err: err}
		}
		r.sleep(r.unloadPoll)
	}
}

// call runs one program under the per-call timeout; an answer that arrives past it is a failure.
func (r *Runner) call(ctx context.Context, name string, args ...string) ([]byte, error) {
	callCtx, cancel := context.WithTimeout(ctx, r.callTimeout)
	defer cancel()
	output, err := r.exec(callCtx, name, args...)
	if ctxErr := callCtx.Err(); ctxErr != nil {
		err = ctxErr
	}
	if err != nil {
		return nil, fmt.Errorf("kwinplace: %s %s: %w", name, strings.Join(args, " "), err)
	}
	return output, nil
}

// acquireWithin takes the lock at path through one blocking call of acquire.
// It waits at most bound and no longer than ctx lasts.
// It reports false when the bound passes first.
// An acquire given up on releases its lock at once should it land later.
func acquireWithin(ctx context.Context, path string, bound time.Duration, acquire func(string) (*lock.FileLock, error)) (*lock.FileLock, bool, error) {
	type result struct {
		held *lock.FileLock
		err  error
	}
	results := make(chan result)
	abandoned := make(chan struct{})
	go func() {
		held, err := acquire(path)
		select {
		case results <- result{held, err}:
		case <-abandoned:
			if err == nil {
				if err := held.Release(); err != nil {
					logger.Warn("kwinplace: abandoned lock not released", "path", path, "error", err)
				}
			}
		}
	}()

	timer := time.NewTimer(bound)
	defer timer.Stop()
	select {
	case got := <-results:
		if got.err != nil {
			return nil, false, got.err
		}
		return got.held, true, nil
	case <-timer.C:
		close(abandoned)
		return nil, false, nil
	case <-ctx.Done():
		close(abandoned)
		return nil, false, ctx.Err()
	}
}
