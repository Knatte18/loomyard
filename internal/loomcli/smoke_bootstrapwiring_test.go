//go:build smoke

// smoke_bootstrapwiring_test.go covers two bootstrap behaviours that are correct in their own helper
// and were wrong in production anyway, which is the one shape a Tier 1 test over that helper can
// never catch: a helper nothing calls, and a lock whose parent directory nothing creates.
//
// Both defects were found by crucible round 1 driving a real hub, and neither is visible from a
// hermetic fixture. ensureFrictionDirAfterSeed had four green unit tests while being called from
// nowhere at all, so only a test that goes through the real bootstrap can tell the two states apart.
// The status/pause lock-parent hole is the mirror image: both verbs' own "no status file" messages
// are exercised by unit tests that build the lock path under a t.TempDir() which already exists,
// which is precisely the condition a never-bootstrapped pair does not satisfy.
//
// Like its siblings, every test here spawns ZERO real LLM subprocesses: the fixture wires
// providerlessShuttleConfig (see its doc comment), and each test dispatches at most the two pure-Go
// precondition rows, never an LLM row.
package loomcli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// plantFrictionNote writes a friction note named name inside loc's Tier 2 friction directory --
// composed through the production accessor, never a hand-built literal -- creating the directory if
// it is absent, and returns the path it wrote.
func plantFrictionNote(t *testing.T, loc *lyxcwd.Location, name string) string {
	t.Helper()
	path := filepath.Join(loomengine.LoomFrictionDir(loc), name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create friction directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("# a friction note\n"), 0o644); err != nil {
		t.Fatalf("write friction note: %v", err)
	}
	return path
}

