// run_test.go drives the run verb's four dispositions against a hand-populated receiver and a
// hand-written status file under t.TempDir(), bypassing wire entirely: every Env seam here is a
// fake that performs no real I/O, no git spawn, and no process spawn.

package battencli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/state"
)

// newFakeReceiver builds a *battenCLI whose Env is filled entirely with fakes that perform no
// real I/O, over a fresh t.TempDir(). shutdown, when non-nil, replaces the default no-op
// Teardown.Shutdown closure.
func newFakeReceiver(t *testing.T, shutdown func(ctx context.Context) (string, error)) *battenCLI {
	t.Helper()
	dir := t.TempDir()

	c := &battenCLI{
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
		PrimeLock: battenshed.PrimeLock{
			Path: filepath.Join(dir, "prime.lock"),
			Acquire: func() (func() error, bool, error) {
				return func() error { return nil }, true, nil
			},
		},
		CreateWorktree: func(ctx context.Context) error { return nil },
		Teardown: battenshed.TeardownDeps{
			Shutdown: shutdown,
			Remove:   func(ctx context.Context) error { return nil },
		},
		InnerRun: battenshed.InnerRunDeps{
			Spawn: func(ctx context.Context) error { return nil },
			ResolveStatus: func() (string, string, error) {
				return filepath.Join(dir, "loom-status.json"), filepath.Join(dir, "loom-status.json.lock"), nil
			},
			ReadStatus: func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
				return shedengine.Status{State: shedengine.StateDone}, true, nil
			},
			ReadDecision: func() (battenshed.ChildDecision, bool, error) {
				return battenshed.ChildDecision{}, false, nil
			},
			DriverAlive: func(context.Context) (bool, error) { return false, nil },
		},
		// SeedChild is filled with trivial no-op fakes -- never invoked by any case in this file,
		// since every one of them resumes from a status file already past Worktree-Create -- so the
		// recipe registry's own non-nil-field validation does not refuse before the verb body runs.
		SeedChild: battenshed.SeedChildDeps{
			ReadBoardType:        func(ctx context.Context) (string, error) { return "", nil },
			ChildDriver:          func(string) (string, error) { return "", nil },
			ReadRunSettings:      func(ctx context.Context) (map[string]any, error) { return nil, nil },
			PrepareRunSettings:   func(settings map[string]any) (map[string][]byte, error) { return nil, nil },
			WriteLocalOverride:   func(ctx context.Context, module string, document []byte) error { return nil },
			RemoveLocalOverrides: func(ctx context.Context, keep []string) ([]string, error) { return nil, nil },
			WriteSeed:            func(ctx context.Context, recipe, driver string) error { return nil },
			CommitSeed:           func(ctx context.Context) error { return nil },
			PushSeed:             func(ctx context.Context) error { return nil },
		},
		// Land is filled with trivial fakes: a parent that is not the prime's branch skips the pull and deploy, so the row lands over the fakes with nothing to read.
		Land: battenshed.LandDeps{
			ParentBranch: func() (string, error) { return "feature", nil },
			PrimeBranch:  func() (string, error) { return "main", nil },
			PrimeDirty:   func() ([]string, error) { return nil, nil },
			Pull:         func(ctx context.Context) error { return nil },
			Landing:      func() (battenshed.LandingSettings, error) { return battenshed.LandingSettings{}, nil },
			RunDeploy: func(context.Context, string, string, time.Duration) (battenshed.DeployResult, error) {
				return battenshed.DeployResult{}, nil
			},
			ReadyTasks:     func() ([]string, error) { return nil, nil },
			PruneBoard:     func() error { return nil },
			PauseRequested: func() (bool, error) { return false, nil },
			LandLock: battenshed.PrimeLock{
				Path: filepath.Join(dir, "land.lock"),
				Acquire: func() (func() error, bool, error) {
					return func() error { return nil }, true, nil
				},
			},
		},
	}
	off := shedadapters.SwitchedAction{
		Enabled: func() (bool, error) { return false, nil },
		Act:     func(context.Context) error { return nil },
	}
	c.env.OpenWindow, c.env.SeedIDE, c.env.OpenIDE = off, off, off
	c.untilStop = inProcessUntilStop(t, c)
	return c
}

