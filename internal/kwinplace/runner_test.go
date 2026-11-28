// runner_test.go pins the script runs and the script lock.
// Recordings answer for qdbus6, journalctl and kscreen-doctor.

package kwinplace

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	queryRunID = "3e269ffe3222"
	applyRunID = "d5083f4ee1ed"
)

// recordedExec stands in for qdbus6, journalctl and kscreen-doctor, answering each call with a recording.
type recordedExec struct {
	t *testing.T

	mu     sync.Mutex
	loaded map[string]bool
	calls  []string

	// journal is what journalctl answers.
	journal []byte
	// refuseLoad answers every load as one of a plugin name already loaded.
	refuseLoad bool
	// sticky keeps a script loaded through its unload.
	sticky bool
	// hangLoad answers a load only once its context ends.
	hangLoad bool
	// onRun is called on a script's run call.
	onRun func()
	// lingers is how many isScriptLoaded reads after an unload still answer that the script is loaded.
	lingers int
	// lateEnd cuts the first journal read before the query run's end marker.
	lateEnd bool
	// hangState answers an isScriptLoaded read only once its context ends.
	hangState bool
	// journalArgs are the arguments of the latest journalctl call.
	journalArgs []string
}

func newRecordedExec(t *testing.T) *recordedExec {
	t.Helper()
	journal := append(readRecording(t, "neve-query/journal.txt"), readRecording(t, "neve-apply/journal.txt")...)
	return &recordedExec{t: t, loaded: map[string]bool{}, journal: journal}
}

func (e *recordedExec) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	verb := name
	if name == qdbusProgram {
		verb = args[2]
	}
	plugin := ""
	switch verb {
	case "loadScript":
		plugin = args[4]
	case "unloadScript", "isScriptLoaded":
		plugin = args[3]
	}

	e.mu.Lock()
	e.calls = append(e.calls, strings.TrimSpace(verb+" "+plugin))
	e.mu.Unlock()

	switch verb {
	case "loadScript":
		if e.hangLoad {
			<-ctx.Done()
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.refuseLoad || e.loaded[plugin] {
			return readRecording(e.t, "neve-qdbus/load-taken.txt"), nil
		}
		e.loaded[plugin] = true
		return readRecording(e.t, "neve-qdbus/load.txt"), nil
	case "run":
		// The recorded load answers script id 1.
		if args[1] != "/Scripting/Script1" {
			e.t.Errorf("run called on %q; want /Scripting/Script1", args[1])
		}
		if e.onRun != nil {
			e.onRun()
		}
		return nil, nil
	case "unloadScript":
		e.mu.Lock()
		defer e.mu.Unlock()
		if !e.sticky {
			delete(e.loaded, plugin)
		}
		return nil, nil
	case "isScriptLoaded":
		if e.hangState {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.loaded[plugin] || e.lingers > 0 {
			e.lingers = max(0, e.lingers-1)
			return readRecording(e.t, "neve-qdbus/loaded-true.txt"), nil
		}
		return readRecording(e.t, "neve-qdbus/loaded-false.txt"), nil
	case journalctlProgram:
		e.mu.Lock()
		defer e.mu.Unlock()
		e.journalArgs = slices.Clone(args)
		if e.lateEnd {
			e.lateEnd = false
			return e.journal[:bytes.Index(e.journal, []byte(queryRunID+" end "))], nil
		}
		return e.journal, nil
	case kscreenProgram:
		return readRecording(e.t, "neve-kscreen/kscreen.json"), nil
	}
	return nil, errors.New("no recording for " + name + " " + strings.Join(args, " "))
}

func (e *recordedExec) log() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.calls)
}

func (e *recordedExec) loads() int {
	return len(slices.DeleteFunc(e.log(), func(c string) bool { return !strings.HasPrefix(c, "loadScript ") }))
}