// TestSmokeBootstrap_FirstSeedClearsFrictionNotesAndReentryKeepsThem is the regression guard for the
// once-per-task friction clear that never shipped.
//
// ensureFrictionDirAfterSeed was written, documented, and unit-tested in run.go, and called from
// nowhere: neither runCmd's RunE nor seedAndCommitBootstrap reached it. Its four tests were green
// over an orphan. The consequence, reproduced live against a real hub in crucible round 1: notes
// left in .lyx/loom/friction/ by an earlier task, or by an earlier run that never reached a
// reflection trigger, survived a genuine first seed and were handed to the NEXT task's reflection
// agent as that task's own friction -- which then filed a GitHub issue about them.
//
// Both branches are asserted through the real bootstrap, because the branch is not the thing that
// was broken; reaching it was. `lyx loom step` is the driving verb rather than `lyx loom run`
// precisely because `step` spawns no driver: `run` delegates to `drive`, which calls
// friction.EnsureDir itself and would mask an unwired clear behind a directory that exists anyway.
func TestSmokeBootstrap_FirstSeedClearsFrictionNotesAndReentryKeepsThem(t *testing.T) {
	exe := buildLyxBinary(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	stalePath := plantFrictionNote(t, loc, "left-over-from-an-earlier-task.md")

	// First `step`: a genuine first seed. It dispatches Preflight, a pure-Go row, and nothing else.
	stdout, _, err := runLoomCLINoFatal(exe, worktree, 60*time.Second, "loom", "step")
	if err != nil {
		t.Fatalf("first loom step: %v; output: %s", err, stdout)
	}

	if _, statErr := os.Stat(stalePath); !os.IsNotExist(statErr) {
		t.Errorf("stale friction note still present after a genuine first seed (stat err=%v); want it cleared -- a fresh task must not inherit an earlier one's notes", statErr)
	}
	frictionDir := loomengine.LoomFrictionDir(loc)
	if info, statErr := os.Stat(frictionDir); statErr != nil || !info.IsDir() {
		t.Errorf("friction directory %q after first seed: stat err=%v; want it recreated as a directory", frictionDir, statErr)
	}

	// Second `step`: an ErrSeedExists re-entry over the same task. A resume's notes are the ones most
	// worth reading, so this branch must leave them exactly where they are.
	resumePath := plantFrictionNote(t, loc, "written-during-this-task.md")
	stdout, _, err = runLoomCLINoFatal(exe, worktree, 60*time.Second, "loom", "step")
	if err != nil {
		t.Fatalf("second loom step: %v; output: %s", err, stdout)
	}

	if _, statErr := os.Stat(resumePath); statErr != nil {
		t.Errorf("friction note written during this task is gone after a re-entry (stat err=%v); want it kept -- only a genuine first seed clears", statErr)
	}
}

// TestSmokeStep_RecordsCleanHandoffMarkerMatchingPersistedStatus is the wiring guard for the step
// clean-handoff marker (crucible round 2, R2-F1): recordStepHandoff and its consume/detect halves
// are unit-tested in stephandoff_test.go, but a helper nothing calls stays green over an orphan --
// the exact shape F-1 shipped in -- so this test drives the real `lyx loom step` binary and asserts
// the marker landed beside the ephemeral tree's other loom files, matching the persisted status.
//
// Without the marker, a completed step leaves state running with a live history and a free run
// lock, which is byte-identical to a mid-run driver death: the next `lyx loom drive` with
// selfreport on then files a spurious crash-resume GitHub issue for a task in which nothing
// crashed.
//
// Like its siblings, this test spawns zero real LLM subprocesses: it dispatches at most the
// pure-Go precondition rows.
func TestSmokeStep_RecordsCleanHandoffMarkerMatchingPersistedStatus(t *testing.T) {
	exe := buildLyxBinary(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	stdout, _, err := runLoomCLINoFatal(exe, worktree, 60*time.Second, "loom", "step")
	if err != nil {
		t.Fatalf("loom step: %v; output: %s", err, stdout)
	}

	persisted, found, err := state.ReadJSONStrict[shedengine.Status](loomengine.LoomStatusFile(loc), loomengine.LoomStatusLock(loc))
	if err != nil || !found {
		t.Fatalf("read persisted status after step: found=%v err=%v", found, err)
	}

	marker, found, err := state.ReadJSONStrict[stepHandoffMarker](loomengine.LoomStepHandoff(loc), loomengine.LoomStepHandoffLock(loc))
	if err != nil {
		t.Fatalf("read step clean-handoff marker: %v", err)
	}
	if !found {
		t.Fatalf("no clean-handoff marker at %s after a completed step; want one matching the persisted status -- without it the next drive files a spurious crash-resume", loomengine.LoomStepHandoff(loc))
	}
	if marker.HistoryLength != len(persisted.History) || marker.State != string(persisted.State) {
		t.Errorf("clean-handoff marker = {history %d, state %q}; want {history %d, state %q} to match the persisted status",
			marker.HistoryLength, marker.State, len(persisted.History), persisted.State)
	}
}

// TestSmokeStatusAndPause_OnNeverBootstrappedPairNameTheRemedy is the regression guard for both
// verbs' own "no status file" messages having been unreachable.
//
// The status file is durable (_lyx/loom/status.json) but its advisory lock is ephemeral
// (.lyx/loom/status.json.lock), a different directory tree that nothing creates until a bootstrap
// runs. internal/lock opens with O_CREATE and never creates a parent, so on a freshly-added pair both
// verbs failed inside lock acquisition, before the `found` value they branch on was ever produced --
// and leaked an internal "no such file or directory" path instead of their own remedy. Neither verb's
// unit tests could see it: they build both paths under a t.TempDir() that already exists.
//
// This is the ly-supervise skill's literal first instruction ("take one `lyx loom status` read...
// This baseline always exists"), so the skill's opening move failed on every brand-new task.
func TestSmokeStatusAndPause_OnNeverBootstrappedPairNameTheRemedy(t *testing.T) {
	exe := buildLyxBinary(t)
	_, _, worktree, _ := newWiredPairFixture(t)

	tests := []struct {
		name string
		verb string
	}{
		{"Status", "status"},
		{"Pause", "pause"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, code, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", tt.verb)
			if err != nil {
				t.Fatalf("loom %s: %v; output: %s", tt.verb, err, stdout)
			}
			if code != 1 {
				t.Fatalf("loom %s on a never-bootstrapped pair exit = %d; want 1", tt.verb, code)
			}

			var envelope struct {
				OK    bool   `json:"ok"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &envelope); err != nil {
				t.Fatalf("decode loom %s refusal envelope %q: %v", tt.verb, stdout, err)
			}
			if envelope.OK {
				t.Fatalf("loom %s refusal envelope ok = true; want false: %s", tt.verb, stdout)
			}
			if !strings.Contains(envelope.Error, "no status file") {
				t.Errorf("loom %s error = %q; want it to say there is no status file", tt.verb, envelope.Error)
			}
			if !strings.Contains(envelope.Error, "loom run") {
				t.Errorf("loom %s error = %q; want it to name the bootstrap verb as the remedy", tt.verb, envelope.Error)
			}
			if strings.Contains(envelope.Error, ".lock") {
				t.Errorf("loom %s error = %q; want no internal lock path leaked to the operator", tt.verb, envelope.Error)
			}
		})
	}
}
