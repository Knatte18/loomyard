//go:build tier3

// smoke_live_test.go extends the orch smoke suite to what the live-ready task changed: the bypass permission mode with the Agent tool and the idle probe, the soft cycle trigger and the compact cycle.
// Every test follows TestSmokeOrch_OneFullCycle's shape: it builds cmd/lym and drives it as a subprocess, skips without tmux or `claude`, and stops orch and takes reed down in cleanup.
// A test's own plain shuttle runs go through an in-process shuttleengine.Runner, since the CLI exposes neither the permission mode nor a run handle.

package orchcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/testkit/llmkit"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// liveFixture is one hub with its seeded configs, a built lym binary, and in-process reed and shuttle handles onto the prime.
type liveFixture struct {
	exe        string
	hub        *hubforge.Hub
	prime      string
	paths      orchengine.Paths
	reed       *reedengine.Engine
	runner     *shuttleengine.Runner
	shuttleCfg shuttleengine.Config
	// orchStopped is set once a test has stopped orch itself, so cleanup does not stop it again.
	orchStopped bool
}

// smokeOrchConfig renders an orch.yaml carrying the full key set, since a config missing a template key fails to load.
func smokeOrchConfig(cycleMode, permissionMode string, softThreshold, hardThreshold, softIdleS int) string {
	return fmt.Sprintf(`model: ""
effort: ""
cycle_mode: %s
permission_mode: %s
soft_threshold_tokens: %d
soft_idle_s: %d
threshold_tokens: %d
idle_grace_s: 3
handoff_timeout_s: 300
poll_interval_ms: 500
`, cycleMode, permissionMode, softThreshold, softIdleS, hardThreshold)
}

// requireLiveSubstrate skips the test without tmux or a claude binary.
func requireLiveSubstrate(t *testing.T) {
	t.Helper()
	if os.Getenv("LYM_LOOM_TMUX") == "" {
		if _, err := exec.LookPath("tmux"); err != nil {
			t.Skip("tmux not found on PATH; set LYM_LOOM_TMUX to override")
		}
	}
	llmkit.Claude(t, "LYM_SHUTTLE_CLAUDE")
}

// newLiveFixture builds cmd/lym, a hub seeded with orchCfg plus extraCfg, and the in-process handles.
// Cleanup stops orch unless the test did, then takes reed down.
func newLiveFixture(t *testing.T, orchCfg string, extraCfg map[string]string) *liveFixture {
	t.Helper()
	requireLiveSubstrate(t)

	exe := lymbin.Build(t)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	cfgs := map[string]string{
		"orch":    orchCfg,
		"reed":    reedengine.ConfigTemplate(),
		"shuttle": shuttleengine.ConfigTemplate(),
	}
	for module, body := range extraCfg {
		cfgs[module] = body
	}
	hubforge.SeedConfig(t, h, cfgs)
	prime := h.Location.AnchorPath()

	reedCfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: prime}, "reed")
	if err != nil {
		t.Fatalf("load reed config: %v", err)
	}
	reedGeom, err := hubgeom.ReedGeometry(h.Location)
	if err != nil {
		t.Fatalf("reed geometry: %v", err)
	}
	reed := reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub())
	// Registered before the first boot, so reed's boot finds the kit's hermetic server on the hub's key.
	tmuxkit.KillOnCleanup(t, reedCfg.Tmux, reedGeom.SocketKey)
	shuttleCfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: prime}, "shuttle")
	if err != nil {
		t.Fatalf("load shuttle config: %v", err)
	}

	f := &liveFixture{
		exe:        exe,
		hub:        h,
		prime:      prime,
		paths:      orchPaths(h.Location),
		reed:       reed,
		runner:     shuttleengine.NewRunner(reed, shuttlefake.EngineSet(claudeengine.NewFromConfig(shuttleCfg, fabricengine.ConfigDirs(h.Location))), reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg),
		shuttleCfg: shuttleCfg,
	}
	t.Cleanup(func() {
		if !f.orchStopped {
			smokeRun(t, exe, prime, 30*time.Second, "orch", "stop")
		}
		_, _ = reed.Down()
	})
	return f
}

// startOrch brings the prime's orch up through BeamUp, failing unless it succeeds, then waits for the strand to be live and returns its guid and run.
func (f *liveFixture) startOrch(t *testing.T) (string, shuttleengine.RunState) {
	t.Helper()
	smokeBeamUp(t, f.exe, f.hub.Location)
	return f.orchRun(t)
}

