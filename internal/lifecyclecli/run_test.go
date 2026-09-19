// run_test.go drives the run verb's four dispositions against a hand-populated receiver and a
// hand-written status file under t.TempDir(), bypassing wire entirely: every Env seam here is a
// fake that performs no real I/O, no git spawn, and no process spawn.

package lifecyclecli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lifecyclerecipe"
	"github.com/Knatte18/loomyard/internal/lifecycleshed"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/state"
)

// newFakeReceiver builds a *lifecycleCLI whose Env is filled entirely with fakes that perform no
// real I/O, over a fresh t.TempDir(). shutdown, when non-nil, replaces the default no-op
// Teardown.Shutdown closure.
func newFakeReceiver(t *testing.T, shutdown func(ctx context.Context) (string, error)) *lifecycleCLI {
	t.Helper()
	dir := t.TempDir()

	c := &lifecycleCLI{
		location: &lyxcwd.Location{RepoName: "example", HubPath: dir, WorktreeName: "hub-repo", AnchorRel: "."},
		slug:     "some-slug",
	}
	c.shedPaths = shedbuild.ShedPaths{
		StatusPath:     filepath.Join(dir, "status.json"),
		LockPath:       filepath.Join(dir, "run.lock"),
		StatusLockPath: filepath.Join(dir, "status.json.lock"),
	}

	if shutdown == nil {
		shutdown = func(ctx context.Context) (string, error) { return "", nil }
	}

	c.env = shedrecipe.Env{
		Slug:       c.slug,
		ScratchDir: dir,
		PrimeLock: lifecycleshed.PrimeLock{
			Path: filepath.Join(dir, "prime.lock"),
			Acquire: func() (func() error, bool, error) {
				return func() error { return nil }, true, nil
			},
		},
		CreateWorktree: func(ctx context.Context) error { return nil },
		Teardown: lifecycleshed.TeardownDeps{
			Shutdown: shutdown,
			Remove:   func(ctx context.Context) error { return nil },
		},
		InnerRun: lifecycleshed.InnerRunDeps{
			Spawn: func(ctx context.Context) error { return nil },
			ResolveStatus: func() (string, string, error) {
				return filepath.Join(dir, "loom-status.json"), filepath.Join(dir, "loom-status.json.lock"), nil
			},
			ReadStatus: func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
				return shedengine.Status{State: shedengine.StateDone}, true, nil
			},
		},
	}
	return c
}

// writeStatus writes st as c's status file, unlocked -- the test owns the file outright before the
// verb ever runs.
func writeStatus(t *testing.T, c *lifecycleCLI, st shedengine.Status) {
	t.Helper()
	if err := state.WriteJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, st); err != nil {
		t.Fatalf("writeStatus: %v", err)
	}
}

// TestRunCmd_ResumeDispositions covers StateRunning, StateBlocked, StateFailed, and StatePaused:
// each resumes silently from the persisted current producer, with every fake succeeding trivially,
// so the whole run completes and the verb reports success.
func TestRunCmd_ResumeDispositions(t *testing.T) {
	for _, state := range []shedengine.State{shedengine.StateRunning, shedengine.StateBlocked, shedengine.StateFailed, shedengine.StatePaused} {
		t.Run(string(state), func(t *testing.T) {
			c := newFakeReceiver(t, nil)
			writeStatus(t, c, shedengine.Status{
				CurrentProducer: lifecyclerecipe.NameWorktreeCreate,
				State:           state,
			})

			var out bytes.Buffer
			exitCode := clihelp.Execute(lifecycleVerbCommand(c, "run"), &out, []string{c.slug})

			if exitCode != 0 {
				t.Fatalf("run(%s) exit code = %d; want 0; output: %s", state, exitCode, out.String())
			}
			if !strings.Contains(out.String(), `"ok":true`) {
				t.Errorf("run(%s) output missing ok:true envelope; got: %q", state, out.String())
			}
		})
	}
}

// TestRunCmd_StateDoneRefusesNamingTheLifecycleDir asserts a StateDone slug refuses on the envelope
// rather than silently re-running, naming the per-slug directory to delete.
func TestRunCmd_StateDoneRefusesNamingTheLifecycleDir(t *testing.T) {
	c := newFakeReceiver(t, nil)
	writeStatus(t, c, shedengine.Status{
		CurrentProducer: lifecyclerecipe.NameWorktreeTeardown,
		State:           shedengine.StateDone,
	})

	var out bytes.Buffer
	exitCode := clihelp.Execute(lifecycleVerbCommand(c, "run"), &out, []string{c.slug})

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d; want 1; output: %s", exitCode, out.String())
	}
	wantDir := LifecycleDir(c.location, c.slug)
	if !strings.Contains(out.String(), wantDir) {
		t.Errorf("run() output = %q; want it to name the per-slug directory %q", out.String(), wantDir)
	}
}

