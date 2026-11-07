// smoke_coldstart_test.go pins the cold-start behavior of `lym reed add` and `lym reed attach`.
// Each step runs in its own sibling worktree that was never brought up and holds no persisted state file.
// Both verbs boot the substrate themselves instead of refusing with the no-session error.
//
// The scenario drives a real multiplexer against one forged hub, with the fixtures of smoke_test.go.
//
// A freshly forged worktree refuses the status verb before anything boots.
// So a test derives the socket and session name before any boot from reedengine.ServerName and reedengine.SessionName.

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

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// runReedCLINoFatal runs the built lym binary with args in dir and returns its combined
// stdout+stderr, never failing the test itself.
// The cold-attach scenario needs the exit status and the multiplexer's own error text.
// The terminal-handover tail writes to the OS process's stdio, not to a writer the test controls;
// an in-process capture cannot see that output.
func runReedCLINoFatal(t *testing.T, exe, dir string, timeout time.Duration, args ...string) string {
	t.Helper()
	out, _, err := lymbin.Run(exe, dir, timeout, args...)
	if err != nil {
		t.Fatalf("lym %v: %v", args, err)
	}
	return out
}

// TestSmokeColdStart runs the cold-start claims against one hub.
// Each step runs on its own fresh sibling worktree.
// That worktree is never brought up and carries no state file when the step begins.
// The steps do not depend on each other's results, but they share the hub's one tmux server, so they run serially.
// The test as a whole still calls t.Parallel.
func TestSmokeColdStart(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	tmuxPath := tmuxBinaryPath(t)
	lymExe := lymbin.Build(t)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	deferHubRelease(t, h.PrimeWorktree())
	socket := reedengine.ServerName(h.Path)
	tmuxkit.KillOnCleanup(t, tmuxPath, socket)

	// coldWorktree clones a fresh sibling worktree for the calling step and registers its teardown.
	coldWorktree := func(t *testing.T, name string) string {
		t.Helper()
		worktree := materializeSibling(t, h, name)
		deferHubRelease(t, worktree)
		t.Cleanup(func() {
			var buf bytes.Buffer
			RunCLIIn(worktree, &buf, []string{"down"})
		})
		return worktree
	}

	// ColdAddBootsAndAddsInOneCall pins the main cold-start claim.
	// On a worktree never brought up, with no persisted state file, `add` exits zero and its envelope carries a guid.
	// The multiplexer afterwards lists this worktree's session on the hub socket.
	// The session holds the header pane as well as the strand's own pane.
	// That proves the delegate path ran the whole boot body that an explicit `up` plus `add` would run:
	// it did not only spawn a bare server.
	if !t.Run("ColdAddBootsAndAddsInOneCall", func(t *testing.T) {
		worktree := coldWorktree(t, "cold-headline")

		// addStrandIn already fails the test unless the add exits zero and its envelope carries a non-empty guid:
		// both halves of the scenario's assertion.
		addStrandIn(t, worktree, smokeReapLaunchCmd(), "--name", "cold-headline")

		session := reedengine.SessionName(worktree)
		if !sessionAlive(tmuxPath, socket, session) {
			t.Errorf("session %s not alive on socket %s after a cold add; want the boot to have deposited it", session, socket)
		}
		panes := listPaneLines(t, tmuxPath, socket, session)
		if len(panes) != 2 {
			t.Errorf("panes after a cold add = %v; want 2 (the always-present header pane plus the strand's own pane) -- a shortfall means the delegate path only spawned a server rather than running the whole boot body", panes)
		}
	}) {
		return
	}

	// ColdAttachBootsThenFailsOnTheTerminalHandoverNotOnNoSession pins `attach` on a worktree never brought up:
	// run without a controlling terminal, it must fail with the multiplexer's own terminal error.
	// It must never fail with the no-session JSON envelope.
	// The session must exist on the socket afterwards either way.
	// The distinction between those two failure modes is the assertion.
	// A no-session refusal would mean attach never reached the boot at all.
	// The terminal error is reachable only once the boot has succeeded and the handover tail took over stdio.
	if !t.Run("ColdAttachBootsThenFailsOnTheTerminalHandoverNotOnNoSession", func(t *testing.T) {
		worktree := coldWorktree(t, "cold-attach")

		output := runReedCLINoFatal(t, lymExe, worktree, 30*time.Second, "reed", "attach")

		if strings.Contains(output, "no reed session") {
			t.Fatalf("cold attach output = %q; want no trace of the no-session refusal -- attach must boot before it can ever reach that check", output)
		}
		if !strings.Contains(strings.ToLower(output), "not a terminal") {
			t.Errorf("cold attach output = %q; want the multiplexer's own terminal-handover error, proving the boot succeeded and the handover itself was reached", output)
		}

		session := reedengine.SessionName(worktree)
		if !sessionAlive(tmuxPath, socket, session) {
			t.Errorf("session %s not alive on socket %s after a cold attach; want the boot to have deposited it even though the handover then failed", session, socket)
		}
	}) {
		return
	}

	// ColdAddWithIfAbsentRelaunchesOnlyTheMatchedStrand pins what `add --if-absent` does on a cold worktree.
	// With two strands persisted and the server dead, it boots and relaunches exactly the named strand.
	// That strand gets one pane, not two entries under one name, and the other strand stays unrelaunched.
	// A cold boot never replays the whole persisted table.
	// It kills the hub's tmux server, so it runs after the steps that need a live session of their own.
	if !t.Run("ColdAddWithIfAbsentRelaunchesOnlyTheMatchedStrand", func(t *testing.T) {
		worktree := coldWorktree(t, "cold-ifabsent")

		var out bytes.Buffer
		if code := RunCLIIn(worktree, &out, []string{"up"}); code != 0 {
			t.Fatalf("up = %d; want 0, output: %s", code, out.String())
		}
		matchedGUID := addStrandIn(t, worktree, smokeReapLaunchCmd(), "--name", "reopen-me")
		otherGUID := addStrandIn(t, worktree, smokeReapLaunchCmd(), "--name", "leave-me-alone")

		socket, session := socketAndSessionIn(t, worktree)
		if err := exec.Command(tmuxPath, "-L", socket, "kill-server").Run(); err != nil {
			t.Fatalf("kill-server: %v", err)
		}
		waitServerGone(t, tmuxPath, socket, session)

		relaunchedGUID := addStrandIn(t, worktree, smokeReapLaunchCmd(), "--if-absent", "--name", "reopen-me")
		if relaunchedGUID != matchedGUID {
			t.Fatalf("cold add --if-absent guid = %s; want %s (the relaunched match, same guid, not a second strand)", relaunchedGUID, matchedGUID)
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

		matchedCount := 0
		var matchedLive, otherLive bool
		for _, s := range strands {
			strand, _ := s.(map[string]any)
			switch strand["guid"] {
			case matchedGUID:
				matchedCount++
				matchedLive, _ = strand["live"].(bool)
			case otherGUID:
				otherLive, _ = strand["live"].(bool)
			}
		}
		if matchedCount != 1 {
			t.Fatalf("status strands carrying guid %s = %d; want exactly 1 -- up semantics never stack a duplicate entry under a matched name", matchedGUID, matchedCount)
		}
		if !matchedLive {
			t.Errorf("status strand %s live = false after the cold add --if-absent relaunched it; want true", matchedGUID)
		}
		if otherLive {
			t.Errorf("status strand %s live = true; want false -- a cold boot clears every persisted binding, and only the --if-absent match is relaunched, never the other persisted strand", otherGUID)
		}

		panes := listPaneLines(t, tmuxPath, socket, session)
		if len(panes) != 2 {
			t.Fatalf("panes after the cold add --if-absent relaunch = %v; want 2 (the header pane plus the one relaunched strand's pane, never a second pane for the untouched strand)", panes)
		}
	}) {
		return
	}

	// ColdAddOnAnUnreadableStateFileBootsThenFailsWithTheResidueAccepted pins the corrupt-state case.
	// A corrupt `reed.json` boots the session first and then fails with the state loader's corrupt-file diagnosis.
	// The failure leaves a bare session behind.
	// On a worktree never brought up whose .lym/reed.json holds truncated bytes, `add` exits non-zero.
	// The error names the file and the teardown verb.
	// Both the non-zero exit and the surviving session are the assertion.
	// The residue is an accepted cost of matching `up`'s behaviour.
	// A later change to that posture must fail this step loudly instead of passing silently.
	t.Run("ColdAddOnAnUnreadableStateFileBootsThenFailsWithTheResidueAccepted", func(t *testing.T) {
		worktree := coldWorktree(t, "cold-unreadable")

		statePath := filepath.Join(worktree, ".lym", "reed.json")
		if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
			t.Fatalf("create %s: %v", filepath.Dir(statePath), err)
		}
		// A truncated write is the ordinary way this file becomes unreadable:
		// a crash, a kill -9, a full disk, or a power loss partway through a save.
		if err := os.WriteFile(statePath, []byte(`{"socket":"lym-x","strands":[{"gu`), 0o600); err != nil {
			t.Fatalf("write %s: %v", statePath, err)
		}

		var out bytes.Buffer
		if code := RunCLIIn(worktree, &out, []string{"add", "--cmd", smokeReapLaunchCmd()}); code == 0 {
			t.Fatalf("cold add with an unreadable reed.json = 0; want a failure, output: %s", out.String())
		}
		unreadable := envelopeError(t, out.Bytes())
		for _, want := range []string{"unreadable", "reed.json", "lym reed down"} {
			if !strings.Contains(unreadable, want) {
				t.Errorf("cold add error = %s; want it to contain %q (the state loader's corrupt-file diagnosis, naming the file and the teardown verb)", unreadable, want)
			}
		}

		// The residue is accepted:
		// add boots the session before loading state, matching `up`, so a corrupt reed.json leaves a bare session behind.
		session := reedengine.SessionName(worktree)
		if !sessionAlive(tmuxPath, socket, session) {
			t.Errorf("session %s not alive on socket %s after a cold add on an unreadable state file; want the boot's residue left behind, matching up's own accepted behaviour", session, socket)
		}
	})
}