// fakeClock moves only when something sleeps on it.
type fakeClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *fakeClock) sleep(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

func newTestRunner(stateDir, runID string, answers *recordedExec) *Runner {
	clock := &fakeClock{at: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	return &Runner{
		stateDir:       stateDir,
		exec:           answers.run,
		now:            clock.now,
		sleep:          clock.sleep,
		newRunID:       func() string { return runID },
		callTimeout:    time.Minute,
		scriptLockWait: time.Minute,
		journalWait:    defaultJournalWait,
		journalPoll:    defaultJournalPoll,
		unloadWait:     defaultUnloadWait,
		unloadPoll:     defaultUnloadPoll,
	}
}

func loadedName(t *testing.T, stateDir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(stateDir, loadedScriptFileName))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

func holdScriptLock(t *testing.T, stateDir string) *lock.FileLock {
	t.Helper()
	held, err := lock.AcquireWriteLock(filepath.Join(stateDir, scriptLockFileName))
	if err != nil {
		t.Fatal(err)
	}
	return held
}

func TestRunner_ScriptRuns(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const plugin = pluginNamePrefix + queryRunID
	query := func(r *Runner, ctx context.Context) error {
		_, _, err := r.Query(ctx, "DP-2", time.Time{}, time.Minute)
		return err
	}
	// requireUnloaded asserts the run's plugin was unloaded, the record emptied and the script lock released.
	requireUnloaded := func(t *testing.T, stateDir string, answers *recordedExec) {
		t.Helper()
		if !slices.Contains(answers.log(), "unloadScript "+plugin) {
			t.Errorf("calls = %q; want an unload of %s", answers.log(), plugin)
		}
		if got := loadedName(t, stateDir); got != "" {
			t.Errorf("loaded script record = %q; want empty", got)
		}
		held, taken, err := lock.TryAcquireWriteLock(filepath.Join(stateDir, scriptLockFileName))
		if err != nil || !taken {
			t.Fatalf("script lock not released: %v", err)
		}
		held.Release()
	}
	setup := func(t *testing.T) (string, *recordedExec, *Runner) {
		stateDir := t.TempDir()
		answers := newRecordedExec(t)
		return stateDir, answers, newTestRunner(stateDir, queryRunID, answers)
	}

	t.Run("QueryReadsItsSnapshot", func(t *testing.T) {
		t.Parallel()
		stateDir, answers, runner := setup(t)
		snapshot, _, err := runner.Query(t.Context(), "DP-2", time.Time{}, time.Minute)
		if err != nil || len(snapshot.Desktops) != 10 {
			t.Fatalf("Query() = %d desktops, %v; want 10", len(snapshot.Desktops), err)
		}
		// The journal is read from the load, which the test clock puts at 2026-01-02 03:04:05 UTC.
		want := []string{"--user", "-u", "plasma-kwin_wayland.service", "--since", "@1767323045", "-o", "cat"}
		if !slices.Equal(answers.journalArgs, want) {
			t.Errorf("journalctl arguments = %q; want %q", answers.journalArgs, want)
		}
		requireUnloaded(t, stateDir, answers)
	})

	t.Run("EndMarkerOnTheSecondReadSucceeds", func(t *testing.T) {
		t.Parallel()
		stateDir, answers, runner := setup(t)
		answers.lateEnd = true
		if err := query(runner, t.Context()); err != nil {
			t.Errorf("Query() = %v; want the second journal read to end the run", err)
		}
		requireUnloaded(t, stateDir, answers)
	})

	t.Run("UnloadSeenOnTheSecondReadSucceeds", func(t *testing.T) {
		t.Parallel()
		stateDir, answers, runner := setup(t)
		answers.lingers = 1
		if err := query(runner, t.Context()); err != nil {
			t.Fatalf("Query() error = %v", err)
		}
		requireUnloaded(t, stateDir, answers)
	})

	t.Run("RefusedLoadFails", func(t *testing.T) {
		t.Parallel()
		stateDir, answers, runner := setup(t)
		answers.refuseLoad = true
		if err := query(runner, t.Context()); err == nil {
			t.Error("Query() = nil; want the refused load")
		}
		requireUnloaded(t, stateDir, answers)
	})

	t.Run("LoadPastTheCallTimeoutFails", func(t *testing.T) {
		t.Parallel()
		stateDir, answers, runner := setup(t)
		answers.hangLoad = true
		runner.callTimeout = 20 * time.Millisecond
		if err := query(runner, t.Context()); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Query() = %v; want a deadline error", err)
		}
		requireUnloaded(t, stateDir, answers)
	})

	t.Run("LeftoverIsUnloadedBeforeTheNextLoad", func(t *testing.T) {
		t.Parallel()
		stateDir, answers, runner := setup(t)
		answers.loaded["lym-kwinplace-dead"] = true
		if err := os.WriteFile(filepath.Join(stateDir, loadedScriptFileName), []byte("lym-kwinplace-dead"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := query(runner, t.Context()); err != nil {
			t.Fatalf("Query() error = %v", err)
		}
		calls := answers.log()
		unloaded := slices.Index(calls, "unloadScript lym-kwinplace-dead")
		if unloaded < 0 || unloaded > slices.Index(calls, "loadScript "+plugin) {
			t.Errorf("calls = %q; want the leftover unloaded before the load", calls)
		}
	})

	t.Run("ScriptThatStaysLoadedStopsTheNextRun", func(t *testing.T) {
		t.Parallel()
		stateDir, answers, runner := setup(t)
		answers.sticky = true
		if err := query(runner, t.Context()); err != nil {
			t.Fatalf("Query() error = %v", err)
		}
		if got := loadedName(t, stateDir); got != plugin {
			t.Errorf("loaded script record = %q; want %q", got, plugin)
		}

		_, err := runner.Apply(t.Context(), ApplyOrder{})
		var leftover *LeftoverScriptError
		if !errors.As(err, &leftover) || !strings.Contains(err.Error(), "unloadScript "+plugin) {
			t.Errorf("Apply() = %v; want a LeftoverScriptError naming the unload of %s", err, plugin)
		}
		if got := answers.loads(); got != 1 {
			t.Errorf("loads = %d; want 1", got)
		}

		answers.hangState = true
		runner.callTimeout = 20 * time.Millisecond
		_, err = runner.Apply(t.Context(), ApplyOrder{})
		if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "could not be read") || !strings.Contains(err.Error(), "unloadScript "+plugin) {
			t.Errorf("Apply() with an unreadable state = %v; want it to say so, wrap the cause and name the unload", err)
		}
	})

	t.Run("NoEndMarkerTimesOut", func(t *testing.T) {
		t.Parallel()
		stateDir, answers, runner := setup(t)
		answers.journal = answers.journal[:strings.Index(string(answers.journal), queryRunID+" end ")]
		if err := query(runner, t.Context()); !errors.Is(err, ErrScriptTimeout) {
			t.Errorf("Query() = %v; want ErrScriptTimeout", err)
		}
		requireUnloaded(t, stateDir, answers)
	})

	t.Run("CancelStillUnloads", func(t *testing.T) {
		t.Parallel()
		stateDir, answers, runner := setup(t)
		ctx, cancel := context.WithCancel(t.Context())
		answers.onRun = cancel
		if err := query(runner, ctx); !errors.Is(err, context.Canceled) {
			t.Errorf("Query() = %v; want context.Canceled", err)
		}
		requireUnloaded(t, stateDir, answers)
	})

	t.Run("HeldScriptLockLoadsNothing", func(t *testing.T) {
		t.Parallel()
		bounds := []struct{ lockWait, within time.Duration }{
			{lockWait: 20 * time.Millisecond, within: time.Hour},
			{lockWait: time.Hour, within: 20 * time.Millisecond},
		}
		for _, bound := range bounds {
			stateDir, answers, runner := setup(t)
			held := holdScriptLock(t, stateDir)
			runner.scriptLockWait = bound.lockWait
			_, _, err := runner.Query(t.Context(), "DP-2", time.Time{}, bound.within)
			if !errors.Is(err, errScriptLockBusy) || answers.loads() != 0 {
				t.Errorf("Query() with lock wait %v within %v = %v after %d loads; want the lock busy, no load", bound.lockWait, bound.within, err, answers.loads())
			}
			held.Release()
		}

		stateDir, answers, runner := setup(t)
		held := holdScriptLock(t, stateDir)
		defer held.Release()
		runner.scriptLockWait = 20 * time.Millisecond
		if _, err := runner.Apply(t.Context(), ApplyOrder{}); !errors.Is(err, errScriptLockBusy) || answers.loads() != 0 {
			t.Errorf("Apply() over a held lock = %v after %d loads; want the lock busy, no load", err, answers.loads())
		}
	})

	t.Run("NewerSharedReportLoadsNothing", func(t *testing.T) {
		t.Parallel()
		_, answers, runner := setup(t)
		start := runner.now()
		_, ended, err := runner.Query(t.Context(), "DP-2", start, time.Minute)
		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}
		snapshot, _, err := runner.Query(t.Context(), "DP-2", ended.Add(-time.Second), time.Minute)
		if err != nil || len(snapshot.Desktops) != 10 || answers.loads() != 1 {
			t.Errorf("Query() after the report = %d desktops, %v, %d loads; want 10 desktops from 1 load", len(snapshot.Desktops), err, answers.loads())
		}
		if _, _, err := runner.Query(t.Context(), "DP-2", ended.Add(time.Second), time.Minute); err != nil || answers.loads() != 2 {
			t.Errorf("Query() past the report = %v after %d loads; want 2 loads", err, answers.loads())
		}
		if _, _, err := runner.Query(t.Context(), "HDMI-A-1", ended.Add(-time.Second), time.Minute); err != nil || answers.loads() != 3 {
			t.Errorf("Query() of another output = %v after %d loads; want its own load, the third", err, answers.loads())
		}
	})

	t.Run("ReadsTheTilingOutput", func(t *testing.T) {
		t.Parallel()
		_, _, runner := setup(t)
		if got, err := runner.ReadTilingOutput(t.Context()); err != nil || got != "DP-2" {
			t.Errorf("ReadTilingOutput() = %q, %v; want DP-2", got, err)
		}
	})
}

func TestRunner_OverlappingRunsTakeTheScriptLockInTurn(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	stateDir := t.TempDir()
	answers := newRecordedExec(t)
	querier := newTestRunner(stateDir, queryRunID, answers)
	applier := newTestRunner(stateDir, applyRunID, answers)

	var group sync.WaitGroup
	group.Go(func() {
		snapshot, _, err := querier.Query(t.Context(), "DP-2", time.Time{}, time.Minute)
		if err != nil || len(snapshot.Desktops) != 10 {
			t.Errorf("Query() = %d desktops, %v; want 10", len(snapshot.Desktops), err)
		}
	})
	group.Go(func() {
		report, err := applier.Apply(t.Context(), ApplyOrder{})
		if err != nil || len(report.Tiles) != 4 {
			t.Errorf("Apply() = %d zones, %v; want 4", len(report.Tiles), err)
		}
	})
	group.Wait()

	var order []string
	for _, call := range answers.log() {
		if strings.HasPrefix(call, "loadScript ") || strings.HasPrefix(call, "unloadScript ") {
			order = append(order, call)
		}
	}
	if len(order) != 4 {
		t.Fatalf("loads and unloads = %q; want one of each per run", order)
	}
	for i := 0; i < len(order); i += 2 {
		plugin := strings.TrimPrefix(order[i], "loadScript ")
		if order[i+1] != "unloadScript "+plugin {
			t.Errorf("loads and unloads = %q; want each load followed by its own unload", order)
		}
	}
	if order[0] == order[2] {
		t.Errorf("both runs loaded under %q", order[0])
	}
}

func TestAcquireWithin(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	path := filepath.Join(t.TempDir(), scriptLockFileName)
	var calls atomic.Int32
	counted := func(path string) (*lock.FileLock, error) {
		calls.Add(1)
		return lock.AcquireWriteLock(path)
	}
	hold := func() *lock.FileLock {
		held, err := lock.AcquireWriteLock(path)
		if err != nil {
			t.Fatal(err)
		}
		return held
	}

	holder := hold()
	got, taken, err := acquireWithin(t.Context(), path, 20*time.Millisecond, counted)
	if got != nil || taken || err != nil || calls.Load() != 1 {
		t.Fatalf("acquireWithin() over a held lock = %v, %v, %v after %d calls; want not taken after 1", got, taken, err, calls.Load())
	}

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	// Not the counted acquire: a call given up on may start after the count is reset below.
	got, taken, err = acquireWithin(cancelled, path, time.Minute, lock.AcquireWriteLock)
	if got != nil || taken || !errors.Is(err, context.Canceled) {
		t.Fatalf("acquireWithin() under a cancelled context = %v, %v, %v; want context.Canceled", got, taken, err)
	}

	holder.Release()
	got, taken, err = acquireWithin(t.Context(), path, time.Minute, counted)
	if !taken || err != nil {
		t.Fatalf("acquireWithin() after the release = %v, %v; want taken, so the abandoned call let go", taken, err)
	}
	got.Release()

	holder = hold()
	calls.Store(0)
	released := time.AfterFunc(20*time.Millisecond, func() { holder.Release() })
	defer released.Stop()
	got, taken, err = acquireWithin(t.Context(), path, time.Minute, counted)
	if !taken || err != nil || calls.Load() != 1 {
		t.Fatalf("acquireWithin() released within the bound = %v, %v after %d calls; want taken after 1", taken, err, calls.Load())
	}
	got.Release()
}
