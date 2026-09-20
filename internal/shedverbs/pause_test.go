// pause_test.go covers the generic pause body: PauseRequested is set on the persisted status,
// both told absent-file wordings are reported, and the success envelope carries exactly the one
// key status_file.

package shedverbs

import (
	"os"
	"path/filepath"
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

// TestPauseCmd_AbsentFile covers both shipped absent-file wordings: pause reports the told message
// verbatim when the status file does not exist.
func TestPauseCmd_AbsentFile(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{name: "LoomWording", message: "loom: no status file at /x; there is nothing running to pause -- run \"lyx loom start\" first to bootstrap this task"},
		{name: "BattenWording", message: "battencli: no status file at /x; nothing is running for this slug"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := newTestPaths(t)
			// Deliberately never seed a status file.

			spec := &Spec{
				StatusPath:         paths.StatusPath,
				StatusLockPath:     paths.StatusLockPath,
				PauseAbsentMessage: tt.message,
			}

			env, code := execEnvelope(t, pauseCmd(pauseTexts(), spec), nil)
			if code != 1 {
				t.Fatalf("exit code = %d; want 1", code)
			}
			if env["error"] != tt.message {
				t.Errorf("error = %v; want %q", env["error"], tt.message)
			}
		})
	}
}

// TestPauseCmd_EnsureStatusLockDir asserts pause's own MkdirAll call matches card 13's two-argument
// ensureStatusLockDir signature: when EnsureStatusLockDir is true over a never-created parent, the
// parent is created and the call proceeds to the told absent-file message rather than failing in
// lock acquisition.
func TestPauseCmd_EnsureStatusLockDir(t *testing.T) {
	paths := newTestPaths(t)
	paths.StatusLockPath = filepath.Join(filepath.Dir(paths.StatusLockPath), "ephemeral", "status.lock")

	spec := &Spec{
		StatusPath:          paths.StatusPath,
		StatusLockPath:      paths.StatusLockPath,
		DecodeErrPrefix:     "loom:",
		EnsureStatusLockDir: true,
		PauseAbsentMessage:  "loom: nothing running to pause",
	}

	env, code := execEnvelope(t, pauseCmd(pauseTexts(), spec), nil)
	if code != 1 {
		t.Fatalf("exit code = %d; want 1 (the absent-file message, not a lock failure): %v", code, env)
	}
	if env["error"] != spec.PauseAbsentMessage {
		t.Errorf("error = %v; want %q", env["error"], spec.PauseAbsentMessage)
	}
	if _, err := os.Stat(filepath.Dir(paths.StatusLockPath)); err != nil {
		t.Errorf("status lock parent directory was not created: %v", err)
	}
}
