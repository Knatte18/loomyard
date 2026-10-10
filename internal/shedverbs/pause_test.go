// pause_test.go covers the generic pause body: PauseRequested is set on the persisted status,
// both told absent-file wordings are reported, the bare success envelope carries exactly the one
// key status_file, and the --before, --after and --clear conditions.

package shedverbs

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

func pauseTexts() VerbTexts {
	return VerbTexts{Pause: VerbText{Use: "pause", Short: "pause the fake shed"}}
}

// TestPauseCmd_SetsPauseRequested asserts PauseRequested is set true on the persisted status, and
// the success envelope carries exactly the one key status_file.
func TestPauseCmd_SetsPauseRequested(t *testing.T) {
	paths := newTestPaths(t)
	seedStatus(t, paths, "Only")

	spec := &Spec{
		StatusPath:         paths.StatusPath,
		StatusLockPath:     paths.StatusLockPath,
		PauseAbsentMessage: "loom: no status file; nothing running to pause",
	}

	env, code := execEnvelope(t, pauseCmd(pauseTexts(), spec), nil)
	if code != 0 {
		t.Fatalf("exit code = %d; want 0", code)
	}
	wantKeys := map[string]bool{"status_file": true, "ok": true}
	if len(env) != len(wantKeys) {
		t.Fatalf("envelope keys = %v; want exactly %v", env, wantKeys)
	}
	if env["status_file"] != paths.StatusPath {
		t.Errorf("status_file = %v; want %q", env["status_file"], paths.StatusPath)
	}

	st, found, err := state.ReadJSONStrict[shedengine.Status](paths.StatusPath, paths.StatusLockPath)
	if err != nil || !found {
		t.Fatalf("re-read status: found=%v err=%v", found, err)
	}
	if !st.PauseRequested {
		t.Error("PauseRequested = false; want true after pause")
	}
}

// TestPauseCmd_AbsentFile covers both shipped absent-file wordings: pause reports the told message verbatim when the status file does not exist.
// With EnsureStatusLockDir over a never-created lock parent, the parent is created and the call proceeds to the told message rather than failing in lock acquisition.
func TestPauseCmd_AbsentFile(t *testing.T) {
	tests := []struct {
		name                string
		message             string
		ensureStatusLockDir bool
	}{
		{name: "LoomWording", message: "loom: no status file at /x; there is nothing running to pause -- run \"lyx loom start\" first to bootstrap this task"},
		{name: "BattenWording", message: "battencli: no status file at /x; nothing is running for this slug"},
		{name: "EnsureStatusLockDirCreatesTheParent", message: "loom: nothing running to pause", ensureStatusLockDir: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := newTestPaths(t)
			if tt.ensureStatusLockDir {
				paths.StatusLockPath = filepath.Join(filepath.Dir(paths.StatusLockPath), "ephemeral", "status.lock")
			}
			// Deliberately never seed a status file.

			spec := &Spec{
				StatusPath:          paths.StatusPath,
				StatusLockPath:      paths.StatusLockPath,
				DecodeErrPrefix:     "loom:",
				EnsureStatusLockDir: tt.ensureStatusLockDir,
				PauseAbsentMessage:  tt.message,
			}

			env, code := execEnvelope(t, pauseCmd(pauseTexts(), spec), nil)
			if code != 1 {
				t.Fatalf("exit code = %d; want 1 (the absent-file message, not a lock failure): %v", code, env)
			}
			if env["error"] != tt.message {
				t.Errorf("error = %v; want %q", env["error"], tt.message)
			}
			if tt.ensureStatusLockDir {
				if _, err := os.Stat(filepath.Dir(paths.StatusLockPath)); err != nil {
					t.Errorf("status lock parent directory was not created: %v", err)
				}
			}
		})
	}
}

