// smoke_selvage_keepalive_test.go pins Selvage's keepalive guarantee:
// a session with Selvage in it never dies just because every strand died.
// The guarantee is pinned against the real mechanism: an ordinary shell must survive a killed sibling pane.
// It must also survive Ctrl-C at its own idle prompt, and Ctrl-C to a foreground job running inside it.
// Tagged smoke because untagged reedcli tests must not spawn processes (Test Tier Purity Invariant).
// The scenario also pins Selvage's placement: the sole pane of the shell window, never a strand's pane.
// It also pins the status-line options `up` sets in the session.

package reedcli

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// selvagePaneID reads the persisted SelvagePaneID for worktree, and fails the test if it is empty.
// RunCLI's status output carries no Selvage field, so every caller reads the id straight from reed.json.
func selvagePaneID(t *testing.T, worktree string) string {
	t.Helper()
	st, err := reedengine.LoadState(filepath.Join(worktree, ".lym"))
	if err != nil || st == nil || st.SelvagePaneID == "" {
		t.Fatalf("LoadState(%s) = (%+v, %v), want a persisted SelvagePaneID", worktree, st, err)
	}
	return st.SelvagePaneID
}

// TestSmokeSelvage runs the Selvage claims against one hub's prime worktree.
// Each step starts from a freshly restarted session.
// The steps run serially in a fixed order and do not rely on each other's results.
// The scenario calls t.Parallel but no step does, because every step shares the one hub and its tmux server.
func TestSmokeSelvage(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime := h.PrimeWorktree()
	deferHubRelease(t, prime)
	tmuxkit.KillOnCleanup(t, tmuxPath, reedengine.ServerName(h.Path))
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(prime, &buf, []string{"down"})
	})

	// SurvivesUpAddRemoveAndReconcile pins Selvage's keepalive guarantee across a full up -> add -> remove -> add cycle.
	// Selvage must survive every reconcile along the way.
	// It must also stay alive when the strand table drops to zero after a remove.
	// Selvage is never a strand's pane, since each strand's pane is created fresh in a window of its own.
	// Reconcile's kill schedule exempts Selvage from both halves.
	// The check runs tmux list-panes against the real binary, not reed's own reporting.
	if !t.Run("SurvivesUpAddRemoveAndReconcile", func(t *testing.T) {
		restartSession(t, prime)

		// up boots Selvage before any strand exists.
		// Read the persisted pane id from reed.json, since status carries no Selvage field.
		// Do not assume which of the session's panes it is.
		selvage := selvagePaneID(t, prime)

		socket, session := socketAndSessionIn(t, prime)
		requireSelvageAlive := func(when string) {
			t.Helper()
			lines := listPaneLines(t, tmuxPath, socket, session)
			if !paneLiveOnSession(lines, selvage) {
				t.Fatalf("Selvage pane %s not alive %s; panes=%v", selvage, when, lines)
			}
		}
		requireSelvageAlive("right after up (zero strands)")

		// add: the preceding up's reconcile already reaped the session's pre-Selvage pane.
		// So Selvage is the session's only pane when this first strand is added.
		// The strand lands on a pane in a window of its own, never on Selvage.
		guid := addStrandIn(t, prime, "pwsh -NoExit -Command Write-Host ready", "--name", "first")
		requireSelvageAlive("after add")

		strand, found := statusStrand(t, mustRunReed(t, prime, "status"), guid)
		if !found {
			t.Fatalf("status missing strand %s", guid)
		}
		if live, _ := strand["live"].(bool); !live {
			t.Errorf("strand %s live = false; want true", guid)
		}
		strandPaneID, _ := strand["paneId"].(string)
		if strandPaneID == "" || strandPaneID == selvage {
			t.Fatalf("strand %s paneId = %q, want a real, non-Selvage pane id", guid, strandPaneID)
		}

		// remove: the session's only strand is gone.
		// Selvage, a permanent second pane, must keep the session and itself alive.
		mustRunReed(t, prime, "remove", guid)
		if !sessionAlive(tmuxPath, socket, session) {
			t.Fatalf("session %s died after removing its sole strand; Selvage must have kept it alive", session)
		}
		requireSelvageAlive("after removing the sole strand (zero strands tracked)")

		// A reconciling up with zero strands must not disturb Selvage.
		mustRunReed(t, prime, "up")
		requireSelvageAlive("after a reconciling up with zero strands")

		// add again: Selvage must never become the new strand's own pane, and the new strand must come up live.
		// The session Selvage keeps alive stays usable for new strands.
		second := addStrandIn(t, prime, "pwsh -NoExit -Command Write-Host ready", "--name", "second")
		requireSelvageAlive("after a second add with strands now bound")

		strand2, found := statusStrand(t, mustRunReed(t, prime, "status"), second)
		if !found {
			t.Fatalf("status missing strand %s", second)
		}
		if live, _ := strand2["live"].(bool); !live {
			t.Errorf("strand %s live = false; want true", second)
		}
		if paneID, _ := strand2["paneId"].(string); paneID == selvage {
			t.Errorf("strand %s was bound to Selvage %s; Selvage must never become a strand's own pane", second, selvage)
		}
	}) {
		return
	}

	// SurvivesKillingEveryStrandPane pins the keepalive job itself.
	// tmux destroys a session once its last window loses its last pane.
	// With every strand pane force-killed, Selvage alone must keep the session alive and a later reed verb usable.
	// The kills go through tmux directly, not reed's own remove, which would already tear its bookkeeping down cleanly.
	if !t.Run("SurvivesKillingEveryStrandPane", func(t *testing.T) {
		restartSession(t, prime)
		selvage := selvagePaneID(t, prime)

		guids := []string{
			addStrandIn(t, prime, smokeReapLaunchCmd(), "--name", "first"),
			addStrandIn(t, prime, smokeReapLaunchCmd(), "--name", "second"),
		}

		socket, session := socketAndSessionIn(t, prime)
		for _, guid := range guids {
			paneID := paneIDForStrandIn(t, prime, guid)
			if err := exec.Command(tmuxPath, "-L", socket, "kill-pane", "-t", paneID).Run(); err != nil {
				t.Fatalf("kill-pane %s (strand %s): %v", paneID, guid, err)
			}
		}

		if !sessionAlive(tmuxPath, socket, session) {
			t.Fatalf("session %s died after killing every strand pane; Selvage must have kept it alive", session)
		}
		if panes := listPaneLines(t, tmuxPath, socket, session); !paneLiveOnSession(panes, selvage) {
			t.Fatalf("Selvage pane %s not alive after killing every strand pane; panes=%v", selvage, panes)
		}

		// status must still answer after the strand panes are gone.
		mustRunReed(t, prime, "status")
	}) {
		return
	}

	// Every strand lands in a window of its own.
	// So Selvage stays the only pane of the shell window in every state that adds and removes can leave the session in.
	if !t.Run("StaysSoleInShellWindowAcrossAddsAndRemoves", func(t *testing.T) {
		restartSession(t, prime)
		selvage := selvagePaneID(t, prime)
		socket, _ := socketAndSessionIn(t, prime)

		requireSoleInShellWindow := func(when string) {
			t.Helper()
			// A pane target without -s lists only the panes of the window holding that pane.
			out, err := exec.Command(tmuxPath, "-L", socket, "list-panes", "-t", selvage, "-F", "#{pane_id}").Output()
			if err != nil {
				t.Fatalf("%s: list-panes -t %s: %v", when, selvage, err)
			}
			if got := strings.TrimSpace(string(out)); got != selvage {
				t.Errorf("%s: shell window panes = %q; want only Selvage %s", when, got, selvage)
			}
		}
		requireSoleInShellWindow("right after up")

		first := addStrandIn(t, prime, smokeReapLaunchCmd(), "--name", "first")
		requireSoleInShellWindow("after adding the first strand")

		second := addStrandIn(t, prime, smokeReapLaunchCmd(), "--name", "second")
		requireSoleInShellWindow("after adding a second strand")

		mustRunReed(t, prime, "remove", first)
		requireSoleInShellWindow("after removing the first strand")

		addStrandIn(t, prime, smokeReapLaunchCmd(), "--name", "third")
		requireSoleInShellWindow("after adding a third strand")

		mustRunReed(t, prime, "remove", second)
		requireSoleInShellWindow("after removing the second strand")
	}) {
		return
	}

	// SurvivesCtrlCAtIdlePrompt checks that a Ctrl-C sent directly to Selvage at its idle prompt never kills the pane.
	// The session that Selvage anchors must survive as well.
	// Interactive bash ignores SIGINT while it waits at its own prompt, and that is what the survival rests on.
	if !t.Run("SurvivesCtrlCAtIdlePrompt", func(t *testing.T) {
		restartSession(t, prime)
		selvage := selvagePaneID(t, prime)
		socket, session := socketAndSessionIn(t, prime)

		// Bash ignores SIGINT only once it sits at its prompt;
		// a Ctrl-C that lands during startup kills it, so wait for the prompt to render.
		waitForCondition(t, 10*time.Second, func() bool {
			return strings.TrimSpace(capturePane(t, tmuxPath, socket, selvage)) != ""
		})

		if err := exec.Command(tmuxPath, "-L", socket, "send-keys", "-t", selvage, "C-c").Run(); err != nil {
			t.Fatalf("send-keys C-c to idle Selvage %s: %v", selvage, err)
		}
		time.Sleep(1 * time.Second)

		if !sessionAlive(tmuxPath, socket, session) {
			t.Fatalf("session %s died after Ctrl-C to Selvage's idle prompt", session)
		}
		if panes := listPaneLines(t, tmuxPath, socket, session); !paneLiveOnSession(panes, selvage) {
			t.Fatalf("Selvage pane %s not alive after Ctrl-C at its idle prompt; panes=%v", selvage, panes)
		}
	}) {
		return
	}

	// SurvivesCtrlCKillingAForegroundJob checks that Ctrl-C to a foreground job in Selvage kills only that job.
	// The shell pane itself must survive.
	if !t.Run("SurvivesCtrlCKillingAForegroundJob", func(t *testing.T) {
		restartSession(t, prime)
		selvage := selvagePaneID(t, prime)
		socket, session := socketAndSessionIn(t, prime)

		// Baseline: Selvage's own shell and whatever it has spawned before the foreground job starts.
		// panePaneSubtree returns the descendant closure of the pane, read from /proc on linux and from WMI on windows.
		before := map[int]bool{}
		for _, pid := range panePaneSubtree(t, tmuxPath, socket, session, selvage) {
			before[pid] = true
		}

		// A foreground job is typed into Selvage's own shell, as an operator would type a long command at the prompt.
		// reed does not start it.
		sendKeysLine(t, tmuxPath, socket, selvage, smokeReapLaunchCmd())

		var jobPIDs []int
		deadline := time.Now().Add(10 * time.Second)
		for {
			for _, pid := range panePaneSubtree(t, tmuxPath, socket, session, selvage) {
				if !before[pid] {
					jobPIDs = append(jobPIDs, pid)
				}
			}
			if len(jobPIDs) > 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("Selvage's foreground job never showed up as a new descendant within 10s")
			}
			time.Sleep(200 * time.Millisecond)
		}

		if err := exec.Command(tmuxPath, "-L", socket, "send-keys", "-t", selvage, "C-c").Run(); err != nil {
			t.Fatalf("send-keys C-c to Selvage %s running a foreground job: %v", selvage, err)
		}

		for _, pid := range jobPIDs {
			pollProcessGone(t, pid, 10*time.Second)
		}

		if !sessionAlive(tmuxPath, socket, session) {
			t.Fatalf("session %s died after Ctrl-C killed Selvage's foreground job", session)
		}
		if panes := listPaneLines(t, tmuxPath, socket, session); !paneLiveOnSession(panes, selvage) {
			t.Fatalf("Selvage pane %s not alive after Ctrl-C killed its foreground job; panes=%v", selvage, panes)
		}
	}) {
		return
	}

	// StatusLinePinsIdentityAndPosition checks the status-bar pins that pinGeometryOptionsLocked sets after `up`.
	// `#{status}` reads "3" (the three-line bar) and `#{status-position}` reads "bottom".
	// The bar's content is not a pane, so the assertion is a `display-message -p` readback rather than a pane-content poll.
	//
	// On Windows the identity values are not asserted directly:
	// psmux may refuse some or all of the status-bar set-option calls.
	// reed does not branch to compensate.
	// What is asserted there instead is the self-correcting half: whatever `#{status}` reads back is checked.
	// The reserved-row count it implies must match the window the layout was planned against,
	// so a psmux that refuses the options fails the identity assertion loudly, not silently corrupting the layout.
	t.Run("StatusLinePinsIdentityAndPosition", func(t *testing.T) {
		restartSession(t, prime)
		socket, session := socketAndSessionIn(t, prime)
		target := "=" + session + ":"

		readOption := func(option string) string {
			t.Helper()
			out, err := exec.Command(tmuxPath, "-L", socket, "display-message", "-p", "-t", target, "#{"+option+"}").Output()
			if err != nil {
				t.Fatalf("display-message #{%s}: %v", option, err)
			}
			return strings.TrimSpace(string(out))
		}

		if runtime.GOOS == "windows" {
			// The mapping from #{status} to reserved rows, which this test expects:
			// "off" -> 0, "on" -> 1, a non-negative integer string -> that integer verbatim.
			status := strings.ToLower(readOption("status"))
			reserved := 0
			switch status {
			case "off":
				reserved = 0
			case "on":
				reserved = 1
			default:
				n, err := strconv.Atoi(status)
				if err != nil || n < 0 {
					t.Fatalf("#{status} = %q; want off, on, or a non-negative integer", status)
				}
				reserved = n
			}

			windowHeight, err := strconv.Atoi(readOption("window_height"))
			if err != nil {
				t.Fatalf("parse #{window_height}: %v", err)
			}
			lines := listPaneLines(t, tmuxPath, socket, session)
			paneRowsSum := 0
			for _, line := range lines {
				fields := strings.Fields(line)
				if len(fields) < 4 {
					continue
				}
				height, err := strconv.Atoi(fields[3])
				if err != nil {
					t.Fatalf("parse pane_height %q: %v", fields[3], err)
				}
				paneRowsSum += height
			}
			// tmux draws one border row between each pair of vertically stacked panes.
			borders := len(lines) - 1
			if borders < 0 {
				borders = 0
			}
			if paneRowsSum+borders+reserved != windowHeight {
				t.Errorf("panes(%d) + borders(%d) + status-reserved(%d) = %d; want the live window height %d — whatever #{status} reads back must match the window the layout was actually planned against", paneRowsSum, borders, reserved, paneRowsSum+borders+reserved, windowHeight)
			}
			return
		}

		if got := readOption("status"); got != "3" {
			t.Errorf("#{status} = %q; want \"3\"", got)
		}
		if got := readOption("status-position"); got != "bottom" {
			t.Errorf("#{status-position} = %q; want \"bottom\"", got)
		}
	})
}
