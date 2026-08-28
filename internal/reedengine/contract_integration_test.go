//go:build integration

// contract_integration_test.go asserts the full psmux/tmux wire contract that
// doc.go's "Multiplexer contract surface" section pins, against a real,
// running instance of the binary LoadConfig resolves (psmux on Windows
// today, tmux on Linux in the deferred follow-up). It is the canary for both
// version drift in the on-box binary and the eventual tmux swap — the same
// assertions run unmodified against whichever binary is configured, and the
// test self-skips cleanly when that binary is absent. It complements, and
// does not replace, the existing agent-driven SANDBOX-REED-SUITE: that suite
// drives a real hub end to end through the CLI, while this test pins the
// narrower wire-level contract reedengine's own godoc claims, in isolation,
// on its own scratch socket so it can never collide with a real hub server.

package reedengine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// seedReedConfig writes the minimal on-disk config structure for LoadConfig.
func seedReedConfig(t *testing.T, tmpDir string) {
	t.Helper()
	lyxDir := filepath.Join(tmpDir, lyxdirs.LyxDirName)
	if err := os.Mkdir(lyxDir, 0o755); err != nil {
		t.Fatalf("mkdir _lyx: %v", err)
	}
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatalf("mkdir _lyx/config: %v", err)
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
	tmpDir := t.TempDir()
	seedReedConfig(t, tmpDir)

	cfg, err := LoadConfig(tmpDir, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// The self-skip: this test's whole point is to validate whatever binary
	// is actually configured, so an absent binary is "nothing to validate
	// here", not a failure.
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}

	socket := fmt.Sprintf("lyx-contract-test-%d-%d", os.Getpid(), time.Now().UnixNano())

	session := "contract-session"
	reed := NewTmuxCmd(cfg.Tmux, socket)

	t.Cleanup(func() {
		// Always torn down, success or failure: a leaked scratch server on a
		// pid/timestamp socket is harmless to a real hub server, but leaves a
		// stray process behind if the test does not clean up after itself.
		_ = reed.run("kill-server")
	})

	// new-session: the same shape ensureServerAndSessionLocked spawns
	// (-x/-y sizing plus a real shell command as the initial pane's command),
	// against a scratch session/socket this test owns exclusively.
	if err := reed.run("new-session", "-d", "-s", session, "-x", "80", "-y", "24", cfg.Shell); err != nil {
		t.Fatalf("new-session: %v", err)
	}

	// remain-on-exit: production always sets this at boot (lifecycle.go);
	// the dead-pane assertion below depends on it being set here too, since
	// this scratch session boots independently of ensureServerAndSessionLocked.
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

	// (a) The exact list-panes -F output shape and its parsePaneList parse.
	// Call the raw format string directly (not through listPanes) so this
	// assertion catches drift in the literal string itself, not just in
	// parsePaneList's tolerance of whatever the binary happens to emit.
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

	// listPanes (overlay.go) must agree with the raw parse above — it is a
	// thin wrapper around the same format string and parser.
	viaListPanes, err := reed.listPanes(session)
	if err != nil {
		t.Fatalf("listPanes: %v", err)
	}
	if len(viaListPanes) != 1 || viaListPanes[0].ID != initialPane.ID {
		t.Errorf("listPanes() = %+v, want a single pane matching the raw parse %+v", viaListPanes, initialPane)
	}

	// pwsh takes a moment to load its profile and print its first prompt;
	// sending keys before that happens types into a not-yet-listening shell
	// and capture-pane sees nothing. Poll until the pane has produced some
	// output before driving it, rather than a fixed sleep.
	waitUntil(t, 15*time.Second, "initial pane never produced a prompt", func() bool {
		out, err := reed.output("capture-pane", "-p", "-t", initialPane.ID)
		return err == nil && strings.TrimSpace(out) != ""
	})

	// (c) send-keys -l literal handling of a leading-dash payload: psmux/tmux
	// parses a bare '-'-prefixed literal argument as flags and silently drops
	// it, so sendKeysLiteralArg's one-space prefix must make it through to
	// the pane verbatim. Typed without a trailing Enter so this checks
	// delivery, not shell execution semantics.
	const dashPayload = "-contract-dash-marker"
	if err := reed.run("send-keys", "-t", initialPane.ID, "-l", sendKeysLiteralArg(dashPayload)); err != nil {
		t.Fatalf("send-keys -l %q: %v", dashPayload, err)
	}
	// capture-pane wraps its 80-column pane at the terminal width, which can
	// split the payload word across a line break (e.g. a long cwd in the
	// prompt pushes the wrap point mid-word). Stripping embedded newlines
	// before matching re-joins any such wrap without altering the typed
	// character sequence itself, so the assertion checks delivery, not
	// terminal line-wrapping.
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

	// (b) split-window: the tallest-pane split path launchStrandLocked uses.
	splitOut, err := reed.output("split-window", "-t", initialPane.ID, "-P", "-F", "#{pane_id}")
	if err != nil {
		t.Fatalf("split-window: %v", err)
	}
	secondPaneID := strings.TrimSpace(splitOut)
	if secondPaneID == "" || secondPaneID == initialPane.ID {
		t.Fatalf("split-window -P -F #{pane_id} = %q, want a new, distinct pane id (target %s)", splitOut, initialPane.ID)
	}

	// (c) select-layout succeeds against the live pane set: a built-in
	// tmux/psmux layout keyword is enough here — apply_test.go's hermetic
	// tests already pin the render.Rules-generated layout string's shape;
	// this only needs to prove the subcommand itself works against a live,
	// two-pane session.
	if err := reed.run("select-layout", "-t", session, "even-vertical"); err != nil {
		t.Fatalf("select-layout: %v", err)
	}

	// (b) select-pane: focus the second pane.
	if err := reed.run("select-pane", "-t", secondPaneID); err != nil {
		t.Fatalf("select-pane: %v", err)
	}

	// set-hook / resize-pane: reed's first deliberately OPTIONAL wire surface — absent from
	// requiredSubcommands on purpose (doc.go's "Subcommand set" paragraph), so this section
	// documents their semantics rather than gating the capability probe on them. Both wire
	// behaviours the resize-pin hook (windowsize.go) rests on, and that no unit test can reach,
	// are pinned here: array independence across set-hook -u / set-hook / set-hook -a, and a
	// window-resized hook firing AFTER tmux has already resized the layout.
	windowTarget := exactSessionWindowTarget(session)

	// First: set-hook -u followed by set-hook and one set-hook -a yields INDEPENDENT array
	// entries. Entry [0] deliberately names a pane id no session on this socket owns, so the
	// live-firing half below can assert its failure does not take entry [1] down with it.
	if err := reed.run("set-hook", "-u", "-w", "-t", windowTarget, "window-resized"); err != nil {
		t.Fatalf("set-hook -u -w -t %q window-resized: %v", windowTarget, err)
	}
	if err := reed.run("set-hook", "-w", "-t", windowTarget, "window-resized", "resize-pane -t %99 -y 1"); err != nil {
		t.Fatalf("set-hook -w -t %q window-resized (entry [0]): %v", windowTarget, err)
	}
	if err := reed.run("set-hook", "-a", "-w", "-t", windowTarget, "window-resized", fmt.Sprintf("resize-pane -t %s -y 1", initialPane.ID)); err != nil {
		t.Fatalf("set-hook -a -w -t %q window-resized (entry [1]): %v", windowTarget, err)
	}

	// Read the array back: one line per entry, each naming its own pane id — the readback shape
	// verified live on tmux 3.6.
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

	// Second: a window-resized hook fires AFTER tmux has resized the layout, so a resize-pane -y
	// inside it survives the resize that triggered it — and the dead entry [0] above does not
	// take entry [1] down with it. window-size manual is required to make a DETACHED session
	// resizable at all.
	if err := reed.run("set-option", "-w", "-t", windowTarget, "window-size", "manual"); err != nil {
		t.Fatalf("set-option -w -t %q window-size manual: %v", windowTarget, err)
	}
	if err := reed.run("resize-window", "-t", windowTarget, "-x", "80", "-y", "60"); err != nil {
		t.Fatalf("resize-window -t %q -x 80 -y 60: %v", windowTarget, err)
	}
	waitUntil(t, 10*time.Second, "listPanes never reported the window fully laid out at 60 rows", func() bool {
		live, err := reed.listPanes(session)
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
	afterResizeLive, err := reed.listPanes(session)
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

	// Leave the window as the later steps of this test expect to find it, using two named
	// mechanisms and no ad-hoc readback-and-restore: clear the hook array, and drop the
	// window-size override with the -u unset form rather than reading the prior value back and
	// re-setting it.
	if err := reed.run("set-hook", "-u", "-w", "-t", windowTarget, "window-resized"); err != nil {
		t.Fatalf("set-hook -u -w -t %q window-resized (final clear): %v", windowTarget, err)
	}
	if err := reed.run("set-option", "-uw", "-t", windowTarget, "window-size"); err != nil {
		t.Fatalf("set-option -uw -t %q window-size (unset): %v", windowTarget, err)
	}

	// (b) list-sessions: the subcommand serverPIDLocked's sibling reap
	// helpers use to distinguish "no server" from "server up".
	sessionsOut, err := reed.output("list-sessions", "-F", "#{session_name}")
	if err != nil {
		t.Fatalf("list-sessions: %v", err)
	}
	if strings.TrimSpace(sessionsOut) != session {
		t.Errorf("list-sessions -F #{session_name} = %q, want %q", strings.TrimSpace(sessionsOut), session)
	}

	// (b) display-message: the #{pid} format variable serverPIDLocked relies
	// on to name the server's OS pid for Down's process-exit wait.
	pidOut, err := reed.output("display-message", "-p", "-t", session, "#{pid}")
	if err != nil {
		t.Fatalf("display-message: %v", err)
	}
	if pid, err := strconv.Atoi(strings.TrimSpace(pidOut)); err != nil || pid <= 0 {
		t.Errorf("display-message -p #{pid} = %q, want a positive integer pid", pidOut)
	}

	// (c) remain-on-exit keeps a dead pane visible with pane_dead=1: make the
	// second pane's shell exit, then poll until list-panes reports it dead
	// rather than absent.
	if err := reed.run("send-keys", "-t", secondPaneID, "-l", "exit"); err != nil {
		t.Fatalf("send-keys exit: %v", err)
	}
	if err := reed.run("send-keys", "-t", secondPaneID, "Enter"); err != nil {
		t.Fatalf("send-keys Enter: %v", err)
	}
	waitUntil(t, 10*time.Second, "second pane never reported dead under remain-on-exit", func() bool {
		live, err := reed.listPanes(session)
		if err != nil {
			return false
		}
		for _, p := range live {
			if p.ID == secondPaneID {
				return p.Dead
			}
		}
		// Absent entirely (not merely dead) would be remain-on-exit failing
		// to keep the corpse visible — treat that as "not yet satisfied"
		// too, so the deadline message covers both failure shapes.
		return false
	})

	// (b) kill-pane: reap the now-dead second pane.
	if err := reed.run("kill-pane", "-t", secondPaneID); err != nil {
		t.Fatalf("kill-pane: %v", err)
	}
	live, err := reed.listPanes(session)
	if err != nil {
		t.Fatalf("list panes after kill-pane: %v", err)
	}
	for _, p := range live {
		if p.ID == secondPaneID {
			t.Errorf("pane %s still present after kill-pane", secondPaneID)
		}
	}

	// (b) kill-session: tear the session down while the scratch server
	// itself is left for t.Cleanup's kill-server to reap.
	if err := reed.run("kill-session", "-t", session); err != nil {
		t.Fatalf("kill-session: %v", err)
	}
	if stillUp, err := reed.hasSession(session); err == nil && stillUp {
		t.Errorf("has-session reports %q still present after kill-session", session)
	}
}

// TestExactSessionTargetsNeverPrefixMatchSiblings pins the exact-match target forms
// exactSessionTarget ("=<name>") and exactSessionWindowTarget ("=<name>:") against a real
// multiplexer.
// tmux resolves a bare -t session name by exact match first but falls back to PREFIX matching when
// no exact match exists — so with sessions "repo" and "repo2" on one shared per-hub server (exactly
// what two prefix-sharing sibling worktrees produce), a bare `kill-session -t repo` issued after
// "repo" is already gone KILLS "repo2", and a bare `has-session -t repo` false-positives on it
// (both verified live on tmux 3.6, which is what motivated the "=" forms).
// This test asserts the engine's two target grammars behave exactly: they resolve the exact-named
// session while it exists, error (rather than prefix-match the sibling) once it is gone, and never
// touch the sibling — the canary for a configured binary (psmux) that does not implement the "="
// target syntax.
func TestExactSessionTargetsNeverPrefixMatchSiblings(t *testing.T) {
	tmpDir := t.TempDir()
	seedReedConfig(t, tmpDir)

	cfg, err := LoadConfig(tmpDir, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}

	socket := fmt.Sprintf("lyx-contract-exact-target-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	// sibling's name deliberately extends session's, so any prefix-match
	// fallback in the binary would resolve session-targets onto the sibling
	// once the exact-named session is gone.
	const session = "exact-target"
	const sibling = "exact-target2"
	reed := NewTmuxCmd(cfg.Tmux, socket)

	t.Cleanup(func() {
		_ = reed.run("kill-server")
	})

	for _, name := range []string{session, sibling} {
		if err := reed.run("new-session", "-d", "-s", name, "-x", "80", "-y", "24", cfg.Shell); err != nil {
			t.Fatalf("new-session %s: %v", name, err)
		}
	}

	// While the exact-named session exists, both grammars resolve it.
	if up, err := reed.hasSession(session); err != nil || !up {
		t.Fatalf("hasSession(%q) = (%v, %v), want (true, nil) while it exists", session, up, err)
	}
	if _, err := reed.listPanes(session); err != nil {
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
	if _, err := reed.listPanes(session); err == nil {
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

// TestDisplayMessageDoesNotErrorForAnAbsentSession pins the wire-contract fact the pane-generation
// guards rest on, and which the code that spends it originally documented backwards (R6 review
// finding R6-F4).
//
// Unlike has-session and list-panes, which exit 1 for a -t target naming no session, display-message
// exits 0 and expands every session-scoped format to empty while the server-global #{pid} still
// fills — and it does NOT fall back to a current session when other sessions exist on the socket.
// The generation probe's absent-session case therefore surfaces as parsePaneGeneration's empty-field
// rejection rather than as a tmux error, so that rejection is load-bearing rather than cosmetic:
// relaxing it would turn the still-running-orphan check into one that never refuses and
// adoptPaneGenerationLocked into one that clears every worktree's bindings on every op.
// The assertions below are therefore paired — the raw wire answer AND what parsePaneGeneration makes
// of it — so the connection cannot be broken from either end without a failure.
func TestDisplayMessageDoesNotErrorForAnAbsentSession(t *testing.T) {
	tmpDir := t.TempDir()
	seedReedConfig(t, tmpDir)

	cfg, err := LoadConfig(tmpDir, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}

	socket := fmt.Sprintf("lyx-contract-absent-session-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	const present = "absent-probe-present"
	const absent = "absent-probe-missing"
	reed := NewTmuxCmd(cfg.Tmux, socket)

	t.Cleanup(func() {
		_ = reed.run("kill-server")
	})

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

	// The absent session answers with EXIT 0 — the whole point of this test. A future tmux that
	// errors here instead would make the probe's error path mean two different things.
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
// validateToldTmuxIdentity (server.go): tmux does not REJECT a session name containing '.' or ':' —
// it rewrites each to '_', creates the session under the rewritten name, and exits 0.
// That silence is the whole hazard. Every session target this package issues is the exact-match
// "=<name>" form, so the created session is unreachable by the very name that created it: the boot
// loop polls forever, the operator sees a timeout naming no cause, and the rewritten session is left
// squatting on a shared per-hub server with no reed verb able to address it.
// Pinning it here, in the file that owns doc.go's multiplexer-contract claims, is what makes a
// future tmux behaviour change (a hard rejection, a different substitute character, a wider rewrite
// set) surface as a test failure rather than as a silently-weakened guard.
func TestSessionNameRewriteIsSilentAndExactTargetsMissIt(t *testing.T) {
	tmpDir := t.TempDir()
	seedReedConfig(t, tmpDir)

	cfg, err := LoadConfig(tmpDir, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}

	socket := fmt.Sprintf("lyx-contract-rewrite-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	reed := NewTmuxCmd(cfg.Tmux, socket)
	t.Cleanup(func() {
		_ = reed.run("kill-server")
	})

	tests := []struct {
		name      string
		requested string
		rewritten string
	}{
		{"dot", "rewrite-dot.v2", "rewrite-dot_v2"},
		{"colon", "rewrite-colon:v2", "rewrite-colon_v2"},
		// The vis-encode half of the rewrite (R3-F1): a raw TAB becomes the
		// TWO literal characters backslash-t, not '_' — a different rewrite
		// mechanism with the same silence, pinned here so a tmux that starts
		// rejecting (or differently encoding) control characters surfaces as
		// a failure rather than a silently-weakened guard.
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

			// And the exact-match target this package always uses misses it,
			// which is what makes the created session untearable by reed.
			if up, err := reed.hasSession(tt.requested); err != nil || up {
				t.Fatalf("hasSession(%q) = (%v, %v); want (false, nil) — an exact target must not resolve the rewritten session", tt.requested, up, err)
			}
			// Deliberately NOT killed per case: killing the last session on a
			// socket takes the server down with it, and the next case's
			// new-session then races that asynchronous teardown ("server exited
			// unexpectedly", observed while writing this test — the same
			// async-kill hazard lifecycle.go's own doc comment describes).
			// Every case's session is torn down together by the kill-server in
			// t.Cleanup above.
		})
	}
}

// TestRemoveStrand_SoleStrandEmptiesSessionSucceeds is the header-pane keepalive regression this
// batch adds: with the always-present header pane booted, removing a session's sole non-hidden
// strand must return success, leave reed.json holding zero persisted strands, AND leave both the
// session and the header pane specifically alive — the header's whole purpose.
// This supersedes the original pre-header regression (removing a session's true last pane used to
// be backend-dependent: tmux destroyed the session outright, forcing RemoveStrand to swallow the
// resulting "no server running" error as an expected success — see removalEmptiedSession,
// strand.go).
// With the header pane as a permanent second pane, killing the strand's pane is never a
// last-pane-destroy on ANY backend, so that swallow branch is no longer reached by this scenario at
// all;
// it remains in place for the (now believed unreachable in practice, but still defensive) case
// where the header pane is itself somehow absent.
func TestRemoveStrand_SoleStrandEmptiesSessionSucceeds(t *testing.T) {
	tmpDir := t.TempDir()
	seedReedConfig(t, tmpDir)

	cfg, err := LoadConfig(tmpDir, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// The self-skip: on a box without the configured multiplexer binary
	// there is nothing to drive, matching TestMultiplexerContract's guard.
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}

	// A Geometry literal rooted at a scratch tmpDir, mirroring newTestEngine's
	// (lock_test.go) hub/worktree-root shape but built against the real,
	// LoadConfig-resolved cfg rather than the does-not-exist stub paths
	// newTestEngine deliberately uses. Built directly rather than via
	// hubgeom.ReedGeometry: hubgeom imports reedengine, so an in-package test
	// importing it would close an import cycle.
	hub := filepath.Dir(tmpDir)
	geom := Geometry{
		SocketKey:    ServerName(hub),
		SessionName:  SessionName(tmpDir),
		AnchorPath:   tmpDir,
		PaneCwd:      tmpDir,
		WorktreeRoot: tmpDir,
		LogsDir:      filepath.Join(hub, "logs"),
		RepoName:     "test-repo",
		HubPath:      hub,
	}
	e := New(cfg, geom)

	t.Cleanup(func() {
		// Best-effort: the fix under test is expected to have already torn
		// the session (and, on tmux, the whole server, since it was this
		// scratch server's only session) down, so Down's own error here is
		// unsurprising and ignored. The raw kill-server afterward is the
		// belt-and-suspenders guard against a leaked scratch server on a
		// genuine test failure that never reached RemoveStrand.
		_, _ = e.Down()
		_ = e.tmux.run("kill-server")
	})

	if _, err := e.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}

	// The header pane is booted as part of Up, before any strand exists;
	// capture its id so the post-remove assertions below can confirm it
	// specifically (not merely "some pane") survived.
	upSt, err := LoadState(e.stateDir())
	if err != nil || upSt == nil || upSt.HeaderPaneID == "" {
		t.Fatalf("LoadState after Up = (%+v, %v), want a persisted HeaderPaneID", upSt, err)
	}
	headerPaneID := upSt.HeaderPaneID

	// One non-hidden strand, anchored so it is realized into a live pane at
	// add time; a long-lived command so it is still running when removed.
	strand, err := e.AddStrand(AddSpec{
		Cmd:     "sleep 300",
		Display: render.Display{Anchor: render.AnchorBelowParent},
	})
	if err != nil {
		t.Fatalf("AddStrand: %v", err)
	}

	removed, err := e.RemoveStrand(strand.GUID, false)
	if err != nil {
		t.Fatalf("RemoveStrand(sole strand) = %v, want nil error (the header pane keeps the session alive, so emptying the strand table is never a last-pane-destroy)", err)
	}
	if len(removed.Strands) != 1 || removed.Strands[0].GUID != strand.GUID {
		t.Fatalf("RemoveStrand.Removed.Strands = %+v, want exactly guid %q", removed.Strands, strand.GUID)
	}

	// Persistence is the resurrect-on-resume guard: removeStrandLocked only
	// prunes st.Strands in memory, so this must reload from disk rather than
	// trust the in-memory Removed result above.
	waitUntil(t, 5*time.Second, "persisted reed.json never reflected the emptied strand table", func() bool {
		st, err := LoadState(e.stateDir())
		return err == nil && st != nil && len(st.Strands) == 0
	})

	// The keepalive guarantee this batch adds: the session, and the header
	// pane specifically, must still be alive with zero strands tracked.
	up, err := e.tmux.hasSession(e.SessionName())
	if err != nil || !up {
		t.Fatalf("hasSession after removing the sole strand = (%v, %v), want (true, nil) — the header pane must keep the session alive", up, err)
	}
	live, err := e.tmux.listPanes(e.SessionName())
	if err != nil {
		t.Fatalf("listPanes after removing the sole strand: %v", err)
	}
	headerFound := false
	for _, p := range live {
		if p.ID == headerPaneID {
			headerFound = true
			if p.Dead {
				t.Errorf("header pane %s reports Dead = true after removing the sole strand, want it alive", headerPaneID)
			}
		}
	}
	if !headerFound {
		t.Fatalf("header pane %s missing from live panes %+v after removing the sole strand", headerPaneID, live)
	}
}

// TestDeadHeaderPaneIsHealedByUpWithoutCorruptingLayout drives the dead-header lifecycle the
// fable-header-r1 round found broken, end to end against a real multiplexer: the header's keepalive
// process exits (pane_dead=1 under remain-on-exit), a subsequent AddStrand must keep the corpse
// enumerable (reconcile's dead-kill exemption) and lay out with no window-bottom overflow and no
// stale-cell scramble (planLayout's presence filter), and the next Up must heal the header — kill
// the corpse, split a fresh header back in at the physical top, persist the new id — instead of
// treating the corpse as a working header (the old presence-keyed idempotency check) or wedging on
// a too-short split target (the old first-alive targeting).
// Pre-fix this sequence scrambled every strand's height on the add and then failed every up with
// "no space for new pane" until a full down.
func TestDeadHeaderPaneIsHealedByUpWithoutCorruptingLayout(t *testing.T) {
	tmpDir := t.TempDir()
	seedReedConfig(t, tmpDir)

	cfg, err := LoadConfig(tmpDir, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}

	hub := filepath.Dir(tmpDir)
	geom := Geometry{
		SocketKey:    ServerName(hub),
		SessionName:  SessionName(tmpDir),
		AnchorPath:   tmpDir,
		PaneCwd:      tmpDir,
		WorktreeRoot: tmpDir,
		LogsDir:      filepath.Join(hub, "logs"),
		RepoName:     "test-repo",
		HubPath:      hub,
	}
	e := New(cfg, geom)

	t.Cleanup(func() {
		_, _ = e.Down()
		_ = e.tmux.run("kill-server")
	})

	if _, err := e.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}
	st, err := LoadState(e.stateDir())
	if err != nil || st == nil || st.HeaderPaneID == "" {
		t.Fatalf("LoadState after Up = (%+v, %v), want a persisted HeaderPaneID", st, err)
	}
	deadHeaderID := st.HeaderPaneID

	if _, err := e.AddStrand(AddSpec{Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorBelowParent}}); err != nil {
		t.Fatalf("AddStrand (first): %v", err)
	}

	// Kill the header's keepalive by pid: the pane's shell/keepalive does
	// not read stdin (that is the keepalive's whole contract), so typing
	// exit would go nowhere — killing #{pane_pid} is how a real keepalive
	// death looks to tmux. remain-on-exit then corpses the pane
	// (pane_dead=1); the flip is asynchronous, so poll.
	live, err := e.tmux.listPanes(e.SessionName())
	if err != nil {
		t.Fatalf("listPanes before header kill: %v", err)
	}
	for _, p := range live {
		if p.ID == deadHeaderID {
			proc, err := os.FindProcess(p.PID)
			if err != nil {
				t.Fatalf("FindProcess(header pane pid %d): %v", p.PID, err)
			}
			if err := proc.Kill(); err != nil {
				t.Fatalf("kill header pane pid %d: %v", p.PID, err)
			}
		}
	}
	waitUntil(t, 10*time.Second, "header pane never reported dead", func() bool {
		live, err := e.tmux.listPanes(e.SessionName())
		if err != nil {
			return false
		}
		for _, p := range live {
			if p.ID == deadHeaderID {
				return p.Dead
			}
		}
		return false
	})

	// An add with the header dead: must succeed, keep the corpse enumerable,
	// and produce a sane layout (no pane past the window's bottom edge — the
	// stale-cell scramble's signature was positional misassignment).
	if _, err := e.AddStrand(AddSpec{Cmd: "sleep 300", Display: render.Display{Anchor: render.AnchorBelowParent}}); err != nil {
		t.Fatalf("AddStrand with dead header: %v", err)
	}
	live, err = e.tmux.listPanes(e.SessionName())
	if err != nil {
		t.Fatalf("listPanes after add-with-dead-header: %v", err)
	}
	corpsePresent := false
	for _, p := range live {
		if p.ID == deadHeaderID {
			corpsePresent = true
			if !p.Dead {
				t.Errorf("header corpse %s reports alive after add, want still dead", deadHeaderID)
			}
		}
		if p.Top+p.Height > cfg.Height {
			t.Errorf("pane %s top+height = %d+%d exceeds window height %d after add-with-dead-header (stale-cell scramble)", p.ID, p.Top, p.Height, cfg.Height)
		}
	}
	if !corpsePresent {
		t.Fatalf("header corpse %s was killed by the add's reconcile; it must stay enumerable until up/resume heals it (panes: %+v)", deadHeaderID, live)
	}

	// The heal: Up must replace the corpse with a fresh, alive, physically
	// topmost header and persist its id.
	if _, err := e.Up(); err != nil {
		t.Fatalf("Up (heal) = %v, want success — pre-fix this wedged on \"no space for new pane\"", err)
	}
	st, err = LoadState(e.stateDir())
	if err != nil || st == nil || st.HeaderPaneID == "" {
		t.Fatalf("LoadState after healing Up = (%+v, %v), want a persisted HeaderPaneID", st, err)
	}
	if st.HeaderPaneID == deadHeaderID {
		t.Fatalf("HeaderPaneID still names the corpse %s after the healing Up", deadHeaderID)
	}
	live, err = e.tmux.listPanes(e.SessionName())
	if err != nil {
		t.Fatalf("listPanes after healing Up: %v", err)
	}
	headerSeen := false
	for _, p := range live {
		if p.ID == deadHeaderID {
			t.Errorf("header corpse %s still present after the healing Up, want it killed and replaced", deadHeaderID)
		}
		if p.ID == st.HeaderPaneID {
			headerSeen = true
			if p.Dead {
				t.Errorf("healed header pane %s reports dead", p.ID)
			}
			if p.Top != 0 {
				t.Errorf("healed header pane %s top = %d, want 0 (physically topmost — render.Rules emits its cell first)", p.ID, p.Top)
			}
		}
		if p.Top+p.Height > cfg.Height {
			t.Errorf("pane %s top+height = %d+%d exceeds window height %d after the healing Up", p.ID, p.Top, p.Height, cfg.Height)
		}
	}
	if !headerSeen {
		t.Fatalf("healed header pane %s missing from live panes %+v", st.HeaderPaneID, live)
	}
}

// TestHeaderNeverGetsZeroHeightLayoutCell pins clampHeaderHeight's never-below-1 floor (height.go)
// against a real multiplexer.
// A pathological config — height_rows large relative to a tiny window height — used to let
// clampHeaderHeight legally return 0, which bandHeader would then emit as a literal "WxH,..."
// header cell with H=0 in the window_layout string.
// Manual probing against a live tmux 3.6 instance showed that a genuinely zero-height cell is NOT
// rendered as "no header": select-layout accepts the string (no error),
// but silently keeps a row for the header pane anyway, pushing every pane below it down by one row
// and overflowing the bottom of the window by exactly one row (the last pane's top+height exceeds
// the window height).
// clampHeaderHeight now floors the header at 1 row whenever it exists, which this test confirms
// produces a layout the real multiplexer applies cleanly, with every pane's top+height staying
// within the window.
func TestHeaderNeverGetsZeroHeightLayoutCell(t *testing.T) {
	tmpDir := t.TempDir()
	seedReedConfig(t, tmpDir)

	cfg, err := LoadConfig(tmpDir, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}

	const windowRows = 6
	socket := fmt.Sprintf("lyx-contract-header-floor-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	session := "header-floor-session"
	reed := NewTmuxCmd(cfg.Tmux, socket)

	t.Cleanup(func() {
		_ = reed.run("kill-server")
	})

	if err := reed.run("new-session", "-d", "-s", session, "-x", "80", "-y", strconv.Itoa(windowRows), cfg.Shell); err != nil {
		t.Fatalf("new-session: %v", err)
	}

	headerOut, err := reed.output("split-window", "-t", session, "-b", "-P", "-F", "#{pane_id}")
	if err != nil {
		t.Fatalf("split-window (header): %v", err)
	}
	headerPaneID := strings.TrimSpace(headerOut)

	live, err := reed.listPanes(session)
	if err != nil {
		t.Fatalf("list panes: %v", err)
	}
	if len(live) != 2 {
		t.Fatalf("listPanes after split = %d pane(s), want 2", len(live))
	}
	var strandPaneID string
	for _, p := range live {
		if p.ID != headerPaneID {
			strandPaneID = p.ID
		}
	}
	if strandPaneID == "" {
		t.Fatalf("could not identify the non-header pane among %+v", live)
	}

	// A pathological config (MinFullRows far larger than the window, plus an
	// oversized configured height_rows) that pre-fix would have driven
	// clampHeaderHeight all the way to 0.
	strands := []render.Strand{
		{GUID: "s1", PaneID: strandPaneID, Display: render.Display{Anchor: render.AnchorBelowParent}, Live: true},
	}
	box := render.Box{X: 0, Y: 0, W: 80, H: windowRows}
	params := render.Params{
		MinFullRows: windowRows * 5,
		Header:      render.Header{PaneID: headerPaneID, HeightRows: windowRows * 5},
	}
	layout, _, err := render.Rules(strands, box, params, []string{headerPaneID, strandPaneID})
	if err != nil {
		t.Fatalf("render.Rules: %v", err)
	}

	if err := reed.run("select-layout", "-t", session, layout); err != nil {
		t.Fatalf("select-layout %q: %v (a real multiplexer rejecting this layout means clampHeaderHeight's floor no longer matches what select-layout accepts)", layout, err)
	}

	live, err = reed.listPanes(session)
	if err != nil {
		t.Fatalf("list panes after select-layout: %v", err)
	}
	for _, p := range live {
		if p.Height < 1 {
			t.Errorf("pane %s height = %d after select-layout %q, want >= 1 (a zero-height cell must never survive the real multiplexer)", p.ID, p.Height, layout)
		}
		if p.Top+p.Height > windowRows {
			t.Errorf("pane %s top+height = %d+%d = %d after select-layout %q, want <= window height %d (the off-by-one overflow a bare H=0 header cell used to cause)", p.ID, p.Top, p.Height, p.Top+p.Height, layout, windowRows)
		}
	}
}