// TestPauseCmd_Conditions drives --before, --after and --clear through pauseCmd, asserting each row's envelope keys and the status file re-read after the verb.
func TestPauseCmd_Conditions(t *testing.T) {
	const (
		wantContradiction = "shedverbs: pause --clear cannot be combined with --before or --after; way forward: run pause --clear alone, then pause --before or --after"
		wantUnknown       = "shedverbs: pause target Nope names no producer; way forward: re-run pause with --before or --after naming one of: One, Two"
	)
	tests := []struct {
		name string
		args []string
		// seedBefore, seedAfter and seedRequested are the conditions and bare request recorded before the verb runs.
		seedBefore, seedAfter string
		seedRequested         bool
		wantError             string
		wantEnvelope          map[string]any
		wantBefore, wantAfter string
		wantRequested         bool
	}{
		{name: "set before", args: []string{"--before", "Two"},
			wantEnvelope: map[string]any{"before": "Two", "after": ""}, wantBefore: "Two"},
		{name: "set after", args: []string{"--after", "One"},
			wantEnvelope: map[string]any{"before": "", "after": "One"}, wantAfter: "One"},
		{name: "replace names the replaced target", args: []string{"--before", "Two"}, seedBefore: "One",
			wantEnvelope: map[string]any{"before": "Two", "after": "", "replaced_before": "One"}, wantBefore: "Two"},
		{name: "clear removes both and leaves a recorded bare request", args: []string{"--clear"},
			seedBefore: "One", seedAfter: "Two", seedRequested: true,
			wantEnvelope:  map[string]any{"before": "", "after": "", "removed_before": "One", "removed_after": "Two"},
			wantRequested: true},
		{name: "clear with a condition flag is refused", args: []string{"--clear", "--before", "One"},
			seedBefore: "Two", wantError: wantContradiction, wantBefore: "Two"},
		{name: "unknown target is refused", args: []string{"--after", "Nope"},
			seedAfter: "One", wantError: wantUnknown, wantAfter: "One"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := newTestPaths(t)
			if err := state.WriteJSON(paths.StatusPath, paths.StatusLockPath, shedengine.Status{
				CurrentProducer: "One",
				State:           shedengine.StateRunning,
				PauseBefore:     tt.seedBefore,
				PauseAfter:      tt.seedAfter,
				PauseRequested:  tt.seedRequested,
				History:         []shedengine.HistoryEntry{},
			}); err != nil {
				t.Fatalf("seed status: %v", err)
			}
			spec := &Spec{
				StatusPath:         paths.StatusPath,
				StatusLockPath:     paths.StatusLockPath,
				PauseAbsentMessage: "no status file",
				Routing:            shedengine.Routing{Producers: []shedengine.ProducerDef{stubRow("One"), stubRow("Two")}},
			}

			env, code := execEnvelope(t, pauseCmd(pauseTexts(), spec), tt.args)
			if tt.wantError != "" {
				if code != 1 || env["error"] != tt.wantError {
					t.Errorf("exit code, error = %d, %v; want 1, %q", code, env["error"], tt.wantError)
				}
			} else {
				if code != 0 {
					t.Fatalf("exit code = %d; want 0: %v", code, env)
				}
				want := map[string]any{"ok": true, "status_file": paths.StatusPath}
				for key, value := range tt.wantEnvelope {
					want[key] = value
				}
				if !reflect.DeepEqual(env, want) {
					t.Errorf("envelope = %v; want %v", env, want)
				}
			}

			got, found, err := state.ReadJSONStrict[shedengine.Status](paths.StatusPath, paths.StatusLockPath)
			if err != nil || !found {
				t.Fatalf("re-read status: found=%v err=%v", found, err)
			}
			if got.PauseBefore != tt.wantBefore || got.PauseAfter != tt.wantAfter || got.PauseRequested != tt.wantRequested {
				t.Errorf("persisted PauseBefore, PauseAfter, PauseRequested = %q, %q, %v; want %q, %q, %v", got.PauseBefore, got.PauseAfter, got.PauseRequested, tt.wantBefore, tt.wantAfter, tt.wantRequested)
			}
		})
	}
}
