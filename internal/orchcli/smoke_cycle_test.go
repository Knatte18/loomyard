//go:build llm

// smoke_cycle_test.go drives one full handoff cycle against a real Claude Code session in a real reed strand: start, one turn that launches a background task, a manual cycle, and the assertions that the handoff was written, the session was cleared and the resume prompt was typed.
// It also records, without failing, the two questions only a live session answers: whether a background task's completion notification survives `/clear`, and whether the session's `SendMessage` address does.
//
// Like the loomcli smoke tests it drives the real built cmd/lyx binary as a subprocess, never RunCLI in-process, because `lyx orch start` spawns a detached `lyx orch watch` from os.Executable(), which must never be this test binary (Live-Substrate Spawn Observability Invariant).
// The threshold is set far above anything the session can reach so the manual `lyx orch refresh` is the only trigger;
// the threshold-triggered path is covered by the watcher's unit cases.
//
// The test skips when tmux or a `claude` binary is unavailable, and needs a logged-in Claude Code install.

package orchcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/testkit/llmkit"
	"github.com/Knatte18/loomyard/internal/testkit/lyxbin"
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

// latestTranscript returns the transcript_path of the newest Stop payload in the run's events file, or "" when there is none yet.
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

const (
	// smokeBackgroundSleepS is the background task's sleep, long enough that its completion notification arrives after `/clear` even when the handoff turn is slow.
	smokeBackgroundSleepS = 120
	// smokeNotificationMargin is how long past the sleep's end the notification observation waits before reading.
	smokeNotificationMargin = 20 * time.Second
	// smokeAddressPrefix marks the one answer line an address turn asks the session for.
	smokeAddressPrefix = "AGENT-ADDRESS:"
)

// lastTurnEnd returns the message of the newest turn end in the run's events file, or "" when there is none.
func lastTurnEnd(eventsPath string) string {
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		return ""
	}
	events, err := claudeengine.New().ParseEvents(data)
	if err != nil {
		return ""
	}
	message := ""
	for _, ev := range events {
		if ev.Kind == shuttleengine.EventStop || ev.Kind == shuttleengine.EventWaiting {
			message = ev.Message
		}
	}
	return message
}

// askAgentAddress sends a turn asking the session for its own SendMessage address as ListAgents shows it, waits for that turn to end,
// and returns the answer, or "" when the turn ended without the marked line.
func askAgentAddress(t *testing.T, reed *reedengine.Engine, guid, eventsPath string) string {
	t.Helper()
	before := turnEnds(eventsPath)
	prompt := "Call the ListAgents tool, then reply with exactly one line of the form `" + smokeAddressPrefix + " <the name ListAgents shows for this session>` and end your turn."
	if err := reed.SendText(guid, prompt, true); err != nil {
		t.Fatalf("send address turn: %v", err)
	}
	waitFor(t, 180, "the address turn to end", func() bool { return turnEnds(eventsPath) > before })
	for _, line := range strings.Split(lastTurnEnd(eventsPath), "\n") {
		if _, answer, found := strings.Cut(line, smokeAddressPrefix); found {
			return strings.Trim(strings.TrimSpace(answer), "`")
		}
	}
	return ""
}

// turnEnds counts the turn ends (Stop or Waiting) the run's events file holds so far, 0 while it is unreadable.
func turnEnds(eventsPath string) int {
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		return 0
	}
	events, err := claudeengine.New().ParseEvents(data)
	if err != nil {
		return 0
	}
	n := 0
	for _, ev := range events {
		if ev.Kind == shuttleengine.EventStop || ev.Kind == shuttleengine.EventWaiting {
			n++
		}
	}
	return n
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
	llmkit.Claude(t, "LYX_SHUTTLE_CLAUDE")

	exe := lyxbin.Build(t)

	h := hubforge.NewHub(t, ".")
	// The soft threshold sits above the hard one, so the manual cycle stays the only trigger.
	orchCfg := smokeOrchConfig("clear", "bypass", 200000000, 100000000, 300)
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
	reedGeom, err := hubgeom.ReedGeometry(h.Location)
	if err != nil {
		t.Fatalf("reed geometry: %v", err)
	}
	reed := reedengine.New(reedCfg, reedGeom)
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
	// The start prompt's turn end already names the transcript, so only a turn-end count past it proves the background-task turn ended.
	// The pre-cycle address is asked first, so its turn does not delay the cycle past the background task's sleep.
	waitFor(t, 120, "the start prompt's turn to end", func() bool { return turnEnds(run.EventsPath) > 0 })
	addressBefore := askAgentAddress(t, reed, guid, run.EventsPath)
	turnEndsBefore := turnEnds(run.EventsPath)
	if err := reed.SendText(guid, fmt.Sprintf("Start a background shell task with `sleep %d; echo BGDONE` (run_in_background), then end your turn immediately without waiting for it.", smokeBackgroundSleepS), true); err != nil {
		t.Fatalf("send background-task turn: %v", err)
	}
	backgroundDue := time.Now().Add(smokeBackgroundSleepS * time.Second)
	waitFor(t, 180, "the background-task turn to end", func() bool {
		return turnEnds(run.EventsPath) > turnEndsBefore && smokeStatusIdle(t, reed, guid)
	})
	preCycleTranscript := latestTranscript(t, run.EventsPath)
	if preCycleTranscript == "" {
		t.Fatalf("no Stop payload names a transcript after %d turn ends", turnEnds(run.EventsPath))
	}
	prePane, _ := reed.CapturePane(guid)

	// (4)+(5) request one cycle and wait for it to settle.
	if out, code := smokeRun(t, exe, prime, 30*time.Second, "orch", "refresh"); code != 0 {
		t.Fatalf("orch refresh exited %d: %s", code, out)
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
	// The background task's notification cannot arrive before its sleep ends, so the observation waits past that deadline first;
	// the address turn after it also gives a pending notification a turn to be delivered on.
	if wait := time.Until(backgroundDue.Add(smokeNotificationMargin)); wait > 0 {
		time.Sleep(wait)
	}
	addressAfter := askAgentAddress(t, reed, guid, run.EventsPath)
	resumedTranscript, _ := os.ReadFile(latestTranscript(t, run.EventsPath))
	finalPane, _ := reed.CapturePane(guid)
	t.Logf("background task notification reached the resumed session: transcript=%v pane=%v",
		bytes.Contains(resumedTranscript, []byte("BGDONE")), strings.Contains(finalPane, "BGDONE"))
	t.Logf("SendMessage address across /clear: before=%q after=%q stable=%v",
		addressBefore, addressAfter, addressBefore != "" && addressBefore == addressAfter)

	stopped = true
	if out, code := smokeRun(t, exe, prime, 30*time.Second, "orch", "stop"); code != 0 {
		t.Errorf("orch stop exited %d: %s", code, out)
	}
}

// smokeStatusIdle reports whether the session's pane shows an idle input box, via the pane capture alone (the provider's own probe is exercised by the watcher).
func smokeStatusIdle(t *testing.T, reed *reedengine.Engine, guid string) bool {
	t.Helper()
	pane, err := reed.CapturePane(guid)
	if err != nil {
		return false
	}
	return !strings.Contains(pane, "esc to interrupt")
}
