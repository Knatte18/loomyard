//go:build smoke

package muxcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/muxengine"
)

// TestSmokeCrashRecovery covers the discussion's "server dead (reboot)"
// recovery state end-to-end against a live tmux server: after the server is
// killed out from under mux, `up` must reboot the substrate and reconcile the
// strand to not-live (its stale pane binding cleared, not mistaken for the
// reborn session's reused initial pane id), and `resume` must then rebuild the
// strand into a fresh live pane. This is the path the pane-id-collision fix
// (clearAllPaneBindings on a booted session) exists for; the single-pane
// TestSmokeUpAddStatusDown above never reaches it.
func TestSmokeCrashRecovery(t *testing.T) {
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

	// up + add one strand.
	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	out.Reset()
	if code := RunCLI(&out, []string{"add", "--cmd", "pwsh -NoExit -Command Write-Host ready"}); code != 0 {
		t.Fatalf("add = %d; want 0, output: %s", code, out.String())
	}
	var addResult map[string]any
	if err := json.Unmarshal(out.Bytes(), &addResult); err != nil {
		t.Fatalf("parse add result: %v", err)
	}
	guid, _ := addResult["guid"].(string)
	if guid == "" {
		t.Fatalf("add result missing guid: %v", addResult)
	}

	// Read the socket so we can kill the server directly (simulating a crash).
	out.Reset()
	if code := RunCLI(&out, []string{"status"}); code != 0 {
		t.Fatalf("status = %d; want 0, output: %s", code, out.String())
	}
	var statusResult map[string]any
	if err := json.Unmarshal(out.Bytes(), &statusResult); err != nil {
		t.Fatalf("parse status result: %v", err)
	}
	socket, _ := statusResult["socket"].(string)
	session, _ := statusResult["session"].(string)
	if socket == "" || session == "" {
		t.Fatalf("status result missing socket/session: %v", statusResult)
	}

	// readStrand runs `status` fresh and returns this test's strand record
	// plus the raw JSON, so a failing assertion can print what status saw.
	readStrand := func() (map[string]any, []byte, bool) {
		var buf bytes.Buffer
		if code := RunCLI(&buf, []string{"status"}); code != 0 {
			t.Fatalf("status = %d; want 0, output: %s", code, buf.String())
		}
		raw := append([]byte(nil), buf.Bytes()...)
		strand, ok := statusStrand(t, raw, guid)
		return strand, raw, ok
	}

	// Simulate a psmux crash: kill the whole server out from under mux.
	if err := exec.Command(psmuxPath, "-L", socket, "kill-server").Run(); err != nil {
		t.Fatalf("kill-server: %v", err)
	}
	// kill-server returns before the server has fully released its socket. If
	// we called up while the dying server still answered has-session, mux
	// would treat the session as still up (booted=false), skip the stale-
	// binding clear, and the reused pane id would read falsely live — a race
	// that only surfaces on a loaded machine. A real crash is a dead process,
	// so wait until the server is genuinely gone before simulating recovery.
	waitServerGone(t, psmuxPath, socket, session)

	// up after the crash: reboots the substrate and clears the stale binding
	// (the reborn session's initial pane reuses the old pane id, so without
	// the booted-session binding-clear the strand would look falsely live).
	out.Reset()
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("post-crash up = %d; want 0, output: %s", code, out.String())
	}
	strand, statusRaw, found := readStrand()
	if !found {
		t.Fatalf("strand %s missing after post-crash up; status: %s", guid, statusRaw)
	}
	if live, _ := strand["live"].(bool); live {
		t.Errorf("post-crash up: strand %s live = true; want false (stale binding must be cleared); status: %s", guid, statusRaw)
	}

	// resume: rebuilds the strand into a fresh live pane.
	out.Reset()
	if code := RunCLI(&out, []string{"resume"}); code != 0 {
		t.Fatalf("resume = %d; want 0, output: %s", code, out.String())
	}
	var resumeResult map[string]any
	if err := json.Unmarshal(out.Bytes(), &resumeResult); err != nil {
		t.Fatalf("parse resume result: %v", err)
	}
	if resumed, _ := resumeResult["resumed"].(float64); resumed < 1 {
		t.Errorf("resume resumed = %v; want >= 1 (the crashed strand must be rebuilt)", resumeResult["resumed"])
	}
	strand, statusRaw, found = readStrand()
	if !found {
		t.Fatalf("strand %s missing after resume; status: %s", guid, statusRaw)
	}
	if live, _ := strand["live"].(bool); !live {
		t.Errorf("after resume: strand %s live = false; want true; status: %s", guid, statusRaw)
	}
}