// orchRun waits for the recorded orch strand to be live and returns its guid and run record.
func (f *liveFixture) orchRun(t *testing.T) (string, shuttleengine.RunState) {
	t.Helper()
	var guid string
	waitFor(t, 60, "the orch strand to be live", func() bool {
		env := smokeStatus(t, f.exe, f.prime)
		guid, _ = env["strand"].(string)
		live, _ := env["strand_live"].(bool)
		return guid != "" && live
	})
	state, _, err := shuttleengine.FindRun(f.shuttleCfg, f.prime, guid)
	if err != nil {
		t.Fatalf("FindRun(%s): %v", guid, err)
	}
	return guid, state
}

// stopOrch runs `orch stop` and records that cleanup need not.
func (f *liveFixture) stopOrch(t *testing.T) {
	t.Helper()
	f.orchStopped = true
	if out, code := smokeRun(t, f.exe, f.prime, 30*time.Second, "orch", "stop"); code != 0 {
		t.Errorf("orch stop exited %d: %s", code, out)
	}
}

// sendTurn sends text to guid and waits for the turn it starts to end.
func (f *liveFixture) sendTurn(t *testing.T, guid, eventsPath, text string, attempts int) {
	t.Helper()
	before := turnEnds(eventsPath)
	if err := f.reed.SendText(guid, text, true); err != nil {
		t.Fatalf("send turn: %v", err)
	}
	waitFor(t, attempts, "the turn to end", func() bool { return turnEnds(eventsPath) > before })
}

// permissionDialogMarker is the text Claude Code's permission dialog shows.
const permissionDialogMarker = "Do you want to"

// bypassFooterMarker is the lower-cased text Claude Code's footer shows while the session runs in bypass-permissions mode.
const bypassFooterMarker = "bypass permissions on"

// waitForFile polls for path to exist, failing the test if the pane of guid ever shows a permission dialog meanwhile.
func (f *liveFixture) waitForFile(t *testing.T, guid, path string, attempts int) {
	t.Helper()
	waitFor(t, attempts, "the marker file "+filepath.Base(path), func() bool {
		if pane, err := f.reed.CapturePane(guid); err == nil && strings.Contains(pane, permissionDialogMarker) {
			t.Fatalf("a permission dialog showed in the pane while waiting for %s:\n%s", path, pane)
		}
		_, err := os.Stat(path)
		return err == nil
	})
}

// toolUse is one tool_use block of a transcript.
type toolUse struct {
	Name  string
	Input map[string]any
}

// transcriptToolUses returns every tool_use block in the JSONL transcript at path, in order.
func transcriptToolUses(path string) []toolUse {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var uses []toolUse
	for _, line := range strings.Split(string(data), "\n") {
		var entry struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		var blocks []struct {
			Type  string         `json:"type"`
			Name  string         `json:"name"`
			Input map[string]any `json:"input"`
		}
		if json.Unmarshal(entry.Message.Content, &blocks) != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_use" {
				uses = append(uses, toolUse{Name: b.Name, Input: b.Input})
			}
		}
	}
	return uses
}

// backgroundShells counts the Bash tool calls of the transcript at path that ran in the background.
func backgroundShells(path string) int {
	n := 0
	for _, u := range transcriptToolUses(path) {
		if bg, _ := u.Input["run_in_background"].(bool); u.Name == "Bash" && bg {
			n++
		}
	}
	return n
}

