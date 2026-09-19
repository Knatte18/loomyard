//go:build smoke

// smoke_coldstart_test.go pins the headline scenario this task adds: `lyx reed add` and `lyx reed
// attach`, run against a worktree that was never brought up and carries no persisted state file at
// all, boot the substrate themselves rather than refusing with the friendly no-session error. Each
// test drives a real multiplexer against a forged hub, using the package's existing fixture
// vocabulary (see smoke_test.go) rather than any new fixture machinery.
//
// A freshly forged worktree still refuses the status verb before anything boots, so a test that
// needs this worktree's socket and session name before any boot derives them from reedengine's own
// exported ServerName and SessionName free functions rather than from socketAndSessionIn, which
// drives `status` under the hood.

package reedcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// runReedCLINoFatal runs the built lyx binary with args in dir and returns its combined
// stdout+stderr, never failing the test itself: the cold-attach scenario this backs needs to inspect
// a non-zero exit and the multiplexer's own error text on stdout/stderr, neither of which the
// in-process RunCLIIn seam can produce, since the terminal-handover tail's stdio belongs to the real
// OS process, not to an io.Writer this package controls.
func runReedCLINoFatal(t *testing.T, exe, dir string, timeout time.Duration, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("lyx %v timed out after %s in %s; output so far:\n%s", args, timeout, dir, out)
	}
	if err == nil {
		return string(out)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return string(out)
	}
	t.Fatalf("lyx %v: %v; output:\n%s", args, err, out)
	return ""
}

// TestSmokeColdAddBootsAndAddsInOneCall is the headline scenario this task adds: on a worktree never
// brought up, with no persisted state file, `add` exits zero, its envelope carries a guid, and the
// multiplexer afterwards lists this worktree's session on the hub socket.
func TestSmokeColdAddBootsAndAddsInOneCall(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	worktree := h.PrimeWorktree()
	deferHubRelease(t, worktree)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(worktree, &buf, []string{"down"})
	})

	// addStrandIn already fails the test unless the add exits zero and its envelope carries a
	// non-empty guid -- both halves of the headline assertion.
	addStrandIn(t, worktree, smokeReapLaunchCmd(), "--name", "cold-headline")

	socket := reedengine.ServerName(h.Path)
	session := reedengine.SessionName(worktree)
	if !sessionAlive(tmuxPath, socket, session) {
		t.Errorf("session %s not alive on socket %s after a cold add; want the boot to have deposited it", session, socket)
	}
}

// TestSmokeColdAddYieldsTheSameSubstrateAsAnExplicitBoot pins that a cold add builds the same
// substrate an explicit `up` plus `add` would: after the cold add, the session holds the header pane
// as well as the strand's own pane, proving the delegate path ran the whole boot body rather than
// only spawning a bare server.
func TestSmokeColdAddYieldsTheSameSubstrateAsAnExplicitBoot(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	worktree := h.PrimeWorktree()
	deferHubRelease(t, worktree)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(worktree, &buf, []string{"down"})
	})

	addStrandIn(t, worktree, smokeReapLaunchCmd(), "--name", "cold-substrate")

	socket := reedengine.ServerName(h.Path)
	session := reedengine.SessionName(worktree)
	panes := listPaneLines(t, tmuxPath, socket, session)
	if len(panes) != 2 {
		t.Fatalf("panes after a cold add = %v; want 2 (the always-present header pane plus the strand's own pane) -- a shortfall means the delegate path only spawned a server rather than running the whole boot body", panes)
	}
}

