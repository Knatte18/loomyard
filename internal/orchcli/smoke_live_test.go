//go:build smoke

// smoke_live_test.go extends the orch smoke suite to what the live-ready task changed: the bypass permission mode with the Agent tool and the idle probe, the soft cycle trigger, the compact cycle, and adopting a running session.
// Every test follows TestSmokeOrch_OneFullCycle's shape: it builds cmd/lyx and drives it as a subprocess, skips without tmux or `claude`, and stops orch and takes reed down in cleanup.
// A test's own plain shuttle runs go through an in-process shuttleengine.Runner, since the CLI exposes neither the permission mode nor a run handle.

package orchcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/testkit/lyxbin"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// liveFixture is one hub with its seeded configs, a built lyx binary, and in-process reed and shuttle handles onto the prime.
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
	if os.Getenv("LYX_LOOM_TMUX") == "" {
		if _, err := exec.LookPath("tmux"); err != nil {
			t.Skip("tmux not found on PATH; set LYX_LOOM_TMUX to override")
		}
	}
	if os.Getenv("LYX_SHUTTLE_CLAUDE") == "" {
		if _, err := exec.LookPath("claude"); err != nil {
			t.Skip("claude binary not found on PATH; set LYX_SHUTTLE_CLAUDE to override")
		}
	}
}