// stopEnvelope renders the --until-stop envelope of a step that ended with the run in state at producer, after historyLength history entries.
func stopEnvelope(t *testing.T, fields map[string]any, state, producer string, historyLength int) []byte {
	t.Helper()
	fields["state"] = state
	fields["loop"] = map[string]any{"current_producer": producer, "history_length": historyLength, "state": state}
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("encode the stop envelope: %v", err)
	}
	return raw
}

// inProcessUntilStop returns a step seam that runs c's shed in-process until it stops, in place of the detached --until-stop loop, and renders the envelope the driver parses.
// A stop is ok with the run's state and the loop's position; a run held by another process is the kind-busy error envelope; any other failure is an error envelope with no kind, which the driver hands back without a tackler.
func inProcessUntilStop(t *testing.T, c *battenCLI) func(ctx context.Context, attempt string) ([]byte, error) {
	t.Helper()
	return func(ctx context.Context, _ string) ([]byte, error) {
		shed, err := battenrecipe.New(c.env, c.shedPaths)
		if err != nil {
			return nil, err
		}
		result, runErr := shed.Run(ctx)
		switch {
		case errors.Is(runErr, shedengine.ErrShedBusy):
			return stopEnvelope(t, map[string]any{"ok": false, "kind": shedverbs.KindBusy, "error": runErr.Error()}, "", "", 0), nil
		case runErr != nil:
			status, _, err := c.readRunStatus()
			if err != nil {
				return nil, err
			}
			return stopEnvelope(t, map[string]any{"ok": false, "error": runErr.Error()}, string(status.State), status.CurrentProducer, len(status.History)), nil
		}
		return stopEnvelope(t, map[string]any{"ok": true, "reason": result.Reason}, string(result.Outcome), result.HaltedProducer, len(result.History)), nil
	}
}

// runCommand returns the run verb as "lyx batten run" mounts it: the shedverbs run command with its body replaced by driveRun.
func runCommand(c *battenCLI) *cobra.Command {
	cmd := battenVerbCommand(c, "run")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error { return c.driveRun(cmd) }
	return cmd
}

// writeStatus writes st as c's status file, unlocked -- the test owns the file outright before the
// verb ever runs.
func writeStatus(t *testing.T, c *battenCLI, st shedengine.Status) {
	t.Helper()
	if err := state.WriteJSON(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, st); err != nil {
		t.Fatalf("writeStatus: %v", err)
	}
}