// TestSmokeOrch_BypassAgentAndIdle proves the orch session runs in bypass mode with the Agent tool allowed, and that the idle probe passes once a turn ends.
func TestSmokeOrch_BypassAgentAndIdle(t *testing.T) {
	f := newLiveFixture(t, smokeOrchConfig("clear", "bypass", 200000000, 100000000, 300), nil)
	guid, run := f.startOrch(t)
	waitFor(t, 120, "the start prompt's turn to end", func() bool { return turnEnds(run.EventsPath) > 0 })

	// The footer shows the session's actual permission mode, which a tool the user's own settings already allow could not prove.
	waitFor(t, 30, "the bypass-permissions footer in the orch pane", func() bool {
		pane, err := f.reed.CapturePane(guid)
		return err == nil && strings.Contains(strings.ToLower(pane), bypassFooterMarker)
	})

	agentMarker := filepath.Join(t.TempDir(), "agent-marker")

	before := turnEnds(run.EventsPath)
	if err := f.reed.SendText(guid, fmt.Sprintf("Use the Agent tool to spawn a general-purpose subagent whose only task is to run the Bash command `touch %s`. Wait for it, then end your turn.", agentMarker), true); err != nil {
		t.Fatalf("send agent turn: %v", err)
	}
	f.waitForFile(t, guid, agentMarker, 180)
	waitFor(t, 180, "the agent turn to end", func() bool { return turnEnds(run.EventsPath) > before })

	transcript := latestTranscript(t, run.EventsPath)
	agentCalls := 0
	for _, u := range transcriptToolUses(transcript) {
		if u.Name == "Agent" || u.Name == "Task" {
			agentCalls++
		}
	}
	if agentCalls == 0 {
		t.Errorf("transcript %s holds no Agent call", transcript)
	}
	body, _ := os.ReadFile(transcript)
	if bytes.Contains(body, []byte("nested agents are not available")) {
		t.Errorf("transcript %s holds the Agent deny", transcript)
	}

	polls := 0
	waitFor(t, 30, "the idle probe to pass", func() bool {
		polls++
		probe, err := f.runner.SessionIdle(guid)
		if err == nil && probe.Idle {
			return true
		}
		if polls == 30 {
			pane, _ := f.reed.CapturePane(guid)
			t.Logf("idle probe still failing (err=%v); pane:\n%s", err, pane)
		}
		return false
	})
	t.Logf("SessionIdle passed on poll %d after the turn ended", polls)

	f.stopOrch(t)
}

// TestSmokeOrch_SoftCycle proves the soft trigger starts a cycle at a turn end below an unreachable hard cap.
func TestSmokeOrch_SoftCycle(t *testing.T) {
	// The start prompt's own turn end already qualifies at this soft threshold, so soft_idle_s is long enough for the background-shell turn below to land first.
	f := newLiveFixture(t, smokeOrchConfig("clear", "bypass", 1, 100000000, 45), nil)
	guid, run := f.startOrch(t)
	waitFor(t, 120, "the start prompt's turn to end", func() bool { return turnEnds(run.EventsPath) > 0 })
	startTranscript := latestTranscript(t, run.EventsPath)

	if err := f.reed.SendText(guid, "Start a background shell task with `sleep 900; echo SOFTBG` (run_in_background), then end your turn immediately without waiting for it. If you ever write a handoff, tell the session that resumes from it to start this same background shell again as its first action.", true); err != nil {
		t.Fatalf("send background-shell turn: %v", err)
	}

	var env map[string]any
	waitFor(t, 900, "cycle_count 1 in phase idle", func() bool {
		env = smokeStatus(t, f.exe, f.prime)
		count, _ := env["cycle_count"].(float64)
		return count == 1 && env["phase"] == string(orchengine.PhaseIdle)
	})
	// Read everything the cycle left behind before a second soft cycle can start.
	resumedTranscript := latestTranscript(t, run.EventsPath)
	resumedShells := backgroundShells(resumedTranscript)
	if trigger, _ := env["cycle_trigger"].(string); trigger != orchengine.TriggerSoft {
		t.Errorf("cycle_trigger = %q, want %q", trigger, orchengine.TriggerSoft)
	}
	handoff, _ := env["last_handoff"].(string)
	body, err := os.ReadFile(handoff)
	if err != nil || len(bytes.TrimSpace(body)) == 0 {
		t.Fatalf("handoff %q unreadable or empty: %v", handoff, err)
	}
	// The soft stencil itself says "background shell", so only the test's own command proves the handoff names it,
	// and that the background-shell turn ran before the cycle began.
	lower := strings.ToLower(string(body))
	if !strings.Contains(lower, "softbg") && !strings.Contains(lower, "sleep 900") {
		t.Errorf("handoff does not name the background shell:\n%s", body)
	}
	if resumedTranscript == startTranscript {
		t.Errorf("transcript did not change across /clear (%s)", resumedTranscript)
	}
	// A background shell survives /clear (TestSmokeOrch_OneFullCycle's observation), so a resumed session may find it still running instead of starting another.
	// The count is logged, not asserted.
	t.Logf("background shells the resumed session started: %d", resumedShells)

	f.stopOrch(t)
}

const (
	// compactSoftThreshold sits above a fresh orch session's context and below what the filler turn adds, so only that turn's end fires the soft trigger, and a compacted context falls back under it.
	compactSoftThreshold = 60000
	// compactBackgroundSleepS keeps the background task running through the compaction, so its completion arrives on the compacted session.
	compactBackgroundSleepS = 150
	// compactBackgroundAnswer is what the background task echoes, computed by the shell so the command text itself never contains it.
	compactBackgroundAnswer = "BGCOMPACT-42"
)

