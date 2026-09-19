//go:build smoke

// smoke_warmpath_test.go pins the other half of this task's hard boundary: a `reed add` or `reed
// attach` against a live session holding at least one pane performs no reconcile, no layout apply, no
// SaveState, and no config validation it did not already perform -- add gains one round trip
// (list-panes), attach gains two (has-session, list-panes), and nothing else. Every test here boots
// the session explicitly first, so the self-healing pre-flight under test takes its early return
// rather than its delegate-to-boot branch. See smoke_test.go for the shared fixture vocabulary this
// file builds on, and smoke_coldstart_test.go for its cold-path counterpart.

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

// TestSmokeWarmAttachDoesNotReapAnOperatorsPane pins the sharpest warm-path assertion: with the
// session already up and an untracked pane created directly through the multiplexer (the operator
// hand-split case), attach must leave that pane alive. This is the test that fails loudly if the
// pre-flight is ever routed back through the boot path -- upLocked's tail reaches planReconcile,
// which adds every live non-exempt pane to its kill list the moment the header is alive.
func TestSmokeWarmAttachDoesNotReapAnOperatorsPane(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)
	lyxExe := buildLyxBinary(t)

	h := hubforge.NewHub(t, ".")
	worktree := h.PrimeWorktree()
	deferHubRelease(t, worktree)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(worktree, &buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLIIn(worktree, &out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	addStrandIn(t, worktree, smokeReapLaunchCmd(), "--name", "warm-attach-strand")

	socket, session := socketAndSessionIn(t, worktree)
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
}

// TestSmokeWarmAttachSurvivesAConfigErrorButColdStillRefuses pins that the liveness probe precedes
// the boot path's whole pre-tmux config-validation block: with the session already up, an invalid
// mouse value written into this worktree's reed config must not make attach refuse. The cold
// direction is asserted too, against the identical bad config with no session up, where the same
// config error must still fire -- together the two pin that EnsureSession's early return is what
// skips validation, not a weakened check.
func TestSmokeWarmAttachSurvivesAConfigErrorButColdStillRefuses(t *testing.T) {
	lyxExe := buildLyxBinary(t)

	h := hubforge.NewHub(t, ".")
	worktree := h.PrimeWorktree()
	deferHubRelease(t, worktree)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(worktree, &buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLIIn(worktree, &out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}

	writeReedConfigWithOverride(t, worktree, "mouse", "sideways")

	warmOutput := runReedCLINoFatal(t, lyxExe, worktree, 30*time.Second, "reed", "attach")
	if strings.Contains(warmOutput, "invalid mouse value") {
		t.Errorf("warm attach output = %q; want it NOT to refuse with the config error -- the liveness probe must precede the boot path's validation block for an already-live session", warmOutput)
	}

	out.Reset()
	if code := RunCLIIn(worktree, &out, []string{"down"}); code != 0 {
		t.Fatalf("down = %d; want 0, output: %s", code, out.String())
	}

	out.Reset()
	if code := RunCLIIn(worktree, &out, []string{"attach"}); code == 0 {
		t.Fatalf("cold attach with the same bad config = 0; want a refusal, output: %s", out.String())
	}
	refusal := envelopeError(t, out.Bytes())
	if !strings.Contains(refusal, "invalid mouse value") {
		t.Errorf("cold attach error = %s; want it to contain the config error -- the same bad config must still refuse a boot that has to validate it", refusal)
	}
}

// TestSmokeWarmAttachWritesNoState pins the third additive-only assertion: the modification time and
// bytes of .lyx/reed.json before a warm attach must be identical afterward.
func TestSmokeWarmAttachWritesNoState(t *testing.T) {
	lyxExe := buildLyxBinary(t)

	h := hubforge.NewHub(t, ".")
	worktree := h.PrimeWorktree()
	deferHubRelease(t, worktree)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(worktree, &buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLIIn(worktree, &out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	addStrandIn(t, worktree, smokeReapLaunchCmd(), "--name", "warm-attach-no-write")

	statePath := filepath.Join(worktree, ".lyx", "reed.json")
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
}

// TestSmokeWarmAttachStillRefusesOnAnUnreadableStateFile pins that a warm attach still aborts on the
// JSON envelope with the state loader's diagnosis, exactly as it does today, when .lyx/reed.json
// becomes unparseable underneath a live session. This is the test that fails if the Status call is
// ever dropped from attach's pre-flight -- EnsureSession's own early return reads no state at all.
func TestSmokeWarmAttachStillRefusesOnAnUnreadableStateFile(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	worktree := h.PrimeWorktree()
	deferHubRelease(t, worktree)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(worktree, &buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLIIn(worktree, &out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	addStrandIn(t, worktree, smokeReapLaunchCmd(), "--name", "warm-attach-unreadable")

	statePath := filepath.Join(worktree, ".lyx", "reed.json")
	if err := os.WriteFile(statePath, []byte(`{"socket":"lyx-x","strands":[{"gu`), 0o600); err != nil {
		t.Fatalf("truncate %s: %v", statePath, err)
	}

	out.Reset()
	if code := RunCLIIn(worktree, &out, []string{"attach"}); code == 0 {
		t.Fatalf("warm attach with an unreadable reed.json = 0; want a failure, output: %s", out.String())
	}
	unreadable := envelopeError(t, out.Bytes())
	for _, want := range []string{"unreadable", "reed.json", "lyx reed down"} {
		if !strings.Contains(unreadable, want) {
			t.Errorf("warm attach error = %s; want it to contain %q (Status's state-loader diagnosis)", unreadable, want)
		}
	}
}

// TestSmokeWarmAddIsUnchanged pins the control case: warm add against a live session still yields
// exactly one session, one header pane, one live strand -- unchanged from before this task.
func TestSmokeWarmAddIsUnchanged(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	worktree := h.PrimeWorktree()
	deferHubRelease(t, worktree)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(worktree, &buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLIIn(worktree, &out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	guid := addStrandIn(t, worktree, smokeReapLaunchCmd(), "--name", "warm-add")

	socket := reedengine.ServerName(h.Path)
	session := reedengine.SessionName(worktree)
	if !sessionAlive(tmuxPath, socket, session) {
		t.Fatalf("session %s not alive on socket %s after a warm add", session, socket)
	}

	out.Reset()
	if code := RunCLIIn(worktree, &out, []string{"status"}); code != 0 {
		t.Fatalf("status = %d; want 0, output: %s", code, out.String())
	}
	var statusResult map[string]any
	if err := json.Unmarshal(out.Bytes(), &statusResult); err != nil {
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
}