// newLiveFixture builds cmd/lyx, a hub seeded with orchCfg plus extraCfg, and the in-process handles.
// Cleanup stops orch unless the test did, then takes reed down.
func newLiveFixture(t *testing.T, orchCfg string, extraCfg map[string]string) *liveFixture {
	t.Helper()
	requireLiveSubstrate(t)

	exe := lyxbin.Build(t)

	h := hubforge.NewHub(t, ".")
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

	f := &liveFixture{
		exe:        exe,
		hub:        h,
		prime:      prime,
		paths:      orchPaths(h.Location),
		reed:       reed,
		runner:     shuttleengine.NewRunner(reed, claudeengine.New(), reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg),
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

// startOrch runs `orch start --no-attach` with extra args, failing unless it succeeds, then waits for the strand to be live and returns its guid and run.
func (f *liveFixture) startOrch(t *testing.T, extra ...string) (string, shuttleengine.RunState) {
	t.Helper()
	args := append([]string{"orch", "start", "--no-attach"}, extra...)
	if out, code := smokeRun(t, f.exe, f.prime, 120*time.Second, args...); code != 0 {
		t.Fatalf("orch start exited %d: %s", code, out)
	}
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

// startPlainRun starts an interactive bypass-mode shuttle run in the prime that is never finished by an output file, and returns its run record.
func (f *liveFixture) startPlainRun(t *testing.T, role, prompt string) shuttleengine.RunState {
	t.Helper()
	if _, err := f.reed.Up(); err != nil {
		t.Fatalf("reed up: %v", err)
	}
	if err := os.MkdirAll(f.paths.Dir, 0o755); err != nil {
		t.Fatalf("create %s: %v", f.paths.Dir, err)
	}
	run, err := f.runner.Start(shuttleengine.Spec{
		Prompt:         prompt,
		OutputFiles:    []string{filepath.Join(f.paths.Dir, role+".never")},
		Interactive:    true,
		PermissionMode: "bypass",
		AwaitOperator:  true,
		Role:           role,
		Display:        render.Display{},
	})
	if err != nil {
		t.Fatalf("start plain run %q: %v", role, err)
	}
	state, _, err := shuttleengine.FindRun(f.shuttleCfg, f.prime, run.StrandGUID())
	if err != nil {
		t.Fatalf("FindRun(%s): %v", run.StrandGUID(), err)
	}
	return state
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

// killTreeProcs kills every process whose command line names dir or whose working directory is under it, so a detached loom run and its agents, which a test provoked, never outlive it.
func killTreeProcs(dir string) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == os.Getpid() {
			continue
		}
		cmdline, _ := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		cwd, _ := os.Readlink(filepath.Join("/proc", entry.Name(), "cwd"))
		if bytes.Contains(cmdline, []byte(dir)) || cwd == dir || strings.HasPrefix(cwd, dir+string(filepath.Separator)) {
			_ = proc.KillPID(pid)
		}
	}
}

// TestSmokeOrch_Adopt proves a plain interactive run is adopted as the orch strand with its context, that the adopted session's name is addressable and parents a loom run, and that it cycles.
func TestSmokeOrch_Adopt(t *testing.T) {
	loomCfg := strings.Replace(loomengine.ConfigTemplate(), "selfreport: true", "selfreport: false", 1)
	f := newLiveFixture(t, smokeOrchConfig("clear", "bypass", 200000000, 100000000, 300), map[string]string{
		"loom":    loomCfg,
		"webster": websterengine.ConfigTemplate(),
	})
	const slug = "orch-adopt-task"
	hubforge.AddPair(t, f.hub, slug)
	worktree := f.hub.PairWarpWorktree(slug)
	t.Cleanup(func() { killTreeProcs(f.hub.Location.HubPath) })

	codeword := fmt.Sprintf("kestrel-%d", time.Now().UnixNano()%100000)
	plain := f.startPlainRun(t, "plain", fmt.Sprintf("The codeword is %s. Remember it. Reply with the single word noted and end your turn.", codeword))
	waitFor(t, 120, "the plain run's first turn to end", func() bool { return turnEnds(plain.EventsPath) > 0 })

	// Adoption is refused while the plain run holds the session.
	out, code := smokeRun(t, f.exe, f.prime, 120*time.Second, "orch", "start", "--adopt", plain.SessionID, "--no-attach")
	if code == 0 {
		t.Fatalf("orch start --adopt succeeded while the session was live: %s", out)
	}
	if !strings.Contains(out, "live process") {
		t.Errorf("refusal does not name the live holder: %s", out)
	}
	t.Logf("live-holder refusal: %s", strings.TrimSpace(out))

	// Remove the plain strand and wait for its process to exit.
	if _, err := f.reed.RemoveStrand(plain.StrandGUID, false); err != nil {
		t.Fatalf("remove plain strand: %v", err)
	}
	var startOut string
	waitFor(t, 60, "the adopt to be accepted once the holder exited", func() bool {
		var c int
		startOut, c = smokeRun(t, f.exe, f.prime, 120*time.Second, "orch", "start", "--adopt", plain.SessionID, "--no-attach")
		return c == 0
	})
	var startEnv map[string]any
	if err := json.Unmarshal(bytes.TrimSpace([]byte(startOut)), &startEnv); err != nil {
		t.Fatalf("decode adopt envelope %q: %v", startOut, err)
	}
	if source, _ := startEnv["prompt_source"].(string); source != orchengine.SourceAdopt {
		t.Errorf("prompt_source = %q, want %q", source, orchengine.SourceAdopt)
	}
	guid, run := f.orchRun(t)
	waitFor(t, 120, "the adopt prompt's turn to end", func() bool { return turnEnds(run.EventsPath) > 0 })

	// The adopted session kept its context.
	f.sendTurn(t, guid, run.EventsPath, "What is the codeword? Reply with exactly one line `CODEWORD: <word>` and end your turn.", 180)
	if answer := lastTurnEnd(run.EventsPath); !strings.Contains(answer, codeword) {
		t.Errorf("the adopted session did not answer the codeword %q: %q", codeword, answer)
	}

	// The session's name is <shortname>:orch and it parents a loom run it starts.
	orchName := hubforge.TestShortname + ":orch"
	status, err := f.reed.Status()
	if err != nil {
		t.Fatalf("reed status: %v", err)
	}
	if strand, found := trackedStrand(status.Strands, guid); !found || strand.Name != orchName {
		t.Errorf("orch strand name = %q (found %v), want %q", strand.Name, found, orchName)
	}
	f.sendTurn(t, guid, run.EventsPath, fmt.Sprintf("Run the Bash command `cd %s && lyx loom start --no-attach`, then end your turn.", worktree), 300)
	loc, err := lyxcwd.Resolve(worktree)
	if err != nil {
		t.Fatalf("resolve %s: %v", worktree, err)
	}
	parent, err := hubgeom.ResolveParent(loc)
	if err != nil {
		t.Fatalf("resolve the loom run's parent: %v", err)
	}
	if parent.Name != orchName {
		t.Errorf("run parent = %q, want %q", parent.Name, orchName)
	}
	// The detached loom run would start real agents of its own; its parent is all this test needs from it.
	killTreeProcs(worktree)

	// A second run in the prime reaches the orch session through SendMessage, as a driver's escalation notice does.
	marker := fmt.Sprintf("ESCALATION-%d", time.Now().UnixNano()%100000)
	sender := f.startPlainRun(t, "sender", fmt.Sprintf("Use the SendMessage tool to send the exact text %s to %s, then end your turn.", marker, orchName))
	waitForDiag(t, 180, "the sender run's turn to end", func() bool { return turnEnds(sender.EventsPath) > 0 }, func() string {
		pane, _ := f.reed.CapturePane(sender.StrandGUID)
		return pane
	})
	waitFor(t, 180, "the marker to reach the orch transcript", func() bool {
		body, _ := os.ReadFile(latestTranscript(t, run.EventsPath))
		return bytes.Contains(body, []byte(marker))
	})
	// The sender's pane shares the window and leaves the orch pane too short to draw its input box, which the idle probe needs for the cycle.
	if _, err := f.reed.RemoveStrand(sender.StrandGUID, false); err != nil {
		t.Fatalf("remove sender strand: %v", err)
	}

	// A manual cycle completes on the adopted session.
	if out, code := smokeRun(t, f.exe, f.prime, 30*time.Second, "orch", "cycle"); code != 0 {
		t.Fatalf("orch cycle exited %d: %s", code, out)
	}
	waitForDiag(t, 600, "cycle_count 1 in phase idle", func() bool {
		env := smokeStatus(t, f.exe, f.prime)
		count, _ := env["cycle_count"].(float64)
		return count == 1 && env["phase"] == string(orchengine.PhaseIdle)
	}, func() string {
		status, _ := json.Marshal(smokeStatus(t, f.exe, f.prime))
		pane, _ := f.reed.CapturePane(guid)
		watchLog, _ := os.ReadFile(f.paths.WatchLogPath)
		events, _ := os.ReadFile(run.EventsPath)
		raw := ""
		if st, err := f.reed.Status(); err == nil {
			if strand, ok := trackedStrand(st.Strands, guid); ok {
				out, _ := exec.Command("tmux", "-L", st.Socket, "capture-pane", "-p", "-S", "-60", "-t", strand.PaneID).CombinedOutput()
				raw = string(out)
			}
		}
		return string(status) + "\n" + pane + "\nraw pane with scrollback:\n" + raw + "\nwatch.log:\n" + string(watchLog) + "\nevents tail:\n" + tailString(string(events), 3000)
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

// tailString returns the last n bytes of s.
func tailString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
