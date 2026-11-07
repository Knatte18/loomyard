// smoke_warmpath_test.go covers `reed add` and `reed attach` against a live session that holds at least one pane.
// On that path neither command reconciles, applies the layout, or calls SaveState.
// It adds no config validation.
// Their only extra work is the tmux list-panes and has-session queries the warm path needs.
// The scenario boots the session explicitly first,
// so the self-healing pre-flight under test takes its early return rather than its delegate-to-boot branch.

package reedcli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// writeReedConfigWithOverride writes the reed config for worktree from the full config template.
// Exactly one key's value is replaced, and the file lands at the path the config engine resolves for the reed module.
// A single-key fragment would fail LoadOrTemplate's key-completeness check, so the whole template is written.
func writeReedConfigWithOverride(t *testing.T, worktree, key, value string) {
	t.Helper()
	lines := strings.Split(reedengine.ConfigTemplate(), "\n")
	replaced := false
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trimmed, key+":") {
			indent := line[:len(line)-len(trimmed)]
			lines[i] = indent + key + ": " + value
			replaced = true
			break
		}
	}
	if !replaced {
		t.Fatalf("reed config template has no %q key to override", key)
	}

	configDir := configengine.ConfigDir(worktree)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", configDir, err)
	}
	configFile := configengine.ConfigFile(worktree, "reed")
	if err := os.WriteFile(configFile, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", configFile, err)
	}
}

