//go:build smoke

package muxcli

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/muxengine"
)

// TestSmokeDownReleasesServerBeforeReturning pins the down->up churn race
// this round fixed: tmux's kill-server is asynchronous, and a Down that
// returned while the old server still held the socket let an immediate up
// spawn a duplicate server process that lingered forever as an unreachable
// stray. Down now waits on the server PROCESS itself (tmux's CLI cannot
// report server absence — every probe exits 0), so the moment it returns
// the server must be gone — and an immediate up+add cycle must work. Three
// back-to-back cycles with no sleeps.
func TestSmokeDownReleasesServerBeforeReturning(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	deferHubRelease(t, fixture.Hub)
	t.Chdir(fixture.Hub)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	socket, session := socketAndSession(t)

	launch := "pwsh -NoExit -Command Write-Host ready"
	for cycle := 0; cycle < 3; cycle++ {
		pid := serverPID(t, tmuxPath, socket, session)
		out.Reset()
		if code := RunCLI(&out, []string{"down"}); code != 0 {
			t.Fatalf("cycle %d down = %d; want 0, output: %s", cycle, code, out.String())
		}
		// No sleep: the server process must already be gone when down
		// returns.
		if !processGone(pid) {
			t.Fatalf("cycle %d: tmux server (pid %d) still running immediately after down returned", cycle, pid)
		}
		out.Reset()
		if code := RunCLI(&out, []string{"up"}); code != 0 {
			t.Fatalf("cycle %d up = %d; want 0, output: %s", cycle, code, out.String())
		}
		addStrand(t, launch, "--name", "churn")
	}
}

// TestSmokeDownReapsPaneChildProcesses pins the pane-child reaping gap this
// round fixed: tmux terminates pane children asynchronously, so a down that
// waited only on the server process could return while a pane's shell subtree
// (a deep descendant whose cwd is the worktree) was still alive — a "no stray
// state" violation that surfaced as a worktree-dir-in-use failure under load.
// down now waits for this session's whole pane process subtree to exit before
// returning, so the instant down returns every pane descendant must be gone.
// Loops several add->down cycles to give the async teardown a chance to lag.
func TestSmokeDownReapsPaneChildProcesses(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	deferHubRelease(t, fixture.Hub)
	t.Chdir(fixture.Hub)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	pwshPath := smokePwshPath
	launch := "pwsh -NoExit -Command Write-Host ready"
	for cycle := 0; cycle < 3; cycle++ {
		var out bytes.Buffer
		if code := RunCLI(&out, []string{"up"}); code != 0 {
			t.Fatalf("cycle %d up = %d; want 0, output: %s", cycle, code, out.String())
		}
		addStrand(t, launch, "--name", "reap")
		addStrand(t, launch, "--name", "reap2")
		socket, session := socketAndSession(t)
		pids := paneProcessTree(t, tmuxPath, pwshPath, socket, session)
		if len(pids) == 0 {
			t.Fatalf("cycle %d: session reported no pane process subtree", cycle)
		}

		out.Reset()
		if code := RunCLI(&out, []string{"down"}); code != 0 {
			t.Fatalf("cycle %d down = %d; want 0, output: %s", cycle, code, out.String())
		}
		// No sleep: every pane descendant must already be gone the instant down
		// returned. processGone reuses the same non-child Wait probe the
		// server-pid test uses.
		for _, pid := range pids {
			if !processGone(pid) {
				t.Fatalf("cycle %d: pane subtree pid %d still running immediately after down returned", cycle, pid)
			}
		}
	}
}

// TestSmokeDownLeavesNoTmuxOnSocket pins the stray-server guarantee down's
// robust teardown owns: after down tears the shared server down, ZERO tmux
// process may still name this worktree's socket — not the main server, not its
// __warm__ helper. The tmux server is spawned with the worktree as its cwd, so
// a server that outlives down keeps the worktree directory busy (a real "no
// stray state" leak observed under down->up churn on a saturated machine, where
// a fixed-deadline server wait timed out and aborted down before the socket was
// cleared). Several add->down cycles give the async kill-server a chance to lag.
func TestSmokeDownLeavesNoTmuxOnSocket(t *testing.T) {
	tmuxBinaryPath(t)
	pwshPath := smokePwshPath

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	deferHubRelease(t, fixture.Hub)
	t.Chdir(fixture.Hub)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	launch := "pwsh -NoExit -Command Write-Host ready"
	for cycle := 0; cycle < 3; cycle++ {
		var out bytes.Buffer
		if code := RunCLI(&out, []string{"up"}); code != 0 {
			t.Fatalf("cycle %d up = %d; want 0, output: %s", cycle, code, out.String())
		}
		socket, _ := socketAndSession(t)
		addStrand(t, launch, "--name", "s1")
		addStrand(t, launch, "--name", "s2")

		out.Reset()
		if code := RunCLI(&out, []string{"down"}); code != 0 {
			t.Fatalf("cycle %d down = %d; want 0, output: %s", cycle, code, out.String())
		}
		// No sleep: the moment down returns, the socket must be free of tmux.
		if pids := tmuxSocketPids(t, pwshPath, socket); len(pids) != 0 {
			t.Fatalf("cycle %d: tmux still on socket %s after down returned: pids=%v", cycle, socket, pids)
		}
	}
}