// TestSmokeOrch_CompactCycle proves the compact cycle keeps the session id and the strand, brings the context reading under the soft threshold, counts one cycle, and leaves a background task able to complete into the session.
func TestSmokeOrch_CompactCycle(t *testing.T) {
	// soft_idle_s is long enough for the filler turn below to end before the soft trigger may fire.
	f := newLiveFixture(t, smokeOrchConfig("compact", "bypass", compactSoftThreshold, 100000000, 45), nil)
	guid, run := f.startOrch(t)
	waitFor(t, 120, "the start prompt's turn to end", func() bool { return turnEnds(run.EventsPath) > 0 })

	// The filler turn grows the context past the soft threshold; the background task is started in the same turn, before any cycle.
	prompt := "Run the Bash command `head -c 20000 /dev/urandom | base64 -w0` three times, as three separate Bash calls, without reading their output closely. " +
		fmt.Sprintf("Then start a background shell task with `sleep %d; echo BGCOMPACT-$((6*7))` (run_in_background), and end your turn immediately without waiting for it.", compactBackgroundSleepS)
	if err := f.reed.SendText(guid, prompt, true); err != nil {
		t.Fatalf("send filler turn: %v", err)
	}
	backgroundDue := time.Now().Add(compactBackgroundSleepS * time.Second)

	var env map[string]any
	waitForDiag(t, 900, "cycle_count 1 in phase idle", func() bool {
		env = smokeStatus(t, f.exe, f.prime)
		count, _ := env["cycle_count"].(float64)
		return count == 1 && env["phase"] == string(orchengine.PhaseIdle)
	}, func() string {
		status, _ := json.Marshal(smokeStatus(t, f.exe, f.prime))
		pane, _ := f.reed.CapturePane(guid)
		watchLog, _ := os.ReadFile(f.paths.WatchLogPath)
		return string(status) + "\n" + pane + "\nwatch.log:\n" + string(watchLog)
	})

	if trigger, _ := env["cycle_trigger"].(string); trigger != orchengine.TriggerSoft {
		t.Errorf("cycle_trigger = %q, want %q", trigger, orchengine.TriggerSoft)
	}
	// The compact cycle writes its note first, so a note sits under HandoffsDir after the compaction.
	notes, _ := filepath.Glob(filepath.Join(f.paths.HandoffsDir, "*.md"))
	if len(notes) == 0 {
		t.Errorf("no note under %s after the compact cycle", f.paths.HandoffsDir)
	}
	if after, _ := env["strand"].(string); after != guid {
		t.Errorf("strand = %q after the cycle, want the unchanged %q", after, guid)
	}
	afterState, _, err := shuttleengine.FindRun(f.shuttleCfg, f.prime, guid)
	if err != nil {
		t.Fatalf("FindRun(%s) after the cycle: %v", guid, err)
	}
	if afterState.SessionID != run.SessionID {
		t.Errorf("session id = %q after the cycle, want the unchanged %q", afterState.SessionID, run.SessionID)
	}
	tokens, known := env["context_tokens"].(float64)
	if !known {
		t.Errorf("context_tokens is unknown after the cycle: %v", env)
	} else if tokens >= compactSoftThreshold {
		t.Errorf("context_tokens = %v after the cycle, want below the soft threshold %d", tokens, compactSoftThreshold)
	}
	t.Logf("context reading after the compact cycle: %v", env["context_tokens"])

	// The background task outlives the compaction: its completion must reach the session.
	if wait := time.Until(backgroundDue.Add(smokeNotificationMargin)); wait > 0 {
		time.Sleep(wait)
	}
	waitFor(t, 120, "the background task's completion in the session's transcript", func() bool {
		body, _ := os.ReadFile(latestTranscript(t, run.EventsPath))
		return bytes.Contains(body, []byte(compactBackgroundAnswer))
	})

	f.stopOrch(t)
}

// waitForDiag is waitFor, logging diag's text before it fails the test.
func waitForDiag(t *testing.T, attempts int, what string, cond func() bool, diag func() string) {
	t.Helper()
	for i := 0; i < attempts; i++ {
		if cond() {
			return
		}
		time.Sleep(time.Second)
	}
	t.Logf("%s; diagnostics:\n%s", what, diag())
	t.Fatalf("timed out waiting for %s", what)
}