// TestRunCmd_ResumeDispositions covers StateRunning, StateBlocked, StateFailed and StatePaused, each resuming silently from the persisted current producer, and an absent status file, which is a fresh start and not a refusal: every fake succeeds trivially, so the whole run completes and the verb reports success, with a history_length key matching the persisted history.
// A run whose status carries a pause_after naming the first row stops paused, and the driver hands it back with no tackler; a run whose row fails again ends in an error envelope carrying the status error.
func TestRunCmd_ResumeDispositions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		state       shedengine.State
		producer    string
		pauseAfter  string
		lockErr     error
		wantOutcome string
		wantError   string
	}{
		{name: "running", state: shedengine.StateRunning, wantOutcome: "done"},
		{name: "blocked", state: shedengine.StateBlocked, wantOutcome: "done"},
		{name: "failed", state: shedengine.StateFailed, wantOutcome: "done"},
		{name: "paused", state: shedengine.StatePaused, wantOutcome: "done"},
		{name: "absent_status_file", wantOutcome: "done"},
		{name: "pause_after_the_first_row", state: shedengine.StateRunning, pauseAfter: battenrecipe.NameWorktreeCreate, wantOutcome: "paused"},
		{name: "failed_again", state: shedengine.StateFailed, producer: battenrecipe.NameWorktreeTeardown, lockErr: errors.New("prime lock unavailable"), wantError: "prime lock unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := newFakeReceiver(t, nil)
			if tt.lockErr != nil {
				c.env.PrimeLock.Acquire = func() (func() error, bool, error) { return nil, false, tt.lockErr }
			}
			if tt.state != "" {
				producer := tt.producer
				if producer == "" {
					producer = battenrecipe.NameWorktreeCreate
				}
				writeStatus(t, c, shedengine.Status{
					CurrentProducer: producer,
					State:           tt.state,
					PauseAfter:      tt.pauseAfter,
				})
			}

			var out bytes.Buffer
			exitCode := clihelp.Execute(runCommand(c), &out, []string{c.slug})

			if tt.wantError != "" {
				if exitCode != 1 {
					t.Fatalf("run(%s) exit code = %d; want 1; output: %s", tt.name, exitCode, out.String())
				}
				if !strings.Contains(out.String(), `"ok":false`) || !strings.Contains(out.String(), tt.wantError) {
					t.Errorf("run(%s) output = %q; want an error envelope carrying %q", tt.name, out.String(), tt.wantError)
				}
				return
			}
			if exitCode != 0 {
				t.Fatalf("run(%s) exit code = %d; want 0; output: %s", tt.name, exitCode, out.String())
			}
			if !strings.Contains(out.String(), `"ok":true`) {
				t.Errorf("run(%s) output missing ok:true envelope; got: %q", tt.name, out.String())
			}

			// The envelope's history_length key matches the run's own persisted history length.
			var envelope map[string]any
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("decode envelope: %v; output: %s", err, out.String())
			}
			if envelope["outcome"] != tt.wantOutcome {
				t.Errorf("run(%s) envelope[\"outcome\"] = %v; want %q", tt.name, envelope["outcome"], tt.wantOutcome)
			}
			st, found, err := state.ReadJSONNoMkdir[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
			if err != nil || !found {
				t.Fatalf("read persisted status: found=%v err=%v", found, err)
			}
			if len(st.History) == 0 {
				t.Fatal("persisted history is empty; want the run to have appended at least one entry")
			}
			gotLen, ok := envelope["history_length"].(float64)
			if !ok {
				t.Fatalf("envelope[\"history_length\"] = %v (%T); want a number", envelope["history_length"], envelope["history_length"])
			}
			if int(gotLen) != len(st.History) {
				t.Errorf("envelope[\"history_length\"] = %v; want %d (the run's own persisted history length)", gotLen, len(st.History))
			}
		})
	}
}

// TestRunCmd_StateDoneRefusesNamingTheRunDir asserts a StateDone slug refuses on the envelope
// rather than silently re-running, naming the durable run directory to delete -- the one the
// status file the refusal gates on lives in, never the ephemeral scratch directory.
func TestRunCmd_StateDoneRefusesNamingTheRunDir(t *testing.T) {
	c := newFakeReceiver(t, nil)
	writeStatus(t, c, shedengine.Status{
		CurrentProducer: battenrecipe.NameWorktreeTeardown,
		State:           shedengine.StateDone,
	})

	var out bytes.Buffer
	exitCode := clihelp.Execute(runCommand(c), &out, []string{c.slug})

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d; want 1; output: %s", exitCode, out.String())
	}
	wantDir := shedrun.RunDir(c.location, c.slug)
	if !strings.Contains(out.String(), wantDir) {
		t.Errorf("run() output = %q; want it to name the durable run directory %q", out.String(), wantDir)
	}
	if strings.Contains(out.String(), BattenDir(c.location, c.slug)) {
		t.Errorf("run() output = %q; names the ephemeral scratch directory, which is not the gate", out.String())
	}
	// The same whole-abandon-path remedy the step verb names: the run directory and the task branch
	// the torn-down pair left, locally and on the remote.
	if !strings.Contains(out.String(), "locally and on the remote") {
		t.Errorf("run() output = %q; want the remedy to name the leftover task branch, locally and on the remote", out.String())
	}
}