// TestRunCmd_AbsentStatusFileStartsFresh asserts that no persisted status file at all is a fresh
// start, not a refusal.
func TestRunCmd_AbsentStatusFileStartsFresh(t *testing.T) {
	c := newFakeReceiver(t, nil)

	var out bytes.Buffer
	exitCode := clihelp.Execute(lifecycleVerbCommand(c, "run"), &out, []string{c.slug})

	if exitCode != 0 {
		t.Fatalf("run() exit code = %d; want 0; output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"ok":true`) {
		t.Errorf("run() output missing ok:true envelope; got: %q", out.String())
	}
}

// TestRunCmd_HeldRunLockRefusesNamingTheLockPath takes the per-slug run lock in the test itself,
// before invoking the verb, and asserts the verb refuses on the envelope naming the lock path
// without waiting.
func TestRunCmd_HeldRunLockRefusesNamingTheLockPath(t *testing.T) {
	c := newFakeReceiver(t, nil)
	writeStatus(t, c, shedengine.Status{
		CurrentProducer: lifecyclerecipe.NameWorktreeCreate,
		State:           shedengine.StateBlocked,
	})

	held, err := lock.AcquireWriteLock(c.shedPaths.LockPath)
	if err != nil {
		t.Fatalf("acquire run lock in test: %v", err)
	}
	t.Cleanup(func() { _ = held.Release() })

	var out bytes.Buffer
	exitCode := clihelp.Execute(lifecycleVerbCommand(c, "run"), &out, []string{c.slug})

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d; want 1; output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), c.shedPaths.LockPath) {
		t.Errorf("run() output = %q; want it to name the lock path %q", out.String(), c.shedPaths.LockPath)
	}
}

// TestRunCmd_AbandonedSessionKey asserts the abandonedSession key reaches the envelope on a Done
// teardown when the recorded value is non-empty, and is absent when it is empty.
func TestRunCmd_AbandonedSessionKey(t *testing.T) {
	tests := []struct {
		name             string
		abandonedSession string
		wantKey          bool
	}{
		{"NonEmptyRecordsKey", "some-abandoned-session", true},
		{"EmptyOmitsKey", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c *lifecycleCLI
			shutdown := func(ctx context.Context) (string, error) {
				c.abandonedSession = tt.abandonedSession
				return tt.abandonedSession, nil
			}
			c = newFakeReceiver(t, shutdown)
			writeStatus(t, c, shedengine.Status{
				CurrentProducer: lifecyclerecipe.NameWorktreeTeardown,
				State:           shedengine.StateBlocked,
			})

			var out bytes.Buffer
			exitCode := clihelp.Execute(lifecycleVerbCommand(c, "run"), &out, []string{c.slug})
			if exitCode != 0 {
				t.Fatalf("run() exit code = %d; want 0; output: %s", exitCode, out.String())
			}

			var envelope map[string]any
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("decode envelope: %v; output: %s", err, out.String())
			}
			_, hasKey := envelope["abandonedSession"]
			if hasKey != tt.wantKey {
				t.Errorf("envelope has abandonedSession key = %v; want %v; envelope: %v", hasKey, tt.wantKey, envelope)
			}
		})
	}
}