// TestSmokeClaudeResumeRecallsCodeword is the end-to-end proof of mux's one
// Claude-adjacent responsibility: env hygiene on the server spawn (without
// it, a claude launched from inside a Claude Code session treats itself as
// a nested child and silently stops persisting its transcript) plus the
// opaque resumeCmd replay. It launches a real claude in a strand with a
// codeword prompt, kills the whole psmux server out from under it, resumes
// via the stored `claude --continue`, and asserts the codeword comes back —
// which is only possible if the transcript was persisted and found again.
// Needs a logged-in claude CLI; runs a real subscription session (~1-3 min).
func TestSmokeClaudeResumeRecallsCodeword(t *testing.T) {
	psmuxPath := psmuxBinaryPath(t)
	claudePath := claudeBinaryPath(t)

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

	codeword := fmt.Sprintf("zebra-%d", time.Now().UnixNano()%1000000)
	launch := fmt.Sprintf(`& '%s' 'Remember the codeword %s. Reply with exactly: STORED %s'`, claudePath, codeword, codeword)
	resume := fmt.Sprintf(`& '%s' --continue`, claudePath)

	// Scope the transcript watch to THIS test's claude project directory
	// (derived from the fixture hub — the pane's cwd) and snapshot what is
	// already in it BEFORE the launch, so phase 1 can only ever match the one
	// transcript this test's own claude produces — never a concurrent sibling
	// suite's (each suite has a unique temp hub, hence a unique project dir).
	projectDir := claudeProjectDir(t, fixture.Hub)
	transcriptsBefore := claudeTranscriptFiles(t, projectDir)
	guid := addStrand(t, launch, "--resume-cmd", resume, "--name", "agent")
	socket, session := socketAndSession(t)

	readPane := func() string {
		t.Helper()
		var buf bytes.Buffer
		if code := RunCLI(&buf, []string{"status"}); code != 0 {
			t.Fatalf("status = %d; want 0, output: %s", code, buf.String())
		}
		strand, ok := statusStrand(t, buf.Bytes(), guid)
		if !ok {
			t.Fatalf("status missing strand %s: %s", guid, buf.String())
		}
		paneID, _ := strand["paneId"].(string)
		if paneID == "" {
			t.Fatalf("strand %s has no pane: %s", guid, buf.String())
		}
		return paneID
	}

	// dismissTrust answers claude's one-time "do you trust this folder?"
	// gate (Enter = its default "yes") whenever that screen is visible. A
	// fresh fixture dir triggers it; it is operator setup, not the contract
	// under test. Called on every poll iteration (not once) because a single
	// early Enter can land before the prompt is interactive and be dropped.
	dismissTrust := func(paneID string) {
		content := capturePane(t, psmuxPath, socket, paneID)
		if strings.Contains(content, "trust") && strings.Contains(content, "folder") {
			_ = exec.Command(psmuxPath, "-L", socket, "send-keys", "-t", paneID, "Enter").Run()
		}
	}

	// Phase 1: let claude receive the codeword and PERSIST a transcript
	// before the crash. The persistence gate is the transcript file itself,
	// not a TUI idle marker: claude's "? for shortcuts" hint is on screen
	// even while it is still starting/thinking, so a marker-based wait can
	// fire before the first transcript flush and the crash then truncates
	// before anything reaches disk (which is exactly what a "No conversation
	// found" resume looks like — a test artifact, not a mux defect). Waiting
	// for the .jsonl to appear and stop growing is the direct proof that env
	// hygiene let claude persist — the whole point of this test.
	paneID := readPane()
	transcript := waitTranscriptStable(t, projectDir, transcriptsBefore, dismissTrust, paneID, 180*time.Second)
	t.Logf("phase 1 transcript persisted: %s", transcript)

	// Phase 2: crash the whole server, then resume. The stored resumeCmd is
	// `claude --continue`, which reopens the most recent conversation for
	// this directory — it only finds one because the transcript above
	// persisted.
	if err := exec.Command(psmuxPath, "-L", socket, "kill-server").Run(); err != nil {
		t.Fatalf("kill-server: %v", err)
	}
	waitServerGone(t, psmuxPath, socket, session)

	out.Reset()
	if code := RunCLI(&out, []string{"resume"}); code != 0 {
		t.Fatalf("resume = %d; want 0, output: %s", code, out.String())
	}
	var resumeResult map[string]any
	if err := json.Unmarshal(out.Bytes(), &resumeResult); err != nil {
		t.Fatalf("parse resume result: %v", err)
	}
	if resumed, _ := resumeResult["resumed"].(float64); resumed != 1 {
		t.Fatalf("resumed = %v; want 1", resumeResult["resumed"])
	}

	// Phase 3: the codeword must come back in the RESUMED pane. The resume
	// command line is `claude --continue` — it carries NO codeword, so the
	// codeword appearing here can only come from the persisted transcript
	// being reloaded (the whole point). Match the codeword token alone: it
	// has no internal spaces, so it survives capture-pane's space-stripping
	// of claude's rendered response boxes. `--continue` re-renders the prior
	// turn, so the codeword typically returns on its own; if a future TUI
	// hides history, ask for it explicitly (the question carries no codeword,
	// so it cannot false-match).
	resumedPane := readPane()
	dismissTrust(resumedPane)
	if paneEventuallyContains(t, psmuxPath, socket, resumedPane, codeword, 30*time.Second) {
		return
	}
	sendKeysLine(t, psmuxPath, socket, resumedPane, "What was the codeword I gave you? Reply with only that word.")
	pollPaneContains(t, psmuxPath, socket, resumedPane, codeword, 120*time.Second)
}