// TestRunCmd_HeldRunLockRefusesNamingTheLockPath takes a lock in the test itself, before invoking the verb, and asserts the verb refuses on the envelope without waiting.
// A held run lock, whether the step meets it or the driver's recheck finds it, refuses naming the run lock path; a held driver lock refuses with the batten driver refusal.
func TestRunCmd_HeldRunLockRefusesNamingTheLockPath(t *testing.T) {
	tests := []struct {
		name string
		// heldLock returns the lock path the test holds.
		heldLock func(c *battenCLI) string
		// stepReportsBlocked replaces the in-process step with one that ends blocked, so the driver's recheck is what finds the held run lock.
		stepReportsBlocked bool
		want               func(c *battenCLI) string
	}{
		{
			name:     "the step finds the run lock held",
			heldLock: func(c *battenCLI) string { return c.shedPaths.LockPath },
			want:     func(c *battenCLI) string { return c.shedPaths.LockPath },
		},
		{
			name:               "the recheck finds the run running elsewhere",
			heldLock:           func(c *battenCLI) string { return c.shedPaths.LockPath },
			stepReportsBlocked: true,
			want:               func(c *battenCLI) string { return c.shedPaths.LockPath },
		},
		{
			name:     "a batten driver holds the driver lock",
			heldLock: func(c *battenCLI) string { return battenDriveLock(c.location, c.slug).Path },
			want:     func(*battenCLI) string { return battenDriveRefusal },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newFakeReceiver(t, nil)
			writeStatus(t, c, shedengine.Status{
				CurrentProducer: battenrecipe.NameWorktreeCreate,
				State:           shedengine.StateBlocked,
			})
			if tt.stepReportsBlocked {
				c.untilStop = func(context.Context, string) ([]byte, error) {
					return stopEnvelope(t, map[string]any{"ok": true}, string(shedengine.StateBlocked), battenrecipe.NameWorktreeCreate, 0), nil
				}
			}

			lockPath := tt.heldLock(c)
			if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
				t.Fatalf("create the lock directory: %v", err)
			}
			held, err := lock.AcquireWriteLock(lockPath)
			if err != nil {
				t.Fatalf("acquire the lock in test: %v", err)
			}
			t.Cleanup(func() { _ = held.Release() })

			var out bytes.Buffer
			exitCode := clihelp.Execute(runCommand(c), &out, []string{c.slug})

			if exitCode != 1 {
				t.Fatalf("run() exit code = %d; want 1; output: %s", exitCode, out.String())
			}
			if want := tt.want(c); !strings.Contains(out.String(), want) {
				t.Errorf("run() output = %q; want it to carry %q", out.String(), want)
			}
		})
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
			shutdown := func(ctx context.Context) (string, error) { return tt.abandonedSession, nil }
			c := newFakeReceiver(t, shutdown)
			writeStatus(t, c, shedengine.Status{
				CurrentProducer: battenrecipe.NameWorktreeTeardown,
				State:           shedengine.StateBlocked,
			})

			var out bytes.Buffer
			exitCode := clihelp.Execute(runCommand(c), &out, []string{c.slug})
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