// TestRunCmd_HistoryLengthKey asserts the run envelope's history_length key -- new in this task,
// free from the generic body's own len(result.History) -- matches the run's own persisted history
// length, over a fresh run that completes end to end against the fakes.
func TestRunCmd_HistoryLengthKey(t *testing.T) {
	c := newFakeReceiver(t, nil)

	var out bytes.Buffer
	exitCode := clihelp.Execute(lifecycleVerbCommand(c, "run"), &out, []string{c.slug})
	if exitCode != 0 {
		t.Fatalf("run() exit code = %d; want 0; output: %s", exitCode, out.String())
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v; output: %s", err, out.String())
	}

	st, found, err := state.ReadJSONStrict[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
	if err != nil || !found {
		t.Fatalf("read persisted status: found=%v err=%v", found, err)
	}
	if len(st.History) == 0 {
		t.Fatal("persisted history is empty; want the fresh run to have appended at least one entry")
	}

	gotLen, ok := envelope["history_length"].(float64)
	if !ok {
		t.Fatalf("envelope[\"history_length\"] = %v (%T); want a number", envelope["history_length"], envelope["history_length"])
	}
	if int(gotLen) != len(st.History) {
		t.Errorf("envelope[\"history_length\"] = %v; want %d (the run's own persisted history length)", gotLen, len(st.History))
	}
}

// TestPauseCmd_SetsPauseRequestedAndEnvelope asserts pause -- new in this task -- sets
// PauseRequested on a seeded status file and reports an envelope carrying exactly the single key
// status_file.
func TestPauseCmd_SetsPauseRequestedAndEnvelope(t *testing.T) {
	c := newFakeReceiver(t, nil)
	writeStatus(t, c, shedengine.Status{
		CurrentProducer: lifecyclerecipe.NameWorktreeCreate,
		State:           shedengine.StateBlocked,
	})

	var out bytes.Buffer
	exitCode := clihelp.Execute(lifecycleVerbCommand(c, "pause"), &out, []string{c.slug})
	if exitCode != 0 {
		t.Fatalf("pause() exit code = %d; want 0; output: %s", exitCode, out.String())
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v; output: %s", err, out.String())
	}
	if len(envelope) != 2 || envelope["ok"] != true || envelope["status_file"] != c.shedPaths.StatusPath {
		t.Errorf("pause() envelope = %v; want exactly ok and status_file=%q", envelope, c.shedPaths.StatusPath)
	}

	st, found, err := state.ReadJSONStrict[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
	if err != nil || !found {
		t.Fatalf("read persisted status: found=%v err=%v", found, err)
	}
	if !st.PauseRequested {
		t.Error("PauseRequested = false after pause(); want true")
	}
}

// TestPauseCmd_AbsentFileRefuses asserts pause refuses over a slug whose per-slug directory
// exists but holds no status.json -- the absent-file precondition lifecycle-absent-file-needs-an-
// existing-directory requires, since pause reaches state.UpdateJSON directly.
func TestPauseCmd_AbsentFileRefuses(t *testing.T) {
	c := newFakeReceiver(t, nil)

	var out bytes.Buffer
	exitCode := clihelp.Execute(lifecycleVerbCommand(c, "pause"), &out, []string{c.slug})
	if exitCode != 1 {
		t.Fatalf("pause() exit code = %d; want 1; output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), "there is nothing running to pause") {
		t.Errorf("pause() output = %q; want it to name the absent-file refusal", out.String())
	}
	if !strings.Contains(out.String(), "lyx lifecycle run") {
		t.Errorf("pause() output = %q; want it to name lifecycle's own entry verb as the remedy", out.String())
	}
}

// TestStatusCmd_RegistersWatchAndIntervalFlags asserts status -- new in this task -- exposes
// --watch and --interval, the two flags shedverbs' generic status body itself reads.
func TestStatusCmd_RegistersWatchAndIntervalFlags(t *testing.T) {
	c := newFakeReceiver(t, nil)
	cmd := lifecycleVerbCommand(c, "status")

	if cmd.Flags().Lookup("watch") == nil {
		t.Error(`status command is missing the --watch flag`)
	}
	if cmd.Flags().Lookup("interval") == nil {
		t.Error(`status command is missing the --interval flag`)
	}
}

// TestStatusCmd_WatchOverAbsentFileExitsImmediately asserts --watch over a slug whose per-slug
// directory exists but holds no status.json exits immediately with the found: false envelope
// rather than entering the tail -- the absent-file disposition short-circuits before --watch is
// ever read, per the lifecycle-absent-file-needs-an-existing-directory Shared Decision. The
// directory must already exist for this disposition to be reachable at all: newFakeReceiver's own
// t.TempDir() is that directory.
func TestStatusCmd_WatchOverAbsentFileExitsImmediately(t *testing.T) {
	c := newFakeReceiver(t, nil)

	var out bytes.Buffer
	exitCode := clihelp.Execute(lifecycleVerbCommand(c, "status"), &out, []string{"--watch", c.slug})
	if exitCode != 0 {
		t.Fatalf("status(--watch) exit code = %d; want 0; output: %s", exitCode, out.String())
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v; output: %s", err, out.String())
	}
	if envelope["found"] != false {
		t.Errorf(`status(--watch) envelope["found"] = %v; want false`, envelope["found"])
	}
	if envelope["status_path"] != c.shedPaths.StatusPath {
		t.Errorf(`status(--watch) envelope["status_path"] = %v; want %q`, envelope["status_path"], c.shedPaths.StatusPath)
	}
}