// TestSmokeRemoveReapsRemovedPaneChildProcesses pins the reap gap this round
// generalized from down to remove: kill-pane on a removed strand's pane
// terminates that pane's children asynchronously, and on Windows the process
// actually holding the worktree directory is a deep descendant of
// #{pane_pid} — so a remove that returned without reaping could leave a
// removed strand's grandchild alive and the worktree dir busy under load
// (the same class down's reap already closed). remove now snapshots the
// removed panes' process subtrees before kill-pane and waits for them to
// exit before returning, so the instant remove returns every descendant of
// the removed pane must be gone. A sibling strand is kept alive throughout
// so the session survives and the removed pane is never the sole pane.
func TestSmokeRemoveReapsRemovedPaneChildProcesses(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	deferHubRelease(t, fixture.Hub)
	t.Chdir(fixture.Hub)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	pwshPath := smokePwshPath
	launch := "pwsh -NoExit -Command Write-Host ready"
	for cycle := 0; cycle < 3; cycle++ {
		var out bytes.Buffer
		if code := RunCLI(&out, []string{"up"}); code != 0 {
			t.Fatalf("cycle %d up = %d; want 0, output: %s", cycle, code, out.String())
		}
		// Keeper first (adopts the initial pane), then the victim we remove.
		keeper := addStrand(t, launch, "--name", "keeper")
		victim := addStrand(t, launch, "--name", "victim")
		socket, session := socketAndSession(t)

		// Resolve the victim's pane, then snapshot only its process subtree.
		out.Reset()
		if code := RunCLI(&out, []string{"status"}); code != 0 {
			t.Fatalf("cycle %d status = %d; want 0, output: %s", cycle, code, out.String())
		}
		vs, ok := statusStrand(t, out.Bytes(), victim)
		if !ok {
			t.Fatalf("cycle %d: status missing victim %s: %s", cycle, victim, out.String())
		}
		victimPane, _ := vs["paneId"].(string)
		if victimPane == "" {
			t.Fatalf("cycle %d: victim %s has no pane: %s", cycle, victim, out.String())
		}
		pids := panePaneSubtree(t, tmuxPath, pwshPath, socket, session, victimPane)
		if len(pids) == 0 {
			t.Fatalf("cycle %d: victim pane %s reported no process subtree", cycle, victimPane)
		}

		out.Reset()
		if code := RunCLI(&out, []string{"remove", victim}); code != 0 {
			t.Fatalf("cycle %d remove = %d; want 0, output: %s", cycle, code, out.String())
		}
		// No sleep: every descendant of the removed pane must already be gone
		// the instant remove returned.
		for _, pid := range pids {
			if !processGone(pid) {
				t.Fatalf("cycle %d: removed pane %s subtree pid %d still running immediately after remove returned", cycle, victimPane, pid)
			}
		}

		// The keeper must survive the remove untouched, and down cleans up for
		// the next cycle.
		out.Reset()
		if code := RunCLI(&out, []string{"status"}); code != 0 {
			t.Fatalf("cycle %d post-remove status = %d; want 0, output: %s", cycle, code, out.String())
		}
		if ks, ok := statusStrand(t, out.Bytes(), keeper); !ok {
			t.Fatalf("cycle %d: keeper %s gone after removing victim: %s", cycle, keeper, out.String())
		} else if live, _ := ks["live"].(bool); !live {
			t.Errorf("cycle %d: keeper %s live = false after removing victim; want true", cycle, keeper)
		}
		out.Reset()
		if code := RunCLI(&out, []string{"down"}); code != 0 {
			t.Fatalf("cycle %d down = %d; want 0, output: %s", cycle, code, out.String())
		}
	}
}

