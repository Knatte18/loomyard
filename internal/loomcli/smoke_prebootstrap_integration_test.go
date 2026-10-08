//go:build integration

// smoke_prebootstrap_integration_test.go drives the built lyx binary and the production seed primitives against real hub pairs in the states before a run bootstraps, and when the reed config is bad.
// None of these behaviors reaches a tmux server: the refusals come before reed comes up, and the seed commit is plain git.

package loomcli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/preflight"
)

// TestLoomPreBootstrapPair walks one never-bootstrapped pair through the pre-bootstrap states in this order, each step building on the one before:
// no seed, a seed commit, then a second pair added to and removed from the same hub.
func TestLoomPreBootstrapPair(t *testing.T) {
	t.Parallel()

	exe := sharedLyxBinary(t)
	hub, loc, worktree, slug := newWiredPairFixture(t)
	recordsDir := fabricengine.RecordsWorktree(loc)

	// The run verb on a never-seeded pair refuses on the envelope with a message naming the bootstrap verb, writes no driver log, and leaves the records worktree clean.
	t.Run("run on a never-seeded pair refuses and writes nothing", func(t *testing.T) {
		beforeCount := gitkit.RevListCount(t, recordsDir, "HEAD")

		stdout, code, err := runLoomCLINoFatal(exe, worktree, 15*time.Second, "loom", "run")
		if err != nil {
			t.Fatalf("loom run: %v; output: %s", err, stdout)
		}
		if code != 1 {
			t.Fatalf("loom run on a never-seeded pair = %d; want 1", code)
		}

		var envelope struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &envelope); err != nil {
			t.Fatalf("decode refusal envelope %q: %v", stdout, err)
		}
		if envelope.OK {
			t.Fatalf("refusal envelope ok = true; want false: %s", stdout)
		}
		if !strings.Contains(envelope.Error, "loom start") {
			t.Errorf("refusal error = %q; want it to name the bootstrap verb", envelope.Error)
		}

		if _, err := os.Stat(loomengine.LoomDriverLog(loc)); !os.IsNotExist(err) {
			t.Errorf("driver log exists after a never-seeded refusal (stat err=%v); want none written", err)
		}

		clean, reason, err := fabricengine.Clean(loc)
		if err != nil {
			t.Fatalf("fabricengine.Clean: %v", err)
		}
		if !clean {
			t.Errorf("fabricengine.Clean() = (false, %q); want the records worktree left clean after the refusal", reason)
		}
		if afterCount := gitkit.RevListCount(t, recordsDir, "HEAD"); afterCount != beforeCount {
			t.Errorf("records commit count changed from %d to %d; want unchanged on a pre-flight refusal", beforeCount, afterCount)
		}
	})

	// The seed-then-commit mechanism driven directly, not through the full bootstrap: once a driver runs, its persists rewrite the status file's working tree without committing, which would dirty the records worktree for a reason unrelated to the ordering this step pins.
	t.Run("seed commit leaves the records worktree clean and touches only the status file", func(t *testing.T) {
		beforeCount := gitkit.RevListCount(t, recordsDir, "HEAD")

		seedAndCommitStatus(t, loc, slug)

		clean, reason, err := fabricengine.Clean(loc)
		if err != nil {
			t.Fatalf("fabricengine.Clean: %v", err)
		}
		if !clean {
			t.Errorf("fabricengine.Clean() = (false, %q); want clean immediately after the seed commit", reason)
		}

		afterCount := gitkit.RevListCount(t, recordsDir, "HEAD")
		if afterCount != beforeCount+1 {
			t.Errorf("records commit count = %d; want exactly %d (the single seed commit)", afterCount, beforeCount+1)
		}
		wantFiles := []string{smokeStatusRel(loc)}
		if changed := recordsHeadChangedFiles(t, recordsDir); !slices.Equal(changed, wantFiles) {
			t.Errorf("records HEAD changed files = %v; want exactly %v", changed, wantFiles)
		}

		report, _, err := preflight.Check(worktree)
		if err != nil {
			t.Fatalf("preflight.Check: %v", err)
		}
		if report.Has(preflight.CheckWorktreeClean) {
			t.Errorf("Check report still carries CheckWorktreeClean after the seed commit; the bootstrap must reach past the first precondition row rather than block on it")
		}
	})

	// A second pair of the same hub: the run launcher exists in the per-slug launcher directory after the add and is gone after the matching remove.
	t.Run("run launcher exists after a pair is added and is gone after the remove", func(t *testing.T) {
		const launcherSlug = "loom-smoke-launcher"
		hubforge.AddPair(t, hub, launcherSlug)

		ext := ".sh"
		if runtime.GOOS == "windows" {
			ext = ".cmd"
		}
		runLauncherPath := filepath.Join(hub.PairLauncherDir(launcherSlug), "run"+ext)
		if _, err := os.Stat(runLauncherPath); err != nil {
			t.Fatalf("run launcher missing after add: %v", err)
		}

		if _, err := hub.Topology.Remove(hub.Location, launcherSlug, false, false); err != nil {
			t.Fatalf("Remove(%s): %v", launcherSlug, err)
		}
		if _, err := os.Stat(runLauncherPath); !os.IsNotExist(err) {
			t.Fatalf("run launcher still present after remove (stat err=%v); want it gone", err)
		}
	})
}

