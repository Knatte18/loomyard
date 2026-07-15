//go:build integration

// contract_integration_test.go asserts the full psmux/tmux wire contract that
// doc.go's "Multiplexer contract surface" section pins, against a real,
// running instance of the binary LoadConfig resolves (psmux on Windows
// today, tmux on Linux in the deferred follow-up). It is the canary for both
// version drift in the on-box binary and the eventual tmux swap — the same
// assertions run unmodified against whichever binary is configured, and the
// test self-skips cleanly when that binary is absent. It complements, and
// does not replace, the existing agent-driven SANDBOX-MUX-SUITE: that suite
// drives a real hub end to end through the CLI, while this test pins the
// narrower wire-level contract muxengine's own godoc claims, in isolation,
// on its own scratch socket so it can never collide with a real hub server.

package muxengine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/muxengine/render"
)

// seedMuxConfig writes <tmpDir>/_lyx/config/mux.yaml with the module's own
// default template — the minimal on-disk shape LoadConfig needs to resolve a
// Config. This duplicates (rather than imports) config_test.go's
// seedLyxConfig fixture: that helper lives in the external muxengine_test
// package, while this file is package muxengine so it can reach the real,
// unexported listPanes/TmuxCmd helpers the contract assertions below drive
// directly.
func seedMuxConfig(t *testing.T, tmpDir string) {
	t.Helper()
	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
	if err := os.Mkdir(lyxDir, 0o755); err != nil {
		t.Fatalf("mkdir _lyx: %v", err)
	}
	configDir := hubgeometry.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0o755); err != nil {
		t.Fatalf("mkdir _lyx/config: %v", err)
	}
	configFile := hubgeometry.ConfigFile(tmpDir, "mux")
	if err := os.WriteFile(configFile, []byte(ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}

// waitUntil polls cond every 100ms until it reports true or timeout elapses,
// failing the test in the latter case. Pane state changes (a shell exiting,
// remain-on-exit flipping pane_dead) are asynchronous from tmux's own CLI
// return, so assertions on them must poll rather than check once.
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

// TestMultiplexerContract loads the resolved mux Config via LoadConfig (so it
// targets the *configured* binary, never a hardcoded path), skips cleanly
// when that binary is not on this box, then spawns a real server on a
// scratch -L socket and drives it through the exact subcommand set and
// wire shapes doc.go documents: the list-panes -F output string and its
// parsePaneList parse, the required subcommand set, and the load-bearing
// behavioral assumptions (remain-on-exit dead-pane visibility, send-keys -l
// literal handling of a leading-dash payload, select-layout succeeding
// against the live pane set). The scratch server is always torn down via
// t.Cleanup, and its socket name is derived from this test's own pid and a
// timestamp — never from a hub path — so it can never collide with a real
// per-hub server.
func TestMultiplexerContract(t *testing.T) {
	tmpDir := t.TempDir()
	seedMuxConfig(t, tmpDir)

	cfg, err := LoadConfig(tmpDir, "mux")
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
	mux := NewTmuxCmd(cfg.Tmux, socket)

	t.Cleanup(func() {
		// Always torn down, success or failure: a leaked scratch server on a
		// pid/timestamp socket is harmless to a real hub server, but leaves a
		// stray process behind if the test does not clean up after itself.
		_ = mux.run("kill-server")
	})

	// new-session: the same shape ensureServerAndSessionLocked spawns
	// (-x/-y sizing plus a real shell command as the initial pane's command),
	// against a scratch session/socket this test owns exclusively.
	if err := mux.run("new-session", "-d", "-s", session, "-x", "80", "-y", "24", cfg.Shell); err != nil {
		t.Fatalf("new-session: %v", err)
	}

	// remain-on-exit: production always sets this at boot (lifecycle.go);
	// the dead-pane assertion below depends on it being set here too, since
	// this scratch session boots independently of ensureServerAndSessionLocked.
	if err := mux.run("set-option", "-g", "remain-on-exit", "on"); err != nil {
		t.Fatalf("set-option remain-on-exit: %v", err)
	}

	// has-session: the subcommand hasSession wraps.
	up, err := mux.hasSession(session)
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
	rawOut, err := mux.output("list-panes", "-t", session, "-F", paneFormat)
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
	viaListPanes, err := mux.listPanes(session)
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
		out, err := mux.output("capture-pane", "-p", "-t", initialPane.ID)
		return err == nil && strings.TrimSpace(out) != ""
	})

	// (c) send-keys -l literal handling of a leading-dash payload: psmux/tmux
	// parses a bare '-'-prefixed literal argument as flags and silently drops
	// it, so sendKeysLiteralArg's one-space prefix must make it through to
	// the pane verbatim. Typed without a trailing Enter so this checks
	// delivery, not shell execution semantics.
	const dashPayload = "-contract-dash-marker"
	if err := mux.run("send-keys", "-t", initialPane.ID, "-l", sendKeysLiteralArg(dashPayload)); err != nil {
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
		out, err := mux.output("capture-pane", "-p", "-t", initialPane.ID)
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
	if err := mux.run("send-keys", "-t", initialPane.ID, "C-c"); err != nil {
		t.Fatalf("send-keys C-c: %v", err)
	}

	// (b) split-window: the tallest-pane split path launchStrandLocked uses.
	splitOut, err := mux.output("split-window", "-t", initialPane.ID, "-P", "-F", "#{pane_id}")
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
	if err := mux.run("select-layout", "-t", session, "even-vertical"); err != nil {
		t.Fatalf("select-layout: %v", err)
	}

	// (b) select-pane: focus the second pane.
	if err := mux.run("select-pane", "-t", secondPaneID); err != nil {
		t.Fatalf("select-pane: %v", err)
	}

	// (b) list-sessions: the subcommand serverPIDLocked's sibling reap
	// helpers use to distinguish "no server" from "server up".
	sessionsOut, err := mux.output("list-sessions", "-F", "#{session_name}")
	if err != nil {
		t.Fatalf("list-sessions: %v", err)
	}
	if strings.TrimSpace(sessionsOut) != session {
		t.Errorf("list-sessions -F #{session_name} = %q, want %q", strings.TrimSpace(sessionsOut), session)
	}

	// (b) display-message: the #{pid} format variable serverPIDLocked relies
	// on to name the server's OS pid for Down's process-exit wait.
	pidOut, err := mux.output("display-message", "-p", "-t", session, "#{pid}")
	if err != nil {
		t.Fatalf("display-message: %v", err)
	}
	if pid, err := strconv.Atoi(strings.TrimSpace(pidOut)); err != nil || pid <= 0 {
		t.Errorf("display-message -p #{pid} = %q, want a positive integer pid", pidOut)
	}

	// (c) remain-on-exit keeps a dead pane visible with pane_dead=1: make the
	// second pane's shell exit, then poll until list-panes reports it dead
	// rather than absent.
	if err := mux.run("send-keys", "-t", secondPaneID, "-l", "exit"); err != nil {
		t.Fatalf("send-keys exit: %v", err)
	}
	if err := mux.run("send-keys", "-t", secondPaneID, "Enter"); err != nil {
		t.Fatalf("send-keys Enter: %v", err)
	}
	waitUntil(t, 10*time.Second, "second pane never reported dead under remain-on-exit", func() bool {
		live, err := mux.listPanes(session)
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
	if err := mux.run("kill-pane", "-t", secondPaneID); err != nil {
		t.Fatalf("kill-pane: %v", err)
	}
	live, err := mux.listPanes(session)
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
	if err := mux.run("kill-session", "-t", session); err != nil {
		t.Fatalf("kill-session: %v", err)
	}
	if stillUp, err := mux.hasSession(session); err == nil && stillUp {
		t.Errorf("has-session reports %q still present after kill-session", session)
	}
}

// TestRemoveStrand_SoleStrandEmptiesSessionSucceeds reproduces, end to end
// against a real Engine, the bug this batch fixes: removing a session's
// sole non-hidden strand must return success and leave mux.json holding
// zero persisted strands, never surface the raw "no server running" tmux
// error the original reproduction hit.
//
// Coverage note: this regression exercises the Card 2 swallow branch ONLY
// on last-pane-DESTROY backends (tmux, the POSIX default per
// template_posix.go) — there, without the fix, RemoveStrand would return
// the "no server running" error surfaced from reconcileApplyPersistLocked's
// listPanes call once kill-pane has destroyed the session, so a passing
// test genuinely covers the swallow. On corpse backends (psmux/Windows) the
// same scenario passes via RemoveStrand's normal path instead (kill-pane
// corpses the pane, the session survives, reconcileApplyPersistLocked
// succeeds), so the swallow is never reached there — TestRemovalEmptiedSession
// (strand_test.go) covers the pure decision logic on every platform
// regardless of which binary is configured. This test therefore asserts
// only the observable contract (nil error, zero persisted strands), never
// which internal branch RemoveStrand actually took.
func TestRemoveStrand_SoleStrandEmptiesSessionSucceeds(t *testing.T) {
	tmpDir := t.TempDir()
	seedMuxConfig(t, tmpDir)

	cfg, err := LoadConfig(tmpDir, "mux")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// The self-skip: on a box without the configured multiplexer binary
	// there is nothing to drive, matching TestMultiplexerContract's guard.
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}

	// A real *hubgeometry.Layout rooted at a scratch tmpDir, mirroring
	// newTestEngine's (lock_test.go) Cwd/WorktreeRoot/Hub shape but built
	// against the real, LoadConfig-resolved cfg rather than the
	// does-not-exist stub paths newTestEngine deliberately uses.
	layout := &hubgeometry.Layout{
		Cwd:          tmpDir,
		WorktreeRoot: tmpDir,
		Hub:          filepath.Dir(tmpDir),
	}
	e := New(cfg, layout)

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
		t.Fatalf("RemoveStrand(sole strand) = %v, want nil error (emptying the session is an expected terminal state, not a failure)", err)
	}
	if len(removed.Strands) != 1 || removed.Strands[0].GUID != strand.GUID {
		t.Fatalf("RemoveStrand.Removed.Strands = %+v, want exactly guid %q", removed.Strands, strand.GUID)
	}

	// Persistence is the resurrect-on-resume guard: removeStrandLocked only
	// prunes st.Strands in memory, so this must reload from disk rather than
	// trust the in-memory Removed result above. Polled, not asserted once,
	// since the swallow path's own SaveState races the surrounding async
	// pane/session teardown this whole fix is about.
	waitUntil(t, 5*time.Second, "persisted mux.json never reflected the emptied strand table", func() bool {
		st, err := LoadState(layout.DotLyxDir())
		return err == nil && st != nil && len(st.Strands) == 0
	})
}