// TestSmokeWarmPath runs the warm-path claims against one hub.
// Its prime worktree is brought up once with one strand,
// so each step reuses that live session instead of building its own hub and server.
// The steps run serially in a fixed order:
// each leaves the session up with the strand's state intact for the next step, except the last, which tears it down.
// The scenario calls t.Parallel but no step does, because every step shares the one hub and session.
func TestSmokeWarmPath(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	tmuxPath := tmuxBinaryPath(t)
	lymExe := lymbin.Build(t)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	worktree := h.PrimeWorktree()
	deferHubRelease(t, worktree)
	socket := reedengine.ServerName(h.Path)
	tmuxkit.KillOnCleanup(t, tmuxPath, socket)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(worktree, &buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLIIn(worktree, &out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	guid := addStrandIn(t, worktree, smokeReapLaunchCmd(), "--name", "warm-strand")

	session := reedengine.SessionName(worktree)
	statePath := filepath.Join(worktree, ".lym", "reed.json")

	// WarmAddIsUnchanged pins the control case:
	// warm add against a live session still yields exactly one session, one header pane, and one live strand.
	if !t.Run("WarmAddIsUnchanged", func(t *testing.T) {
		if !sessionAlive(tmuxPath, socket, session) {
			t.Fatalf("session %s not alive on socket %s after a warm add", session, socket)
		}

		var statusOut bytes.Buffer
		if code := RunCLIIn(worktree, &statusOut, []string{"status"}); code != 0 {
			t.Fatalf("status = %d; want 0, output: %s", code, statusOut.String())
		}
		var statusResult map[string]any
		if err := json.Unmarshal(statusOut.Bytes(), &statusResult); err != nil {
			t.Fatalf("parse status result: %v", err)
		}
		strands, _ := statusResult["strands"].([]any)
		if len(strands) != 1 {
			t.Fatalf("status strands = %v; want exactly 1", strands)
		}
		strand, _ := strands[0].(map[string]any)
		if strand["guid"] != guid {
			t.Fatalf("status strand guid = %v; want %q", strand["guid"], guid)
		}
		if live, _ := strand["live"].(bool); !live {
			t.Errorf("strand %s live = false; want true", guid)
		}

		panes := listPaneLines(t, tmuxPath, socket, session)
		if len(panes) != 2 {
			t.Fatalf("panes after warm add = %v; want 2 (the header pane plus the one strand's own pane)", panes)
		}
	}) {
		return
	}

	// WarmAttachWritesNoState pins that a warm attach leaves .lym/reed.json untouched:
	// its modification time and bytes must be identical afterward.
	if !t.Run("WarmAttachWritesNoState", func(t *testing.T) {
		before, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatalf("read %s: %v", statePath, err)
		}
		beforeInfo, err := os.Stat(statePath)
		if err != nil {
			t.Fatalf("stat %s: %v", statePath, err)
		}

		runReedCLINoFatal(t, lymExe, worktree, 30*time.Second, "reed", "attach")

		after, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatalf("read %s after warm attach: %v", statePath, err)
		}
		afterInfo, err := os.Stat(statePath)
		if err != nil {
			t.Fatalf("stat %s after warm attach: %v", statePath, err)
		}
		if !bytes.Equal(before, after) {
			t.Errorf("reed.json bytes changed after a warm attach; want it untouched (a warm attach must SaveState nothing)")
		}
		if !beforeInfo.ModTime().Equal(afterInfo.ModTime()) {
			t.Errorf("reed.json mtime changed after a warm attach (%s -> %s); want it untouched", beforeInfo.ModTime(), afterInfo.ModTime())
		}
	}) {
		return
	}

	// WarmAttachDoesNotReapAnOperatorsPane pins that attach leaves an untracked pane alive.
	// The pane is created directly through the multiplexer, as an operator's hand-split does, while the session is up.
	// This step fails if the pre-flight is ever routed back through the boot path.
	// upLocked's tail reaches planReconcile. It adds every live non-exempt pane to its kill list once the header is alive.
	// It leaves the foreign pane in the session for the steps after it.
	if !t.Run("WarmAttachDoesNotReapAnOperatorsPane", func(t *testing.T) {
		before := map[string]bool{}
		for _, line := range listPaneLines(t, tmuxPath, socket, session) {
			before[strings.Fields(line)[0]] = true
		}

		if err := exec.Command(tmuxPath, "-L", socket, "split-window", "-t", session).Run(); err != nil {
			t.Fatalf("foreign split-window: %v", err)
		}
		foreignPaneID := ""
		for _, line := range listPaneLines(t, tmuxPath, socket, session) {
			id := strings.Fields(line)[0]
			if !before[id] {
				foreignPaneID = id
			}
		}
		if foreignPaneID == "" {
			t.Fatalf("no new pane found after the foreign split-window; panes=%v", listPaneLines(t, tmuxPath, socket, session))
		}

		runReedCLINoFatal(t, lymExe, worktree, 30*time.Second, "reed", "attach")

		if !paneLiveOnSession(listPaneLines(t, tmuxPath, socket, session), foreignPaneID) {
			t.Errorf("foreign pane %s not alive after a warm attach; want it untouched -- a warm attach must never reconcile", foreignPaneID)
		}
	}) {
		return
	}

	// WarmAttachStillRefusesOnAnUnreadableStateFile pins that a warm attach aborts on an unparseable .lym/reed.json.
	// The state file is made unparseable under a live session,
	// and the abort is the JSON envelope with the state loader's diagnosis.
	// This step fails if the Status call is ever dropped from attach's pre-flight:
	// EnsureSession's own early return reads no state at all.
	// It restores the state file afterwards, so the steps after it find the strand's state intact.
	if !t.Run("WarmAttachStillRefusesOnAnUnreadableStateFile", func(t *testing.T) {
		original, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatalf("read %s: %v", statePath, err)
		}
		if err := os.WriteFile(statePath, []byte(`{"socket":"lym-x","strands":[{"gu`), 0o600); err != nil {
			t.Fatalf("truncate %s: %v", statePath, err)
		}
		t.Cleanup(func() {
			if err := os.WriteFile(statePath, original, 0o600); err != nil {
				t.Errorf("restore %s: %v", statePath, err)
			}
		})

		var attachOut bytes.Buffer
		if code := RunCLIIn(worktree, &attachOut, []string{"attach"}); code == 0 {
			t.Fatalf("warm attach with an unreadable reed.json = 0; want a failure, output: %s", attachOut.String())
		}
		unreadable := envelopeError(t, attachOut.Bytes())
		for _, want := range []string{"unreadable", "reed.json", "lym reed down"} {
			if !strings.Contains(unreadable, want) {
				t.Errorf("warm attach error = %s; want it to contain %q (Status's state-loader diagnosis)", unreadable, want)
			}
		}
	}) {
		return
	}

	// WarmAttachSurvivesAConfigErrorButColdStillRefuses pins that the liveness probe runs before config validation.
	// With the session up, an invalid mouse value in this worktree's reed config must not make attach refuse.
	// The cold direction is asserted too: with no session up, the identical bad config must still fire the same error.
	// Together the two pin that EnsureSession's early return is what skips validation, not a weakened check.
	// It ends by bringing the session down.
	t.Run("WarmAttachSurvivesAConfigErrorButColdStillRefuses", func(t *testing.T) {
		writeReedConfigWithOverride(t, worktree, "mouse", "sideways")

		warmOutput := runReedCLINoFatal(t, lymExe, worktree, 30*time.Second, "reed", "attach")
		if strings.Contains(warmOutput, "invalid mouse value") {
			t.Errorf("warm attach output = %q; want it NOT to refuse with the config error -- the liveness probe must precede the boot path's validation block for an already-live session", warmOutput)
		}

		var downOut bytes.Buffer
		if code := RunCLIIn(worktree, &downOut, []string{"down"}); code != 0 {
			t.Fatalf("down = %d; want 0, output: %s", code, downOut.String())
		}

		var coldOut bytes.Buffer
		if code := RunCLIIn(worktree, &coldOut, []string{"attach"}); code == 0 {
			t.Fatalf("cold attach with the same bad config = 0; want a refusal, output: %s", coldOut.String())
		}
		refusal := envelopeError(t, coldOut.Bytes())
		if !strings.Contains(refusal, "invalid mouse value") {
			t.Errorf("cold attach error = %s; want it to contain the config error -- the same bad config must still refuse a boot that has to validate it", refusal)
		}
	})
}