// TestLoomStatusAndPauseOnNeverBootstrappedPair asserts the status and pause verbs on a seeded pair with no status file name the remedy.
// A seed must be present so the verbs reach the lock-parent-directory bug this guards: the run-id arm refuses a seedless run before either verb's own absent-status-file check runs.
func TestLoomStatusAndPauseOnNeverBootstrappedPair(t *testing.T) {
	t.Parallel()

	exe := sharedLyxBinary(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	seedGoDriverRun(t, loc)

	for _, verb := range []string{"status", "pause"} {
		t.Run(verb, func(t *testing.T) {
			stdout, code, err := runLoomCLINoFatal(exe, worktree, 30*time.Second, "loom", verb)
			if err != nil {
				t.Fatalf("loom %s: %v; output: %s", verb, err, stdout)
			}
			if code != 1 {
				t.Fatalf("loom %s on a never-bootstrapped pair exit = %d; want 1", verb, code)
			}

			var envelope struct {
				OK    bool   `json:"ok"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &envelope); err != nil {
				t.Fatalf("decode loom %s refusal envelope %q: %v", verb, stdout, err)
			}
			if envelope.OK {
				t.Fatalf("loom %s refusal envelope ok = true; want false: %s", verb, stdout)
			}
			if !strings.Contains(envelope.Error, "no status file") {
				t.Errorf("loom %s error = %q; want it to say there is no status file", verb, envelope.Error)
			}
			if !strings.Contains(envelope.Error, "loom start") {
				t.Errorf("loom %s error = %q; want it to name the bootstrap verb as the remedy", verb, envelope.Error)
			}
			if strings.Contains(envelope.Error, ".lock") {
				t.Errorf("loom %s error = %q; want no internal lock path leaked to the operator", verb, envelope.Error)
			}
		})
	}
}

// TestLoomFailedReedUpRefuses asserts a bad reed config refuses the verbs that bring reed up, before any watchdog or driver is spawned.
// Each driver's seed gets its own pair, because a run's seed cannot be rewritten to the other driver.
func TestLoomFailedReedUpRefuses(t *testing.T) {
	t.Parallel()

	exe := sharedLyxBinary(t)
	const verbTimeout = 60 * time.Second

	requireStartRefusal := func(t *testing.T, loc *lyxcwd.Location, worktree string) {
		t.Helper()
		out, exit, err := runLoomCLINoFatal(exe, worktree, verbTimeout, "loom", "start", "--no-attach")
		if err != nil {
			t.Fatalf("lyx loom start --no-attach: %v; output: %s", err, out)
		}
		if exit != 1 {
			t.Fatalf("lyx loom start exit = %d; want 1; output: %s", exit, out)
		}
		if !strings.Contains(out, `"ok":false`) {
			t.Errorf("start envelope is not ok:false; output: %s", out)
		}
		if pids := findWatchdogPIDs(loc.HubPath); len(pids) != 0 {
			t.Errorf("watchdog pids after a failed reed Up = %v; want none", pids)
		}
		if pids := findDriverPIDs(worktree); len(pids) != 0 {
			t.Errorf("driver pids after a failed reed Up = %v; want none", pids)
		}
	}

	t.Run("go driver", func(t *testing.T) {
		loc, worktree := newBadReedUpFixture(t, seedGoDriverRun)

		t.Run("step refuses with the bootstrap kind", func(t *testing.T) {
			out, exit, err := runLoomCLINoFatal(exe, worktree, verbTimeout, "loom", "step")
			if err != nil {
				t.Fatalf("lyx loom step: %v; output: %s", err, out)
			}
			if exit != 1 {
				t.Fatalf("lyx loom step exit = %d; want 1; output: %s", exit, out)
			}
			// The envelope is the last JSON line of the output.
			var env map[string]any
			for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
				var candidate map[string]any
				if json.Unmarshal([]byte(line), &candidate) == nil {
					env = candidate
				}
			}
			if env == nil || env["kind"] != "bootstrap" {
				t.Errorf("envelope kind = %v; want \"bootstrap\"; output: %s", env["kind"], out)
			}
		})

		t.Run("start refuses and spawns nothing", func(t *testing.T) {
			requireStartRefusal(t, loc, worktree)
		})
	})

	t.Run("llm driver", func(t *testing.T) {
		loc, worktree := newBadReedUpFixture(t, seedLLMDriver)

		t.Run("start refuses and spawns nothing", func(t *testing.T) {
			requireStartRefusal(t, loc, worktree)
		})
	})
}
