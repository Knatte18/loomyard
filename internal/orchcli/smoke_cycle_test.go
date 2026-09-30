//go:build smoke

// smoke_cycle_test.go drives one full handoff cycle against a real Claude Code session in a real reed
// strand: start, one turn that launches a background task, a manual cycle, and the assertions that the
// handoff was written, the session was cleared and the resume prompt was typed.
// It also records, without failing, the two questions only a live session answers: whether a background
// task's completion notification survives `/clear`, and whether the session's `SendMessage` address does.
//
// Like the loomcli smoke tests it drives the real built cmd/lyx binary as a subprocess, never RunCLI
// in-process, because `lyx orch start` spawns a detached `lyx orch watch` from os.Executable(), which
// must never be this test binary (Live-Substrate Spawn Observability Invariant).
// The threshold is set far above anything the session can reach so the manual `lyx orch cycle` is the
// only trigger; the threshold-triggered path is covered by the watcher's unit cases.
//
// The test skips when tmux or a `claude` binary is unavailable, and needs a logged-in Claude Code install.

package orchcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// smokeRun runs exe with args in dir, bounded by timeout, and returns the combined output and exit code.
func smokeRun(t *testing.T, exe, dir string, timeout time.Duration, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("lyx %v timed out after %s; output so far:\n%s", args, timeout, out)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return string(out), exitErr.ExitCode()
	}
	if err != nil {
		t.Fatalf("lyx %v: %v; output:\n%s", args, err, out)
	}
	return string(out), 0
}

// smokeStatus runs `lyx orch status` and decodes its envelope.
func smokeStatus(t *testing.T, exe, prime string) map[string]any {
	t.Helper()
	out, code := smokeRun(t, exe, prime, 30*time.Second, "orch", "status")
	if code != 0 {
		t.Fatalf("orch status exited %d: %s", code, out)
	}
	var env map[string]any
	if err := json.Unmarshal(bytes.TrimSpace([]byte(out)), &env); err != nil {
		t.Fatalf("decode status %q: %v", out, err)
	}
	return env
}

// latestTranscript returns the transcript_path of the newest Stop payload in the run's events file,
// or "" when there is none yet.
func latestTranscript(t *testing.T, eventsPath string) string {
	t.Helper()
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		return ""
	}
	var latest string
	for _, line := range strings.Split(string(data), "\n") {
		var payload struct {
			TranscriptPath string `json:"transcript_path"`
		}
		if json.Unmarshal([]byte(line), &payload) == nil && payload.TranscriptPath != "" {
			latest = payload.TranscriptPath
		}
	}
	return latest
}

