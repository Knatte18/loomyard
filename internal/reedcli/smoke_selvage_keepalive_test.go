//go:build smoke

// smoke_selvage_keepalive_test.go pins Selvage's keepalive guarantee — the job the header pane's
// `--blocking` process once served before this task, now served by a permanent, deliberately
// ordinary shell pane instead of a custom blocking mechanism. There is no analogue of the old
// blockForever/deadlock-avoidance test here: Selvage runs the operator's configured shell
// (e.cfg.Shell) directly, so there is no reed-authored blocking loop left to pin against a runtime
// deadlock detector. What survives is the guarantee itself — a session with Selvage in it never
// dies just because every strand died — pinned instead against the real mechanism: an ordinary
// shell surviving a killed sibling pane, surviving Ctrl-C at its own idle prompt, and surviving
// Ctrl-C to a foreground job running inside it. Tagged smoke because untagged reedcli tests must
// not spawn processes (Test Tier Purity Invariant).

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
)

// selvagePaneID reads the persisted SelvagePaneID for worktree, failing the test if it is empty —
// RunCLI/status carries no Selvage field of its own, so every caller here reads it straight from
// reed.json exactly as the header-pane tests once did for HeaderPaneID.
func selvagePaneID(t *testing.T, worktree string) string {
	t.Helper()
	st, err := reedengine.LoadState(filepath.Join(worktree, ".lyx"))
	if err != nil || st == nil || st.SelvagePaneID == "" {
		t.Fatalf("LoadState(%s) = (%+v, %v), want a persisted SelvagePaneID", worktree, st, err)
	}
	return st.SelvagePaneID
}

