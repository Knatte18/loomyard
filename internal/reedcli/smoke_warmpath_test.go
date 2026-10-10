//go:build tmux

// smoke_warmpath_test.go pins the other half of this task's hard boundary: a `reed add` or `reed attach` against a live session holding at least one pane performs no reconcile, no layout apply, no SaveState, and no config validation it did not already perform -- add gains one round trip (list-panes), attach gains two (has-session, list-panes), and nothing else.
// The scenario boots the session explicitly first, so the self-healing pre-flight under test takes its early return rather than its delegate-to-boot branch.
// See smoke_test.go for the shared fixture vocabulary this file builds on, and smoke_coldstart_test.go for its cold-path counterpart.

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
	"github.com/Knatte18/loomyard/internal/testkit/lyxbin"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// writeReedConfigWithOverride seeds worktree's reed config the way the engine's own integration
// fixture seeds it (contract_integration_test.go's seedReedConfig): the full config template,
// written to the config path the config engine resolves for the reed module under this worktree's
// anchor -- with exactly one key's value replaced, rather than a single-key fragment that would fail
// LoadOrTemplate's key-completeness check.
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

// TestSmokeWarmPath runs the warm-path claims against one hub whose prime worktree is brought up once with one strand, so each step reuses that live session instead of building its own hub and server.
// The steps run serially in a fixed order: each leaves the session up with the strand's state intact for the next, except the last, which tears the session down.
// The scenario calls t.Parallel but no step does, because every step shares the one hub and session.
func TestSmokeWarmPath(t *testing.T) {
	t.Parallel()
	tmuxPath := tmuxBinaryPath(t)
	lyxExe := lyxbin.Build(t)

	h := hubforge.NewHub(t, ".")
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
	statePath := filepath.Join(worktree, ".lyx", "reed.json")

	// WarmAddIsUnchanged pins the control case: warm add against a live session still yields exactly one session, one header pane, one live strand.
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

	// WarmAttachWritesNoState pins the third additive-only assertion: the modification time and bytes of .lyx/reed.json before a warm attach must be identical afterward.
	if !t.Run("WarmAttachWritesNoState", func(t *testing.T) {
		before, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatalf("read %s: %v", statePath, err)
		}
		beforeInfo, err := os.Stat(statePath)
		if err != nil {
			t.Fatalf("stat %s: %v", statePath, err)
		}

		runReedCLINoFatal(t, lyxExe, worktree, 30*time.Second, "reed", "attach")

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

	// WarmAttachDoesNotReapAnOperatorsPane pins the sharpest warm-path assertion: with the session already up and an untracked pane created directly through the multiplexer (the operator hand-split case), attach must leave that pane alive.
	// This is the step that fails loudly if the pre-flight is ever routed back through the boot path -- upLocked's tail reaches planReconcile, which adds every live non-exempt pane to its kill list the moment the header is alive.
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

		runReedCLINoFatal(t, lyxExe, worktree, 30*time.Second, "reed", "attach")

		if !paneLiveOnSession(listPaneLines(t, tmuxPath, socket, session), foreignPaneID) {
			t.Errorf("foreign pane %s not alive after a warm attach; want it untouched -- a warm attach must never reconcile", foreignPaneID)
		}
	}) {
		return
	}

	// WarmAttachStillRefusesOnAnUnreadableStateFile pins that a warm attach still aborts on the JSON envelope with the state loader's diagnosis, exactly as it does today, when .lyx/reed.json becomes unparseable underneath a live session.
	// This is the step that fails if the Status call is ever dropped from attach's pre-flight -- EnsureSession's own early return reads no state at all.
	// It restores the state file afterwards, so the steps after it find the strand's state intact.
	if !t.Run("WarmAttachStillRefusesOnAnUnreadableStateFile", func(t *testing.T) {
		original, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatalf("read %s: %v", statePath, err)
		}
		if err := os.WriteFile(statePath, []byte(`{"socket":"lyx-x","strands":[{"gu`), 0o600); err != nil {
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
		for _, want := range []string{"unreadable", "reed.json", "lyx reed down"} {
			if !strings.Contains(unreadable, want) {
				t.Errorf("warm attach error = %s; want it to contain %q (Status's state-loader diagnosis)", unreadable, want)
			}
		}
	}) {
		return
	}

	// WarmAttachSurvivesAConfigErrorButColdStillRefuses pins that the liveness probe precedes the boot path's whole pre-tmux config-validation block: with the session already up, an invalid mouse value written into this worktree's reed config must not make attach refuse.
	// The cold direction is asserted too, against the identical bad config with no session up, where the same config error must still fire -- together the two pin that EnsureSession's early return is what skips validation, not a weakened check.
	// It ends by bringing the session down, so it is the last step.
	t.Run("WarmAttachSurvivesAConfigErrorButColdStillRefuses", func(t *testing.T) {
		writeReedConfigWithOverride(t, worktree, "mouse", "sideways")

		warmOutput := runReedCLINoFatal(t, lyxExe, worktree, 30*time.Second, "reed", "attach")
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