// TestSmokeColdAttachBootsThenFailsOnTheTerminalHandoverNotOnNoSession pins the other self-healing
// verb: on a worktree never brought up, `attach` run without a controlling terminal must fail with
// the multiplexer's own terminal error, never with the friendly no-session JSON envelope, and the
// session must exist on the socket afterwards either way. The distinction between those two failure
// modes is the whole assertion -- a no-session refusal would mean attach never reached the boot at
// all, while the terminal error is only reachable once the boot has already succeeded and the
// handover tail took over stdio.
func TestSmokeColdAttachBootsThenFailsOnTheTerminalHandoverNotOnNoSession(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)
	lyxExe := buildLyxBinary(t)

	h := hubforge.NewHub(t, ".")
	worktree := h.PrimeWorktree()
	deferHubRelease(t, worktree)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(worktree, &buf, []string{"down"})
	})

	output := runReedCLINoFatal(t, lyxExe, worktree, 30*time.Second, "reed", "attach")

	if strings.Contains(output, "no reed session") {
		t.Fatalf("cold attach output = %q; want no trace of the no-session refusal -- attach must boot before it can ever reach that check", output)
	}
	if !strings.Contains(strings.ToLower(output), "not a terminal") {
		t.Errorf("cold attach output = %q; want the multiplexer's own terminal-handover error, proving the boot succeeded and the handover itself was reached", output)
	}

	socket := reedengine.ServerName(h.Path)
	session := reedengine.SessionName(worktree)
	if !sessionAlive(tmuxPath, socket, session) {
		t.Errorf("session %s not alive on socket %s after a cold attach; want the boot to have deposited it even though the handover then failed", session, socket)
	}
}

// TestSmokeColdAddWithIfAbsentRelaunchesOnlyTheMatchedStrand pins the observable consequence of up
// semantics rather than resume semantics on the --if-absent path: with two strands persisted and the
// server dead, a cold `add --if-absent` naming one of them must boot and relaunch exactly that
// strand -- one pane for it, not two entries under one name -- and must leave the other strand
// unrelaunched, since a cold boot (unlike resume) never replays the whole persisted table.
func TestSmokeColdAddWithIfAbsentRelaunchesOnlyTheMatchedStrand(t *testing.T) {
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
}

// TestSmokeColdAddOnAnUnreadableStateFileBootsThenFailsWithTheResidueAccepted pins the Shared
// Decision that corrupt `reed.json` boots the session first and then fails with the state loader's
// corrupt-file diagnosis, leaving a bare session behind: on a worktree never brought up whose
// .lyx/reed.json holds truncated bytes, `add` exits non-zero naming the file and the teardown verb,
// AND a session now exists on the socket. Both halves are the assertion. The residue is the
// deliberately accepted cost of matching `up`'s own behaviour -- a later change to that posture must
// fail this test loudly instead of passing silently.
func TestSmokeColdAddOnAnUnreadableStateFileBootsThenFailsWithTheResidueAccepted(t *testing.T) {
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	worktree := h.PrimeWorktree()
	deferHubRelease(t, worktree)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(worktree, &buf, []string{"down"})
	})

	statePath := filepath.Join(worktree, ".lyx", "reed.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(statePath), err)
	}
	// A truncated write is the ordinary way this file becomes unreadable: a crash, a kill -9, a full
	// disk, or a power loss partway through a save.
	if err := os.WriteFile(statePath, []byte(`{"socket":"lyx-x","strands":[{"gu`), 0o600); err != nil {
		t.Fatalf("write %s: %v", statePath, err)
	}

	var out bytes.Buffer
	if code := RunCLIIn(worktree, &out, []string{"add", "--cmd", smokeReapLaunchCmd()}); code == 0 {
		t.Fatalf("cold add with an unreadable reed.json = 0; want a failure, output: %s", out.String())
	}
	unreadable := envelopeError(t, out.Bytes())
	for _, want := range []string{"unreadable", "reed.json", "lyx reed down"} {
		if !strings.Contains(unreadable, want) {
			t.Errorf("cold add error = %s; want it to contain %q (the state loader's corrupt-file diagnosis, naming the file and the teardown verb)", unreadable, want)
		}
	}

	// Deliberately accepted residue: add boots the session before ever loading state, matching `up`'s
	// own behaviour, so a corrupt reed.json leaves a bare session behind rather than refusing
	// pre-boot.
	socket := reedengine.ServerName(h.Path)
	session := reedengine.SessionName(worktree)
	if !sessionAlive(tmuxPath, socket, session) {
		t.Errorf("session %s not alive on socket %s after a cold add on an unreadable state file; want the boot's residue left behind, matching up's own accepted behaviour", session, socket)
	}
}
