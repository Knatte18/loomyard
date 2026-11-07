// contract_integration_test.go pins the psmux/tmux wire contract against the real binary that LoadConfig resolves.
// It catches drift in that binary, and the same assertions run unchanged against whichever binary is configured.
// The test skips when that binary is absent.
// It is separate from the agent-driven SANDBOX-REED-SUITE, which drives a real hub through the CLI.
// This test pins the narrower wire contract in isolation.
// It uses its own scratch socket, so it never collides with a real hub server.

package reedengine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// seedReedConfig writes the minimal on-disk config structure for LoadConfig.
func seedReedConfig(t *testing.T, tmpDir string) {
	t.Helper()
	lymDir := filepath.Join(tmpDir, lymdirs.LymDirName)
	if err := os.Mkdir(lymDir, 0o755); err != nil {
		t.Fatalf("mkdir _lym: %v", err)
	}
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatalf("mkdir _lym/config: %v", err)
	}
	configFile := configengine.ConfigFile(tmpDir, "reed")
	if err := os.WriteFile(configFile, []byte(ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}

// waitUntil polls cond every 100ms until it reports true or timeout elapses.
func waitUntil(t *testing.T, timeout time.Duration, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: condition never became true within %s", msg, timeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestMultiplexerContract validates the multiplexer wire contract against a real binary.
func TestMultiplexerContract(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	tmpDir := t.TempDir()
	seedReedConfig(t, tmpDir)

	cfg, err := LoadConfig(configengine.Dirs{Anchor: tmpDir}, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// The test validates whatever binary is configured, so an absent binary means nothing to validate, not a failure.
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}

	socket := tmuxkit.Socket(t, cfg.Tmux)

	session := "contract-session"
	reed := NewTmuxCmd(cfg.Tmux, socket)

	// new-session matches the shape ensureServerAndSessionLocked spawns: -x/-y sizing and a real shell command.
	// The session and socket are scratch ones this test owns exclusively.
	if err := reed.run("new-session", "-d", "-s", session, "-x", "80", "-y", "24", cfg.Shell); err != nil {
		t.Fatalf("new-session: %v", err)
	}

	// remain-on-exit keeps a pane after its command exits, so the dead-pane assertion below can read it.
	// Production sets it at boot in lifecycle.go.
	// This scratch session boots without ensureServerAndSessionLocked, so the test sets it too.
	if err := reed.run("set-option", "-g", "remain-on-exit", "on"); err != nil {
		t.Fatalf("set-option remain-on-exit: %v", err)
	}

	// has-session: the subcommand hasSession wraps.
	up, err := reed.hasSession(session)
	if err != nil {
		t.Fatalf("has-session: %v", err)
	}
	if !up {
		t.Fatal("has-session reported the freshly created session absent")
	}

	// (a) pins the exact list-panes -F output shape and its parsePaneList parse.
	// The raw format string is called directly, not through listPanes.
	// Drift in the literal itself fails here, not only in parsePaneList's tolerance of the output.
	const paneFormat = "#{pane_id} #{pane_dead} #{pane_top} #{pane_width} #{pane_height} #{pane_pid}"
	rawOut, err := reed.output("list-panes", "-t", session, "-F", paneFormat)
	if err != nil {
		t.Fatalf("list-panes: %v", err)
	}
	rawLines := strings.Split(strings.TrimSpace(rawOut), "\n")
	if len(rawLines) != 1 {
		t.Fatalf("list-panes -F %q = %d line(s), want exactly 1 for a freshly created session:\n%s", paneFormat, len(rawLines), rawOut)
	}
	if fields := strings.Fields(rawLines[0]); len(fields) != 6 {
		t.Fatalf("list-panes line %q has %d field(s), want 6 (#{pane_id} #{pane_dead} #{pane_top} #{pane_width} #{pane_height} #{pane_pid})", rawLines[0], len(fields))
	}

	parsed, err := parsePaneList(rawOut)
	if err != nil {
		t.Fatalf("parsePaneList(%q): %v", rawOut, err)
	}
	if len(parsed) != 1 {
		t.Fatalf("parsePaneList(%q) = %d pane(s), want 1", rawOut, len(parsed))
	}
	initialPane := parsed[0]
	if initialPane.Dead {
		t.Errorf("initial pane reports Dead = true, want false (freshly created pane's command has not exited)")
	}
	if initialPane.Width != 80 || initialPane.Height != 24 {
		t.Errorf("initial pane size = %dx%d, want 80x24 (the -x/-y new-session args)", initialPane.Width, initialPane.Height)
	}
	if initialPane.PID <= 0 {
		t.Errorf("initial pane PID = %d, want a positive OS pid", initialPane.PID)
	}

	// listPanes (overlay.go) wraps the same format string and parser, so its result matches the raw parse above.
	viaListPanes, err := reed.listPanes(exactSessionWindowTarget(session))
	if err != nil {
		t.Fatalf("listPanes: %v", err)
	}
	if len(viaListPanes) != 1 || viaListPanes[0].ID != initialPane.ID {
		t.Errorf("listPanes() = %+v, want a single pane matching the raw parse %+v", viaListPanes, initialPane)
	}

	// pwsh takes a moment to print its first prompt, and keys sent before then reach a shell that is not yet listening.
	// Poll until the pane has output before driving it, rather than sleep for a fixed time.
	waitUntil(t, 15*time.Second, "initial pane never produced a prompt", func() bool {
		out, err := reed.output("capture-pane", "-p", "-t", initialPane.ID)
		return err == nil && strings.TrimSpace(out) != ""
	})

	// (c) send-keys -l with a leading-dash payload: psmux/tmux parses a bare '-'-prefixed literal as flags and drops it.
	// sendKeysLiteralArg's one-space prefix must carry the payload through to the pane verbatim.
	// The payload is typed without a trailing Enter, so this checks delivery, not shell execution.
	const dashPayload = "-contract-dash-marker"
	if err := reed.run("send-keys", "-t", initialPane.ID, "-l", sendKeysLiteralArg(dashPayload)); err != nil {
		t.Fatalf("send-keys -l %q: %v", dashPayload, err)
	}
	// capture-pane wraps output at the 80-column pane width, which can split the payload word across a line break.
	// A long cwd in the prompt can push the wrap point mid-word.
	// Stripping embedded newlines before matching rejoins any such wrap.
	// The assertion checks delivery, not terminal line-wrapping.
	var lastCapture string
	waitUntil(t, 10*time.Second, fmt.Sprintf("capture-pane never showed the literal payload %q", dashPayload), func() bool {
		out, err := reed.output("capture-pane", "-p", "-t", initialPane.ID)
		if err != nil {
			return false
		}
		lastCapture = out
		return strings.Contains(strings.ReplaceAll(out, "\n", ""), dashPayload)
	})
	if !strings.Contains(strings.ReplaceAll(lastCapture, "\n", ""), dashPayload) {
		t.Errorf("capture-pane after send-keys -l %q = %q, want it to contain the literal payload (leading-dash bug not worked around)", dashPayload, lastCapture)
	}
	// Clear the typed-but-not-submitted line before the pane is reused below.
	if err := reed.run("send-keys", "-t", initialPane.ID, "C-c"); err != nil {
		t.Fatalf("send-keys C-c: %v", err)
	}

	// (b) split-window: the raw split the wire contract must honour.
	// launchStrandLocked creates every pane through new-window.
	splitOut, err := reed.output("split-window", "-t", initialPane.ID, "-P", "-F", "#{pane_id}")
	if err != nil {
		t.Fatalf("split-window: %v", err)
	}
	secondPaneID := strings.TrimSpace(splitOut)
	if secondPaneID == "" || secondPaneID == initialPane.ID {
		t.Fatalf("split-window -P -F #{pane_id} = %q, want a new, distinct pane id (target %s)", splitOut, initialPane.ID)
	}

	// (c) select-layout succeeds against the live pane set.
	// A built-in tmux/psmux layout keyword is enough:
	// the check only needs to show the subcommand works against a live two-pane session.
	if err := reed.run("select-layout", "-t", session, "even-vertical"); err != nil {
		t.Fatalf("select-layout: %v", err)
	}

	if err := reed.run("select-pane", "-t", secondPaneID); err != nil {
		t.Fatalf("select-pane: %v", err)
	}

	// set-hook and resize-pane are optional wire surface: they are left out of requiredSubcommands,
	// so this section documents their semantics instead of gating the capability probe on them.
	// It pins two wire behaviours that the resize-pin hook in windowsize.go relies on and no unit test can reach:
	// array independence across set-hook -u, set-hook and set-hook -a,
	// and a window-resized hook firing after tmux has already resized the layout.
	windowTarget := exactSessionWindowTarget(session)

	// set-hook -u followed by set-hook and set-hook -a yields independent array entries.
	// Entry [0] names a pane id that no session on this socket owns.
	// The live-firing half below can assert that its failure does not take entry [1] down with it.
	if err := reed.run("set-hook", "-u", "-w", "-t", windowTarget, "window-resized"); err != nil {
		t.Fatalf("set-hook -u -w -t %q window-resized: %v", windowTarget, err)
	}
	if err := reed.run("set-hook", "-w", "-t", windowTarget, "window-resized", "resize-pane -t %99 -y 1"); err != nil {
		t.Fatalf("set-hook -w -t %q window-resized (entry [0]): %v", windowTarget, err)
	}
	if err := reed.run("set-hook", "-a", "-w", "-t", windowTarget, "window-resized", fmt.Sprintf("resize-pane -t %s -y 1", initialPane.ID)); err != nil {
		t.Fatalf("set-hook -a -w -t %q window-resized (entry [1]): %v", windowTarget, err)
	}

	// Read the array back: one line per entry, each naming its own pane id.
	// The readback shape is the one tmux 3.6 prints.
	hooksOut, err := reed.output("show-hooks", "-w", "-t", windowTarget)
	if err != nil {
		t.Fatalf("show-hooks -w -t %q: %v", windowTarget, err)
	}
	if !strings.Contains(hooksOut, "window-resized[0]") || !strings.Contains(hooksOut, "%99") {
		t.Errorf("show-hooks -w -t %q = %q, want a window-resized[0] line naming pane %%99", windowTarget, hooksOut)
	}
	if !strings.Contains(hooksOut, "window-resized[1]") || !strings.Contains(hooksOut, initialPane.ID) {
		t.Errorf("show-hooks -w -t %q = %q, want a window-resized[1] line naming pane %s", windowTarget, hooksOut, initialPane.ID)
	}

	// A window-resized hook fires after tmux has resized the layout,
	// so a resize-pane -y inside it survives the resize that triggered it,
	// and the dead entry [0] does not take entry [1] down with it.
	// window-size manual is required to make a detached session resizable at all.
	if err := reed.run("set-option", "-w", "-t", windowTarget, "window-size", "manual"); err != nil {
		t.Fatalf("set-option -w -t %q window-size manual: %v", windowTarget, err)
	}
	if err := reed.run("resize-window", "-t", windowTarget, "-x", "80", "-y", "60"); err != nil {
		t.Fatalf("resize-window -t %q -x 80 -y 60: %v", windowTarget, err)
	}
	waitUntil(t, 10*time.Second, "listPanes never reported the window fully laid out at 60 rows", func() bool {
		live, err := reed.listPanes(exactSessionWindowTarget(session))
		if err != nil {
			return false
		}
		maxBottom := 0
		for _, p := range live {
			if bottom := p.Top + p.Height; bottom > maxBottom {
				maxBottom = bottom
			}
		}
		return maxBottom == 60
	})
	afterResizeLive, err := reed.listPanes(exactSessionWindowTarget(session))
	if err != nil {
		t.Fatalf("listPanes after resize-window: %v", err)
	}
	var pinnedHeight int
	var pinnedFound bool
	for _, p := range afterResizeLive {
		if p.ID == initialPane.ID {
			pinnedHeight = p.Height
			pinnedFound = true
		}
	}
	if !pinnedFound {
		t.Fatalf("listPanes after resize-window = %+v, want pane %s still present", afterResizeLive, initialPane.ID)
	}
	if pinnedHeight != 1 {
		t.Errorf("pane %s height after resize-window = %d, want exactly 1 (the resize-pane -y entry [1] fired after tmux's own resize and pinned it)", initialPane.ID, pinnedHeight)
	}

	// Leave the window as the later steps expect it.
	// Clear the hook array, and drop the window-size override with the -u unset form.
	// Reading the prior value back and re-setting it would be the ad-hoc alternative.
	if err := reed.run("set-hook", "-u", "-w", "-t", windowTarget, "window-resized"); err != nil {
		t.Fatalf("set-hook -u -w -t %q window-resized (final clear): %v", windowTarget, err)
	}
	if err := reed.run("set-option", "-uw", "-t", windowTarget, "window-size"); err != nil {
		t.Fatalf("set-option -uw -t %q window-size (unset): %v", windowTarget, err)
	}

	// (b) list-sessions: distinguishes "no server" from "server up".
	sessionsOut, err := reed.output("list-sessions", "-F", "#{session_name}")
	if err != nil {
		t.Fatalf("list-sessions: %v", err)
	}
	if strings.TrimSpace(sessionsOut) != session {
		t.Errorf("list-sessions -F #{session_name} = %q, want %q", strings.TrimSpace(sessionsOut), session)
	}

	// (b) display-message: the #{pid} format variable names the server's OS pid that Down's process-exit wait needs.
	pidOut, err := reed.output("display-message", "-p", "-t", session, "#{pid}")
	if err != nil {
		t.Fatalf("display-message: %v", err)
	}
	if pid, err := strconv.Atoi(strings.TrimSpace(pidOut)); err != nil || pid <= 0 {
		t.Errorf("display-message -p #{pid} = %q, want a positive integer pid", pidOut)
	}

	// (c) remain-on-exit keeps a dead pane visible with pane_dead=1:
	// make the second pane's shell exit, then poll until list-panes reports it dead rather than absent.
	if err := reed.run("send-keys", "-t", secondPaneID, "-l", "exit"); err != nil {
		t.Fatalf("send-keys exit: %v", err)
	}
	if err := reed.run("send-keys", "-t", secondPaneID, "Enter"); err != nil {
		t.Fatalf("send-keys Enter: %v", err)
	}
	waitUntil(t, 10*time.Second, "second pane never reported dead under remain-on-exit", func() bool {
		live, err := reed.listPanes(exactSessionWindowTarget(session))
		if err != nil {
			return false
		}
		for _, p := range live {
			if p.ID == secondPaneID {
				return p.Dead
			}
		}
		// An absent pane, not merely a dead one, means remain-on-exit failed to keep the corpse visible,
		// so the poll treats it as not yet satisfied, and the deadline message covers both failure shapes.
		return false
	})

	if err := reed.run("kill-pane", "-t", secondPaneID); err != nil {
		t.Fatalf("kill-pane: %v", err)
	}
	live, err := reed.listPanes(exactSessionWindowTarget(session))
	if err != nil {
		t.Fatalf("list panes after kill-pane: %v", err)
	}
	for _, p := range live {
		if p.ID == secondPaneID {
			t.Errorf("pane %s still present after kill-pane", secondPaneID)
		}
	}

	// (b) kill-session: the scratch server is left running for t.Cleanup's kill-server to reap.
	if err := reed.run("kill-session", "-t", session); err != nil {
		t.Fatalf("kill-session: %v", err)
	}
	if stillUp, err := reed.hasSession(session); err == nil && stillUp {
		t.Errorf("has-session reports %q still present after kill-session", session)
	}
}

// TestExactSessionTargetsNeverPrefixMatchSiblings checks the exact-match target forms.
// They are "=<name>" for a session and "=<name>:" for its window.
// Each resolves only the named session on a real multiplexer.
// A bare -t name falls back to prefix matching when no session matches exactly.
// With sessions "repo" and "repo2" on one server, a bare kill-session -t repo after "repo" is gone kills "repo2",
// and a bare has-session -t repo reports "repo2" present.
// The test asserts that the exact forms resolve the named session while it exists,
// and that they error once it is gone without touching the sibling.
// It is the canary for a configured binary, such as psmux, that lacks the "=" target syntax.
func TestExactSessionTargetsNeverPrefixMatchSiblings(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	tmpDir := t.TempDir()
	seedReedConfig(t, tmpDir)

	cfg, err := LoadConfig(configengine.Dirs{Anchor: tmpDir}, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}

	socket := tmuxkit.PackageServer(t, cfg.Tmux)
	// The sibling's name extends session's, so a prefix-match fallback would hit the sibling once session is gone.
	const session = "exact-target"
	const sibling = "exact-target2"
	reed := NewTmuxCmd(cfg.Tmux, socket)

	for _, name := range []string{session, sibling} {
		if err := reed.run("new-session", "-d", "-s", name, "-x", "80", "-y", "24", cfg.Shell); err != nil {
			t.Fatalf("new-session %s: %v", name, err)
		}
	}

	// While the exact-named session exists, both grammars resolve it.
	if up, err := reed.hasSession(session); err != nil || !up {
		t.Fatalf("hasSession(%q) = (%v, %v), want (true, nil) while it exists", session, up, err)
	}
	if _, err := reed.listPanes(exactSessionWindowTarget(session)); err != nil {
		t.Fatalf("listPanes(%q) via exact window target: %v", session, err)
	}
	if _, err := reed.output("display-message", "-p", "-t", exactSessionWindowTarget(session), "#{pid}"); err != nil {
		t.Fatalf("display-message -t %q: %v", exactSessionWindowTarget(session), err)
	}
	if err := reed.run("select-layout", "-t", exactSessionWindowTarget(session), "even-vertical"); err != nil {
		t.Fatalf("select-layout -t %q: %v", exactSessionWindowTarget(session), err)
	}

	// Kill the exact-named session, leaving only the prefix-sharing sibling.
	if err := reed.run("kill-session", "-t", exactSessionTarget(session)); err != nil {
		t.Fatalf("kill-session -t %q: %v", exactSessionTarget(session), err)
	}

	// The trap the "=" forms exist for: every exact-target probe must now
	// report the session ABSENT/ERROR rather than resolving the sibling.
	if up, err := reed.hasSession(session); err != nil || up {
		t.Fatalf("hasSession(%q) = (%v, %v) with only %q present, want (false, nil) — a true result means the target prefix-matched the sibling", session, up, err, sibling)
	}
	if _, err := reed.listPanes(exactSessionWindowTarget(session)); err == nil {
		t.Fatalf("listPanes(%q) succeeded with only %q present, want an error — success means the window target prefix-matched the sibling", session, sibling)
	}
	// The idempotent-down shape: a second kill-session against the gone
	// session must error, and above all must NOT kill the sibling.
	if err := reed.run("kill-session", "-t", exactSessionTarget(session)); err == nil {
		t.Errorf("kill-session -t %q succeeded with only %q present, want an error — success means it prefix-matched (and killed) the sibling", exactSessionTarget(session), sibling)
	}
	if up, err := reed.hasSession(sibling); err != nil || !up {
		t.Fatalf("hasSession(%q) = (%v, %v) after the re-kill, want (true, nil) — the sibling must survive every exact-target op against its prefix", sibling, up, err)
	}
}

// TestDisplayMessageDoesNotErrorForAnAbsentSession pins the wire-contract fact that the pane-generation
// guards rely on: display-message does not report an absent session as an error.
//
// Unlike has-session and list-panes, which exit 1 for a -t target naming no session, display-message
// exits 0 and expands every session-scoped format to empty while the server-global #{pid} still fills.
// It does not fall back to a current session when other sessions exist on the socket.
// The generation probe's absent-session case therefore surfaces as parsePaneGeneration's empty-field rejection.
// That rejection is load-bearing.
// Relaxing it would stop the still-running-orphan check from ever refusing.
// It would also let adoptPaneGenerationLocked clear every worktree's bindings on every op.
// The assertions below pair the raw wire answer with what parsePaneGeneration makes of it,
// so the connection cannot be broken from either end without a failure.
func TestDisplayMessageDoesNotErrorForAnAbsentSession(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	tmpDir := t.TempDir()
	seedReedConfig(t, tmpDir)

	cfg, err := LoadConfig(configengine.Dirs{Anchor: tmpDir}, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}

	socket := tmuxkit.PackageServer(t, cfg.Tmux)
	const present = "absent-probe-present"
	const absent = "absent-probe-missing"
	reed := NewTmuxCmd(cfg.Tmux, socket)

	if err := reed.run("new-session", "-d", "-s", present, "-x", "80", "-y", "24", cfg.Shell); err != nil {
		t.Fatalf("new-session %s: %v", present, err)
	}

	// The present session answers a complete, parseable stamp.
	out, err := reed.output("display-message", "-p", "-t", exactSessionWindowTarget(present), paneGenerationFormat)
	if err != nil {
		t.Fatalf("display-message for the present session %q: %v", present, err)
	}
	presentGeneration, err := parsePaneGeneration(present, out)
	if err != nil {
		t.Fatalf("parsePaneGeneration(%q) for the present session: %v", out, err)
	}
	if !presentGeneration.Recorded() {
		t.Errorf("parsePaneGeneration(%q) = %+v; want a recorded generation", out, presentGeneration)
	}

	// The absent session answers with exit 0.
	// A tmux that errors here instead would make the probe's error path mean two different things.
	absentOut, err := reed.output("display-message", "-p", "-t", exactSessionWindowTarget(absent), paneGenerationFormat)
	if err != nil {
		t.Fatalf("display-message for the absent session %q = %v; want no error — the guards in generation.go are built on display-message NOT reporting absence as an error, and doc.go states that as a pinned contract", absent, err)
	}

	// It must not resolve to the one session that IS on the socket, which would make an absent
	// session indistinguishable from a live namesake.
	if strings.Contains(absentOut, presentGeneration.TmuxSessionID) {
		t.Errorf("display-message -t %q answered %q, which carries the present session's id %q; want the session-scoped fields empty — a fallback to a current session would let a gone session masquerade as a live one",
			exactSessionWindowTarget(absent), absentOut, presentGeneration.TmuxSessionID)
	}

	// And the empty-field answer must be what parsePaneGeneration rejects, since that rejection is
	// the only thing standing between "the session is gone" and a partial stamp reed would compare
	// against every future probe.
	if _, err := parsePaneGeneration(absent, absentOut); err == nil {
		t.Errorf("parsePaneGeneration(%q) for an absent session = nil error; want a rejection — the orphan check reads a probe failure as 'not identifiable', and a partial stamp accepted here would be compared as an identity", absentOut)
	}
}

// TestSessionNameRewriteIsSilentAndExactTargetsMissIt pins the wire-contract fact behind
// validateToldTmuxIdentity (server.go): tmux does not reject a session name containing '.' or ':'.
// It rewrites each to '_', creates the session under the rewritten name, and exits 0.
// That silence is the hazard.
// Every session target this package issues is the exact-match "=<name>" form,
// so the created session cannot be reached by the name that created it.
// The boot loop polls forever, the operator sees a timeout naming no cause,
// and the rewritten session is left on a shared per-hub server with no reed verb able to address it.
// A future tmux change would then fail a test rather than silently weakening the guard.
// It might be a hard rejection, a different substitute character, or a wider rewrite set.
func TestSessionNameRewriteIsSilentAndExactTargetsMissIt(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	tmpDir := t.TempDir()
	seedReedConfig(t, tmpDir)

	cfg, err := LoadConfig(configengine.Dirs{Anchor: tmpDir}, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}

	socket := tmuxkit.PackageServer(t, cfg.Tmux)
	reed := NewTmuxCmd(cfg.Tmux, socket)

	tests := []struct {
		name      string
		requested string
		rewritten string
	}{
		{"dot", "rewrite-dot.v2", "rewrite-dot_v2"},
		{"colon", "rewrite-colon:v2", "rewrite-colon_v2"},
		// The vis-encode half of the same rewrite: a raw TAB becomes two literal characters, backslash and t, not '_'.
		// It is pinned so that a tmux which starts rejecting or re-encoding control characters fails here.
		{"tab", "rewrite-tab\tv2", `rewrite-tab\tv2`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// new-session SUCCEEDS: this is the silence the guard exists for.
			if err := reed.run("new-session", "-d", "-s", tt.requested, "-x", "80", "-y", "24", cfg.Shell); err != nil {
				t.Fatalf("new-session -s %q = %v; want nil — this test's premise is that tmux ACCEPTS the name rather than rejecting it", tt.requested, err)
			}

			// ...but under a different name than the one asked for.
			out, err := reed.output("list-sessions", "-F", "#{session_name}")
			if err != nil {
				t.Fatalf("list-sessions: %v", err)
			}
			names := strings.Fields(strings.TrimSpace(out))
			if !slices.Contains(names, tt.rewritten) {
				t.Fatalf("list-sessions = %v after new-session -s %q; want it to contain the rewritten name %q", names, tt.requested, tt.rewritten)
			}
			if slices.Contains(names, tt.requested) {
				t.Fatalf("list-sessions = %v; want the requested name %q ABSENT — if tmux stopped rewriting, validateToldTmuxIdentity's ban is now over-strict and must be revisited", names, tt.requested)
			}

			// The exact-match target this package always uses misses it as well.
			// So reed cannot tear the session down.
			if up, err := reed.hasSession(tt.requested); err != nil || up {
				t.Fatalf("hasSession(%q) = (%v, %v); want (false, nil) — an exact target must not resolve the rewritten session", tt.requested, up, err)
			}
			// Sessions are not killed per case: killing the last session on a socket takes the server down with it.
			// The next case's new-session would then race that asynchronous teardown ("server exited unexpectedly").
			// The package server's kill-server tears every case's session down together when the package ends.
		})
	}
}

// TestRemoveStrand_SoleStrandEmptiesSessionSucceeds checks removing a session's sole non-hidden strand.
// It requires success, zero persisted strands in reed.json, and both the session and the Selvage pane still alive.
// Selvage is a permanent second pane, so killing the strand's pane is never the last pane destroyed on any backend.
// The empty-session branch of RemoveStrand is therefore not reached here.
// It stays as a guard for when Selvage is absent.
func TestRemoveStrand_SoleStrandEmptiesSessionSucceeds(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	tmpDir := t.TempDir()
	seedReedConfig(t, tmpDir)

	cfg, err := LoadConfig(configengine.Dirs{Anchor: tmpDir}, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// On a box without the configured multiplexer binary there is nothing to drive,
	// so the test skips, as TestMultiplexerContract does.
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}

	// The Geometry literal is rooted at a scratch tmpDir and built against the LoadConfig-resolved cfg.
	// Its hub and worktree-root shape mirrors newTestEngine in enginefixture_helper_test.go.
	// newTestEngine uses stub paths that do not exist; this literal does not.
	// It is built directly rather than through hubgeom.ReedGeometry, because hubgeom imports reedengine,
	// and an in-package test importing it would close an import cycle.
	hub := filepath.Dir(tmpDir)
	geom := Geometry{
		SocketKey:     ServerName(hub),
		SessionName:   SessionName(tmpDir),
		AnchorPath:    tmpDir,
		PaneCwd:       tmpDir,
		WorktreeRoot:  tmpDir,
		LogsDir:       filepath.Join(hub, "logs"),
		HubPath:       hub,
		WorktreeName:  filepath.Base(tmpDir),
		NameShortname: "tc",
		NameSlug:      "tslug",
	}
	e := New(cfg, geom, testScrub)

	// Registered before the Down cleanup, so it runs after it (cleanups run last-in-first-out).
	// It guards against a scratch server leaked by a test failure that never reached RemoveStrand,
	// and it removes the server's socket file.
	tmuxkit.KillOnCleanup(t, cfg.Tmux, geom.SocketKey)
	t.Cleanup(func() {
		// Best-effort: the code under test has normally already torn the session down.
		// On tmux the whole server goes too, since this scratch server held only that session;
		// Down's error here is therefore expected and ignored.
		_, _ = e.Down()

		// Reed removes the socket file of the server it tore down;
		// the KillOnCleanup above runs after this and would hide a missed removal, so the check sits here.
		if runtime.GOOS != "windows" {
			if _, err := os.Lstat(filepath.Join(socketDirFromEnv(), geom.SocketKey)); err == nil {
				t.Errorf("socket file for key %q remains after Down, want it removed", geom.SocketKey)
			}
		}
	})

	if _, err := e.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Selvage is booted as part of Up, before any strand exists.
	// Its id is captured so the post-remove assertions can confirm that this pane, not merely some pane, survived.
	upSt, err := LoadState(e.stateDir())
	if err != nil || upSt == nil || upSt.SelvagePaneID == "" {
		t.Fatalf("LoadState after Up = (%+v, %v), want a persisted SelvagePaneID", upSt, err)
	}
	selvagePaneID := upSt.SelvagePaneID

	// One non-hidden strand, anchored so it is realized into a live pane at add time.
	// Its command is long-lived, so the strand is still running when removed.
	strand, err := e.AddStrand(AddSpec{
		Cmd:     "sleep 300",
		Display: render.Display{Anchor: render.AnchorOwnWindow},
	})
	if err != nil {
		t.Fatalf("AddStrand: %v", err)
	}

	removed, err := e.RemoveStrand(strand.GUID, false)
	if err != nil {
		t.Fatalf("RemoveStrand(sole strand) = %v, want nil error (Selvage keeps the session alive, so emptying the strand table is never a last-pane-destroy)", err)
	}
	if len(removed.Strands) != 1 || removed.Strands[0].GUID != strand.GUID {
		t.Fatalf("RemoveStrand.Removed.Strands = %+v, want exactly guid %q", removed.Strands, strand.GUID)
	}

	// Persistence is the guard against a removed strand reappearing on resume.
	// removeStrandLocked prunes st.Strands in memory only.
	// So this must reload from disk, not trust the in-memory Removed result above.
	waitUntil(t, 5*time.Second, "persisted reed.json never reflected the emptied strand table", func() bool {
		st, err := LoadState(e.stateDir())
		return err == nil && st != nil && len(st.Strands) == 0
	})

	// The session, and Selvage specifically, must still be alive with zero strands tracked.
	up, err := e.tmux.hasSession(e.SessionName())
	if err != nil || !up {
		t.Fatalf("hasSession after removing the sole strand = (%v, %v), want (true, nil) — Selvage must keep the session alive", up, err)
	}
	live, err := e.tmux.listPanes(exactSessionWindowTarget(e.SessionName()))
	if err != nil {
		t.Fatalf("listPanes after removing the sole strand: %v", err)
	}
	selvageFound := false
	for _, p := range live {
		if p.ID == selvagePaneID {
			selvageFound = true
			if p.Dead {
				t.Errorf("Selvage pane %s reports Dead = true after removing the sole strand, want it alive", selvagePaneID)
			}
		}
	}
	if !selvageFound {
		t.Fatalf("Selvage pane %s missing from live panes %+v after removing the sole strand", selvagePaneID, live)
	}
}

// TestDeadSelvagePaneIsHealedByUpWithoutCorruptingLayout drives the dead-Selvage lifecycle end to end.
// It runs against a real multiplexer.
// Selvage's shell process exits, leaving its pane dead (pane_dead=1 under remain-on-exit).
// The next Up must heal Selvage by splitting a fresh one off the corpse, the window's only pane.
// It then kills the corpse and persists the new id.
// It must not treat the corpse as a working Selvage.
// The shell window must hold exactly one pane after a fresh up.
// It must still hold exactly one after the healing up and after a resize-window with no client attached.
// That pane is the live Selvage, and its height equals the window's height.
// The fixture attaches no client, so window-size latest has none to follow.
func TestDeadSelvagePaneIsHealedByUpWithoutCorruptingLayout(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	tmpDir := t.TempDir()
	seedReedConfig(t, tmpDir)

	cfg, err := LoadConfig(configengine.Dirs{Anchor: tmpDir}, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}

	hub := filepath.Dir(tmpDir)
	geom := Geometry{
		SocketKey:     ServerName(hub),
		SessionName:   SessionName(tmpDir),
		AnchorPath:    tmpDir,
		PaneCwd:       tmpDir,
		WorktreeRoot:  tmpDir,
		LogsDir:       filepath.Join(hub, "logs"),
		HubPath:       hub,
		WorktreeName:  filepath.Base(tmpDir),
		NameShortname: "tc",
		NameSlug:      "tslug",
	}
	e := New(cfg, geom, testScrub)

	tmuxkit.KillOnCleanup(t, cfg.Tmux, geom.SocketKey)
	t.Cleanup(func() {
		_, _ = e.Down()
	})

	if _, err := e.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}
	st, err := LoadState(e.stateDir())
	if err != nil || st == nil || st.SelvagePaneID == "" {
		t.Fatalf("LoadState after Up = (%+v, %v), want a persisted SelvagePaneID", st, err)
	}
	deadSelvageID := st.SelvagePaneID

	assertOnlySelvageFillsTheWindow(t, e, st.SelvagePaneID, "after a fresh up")

	// Kill Selvage's shell by pid: Selvage does not read stdin the way a script-driven pane would react to typed input.
	// Killing #{pane_pid} is how a real shell death looks to tmux.
	// remain-on-exit then corpses the pane (pane_dead=1), and the flip is asynchronous, so the test polls.
	live, err := e.tmux.listPanes(exactSessionWindowTarget(e.SessionName()))
	if err != nil {
		t.Fatalf("listPanes before Selvage kill: %v", err)
	}
	for _, p := range live {
		if p.ID == deadSelvageID {
			proc, err := os.FindProcess(p.PID)
			if err != nil {
				t.Fatalf("FindProcess(Selvage pane pid %d): %v", p.PID, err)
			}
			if err := proc.Kill(); err != nil {
				t.Fatalf("kill Selvage pane pid %d: %v", p.PID, err)
			}
		}
	}
	waitUntil(t, 10*time.Second, "Selvage pane never reported dead", func() bool {
		live, err := e.tmux.listPanes(exactSessionWindowTarget(e.SessionName()))
		if err != nil {
			return false
		}
		for _, p := range live {
			if p.ID == deadSelvageID {
				return p.Dead
			}
		}
		return false
	})

	// The heal: Up must replace the corpse, which is the window's only pane.
	// It must put a fresh, alive Selvage in its place, one that fills the window, and persist its id.
	if _, err := e.Up(); err != nil {
		t.Fatalf("Up (heal) = %v, want success", err)
	}
	st, err = LoadState(e.stateDir())
	if err != nil || st == nil || st.SelvagePaneID == "" {
		t.Fatalf("LoadState after healing Up = (%+v, %v), want a persisted SelvagePaneID", st, err)
	}
	if st.SelvagePaneID == deadSelvageID {
		t.Fatalf("SelvagePaneID still names the corpse %s after the healing Up", deadSelvageID)
	}
	assertOnlySelvageFillsTheWindow(t, e, st.SelvagePaneID, "after the healing Up")

	// No client is attached, so window-size latest has none to follow and the resize holds.
	target := exactSessionWindowTarget(e.SessionName())
	if err := e.tmux.run("resize-window", "-t", target, "-x", "100", "-y", "40"); err != nil {
		t.Fatalf("resize-window: %v", err)
	}
	assertOnlySelvageFillsTheWindow(t, e, st.SelvagePaneID, "after a resize-window")
}

