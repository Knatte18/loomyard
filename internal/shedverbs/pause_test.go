// pause_test.go covers the generic pause body: PauseRequested is set on the persisted status,
// both told absent-file wordings are reported, the bare success envelope carries exactly the one
// key status_file, and the --before, --after and --clear conditions.

package shedverbs

import (
	"bytes"
	"errors"
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

	st, found, err := state.ReadJSONNoMkdir[shedengine.Status](paths.StatusPath, paths.StatusLockPath)
	if err != nil || !found {
		t.Fatalf("re-read status: found=%v err=%v", found, err)
	}
	if !st.PauseRequested {
		t.Error("PauseRequested = false; want true after pause")
	}
	if st.PauseRequestedBy != "" {
		t.Errorf("PauseRequestedBy = %q; want none without --by", st.PauseRequestedBy)
	}

	_, code = execEnvelope(t, pauseCmd(pauseTexts(), spec), []string{"--by", "operator"})
	if code != 0 {
		t.Fatalf("exit code with --by = %d; want 0", code)
	}
	st, _, err = state.ReadJSONNoMkdir[shedengine.Status](paths.StatusPath, paths.StatusLockPath)
	if err != nil {
		t.Fatalf("re-read status after --by: %v", err)
	}
	if !st.PauseRequested || st.PauseRequestedBy != "operator" {
		t.Errorf("PauseRequested, PauseRequestedBy = %v, %q; want true, operator", st.PauseRequested, st.PauseRequestedBy)
	}

	_, code = execEnvelope(t, pauseCmd(pauseTexts(), spec), nil)
	if code != 0 {
		t.Fatalf("exit code with a repeat bare pause = %d; want 0", code)
	}
	st, _, err = state.ReadJSONNoMkdir[shedengine.Status](paths.StatusPath, paths.StatusLockPath)
	if err != nil {
		t.Fatalf("re-read status after a repeat bare pause: %v", err)
	}
	if st.PauseRequestedBy != "operator" {
		t.Errorf("PauseRequestedBy = %q after a repeat bare pause; want the earlier request's operator", st.PauseRequestedBy)
	}

	// A bare --by under a refusing BarePause hook writes nothing: the hook still refuses first.
	refusing := newTestPaths(t)
	seedStatus(t, refusing, "Only")
	before, err := os.ReadFile(refusing.StatusPath)
	if err != nil {
		t.Fatalf("read seeded status: %v", err)
	}
	refusingSpec := &Spec{
		StatusPath:         refusing.StatusPath,
		StatusLockPath:     refusing.StatusLockPath,
		PauseAbsentMessage: "no status file",
		Hooks:              Hooks{BarePause: func(shedengine.Status) error { return errors.New("bare pause refused") }},
	}
	env, code = execEnvelope(t, pauseCmd(pauseTexts(), refusingSpec), []string{"--by", "operator"})
	if code != 1 || env["error"] != "bare pause refused" {
		t.Errorf("exit code, error = %d, %v; want 1, the hook's refusal", code, env["error"])
	}
	if after, _ := os.ReadFile(refusing.StatusPath); !bytes.Equal(before, after) {
		t.Errorf("status file changed under a refused bare --by: %s -> %s", before, after)
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
		wantBareRefusal   = "bare pause refused"
		wantShedRefusal   = "pause --by does not take shed, the label of lyx's own automatic steps; way forward: re-run pause with your own label, such as --by operator"
		pausedReason      = "pause requested by operator"
	)
	tests := []struct {
		name string
		args []string
		// seedBefore, seedAfter and seedRequested are the conditions and bare request recorded before the verb runs.
		seedBefore, seedAfter string
		seedRequested         bool
		// seedBeforeBy and seedAfterBy are the condition labels recorded before the verb runs.
		seedBeforeBy, seedAfterBy string
		// seedPaused records the status as paused with pausedReason before the verb runs.
		seedPaused bool
		// barePause is the spec's BarePause hook, nil for a spec that sets none.
		barePause             func(shedengine.Status) error
		wantError             string
		wantEnvelope          map[string]any
		wantBefore, wantAfter string
		wantRequested         bool
		// wantBeforeBy and wantAfterBy are the persisted condition labels.
		wantBeforeBy, wantAfterBy string
		// wantReason is the persisted pause_reason.
		wantReason string
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
		{name: "a bare pause under a refusing hook is refused and records nothing", args: nil,
			barePause: func(shedengine.Status) error { return errors.New(wantBareRefusal) },
			wantError: wantBareRefusal},
		{name: "a condition under a refusing hook is recorded", args: []string{"--after", "One"},
			barePause:    func(shedengine.Status) error { return errors.New(wantBareRefusal) },
			wantEnvelope: map[string]any{"before": "", "after": "One"}, wantAfter: "One"},
		{name: "before with a label stores it", args: []string{"--before", "Two", "--by", "orch"},
			wantEnvelope: map[string]any{"before": "Two", "after": ""}, wantBefore: "Two", wantBeforeBy: "orch"},
		{name: "after with a label stores it", args: []string{"--after", "One", "--by", "orch"},
			wantEnvelope: map[string]any{"before": "", "after": "One"}, wantAfter: "One", wantAfterBy: "orch"},
		{name: "a replacing condition without a label drops the recorded label", args: []string{"--before", "Two"},
			seedBefore: "One", seedBeforeBy: "operator",
			wantEnvelope: map[string]any{"before": "Two", "after": "", "replaced_before": "One"}, wantBefore: "Two"},
		{name: "clear drops both condition labels", args: []string{"--clear", "--by", "orch"},
			seedBefore: "One", seedAfter: "Two", seedBeforeBy: "operator", seedAfterBy: "orch",
			wantEnvelope: map[string]any{"before": "", "after": "", "removed_before": "One", "removed_after": "Two"}},
		{name: "by shed is refused and writes nothing", args: []string{"--before", "Two", "--by", "shed"},
			seedAfter: "One", seedAfterBy: "operator", wantError: wantShedRefusal, wantAfter: "One", wantAfterBy: "operator"},
		{name: "a condition over a paused status keeps its reason", args: []string{"--before", "Two"},
			seedPaused:   true,
			wantEnvelope: map[string]any{"before": "Two", "after": ""}, wantBefore: "Two", wantReason: pausedReason},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := newTestPaths(t)
			seeded := shedengine.Status{
				CurrentProducer: "One",
				State:           shedengine.StateRunning,
				PauseBefore:     tt.seedBefore,
				PauseAfter:      tt.seedAfter,
				PauseBeforeBy:   tt.seedBeforeBy,
				PauseAfterBy:    tt.seedAfterBy,
				PauseRequested:  tt.seedRequested,
				History:         []shedengine.HistoryEntry{},
			}
			if tt.seedPaused {
				seeded.State = shedengine.StatePaused
				seeded.PauseReason = pausedReason
			}
			if err := state.WriteJSON(paths.StatusPath, paths.StatusLockPath, seeded); err != nil {
				t.Fatalf("seed status: %v", err)
			}
			seededBytes, err := os.ReadFile(paths.StatusPath)
			if err != nil {
				t.Fatalf("read seeded status: %v", err)
			}
			spec := &Spec{
				StatusPath:         paths.StatusPath,
				StatusLockPath:     paths.StatusLockPath,
				PauseAbsentMessage: "no status file",
				Routing:            shedengine.Routing{Producers: []shedengine.ProducerDef{stubRow("One"), stubRow("Two")}},
				Hooks:              Hooks{BarePause: tt.barePause},
			}

			env, code := execEnvelope(t, pauseCmd(pauseTexts(), spec), tt.args)
			if tt.wantError != "" {
				if code != 1 || env["error"] != tt.wantError {
					t.Errorf("exit code, error = %d, %v; want 1, %q", code, env["error"], tt.wantError)
				}
				if after, _ := os.ReadFile(paths.StatusPath); !bytes.Equal(seededBytes, after) {
					t.Errorf("status file changed on a refusal: %s -> %s", seededBytes, after)
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

			got, found, err := state.ReadJSONNoMkdir[shedengine.Status](paths.StatusPath, paths.StatusLockPath)
			if err != nil || !found {
				t.Fatalf("re-read status: found=%v err=%v", found, err)
			}
			if got.PauseBefore != tt.wantBefore || got.PauseAfter != tt.wantAfter || got.PauseRequested != tt.wantRequested {
				t.Errorf("persisted PauseBefore, PauseAfter, PauseRequested = %q, %q, %v; want %q, %q, %v", got.PauseBefore, got.PauseAfter, got.PauseRequested, tt.wantBefore, tt.wantAfter, tt.wantRequested)
			}
			if got.PauseBeforeBy != tt.wantBeforeBy || got.PauseAfterBy != tt.wantAfterBy || got.PauseReason != tt.wantReason {
				t.Errorf("persisted PauseBeforeBy, PauseAfterBy, PauseReason = %q, %q, %q; want %q, %q, %q", got.PauseBeforeBy, got.PauseAfterBy, got.PauseReason, tt.wantBeforeBy, tt.wantAfterBy, tt.wantReason)
			}
		})
	}
}