// TestSmokeDownInOneWorktreeLeavesSiblingSessionAlive codifies the CROSS-WORKTREE
// SCOPE invariant: the tmux server identity is per-HUB (the -L socket derives
// from the hub) and shared by sibling worktrees, so `lyx mux down` in worktree A
// must tear down ONLY A's session, never worktree B's session, panes, or agents
// that share the same hub socket. (This psmux port backs each session with its
// own `psmux.exe server -s <session> -L <socket>` process on the shared socket,
// so "no duplicate server" is verified per session: exactly one backing process
// per live session, never two, zero once killed.) Two clones under one hub `up`
// (same socket, distinct sessions, one backing process each), each adds a live
// strand; A goes `down`; then B's session + pane + agent subtree + its single
// backing server must all still be live while A's session and pane subtree are
// gone. B then `down`s last and the socket must be free of every tmux. The core
// assertion is B's continued liveness AFTER A's down (see assertSiblingStaysLive)
// — a naive "down kills the whole socket's server set" implementation fails this
// test rather than reporting a false green.
func TestSmokeDownInOneWorktreeLeavesSiblingSessionAlive(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)
	pwshPath := smokePwshPath

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	sibling := materializeSibling(t, fixture, "hub-b")

	// Release BOTH worktree dirs before the framework's TempDir RemoveAll.
	// Registered before the down cleanups and the chdirs so they run after them
	// (LIFO) but before RemoveAll.
	deferHubRelease(t, fixture.Hub)
	deferHubRelease(t, sibling)

	// Best-effort teardown nets: down each worktree from its own cwd even if an
	// assertion aborts the body partway through, so no server/session leaks.
	t.Cleanup(func() {
		_ = os.Chdir(sibling)
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})
	t.Cleanup(func() {
		_ = os.Chdir(fixture.Hub)
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	launch := "pwsh -NoExit -Command Write-Host ready"

	// --- worktree A: up + a live strand ---
	mustChdir(t, fixture.Hub)
	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("A up = %d; want 0, output: %s", code, out.String())
	}
	socketA, sessionA := socketAndSession(t)
	aGuid := addStrand(t, launch, "--name", "agent-a")
	aPane := paneIDForStrand(t, aGuid)

	// --- worktree B: up + a live strand ---
	mustChdir(t, sibling)
	out.Reset()
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("B up = %d; want 0, output: %s", code, out.String())
	}
	socketB, sessionB := socketAndSession(t)
	bGuid := addStrand(t, launch, "--name", "agent-b")
	bPane := paneIDForStrand(t, bGuid)

	// Siblings on one hub: shared socket, distinct sessions.
	if socketA != socketB {
		t.Fatalf("worktree A socket %q != worktree B socket %q; siblings on one hub must share the per-hub socket", socketA, socketB)
	}
	if sessionA == sessionB {
		t.Fatalf("worktree A and B both resolved session %q; each worktree must own a distinct session", sessionA)
	}
	socket := socketA

	// Both sessions live on the one socket.
	waitSessionUp(t, tmuxPath, socket, sessionA)
	waitSessionUp(t, tmuxPath, socket, sessionB)

	// Exactly one backing server process per session on the shared socket — no
	// duplicate spawned for either session.
	waitServerProcCountForSession(t, pwshPath, socket, sessionA, 1)
	waitServerProcCountForSession(t, pwshPath, socket, sessionB, 1)

	// B's backing-server pid, for the post-down stability check: it must be
	// unchanged after A's down (B's server neither killed nor restarted).
	bServerPID := serverPID(t, tmuxPath, socket, sessionB)

	// Snapshot BEFORE A's down, while both panes still exist to enumerate:
	// worktree A's whole pane process subtree (asserted GONE after A's down — a
	// transient child that already exited still reads gone, so this is not
	// flaky), and worktree B's pane ROOT pid (asserted ALIVE — only the stable
	// root, never its come-and-go descendants, so the liveness check is robust).
	aSubtree := panePaneSubtree(t, tmuxPath, pwshPath, socket, sessionA, aPane)
	if len(aSubtree) == 0 {
		t.Fatalf("worktree A pane %s reported no process subtree", aPane)
	}
	bPanePID := paneRootPID(t, tmuxPath, socket, sessionB, bPane)

	// --- down in worktree A ---
	mustChdir(t, fixture.Hub)
	out.Reset()
	if code := RunCLI(&out, []string{"down"}); code != 0 {
		t.Fatalf("A down = %d; want 0, output: %s", code, out.String())
	}

	// A's own session is gone and its pane subtree reaped (down reaps this
	// session's pane children before returning — no sleep, mirroring the
	// down-reap tests).
	waitServerGone(t, tmuxPath, socket, sessionA)
	for _, pid := range aSubtree {
		if !processGone(pid) {
			t.Fatalf("worktree A pane subtree pid %d still running immediately after A down returned", pid)
		}
	}
	// A's own backing server process is gone too.
	waitServerProcCountForSession(t, pwshPath, socket, sessionA, 0)

	// CORE: worktree B's session, pane, backing-server pid, and agent root
	// process must ALL stay live throughout a stability window. A down that tore
	// down the shared socket's server set would trip this instead of reporting a
	// false green.
	assertSiblingStaysLive(t, tmuxPath, socket, sessionB, bPane, bServerPID, bPanePID, 2*time.Second)

	// And still exactly ONE backing server for B — no duplicate spawned during
	// the A up/down churn (the one process-table check, done once here rather
	// than per stability-loop iteration).
	if got := serverProcCountForSession(t, pwshPath, socket, sessionB); got != 1 {
		t.Fatalf("worktree B backing-server count = %d after A down; want exactly 1 (0 = killed, 2 = duplicate)", got)
	}

	// --- down in worktree B (the last session): server torn down, socket clear ---
	mustChdir(t, sibling)
	out.Reset()
	if code := RunCLI(&out, []string{"down"}); code != 0 {
		t.Fatalf("B down = %d; want 0, output: %s", code, out.String())
	}
	waitSocketFreeOfTmux(t, pwshPath, socket)
}