// waitFor polls cond every second, up to attempts times, failing the test with what on exhaustion.
func waitFor(t *testing.T, attempts int, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < attempts; i++ {
		if cond() {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestSmokeOrch_OneFullCycle drives start, one background-task turn, a manual cycle and the assertions.
func TestSmokeOrch_OneFullCycle(t *testing.T) {
	tmuxPath := os.Getenv("LYX_LOOM_TMUX")
	if tmuxPath == "" {
		var err error
		if tmuxPath, err = exec.LookPath("tmux"); err != nil {
			t.Skip("tmux not found on PATH; set LYX_LOOM_TMUX to override")
		}
	}
	_ = tmuxPath
	if os.Getenv("LYX_SHUTTLE_CLAUDE") == "" {
		if _, err := exec.LookPath("claude"); err != nil {
			t.Skip("claude binary not found on PATH; set LYX_SHUTTLE_CLAUDE to override")
		}
	}

	exe := filepath.Join(t.TempDir(), "lyx")
	build := exec.Command("go", "build", "-o", exe, "./cmd/lyx")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/lyx: %v\n%s", err, out)
	}

	h := hubforge.NewHub(t, ".")
	orchCfg := `model: ""
effort: ""
threshold_tokens: 100000000
idle_grace_s: 3
handoff_timeout_s: 300
poll_interval_ms: 500
`
	hubforge.SeedConfig(t, h, map[string]string{
		"orch":    orchCfg,
		"reed":    reedengine.ConfigTemplate(),
		"shuttle": shuttleengine.ConfigTemplate(),
	})
	prime := h.Location.AnchorPath()
	paths := orchPaths(h.Location)

	reedCfg, err := reedengine.LoadConfig(prime, "reed")
	if err != nil {
		t.Fatalf("load reed config: %v", err)
	}
	reed := reedengine.New(reedCfg, hubgeom.ReedGeometry(h.Location))
	shuttleCfg, err := shuttleengine.LoadConfig(prime, "shuttle")
	if err != nil {
		t.Fatalf("load shuttle config: %v", err)
	}

	stopped := false
	t.Cleanup(func() {
		if !stopped {
			smokeRun(t, exe, prime, 30*time.Second, "orch", "stop")
		}
		_, _ = reed.Down()
	})

	// (2) start and wait for the strand to report ready.
	if out, code := smokeRun(t, exe, prime, 120*time.Second, "orch", "start", "--no-attach"); code != 0 {
		t.Fatalf("orch start exited %d: %s", code, out)
	}
	var guid string
	waitFor(t, 60, "the orch strand to be live", func() bool {
		env := smokeStatus(t, exe, prime)
		guid, _ = env["strand"].(string)
		live, _ := env["strand_live"].(bool)
		return guid != "" && live
	})
	run, _, err := shuttleengine.FindRun(shuttleCfg, prime, guid)
	if err != nil {
		t.Fatalf("FindRun(%s): %v", guid, err)
	}

	// (3) one turn that starts a short background task, then wait for its turn end.
	waitFor(t, 120, "the start prompt's turn to end", func() bool { return latestTranscript(t, run.EventsPath) != "" })
	beforeTranscript := latestTranscript(t, run.EventsPath)
	if err := reed.SendText(guid, "Start a background shell task with `sleep 40; echo BGDONE` (run_in_background), then end your turn immediately without waiting for it.", true); err != nil {
		t.Fatalf("send background-task turn: %v", err)
	}
	waitFor(t, 180, "the background-task turn to end", func() bool {
		return latestTranscript(t, run.EventsPath) != "" && smokeStatusIdle(t, reed, guid)
	})
	preCycleTranscript := latestTranscript(t, run.EventsPath)
	if preCycleTranscript == "" {
		t.Fatalf("no Stop payload names a transcript (before=%q)", beforeTranscript)
	}
	prePane, _ := reed.CapturePane(guid)

	// (4)+(5) request one cycle and wait for it to settle.
	if out, code := smokeRun(t, exe, prime, 30*time.Second, "orch", "cycle"); code != 0 {
		t.Fatalf("orch cycle exited %d: %s", code, out)
	}
	var env map[string]any
	waitFor(t, 600, "cycle_count 1 in phase idle", func() bool {
		env = smokeStatus(t, exe, prime)
		count, _ := env["cycle_count"].(float64)
		return count == 1 && env["phase"] == string(orchengine.PhaseIdle)
	})

	handoff, _ := env["last_handoff"].(string)
	if handoff == "" {
		t.Fatalf("last_handoff is empty after the cycle: %v", env)
	}
	if filepath.Dir(handoff) != paths.HandoffsDir {
		t.Errorf("last_handoff %q is not under %q", handoff, paths.HandoffsDir)
	}
	body, err := os.ReadFile(handoff)
	if err != nil || len(bytes.TrimSpace(body)) == 0 {
		t.Fatalf("handoff %q unreadable or empty: %v", handoff, err)
	}

	pane, err := reed.CapturePane(guid)
	if err != nil {
		t.Fatalf("capture pane after the cycle: %v", err)
	}
	if !strings.Contains(pane, filepath.Base(handoff)) {
		t.Errorf("pane after the cycle does not show the resume prompt naming %q", handoff)
	}
	if strings.Contains(prePane, "run_in_background") && strings.Contains(pane, "run_in_background") {
		t.Errorf("pane after the cycle still shows the pre-cycle transcript")
	}
	if after := latestTranscript(t, run.EventsPath); after == preCycleTranscript {
		t.Errorf("transcript after the cycle equals the pre-cycle one (%s); /clear did not start a new context", after)
	}

	// Observations that never fail the test.
	t.Logf("background task notification reached the resumed session: %v", strings.Contains(pane, "BGDONE"))
	t.Logf("SendMessage address stability across /clear: not machine-checked; inspect the pane below for the ListAgents output\n%s", pane)

	stopped = true
	if out, code := smokeRun(t, exe, prime, 30*time.Second, "orch", "stop"); code != 0 {
		t.Errorf("orch stop exited %d: %s", code, out)
	}
}

// smokeStatusIdle reports whether the session's pane shows an idle input box, via the pane capture
// alone (the provider's own probe is exercised by the watcher).
func smokeStatusIdle(t *testing.T, reed *reedengine.Engine, guid string) bool {
	t.Helper()
	pane, err := reed.CapturePane(guid)
	if err != nil {
		return false
	}
	return !strings.Contains(pane, "esc to interrupt")
}
