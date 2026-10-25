//go:build tier1 && !windows

// capture_integration_test.go drives the built `lyx drift` verbs in a hubforge hub pair: a detached capture that outlives its caller, its end by the owned-process end of a teardown, and the compare's refusals.

package driftcli_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/lyxbin"
)

const (
	pairSlug  = hubforge.TemplatePairSlug
	verbWait  = 2 * time.Minute
	pollTick  = 50 * time.Millisecond
	pollLimit = 30 * time.Second
)

// driftYAML returns a hub drift.yaml whose capture command fails while mark is absent and otherwise waits for release, then writes a profile file.
// Its compare, when given, is a command that is never run.
func driftYAML(mark, release, pidFile, compare string) string {
	yaml := fmt.Sprintf(`capture:
  command: |
    echo $$ > "$PIDFILE"
    [ -f "$MARK" ] || exit 1
    while [ ! -f "$RELEASE" ]; do sleep 0.05; done
    mkdir -p {baseline_dir} && echo ok > {baseline_dir}/profile.txt
  timeout_min: 1
  env:
    MARK: %q
    RELEASE: %q
    PIDFILE: %q
verify_max_runs: 2
`, mark, release, pidFile)
	if compare != "" {
		yaml += fmt.Sprintf("compare:\n  command: %q\n  timeout_min: 1\n", compare)
	}
	return yaml
}

// lyxRun runs `lyx drift <args>` in dir and decodes its envelope.
func lyxRun(t *testing.T, bin, dir string, args ...string) envelope.Envelope {
	t.Helper()
	out, _, err := lyxbin.Run(bin, dir, verbWait, append([]string{"drift"}, args...)...)
	if err != nil {
		t.Fatalf("lyx drift %v: %v", args, err)
	}
	return envelope.Decode(t, out)
}

// statusBody runs `lyx drift status` and returns its body.
func statusBody(t *testing.T, bin, dir string) map[string]any {
	t.Helper()
	env := lyxRun(t, bin, dir, "status")
	if !env.OK {
		t.Fatalf("lyx drift status: %s", env.Error)
	}
	body, _ := env.Raw["status"].(map[string]any)
	return body
}

// captureState returns the state of the capture record `drift status` prints, or "" when it prints none.
func captureState(body map[string]any) string {
	capture, _ := body["capture"].(map[string]any)
	state, _ := capture["state"].(string)
	return state
}

// waitFor polls cond until it holds or the limit runs out.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(pollLimit)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(pollTick)
	}
}