// assertOnlySelvageFillsTheWindow asserts that the shell window lists exactly one pane: the alive Selvage selvagePaneID.
// That pane's height must equal the window's height.
func assertOnlySelvageFillsTheWindow(t *testing.T, e *Engine, selvagePaneID, step string) {
	t.Helper()
	target := exactSessionWindowTarget(e.SessionName())
	live, err := e.tmux.listPanes(target)
	if err != nil {
		t.Fatalf("(%s) listPanes: %v", step, err)
	}
	if len(live) != 1 || live[0].ID != selvagePaneID {
		t.Fatalf("(%s) panes = %+v, want exactly the Selvage pane %s", step, live, selvagePaneID)
	}
	if live[0].Dead {
		t.Errorf("(%s) Selvage pane %s reports dead", step, selvagePaneID)
	}
	out := mustTmuxOutput(t, e, "display-message", "-p", "-t", target, "#{window_height}")
	windowHeight, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		t.Fatalf("(%s) window_height %q: %v", step, out, err)
	}
	if live[0].Height != windowHeight {
		t.Errorf("(%s) Selvage pane %s height = %d, want the window's height %d", step, selvagePaneID, live[0].Height, windowHeight)
	}
}

// TestStrandOpsIgnoreASecondCurrentWindow pins the window seam against a real multiplexer:
// with another window made current in the session, the seam still resolves the window holding the strands.
// An enumeration op, AddStrand, lands the new strand in a window of its own.
// The other window's pane stays unreaped, and the session-scoped pane-generation read still answers.
func TestStrandOpsIgnoreASecondCurrentWindow(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	e := newColdScratchEngine(t)
	if _, err := e.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}
	first, err := e.AddStrand(AddSpec{Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorOwnWindow}})
	if err != nil {
		t.Fatalf("AddStrand (first): %v", err)
	}
	st, err := LoadState(e.stateDir())
	if err != nil || st == nil {
		t.Fatalf("LoadState = (%+v, %v), want a readable state", st, err)
	}
	strandWindow := windowOfPane(t, e, st.SelvagePaneID)

	otherPane, err := e.tmux.output("new-window", "-P", "-F", "#{pane_id}", "-t", exactSessionWindowTarget(e.SessionName()), "-n", "other", "sleep 300")
	if err != nil {
		t.Fatalf("new-window: %v", err)
	}
	otherPane = strings.TrimSpace(otherPane)
	otherWindow := windowOfPane(t, e, otherPane)
	if otherWindow == strandWindow {
		t.Fatalf("second window id = %q, want it apart from the strand window %q", otherWindow, strandWindow)
	}

	target, err := e.storedStrandWindowTarget()
	if err != nil {
		t.Fatalf("storedStrandWindowTarget: %v", err)
	}
	if target != strandWindow {
		t.Errorf("strand window target = %q with %q current, want the strand window %q", target, otherWindow, strandWindow)
	}

	second, err := e.AddStrand(AddSpec{Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorOwnWindow}})
	if err != nil {
		t.Fatalf("AddStrand (second) with another window current: %v", err)
	}
	for _, guid := range []string{first.GUID, second.GUID} {
		after, err := LoadState(e.stateDir())
		if err != nil || after == nil {
			t.Fatalf("LoadState = (%+v, %v), want a readable state", after, err)
		}
		idx := slices.IndexFunc(after.Strands, func(s Strand) bool { return s.GUID == guid })
		if idx == -1 {
			t.Fatalf("strand %q missing from state %+v", guid, after.Strands)
		}
		if got := windowOfPane(t, e, after.Strands[idx].PaneID); got == strandWindow || got == otherWindow {
			t.Errorf("strand %q pane is in window %q, want a window of its own apart from the strand window %q and the other window %q", guid, got, strandWindow, otherWindow)
		}
	}
	if got := windowOfPane(t, e, otherPane); got != otherWindow {
		t.Errorf("the other window's pane is in window %q, want it left in %q", got, otherWindow)
	}

	if err := e.withOpLock(func() error {
		_, err := e.paneGenerationLocked(e.SessionName())
		return err
	}); err != nil {
		t.Errorf("pane-generation read with another window current: %v", err)
	}
}

// windowOfPane returns the id of the window holding paneID, failing the test when tmux cannot say.
func windowOfPane(t *testing.T, e *Engine, paneID string) string {
	t.Helper()
	out, err := e.tmux.output("display-message", "-p", "-t", paneID, "#{window_id}")
	if err != nil {
		t.Fatalf("window of pane %q: %v", paneID, err)
	}
	return strings.TrimSpace(out)
}
