//go:build tmux

// smoke_teardown_test.go pins the teardown guarantees of `reed down` and `reed remove` against a real tmux server: the server is released before down returns, every pane descendant is reaped, and a worktree's down never touches a sibling worktree's session.

package reedcli

import (
	"bytes"
	"runtime"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
)

// TestSmokeTeardown runs the teardown claims against one hub, each step bringing up its own session on the prime worktree and ending with that session down, so the next step starts from the same cold state.
// The scenario calls t.Parallel but no step does, because every step shares the one hub and its tmux server.
func TestSmokeTeardown(t *testing.T) {
	t.Parallel()
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	prime := h.PrimeWorktree()
	deferHubRelease(t, prime)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(prime, &buf, []string{"down"})
	})

	// run runs one reed verb in worktree and fails the calling test unless it exits zero.
	run := func(t *testing.T, worktree string, args ...string) []byte {
		t.Helper()
		var out bytes.Buffer
		if code := RunCLIIn(worktree, &out, args); code != 0 {
			t.Fatalf("%v = %d; want 0, output: %s", args, code, out.String())
		}
		return out.Bytes()
	}

	// DownReleasesServerBeforeReturning pins the down->up churn race.
	if !t.Run("DownReleasesServerBeforeReturning", func(t *testing.T) {
		run(t, prime, "up")
		socket, session := socketAndSessionIn(t, prime)

		launch := "pwsh -NoExit -Command Write-Host ready"
		for cycle := 0; cycle < 3; cycle++ {
			pid := serverPID(t, tmuxPath, socket, session)
			run(t, prime, "down")
			if !processGone(pid) {
				t.Fatalf("cycle %d: tmux server (pid %d) still running immediately after down returned", cycle, pid)
			}
			run(t, prime, "up")
			addStrandIn(t, prime, launch, "--name", "churn")
		}
		run(t, prime, "down")
	}) {
		return
	}

	// DownReapsPaneChildProcesses pins the pane-child reaping semantics.
	if !t.Run("DownReapsPaneChildProcesses", func(t *testing.T) {
		launch := smokeReapLaunchCmd()
		for cycle := 0; cycle < 3; cycle++ {
			run(t, prime, "up")
			addStrandIn(t, prime, launch, "--name", "reap")
			addStrandIn(t, prime, launch, "--name", "reap2")
			socket, session := socketAndSessionIn(t, prime)
			pids := paneProcessTree(t, tmuxPath, socket, session)
			if len(pids) == 0 {
				t.Fatalf("cycle %d: session reported no pane process subtree", cycle)
			}

			run(t, prime, "down")
			// No sleep: every pane descendant must already be gone the instant down returned. processGone reuses the same non-child Wait probe the server-pid step uses.
			for _, pid := range pids {
				if !processGone(pid) {
					t.Fatalf("cycle %d: pane subtree pid %d still running immediately after down returned", cycle, pid)
				}
			}
		}
	}) {
		return
	}

	// DownForceKillsSighupImmunePaneChildren is the regression guard for the reap-is-inert defect this round's R1 review found: reed's force-kill fallback for a straggling pane child was unreachable, because waitProcessExit blocked on os.Process.Wait, which returns ECHILD IMMEDIATELY for any pid that is not a child of the calling process — and every pane pid is a child of the TMUX server, never of lyx.
	// Every other reap step passed anyway, because their payloads (`sleep 300`, `pwsh -NoExit`) die to tmux's own SIGHUP cascade without reed ever needing to force-kill anything, so they could never distinguish "reed reaped it" from "tmux did".
	//
	// This step's payload is chosen to make exactly that distinction: the pane runs a descendant that TRAPS SIGHUP, so tmux's cascade cannot reap it and only reed's explicit force-kill can.
	// With the defect present, down returned ok while that descendant stayed alive (reproduced live before the fix); with it fixed, down blocks until the descendant is actually gone.
	//
	// POSIX-only: the payload needs a shell-level SIGHUP trap, which has no Windows equivalent.
	if !t.Run("DownForceKillsSighupImmunePaneChildren", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("payload needs a POSIX shell SIGHUP trap to become immune to tmux's own teardown cascade; Windows has no equivalent")
		}
		run(t, prime, "up")

		// `exec` on the outer sleep keeps the immune child's parent pid stable, so
		// the child stays inside the pane's descendant closure reed snapshots.
		immune := `bash -c 'bash -c "trap \"\" HUP; exec sleep 300" & exec sleep 300'`
		addStrandIn(t, prime, immune, "--name", "immune")
		socket, session := socketAndSessionIn(t, prime)

		pids := paneProcessTree(t, tmuxPath, socket, session)
		if len(pids) == 0 {
			t.Fatalf("session reported no pane process subtree")
		}

		run(t, prime, "down")
		// No sleep: down must not return until every snapshotted descendant is
		// actually gone, force-killed if the graceful window expired.
		for _, pid := range pids {
			if !processGone(pid) {
				t.Errorf("pane subtree pid %d still running immediately after down returned; reed's force-kill fallback did not run", pid)
			}
		}
	}) {
		return
	}

	// DownLeavesNoTmuxOnSocket pins the stray-server guarantee down's robust teardown owns: after down tears the shared server down, ZERO tmux process may still name this worktree's socket — not the main server, not its __warm__ helper.
	// The tmux server is spawned with the worktree as its cwd, so a server that outlives down keeps the worktree directory busy (a real "no stray state" leak observed under down->up churn on a saturated machine, where a fixed-deadline server wait timed out and aborted down before the socket was cleared).
	// Several add->down cycles give the async kill-server a chance to lag.
	if !t.Run("DownLeavesNoTmuxOnSocket", func(t *testing.T) {
		launch := smokeReapLaunchCmd()
		for cycle := 0; cycle < 3; cycle++ {
			run(t, prime, "up")
			socket, _ := socketAndSessionIn(t, prime)
			addStrandIn(t, prime, launch, "--name", "s1")
			addStrandIn(t, prime, launch, "--name", "s2")

			run(t, prime, "down")
			// No sleep: the moment down returns, the socket must be free of tmux.
			if pids := tmuxSocketPids(t, tmuxPath, socket); len(pids) != 0 {
				t.Fatalf("cycle %d: tmux still on socket %s after down returned: pids=%v", cycle, socket, pids)
			}
		}
	}) {
		return
	}

	// RemoveReapsRemovedPaneChildProcesses pins the reap gap this round generalized from down to remove:
	// kill-pane on a removed strand's pane terminates that pane's children asynchronously, and on Windows the process actually holding the worktree directory is a deep descendant of #{pane_pid} — so a remove that returned without reaping could leave a removed strand's grandchild alive and the worktree dir busy under load (the same class down's reap already closed).
	// remove now snapshots the removed panes' process subtrees before kill-pane and waits for them to exit before returning, so the instant remove returns every descendant of the removed pane must be gone.
	// A sibling strand is kept alive throughout so the session survives and the removed pane is never the sole pane.
	if !t.Run("RemoveReapsRemovedPaneChildProcesses", func(t *testing.T) {
		launch := smokeReapLaunchCmd()
		for cycle := 0; cycle < 3; cycle++ {
			run(t, prime, "up")
			// Keeper first, then the victim we remove.
			keeper := addStrandIn(t, prime, launch, "--name", "keeper")
			victim := addStrandIn(t, prime, launch, "--name", "victim")
			socket, session := socketAndSessionIn(t, prime)

			// Resolve the victim's pane, then snapshot only its process subtree.
			vs, ok := statusStrand(t, run(t, prime, "status"), victim)
			if !ok {
				t.Fatalf("cycle %d: status missing victim %s", cycle, victim)
			}
			victimPane, _ := vs["paneId"].(string)
			if victimPane == "" {
				t.Fatalf("cycle %d: victim %s has no pane", cycle, victim)
			}
			pids := panePaneSubtree(t, tmuxPath, socket, session, victimPane)
			if len(pids) == 0 {
				t.Fatalf("cycle %d: victim pane %s reported no process subtree", cycle, victimPane)
			}

			run(t, prime, "remove", victim)
			// No sleep: every descendant of the removed pane must already be gone
			// the instant remove returned.
			for _, pid := range pids {
				if !processGone(pid) {
					t.Fatalf("cycle %d: removed pane %s subtree pid %d still running immediately after remove returned", cycle, victimPane, pid)
				}
			}

			// The keeper must survive the remove untouched, and down cleans up for
			// the next cycle.
			if ks, ok := statusStrand(t, run(t, prime, "status"), keeper); !ok {
				t.Fatalf("cycle %d: keeper %s gone after removing victim", cycle, keeper)
			} else if live, _ := ks["live"].(bool); !live {
				t.Errorf("cycle %d: keeper %s live = false after removing victim; want true", cycle, keeper)
			}
			run(t, prime, "down")
		}
	}) {
		return
	}

	// DownInOneWorktreeLeavesSiblingSessionAlive codifies the CROSS-WORKTREE SCOPE invariant: the tmux server identity is per-hub (the -L socket derives from the hub) and shared by sibling worktrees, so `lyx reed down` in worktree A must tear down ONLY A's session, never worktree B's session, panes, or agents that share the same hub socket.
	// (This psmux port backs each session with its own `psmux.exe server -s <session> -L <socket>` process on the shared socket, so "no duplicate server" is verified per session: exactly one backing process per live session, never two, zero once killed.)
	// Two worktrees under one hub `up` (same socket, distinct sessions, one backing process each), each adds a live strand;
	// A goes `down`;
	// then B's session + pane + agent subtree + its single backing server must all still be live while A's session and pane subtree are gone.
	// B then `down`s last and the socket must be free of every tmux.
	// The core assertion is B's continued liveness AFTER A's down (see assertSiblingStaysLive) — a naive "down kills the whole socket's server set" implementation fails this step rather than reporting a false green.
	t.Run("DownInOneWorktreeLeavesSiblingSessionAlive", func(t *testing.T) {
		// Named "sibling", NOT "hub-b": real tmux's has-session/kill-session
		// target resolution fuzzy-matches an unambiguous prefix against a
		// live session when there is no exact name match — "hub" would
		// resolve against a lone remaining "hub-b" session even after "hub"
		// itself was killed, making the post-down waitServerGone(sessionA)
		// check below hang for the full timeout waiting on a has-session
		// probe that can never fail. Verified live: `tmux new-session -s hub`
		// + `-s hub-b`, kill-session -t hub, then `has-session -t hub` still
		// exits 0. A name with no shared prefix sidesteps the ambiguity
		// entirely rather than relying on tmux's target-matching rules.
		sibling := materializeSibling(t, h, "sibling")

		// Release the sibling worktree dir before the framework's TempDir RemoveAll.
		// Registered before the down cleanup so it runs after it (LIFO).
		deferHubRelease(t, sibling)

		// Best-effort teardown net: down the sibling from its own cwd even if an assertion aborts the body partway through, so no server/session leaks.
		t.Cleanup(func() {
			var buf bytes.Buffer
			RunCLIIn(sibling, &buf, []string{"down"})
		})

		launch := smokeReapLaunchCmd()

		// --- worktree A: up + a live strand ---
		run(t, prime, "up")
		socketA, sessionA := socketAndSessionIn(t, prime)
		aGuid := addStrandIn(t, prime, launch, "--name", "agent-a")
		aPane := paneIDForStrandIn(t, prime, aGuid)

		// --- worktree B: up + a live strand ---
		run(t, sibling, "up")
		socketB, sessionB := socketAndSessionIn(t, sibling)
		bGuid := addStrandIn(t, sibling, launch, "--name", "agent-b")
		bPane := paneIDForStrandIn(t, sibling, bGuid)

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
		waitServerProcCountForSession(t, tmuxPath, socket, sessionA, 1)
		waitServerProcCountForSession(t, tmuxPath, socket, sessionB, 1)

		// B's backing-server pid, for the post-down stability check: it must be
		// unchanged after A's down (B's server neither killed nor restarted).
		bServerPID := serverPID(t, tmuxPath, socket, sessionB)

		// Snapshot BEFORE A's down, while both panes still exist to enumerate:
		// worktree A's whole pane process subtree (asserted GONE after A's down — a
		// transient child that already exited still reads gone, so this is not
		// flaky), and worktree B's pane ROOT pid (asserted ALIVE — only the stable
		// root, never its come-and-go descendants, so the liveness check is robust).
		aSubtree := panePaneSubtree(t, tmuxPath, socket, sessionA, aPane)
		if len(aSubtree) == 0 {
			t.Fatalf("worktree A pane %s reported no process subtree", aPane)
		}
		bPanePID := paneRootPID(t, tmuxPath, socket, sessionB, bPane)

		// --- down in worktree A ---
		run(t, prime, "down")

		// A's own session is gone and its pane subtree reaped (down reaps this session's pane children before returning — no sleep, mirroring the down-reap steps).
		waitServerGone(t, tmuxPath, socket, sessionA)
		for _, pid := range aSubtree {
			if !processGone(pid) {
				t.Fatalf("worktree A pane subtree pid %d still running immediately after A down returned", pid)
			}
		}
		// A's own backing server process is gone too.
		waitServerProcCountForSession(t, tmuxPath, socket, sessionA, 0)

		// CORE: worktree B's session, pane, backing-server pid, and agent root
		// process must ALL stay live throughout a stability window. A down that tore
		// down the shared socket's server set would trip this instead of reporting a
		// false green.
		assertSiblingStaysLive(t, tmuxPath, socket, sessionB, bPane, bServerPID, bPanePID, 2*time.Second)

		// And still exactly ONE backing server for B — no duplicate spawned during
		// the A up/down churn (the one process-table check, done once here rather
		// than per stability-loop iteration).
		if got := serverProcCountForSession(t, tmuxPath, socket, sessionB); got != 1 {
			t.Fatalf("worktree B backing-server count = %d after A down; want exactly 1 (0 = killed, 2 = duplicate)", got)
		}

		// --- down in worktree B (the last session): server torn down, socket clear ---
		run(t, sibling, "down")
		waitSocketFreeOfTmux(t, tmuxPath, socket)
	})
}