// touch creates an empty file at path.
func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCapture_OutlivesCaller covers a capture that fails, one that passes after its caller exited, one ended by the owned-process end of a teardown, and the compare's refusals.
func TestCapture_OutlivesCaller(t *testing.T) {
	t.Parallel()

	bin := lyxbin.Build(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{pairSlug}})
	pair := h.PairCodeWorktree(pairSlug)
	loc, err := lyxcwd.Resolve(pair)
	if err != nil {
		t.Fatal(err)
	}
	paths := driftengine.NewPaths(loc.AnchorPath())

	work := t.TempDir()
	mark, release, pidFile := filepath.Join(work, "mark"), filepath.Join(work, "release"), filepath.Join(work, "pid")
	driftFile := configengine.ConfigFile(h.BoardDir(), "drift")
	writeDrift := func(compare string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(driftFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(driftFile, []byte(driftYAML(mark, release, pidFile, compare)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeStatus := func(row string, status shedengine.State) {
		t.Helper()
		lockPath := shedrun.StatusLock(loc, shedrun.SelfRunID)
		if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
			t.Fatal(err)
		}
		err := state.WriteJSON(shedrun.StatusFile(loc, shedrun.SelfRunID), lockPath, shedengine.Status{CurrentProducer: row, State: status})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverGo}); err != nil {
		t.Fatal(err)
	}
	writeDrift("")
	writeStatus("Drift-Await", shedengine.StateBlocked)

	t.Run("a capture with the marker absent records failed", func(t *testing.T) {
		env := lyxRun(t, bin, pair, "capture")
		if !env.OK {
			t.Fatalf("lyx drift capture: %s", env.Error)
		}
		waitFor(t, "the capture record to read failed", func() bool { return captureState(statusBody(t, bin, pair)) == driftengine.CaptureFailed })
	})

	t.Run("a capture returns while it holds the lock and passes after the caller exited", func(t *testing.T) {
		touch(t, mark)
		env := lyxRun(t, bin, pair, "capture")
		if !env.OK {
			t.Fatalf("lyx drift capture: %s", env.Error)
		}
		body := statusBody(t, bin, pair)
		if body["capture_held"] != true || captureState(body) != driftengine.CaptureRunning {
			t.Errorf("status after the caller returned = %v; want the capture lock held and the record running", body)
		}

		touch(t, release)
		waitFor(t, "the capture record to read passed", func() bool { return captureState(statusBody(t, bin, pair)) == driftengine.CapturePassed })
		if body := statusBody(t, bin, pair); body["capture_held"] != false {
			t.Errorf("status after the capture passed = %v; want the capture lock free", body)
		}
	})

	t.Run("a capture is ended with its command by the owned-process end of a teardown", func(t *testing.T) {
		if err := os.Remove(release); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(pidFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if env := lyxRun(t, bin, pair, "capture"); !env.OK {
			t.Fatalf("lyx drift capture: %s", env.Error)
		}
		var commandPid int
		waitFor(t, "the capture command to write its pid", func() bool {
			data, err := os.ReadFile(pidFile)
			if err != nil {
				return false
			}
			commandPid, err = strconv.Atoi(strings.TrimSpace(string(data)))
			return err == nil
		})

		if _, err := runproc.End(fabricengine.RunProcDir(h.Path), loc.WorktreeName, os.Getpid()); err != nil {
			t.Fatalf("runproc.End() = %v", err)
		}
		waitFor(t, "the capture lock to be free", func() bool {
			held, err := driftengine.ProbeLock(paths.CaptureLock)
			return err == nil && !held
		})
		waitFor(t, "the capture command to be gone", func() bool {
			return errors.Is(syscall.Kill(commandPid, 0), syscall.ESRCH)
		})
	})

	t.Run("compare refuses", func(t *testing.T) {
		writeStatus("Drift-Verify", shedengine.StateRunning)
		env := lyxRun(t, bin, pair, "compare")
		if env.OK || !strings.Contains(env.Error, "lyx config drift") {
			t.Errorf("lyx drift compare with no compare declared = ok %v, %q; want a refusal naming lyx config drift", env.OK, env.Error)
		}
		if _, err := os.Stat(paths.Runs); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("a refused compare wrote the run counter: stat err = %v", err)
		}

		writeDrift("true {baseline_dir}")
		if err := driftengine.ResetRuns(paths, "head"); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if _, err := driftengine.TakeRun(paths, 2); err != nil {
				t.Fatal(err)
			}
		}
		spent, err := os.ReadFile(paths.Runs)
		if err != nil {
			t.Fatal(err)
		}

		rows := []struct {
			name   string
			status shedengine.State
			want   string
		}{
			{"the agent session", shedengine.StateRunning, "write what you found to verify-report.md and end your turn"},
			{"a caller at a halt", shedengine.StateBlocked, "lyx loom resume spawns a fresh agent with a fresh budget"},
		}
		for _, row := range rows {
			writeStatus("Drift-Verify", row.status)
			env := lyxRun(t, bin, pair, "compare")
			if env.OK || !strings.Contains(env.Error, row.want) {
				t.Errorf("lyx drift compare for %s = ok %v, %q; want a refusal naming %q", row.name, env.OK, env.Error, row.want)
			}
			after, err := os.ReadFile(paths.Runs)
			if err != nil || string(after) != string(spent) {
				t.Errorf("a refused compare for %s changed runs.json to %q (%v); want %q", row.name, after, err, spent)
			}
		}
	})
}