// TestSmokeSelvageSurvivesKillingEveryStrandPane pins the keepalive job itself: tmux destroys a
// session once its last window loses its last pane, so with every strand pane force-killed via
// tmux (not reed's own remove, which would already tear its bookkeeping down cleanly), Selvage
// alone must be enough to keep the session — and a subsequent reed verb usable — alive.
func TestSmokeSelvageSurvivesKillingEveryStrandPane(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	deferHubRelease(t, h.PrimeWorktree())
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	selvage := selvagePaneID(t, h.PrimeWorktree())

	guids := []string{
		addStrand(t, smokeReapLaunchCmd(), "--name", "first"),
		addStrand(t, smokeReapLaunchCmd(), "--name", "second"),
	}

	socket, session := socketAndSession(t)
	for _, guid := range guids {
		paneID := paneIDForStrand(t, guid)
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

	out.Reset()
	if code := RunCLI(&out, []string{"status"}); code != 0 {
		t.Fatalf("status after killing every strand pane = %d; want 0, output: %s", code, out.String())
	}
}

// TestSmokeSelvageStaysPhysicallyBottomMostAcrossAddsAndRemoves pins the physical-position rule
// module-local to this package (see internal/reedengine/doc.go): Selvage is always the pane with
// the largest pane_top, in every state a series of adds and removes can leave the window in.
func TestSmokeSelvageStaysPhysicallyBottomMostAcrossAddsAndRemoves(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	deferHubRelease(t, h.PrimeWorktree())
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	selvage := selvagePaneID(t, h.PrimeWorktree())
	socket, session := socketAndSession(t)

	requireBottomMost := func(when string) {
		t.Helper()
		panes := listPaneLines(t, tmuxPath, socket, session)
		bottomTop := -1
		bottomID := ""
		for _, line := range panes {
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			top, err := strconv.Atoi(fields[2])
			if err != nil {
				t.Fatalf("parse pane_top %q: %v", fields[2], err)
			}
			if top > bottomTop {
				bottomTop = top
				bottomID = fields[0]
			}
		}
		if bottomID != selvage {
			t.Errorf("%s: physically bottom-most pane = %s; want Selvage %s; panes=%v", when, bottomID, selvage, panes)
		}
	}
	requireBottomMost("right after up")

	first := addStrand(t, smokeReapLaunchCmd(), "--name", "first")
	requireBottomMost("after adding the first strand")

	second := addStrand(t, smokeReapLaunchCmd(), "--name", "second")
	requireBottomMost("after adding a second strand")

	out.Reset()
	if code := RunCLI(&out, []string{"remove", first}); code != 0 {
		t.Fatalf("remove %s = %d; want 0, output: %s", first, code, out.String())
	}
	requireBottomMost("after removing the first strand")

	addStrand(t, smokeReapLaunchCmd(), "--name", "third")
	requireBottomMost("after adding a third strand")

	out.Reset()
	if code := RunCLI(&out, []string{"remove", second}); code != 0 {
		t.Fatalf("remove %s = %d; want 0, output: %s", second, code, out.String())
	}
	requireBottomMost("after removing the second strand")
}

// TestSmokeSelvageSurvivesCtrlCAtIdlePrompt pins the first of the two ordinary-shell survival
// claims in internal/reedengine/doc.go's design record: interactive bash ignores SIGINT while
// waiting at its own idle prompt, so a Ctrl-C sent directly to Selvage must never take the pane —
// or the session it anchors — down.
func TestSmokeSelvageSurvivesCtrlCAtIdlePrompt(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	deferHubRelease(t, h.PrimeWorktree())
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	selvage := selvagePaneID(t, h.PrimeWorktree())
	socket, session := socketAndSession(t)

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
}

// TestSmokeSelvageSurvivesCtrlCKillingAForegroundJob pins the second ordinary-shell survival
// claim: Ctrl-C to a foreground job running inside Selvage kills only that job, never the shell
// pane itself.
func TestSmokeSelvageSurvivesCtrlCKillingAForegroundJob(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	deferHubRelease(t, h.PrimeWorktree())
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	selvage := selvagePaneID(t, h.PrimeWorktree())
	socket, session := socketAndSession(t)

	// Baseline: Selvage's own shell and whatever it has spawned so far, before the foreground job
	// starts — panePaneSubtree is the same cross-platform (linux/proc, windows/WMI) descendant-closure
	// helper the pane-reap smoke tests already use.
	before := map[int]bool{}
	for _, pid := range panePaneSubtree(t, tmuxPath, socket, session, selvage) {
		before[pid] = true
	}

	// A foreground job typed literally into Selvage's own shell — never reed's doing, exactly like
	// an operator running a long command directly at the prompt.
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
}

// TestSmokeStatusLinePinsIdentityAndPosition pins pinGeometryOptionsLocked's status-line pins after
// `up`: `#{status}` reads "on", `#{status-position}` reads "bottom", and `#{status-left}` names both
// the repo and the worktree.
//
// On Windows the identity values are not asserted directly — per the
// windows-status-line-is-an-unbranched-accepted-degrade Shared Decision, psmux may refuse some or all
// of the seven status-line set-option calls, and reed does not branch to compensate. What is asserted
// there instead is the SELF-CORRECTING half: whatever `#{status}` reads back, the reserved-row count
// it implies must match the window the layout was actually planned against, so a psmux that refuses
// the options fails the identity assertion loudly (an operator watching the status-line notices)
// rather than silently corrupting the layout.
func TestSmokeStatusLinePinsIdentityAndPosition(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	deferHubRelease(t, h.PrimeWorktree())
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	socket, session := socketAndSession(t)
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
		// reservedRowsFromStatus's own mapping, inlined here since it is unexported in reedengine:
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

	if got := readOption("status"); got != "on" {
		t.Errorf("#{status} = %q; want \"on\"", got)
	}
	if got := readOption("status-position"); got != "bottom" {
		t.Errorf("#{status-position} = %q; want \"bottom\"", got)
	}
	statusLeft := readOption("status-left")
	if !strings.Contains(statusLeft, h.Location.RepoName) {
		t.Errorf("#{status-left} = %q; want it to contain the repo name %q", statusLeft, h.Location.RepoName)
	}
	if !strings.Contains(statusLeft, h.Location.WorktreeName) {
		t.Errorf("#{status-left} = %q; want it to contain the worktree name %q", statusLeft, h.Location.WorktreeName)
	}
}