// TestPauseCmd_AbsentFileRefuses asserts pause refuses over a slug whose per-slug directory
// exists but holds no status.json -- the absent-file precondition batten-absent-file-needs-an-
// existing-directory requires, since pause reaches state.UpdateJSON directly.
func TestPauseCmd_AbsentFileRefuses(t *testing.T) {
	c := newFakeReceiver(t, nil)

	var out bytes.Buffer
	exitCode := clihelp.Execute(battenVerbCommand(c, "pause"), &out, []string{c.slug})
	if exitCode != 1 {
		t.Fatalf("pause() exit code = %d; want 1; output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), "there is nothing running to pause") {
		t.Errorf("pause() output = %q; want it to name the absent-file refusal", out.String())
	}
	if !strings.Contains(out.String(), "lyx batten run") {
		t.Errorf("pause() output = %q; want it to name batten's own entry verb as the remedy", out.String())
	}
}

// TestStatusAndPause_ReadADurableStatusWhoseLockDirIsAbsent pins the cold-machine shape: the durable
// status file is present (it travelled with the pair) while the ephemeral lock directory is not (this
// machine never stepped the slug). Both read-only verbs must answer from the file rather than fail to
// open a lock in a directory nothing has created yet.
func TestStatusAndPause_ReadADurableStatusWhoseLockDirIsAbsent(t *testing.T) {
	for _, verb := range []string{"status", "pause"} {
		t.Run(verb, func(t *testing.T) {
			c := newFakeReceiver(t, nil)
			writeStatus(t, c, shedengine.Status{
				CurrentProducer: battenrecipe.NameRunShed,
				State:           shedengine.StateRunning,
				History:         []shedengine.HistoryEntry{},
			})
			c.shedPaths.StatusLockPath = filepath.Join(filepath.Dir(c.shedPaths.StatusPath), "scratch", "status.json.lock")

			var out bytes.Buffer
			exitCode := clihelp.Execute(battenVerbCommand(c, verb), &out, []string{c.slug})
			if exitCode != 0 {
				t.Fatalf("%s() exit code = %d; want 0; output: %s", verb, exitCode, out.String())
			}
			if verb == "status" && !strings.Contains(out.String(), `"found":true`) {
				t.Errorf("status() output = %q; want the durable status reported as found", out.String())
			}
			if verb != "pause" {
				return
			}

			// pause reports an envelope carrying exactly ok and status_file, and sets PauseRequested.
			var envelope map[string]any
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("decode envelope: %v; output: %s", err, out.String())
			}
			if len(envelope) != 2 || envelope["ok"] != true || envelope["status_file"] != c.shedPaths.StatusPath {
				t.Errorf("pause() envelope = %v; want exactly ok and status_file=%q", envelope, c.shedPaths.StatusPath)
			}
			st, found, err := state.ReadJSONNoMkdir[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
			if err != nil || !found {
				t.Fatalf("read persisted status: found=%v err=%v", found, err)
			}
			if !st.PauseRequested {
				t.Error("PauseRequested = false after pause(); want true")
			}
		})
	}
}

// TestStatusCmd_WatchOverAbsentFileExitsImmediately asserts --watch over a slug whose per-slug
// directory exists but holds no status.json exits immediately with the found: false envelope
// rather than entering the tail -- the absent-file disposition short-circuits before --watch is
// ever read, per the batten-absent-file-needs-an-existing-directory Shared Decision. The
// directory must already exist for this disposition to be reachable at all: newFakeReceiver's own
// t.TempDir() is that directory.
func TestStatusCmd_WatchOverAbsentFileExitsImmediately(t *testing.T) {
	c := newFakeReceiver(t, nil)

	// status exposes --watch and --interval, the two flags shedverbs' generic status body itself reads.
	statusCmd := battenVerbCommand(c, "status")
	for _, flag := range []string{"watch", "interval"} {
		if statusCmd.Flags().Lookup(flag) == nil {
			t.Errorf("status command is missing the --%s flag", flag)
		}
	}

	var out bytes.Buffer
	exitCode := clihelp.Execute(battenVerbCommand(c, "status"), &out, []string{"--watch", c.slug})
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

// TestRecentHistory asserts the status envelope's history is bounded to the most recent entries and
// reports whether anything was dropped.
func TestRecentHistory(t *testing.T) {
	entry := func(n int) shedengine.HistoryEntry {
		return shedengine.HistoryEntry{Producer: "Run-Shed", Outcome: shedengine.Stuck, Output: fmt.Sprint(n)}
	}
	build := func(count int) []shedengine.HistoryEntry {
		out := make([]shedengine.HistoryEntry, 0, count)
		for i := 0; i < count; i++ {
			out = append(out, entry(i))
		}
		return out
	}

	tests := []struct {
		name          string
		history       []shedengine.HistoryEntry
		wantLen       int
		wantTruncated bool
		wantLastOut   string
	}{
		{name: "nil_history_is_returned_untouched", history: nil, wantLen: 0},
		{name: "a_short_history_is_returned_whole", history: build(3), wantLen: 3, wantLastOut: "2"},
		{name: "exactly_at_the_cap_is_not_truncated", history: build(maxStatusHistoryEntries), wantLen: maxStatusHistoryEntries, wantLastOut: fmt.Sprint(maxStatusHistoryEntries - 1)},
		{
			name:          "a_full_watch_window_keeps_only_the_most_recent",
			history:       build(1440),
			wantLen:       maxStatusHistoryEntries,
			wantTruncated: true,
			wantLastOut:   "1439",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, truncated := recentHistory(tt.history)
			if len(got) != tt.wantLen {
				t.Errorf("recentHistory(%d entries) returned %d; want %d", len(tt.history), len(got), tt.wantLen)
			}
			if truncated != tt.wantTruncated {
				t.Errorf("recentHistory(%d entries) truncated = %v; want %v", len(tt.history), truncated, tt.wantTruncated)
			}
			if tt.wantLastOut != "" && got[len(got)-1].Output != tt.wantLastOut {
				t.Errorf("recentHistory(...) last entry output = %q; want the most recent entry %q", got[len(got)-1].Output, tt.wantLastOut)
			}
		})
	}
}

// TestReadStuckReason asserts the status verb recovers the producer-supplied reason battenshed
// wrote, and stays silent rather than guessing when there is none.
func TestReadStuckReason(t *testing.T) {
	scratchDir := t.TempDir()
	const producer = "Worktree-Create"
	const reason = `branch "crashcreate" already exists; switch a pair onto it with "lyx fabric checkout crashcreate"`

	if _, found := readStuckReason(scratchDir, producer); found {
		t.Fatalf("precondition: readStuckReason found a reason before any was written")
	}

	if err := os.WriteFile(battenshed.StuckReasonFile(scratchDir, producer), []byte(reason+"\n"), 0o644); err != nil {
		t.Fatalf("write stuck reason: %v", err)
	}

	got, found := readStuckReason(scratchDir, producer)
	if !found {
		t.Fatalf("readStuckReason(%q, %q) found = false; want the reason battenshed just wrote", scratchDir, producer)
	}
	if got != reason {
		t.Errorf("readStuckReason(...) = %q; want %q", got, reason)
	}

	if _, found := readStuckReason(scratchDir, "Some-Other-Row"); found {
		t.Error("readStuckReason reported a reason for a producer that wrote none")
	}
	if _, found := readStuckReason("", producer); found {
		t.Error("readStuckReason reported a reason for an empty scratch directory")
	}
}

// TestHaltedOverDoneChild asserts the flag appears only for a failed, blocked or paused batten over a found, done child, and names the run that closes the pair.
func TestHaltedOverDoneChild(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		batten     shedengine.State
		child      shedengine.State
		childFound bool
		wantFlag   bool
	}{
		{name: "failed batten over a done child", batten: shedengine.StateFailed, child: shedengine.StateDone, childFound: true, wantFlag: true},
		{name: "blocked batten over a done child", batten: shedengine.StateBlocked, child: shedengine.StateDone, childFound: true, wantFlag: true},
		{name: "paused batten over a done child", batten: shedengine.StatePaused, child: shedengine.StateDone, childFound: true, wantFlag: true},
		{name: "running batten over a done child", batten: shedengine.StateRunning, child: shedengine.StateDone, childFound: true},
		{name: "done batten over a done child", batten: shedengine.StateDone, child: shedengine.StateDone, childFound: true},
		{name: "failed batten over a blocked child", batten: shedengine.StateFailed, child: shedengine.StateBlocked, childFound: true},
		{name: "failed batten over an unread child", batten: shedengine.StateFailed, child: shedengine.StateDone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, flagged := haltedOverDoneChild("some-slug", shedengine.Status{State: tt.batten}, shedengine.Status{State: tt.child}, tt.childFound)

			if flagged != tt.wantFlag {
				t.Fatalf("haltedOverDoneChild(...) flagged = %v; want %v", flagged, tt.wantFlag)
			}
			if !tt.wantFlag {
				return
			}
			for _, want := range []string{"batten is " + string(tt.batten), `"lyx batten run some-slug"`, "lands the run through Land"} {
				if !strings.Contains(got, want) {
					t.Errorf("flag %q lacks %q", got, want)
				}
			}
		})
	}
}
