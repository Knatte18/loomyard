package reedcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/llmkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// smokeClaudeModel is the model every real `claude` process this package spawns must run on.
// The suite's assertions concern reed's env hygiene and resumeCmd replay, not model capability,
// so the cheapest model is always the right one; leaving it unpinned bills the operator's default model on every run.
const smokeClaudeModel = "haiku"

// claudeProjectDir returns the ~/.claude/projects/<encoded-cwd> directory for cwd dir.
func claudeProjectDir(t *testing.T, dir string) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("resolve home dir: %v", err)
	}
	encoded := []byte(dir)
	for i, c := range encoded {
		isAlnum := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if !isAlnum {
			encoded[i] = '-'
		}
	}
	return filepath.Join(home, ".claude", "projects", string(encoded))
}

// claudeTranscriptFiles returns the set of every *.jsonl transcript path under projectDir.
func claudeTranscriptFiles(t *testing.T, projectDir string) map[string]bool {
	t.Helper()
	found := map[string]bool{}
	_ = filepath.WalkDir(projectDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".jsonl") {
			found[path] = true
		}
		return nil
	})
	return found
}

// waitTranscriptStable blocks until a new transcript appears in projectDir and stops growing.
func waitTranscriptStable(t *testing.T, projectDir string, before map[string]bool, dismissTrust func(paneID string), paneID string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	sizes := map[string]int64{}
	for {
		dismissTrust(paneID)

		for path := range claudeTranscriptFiles(t, projectDir) {
			if before[path] {
				continue // pre-existing — not this test's transcript
			}
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			prev, seen := sizes[path]
			if seen && prev > 0 && info.Size() == prev {
				return path
			}
			sizes[path] = info.Size()
		}

		if time.Now().After(deadline) {
			t.Fatalf("no new claude transcript persisted+stabilized within %s (env hygiene may be broken — claude in a nested Claude Code session stops writing transcripts)", timeout)
		}
		time.Sleep(2 * time.Second)
	}
}

// TestSmokeCrashRecovery covers crash recovery with server reboot.
func TestSmokeCrashRecovery(t *testing.T) {
	tiergate.Require(t, 3)
	tmuxPath := tmuxBinaryPath(t)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	deferHubRelease(t, h.PrimeWorktree())
	tmuxkit.KillOnCleanup(t, tmuxPath, reedengine.ServerName(h.Path))
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

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
	tmuxkit.KillOnCleanup(t, tmuxPath, socket)

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

	// Simulate a tmux crash: kill the whole server out from under reed.
	if err := exec.Command(tmuxPath, "-L", socket, "kill-server").Run(); err != nil {
		t.Fatalf("kill-server: %v", err)
	}
	// kill-server returns before the server has fully released its socket.
	// Calling up while the dying server still answers has-session makes reed treat the session as up;
	// it then skips the stale-binding clear and reads the reused pane id as falsely live.
	// That race only surfaces on a loaded machine.
	// A real crash is a dead process, so wait until the server is genuinely gone before simulating recovery.
	waitServerGone(t, tmuxPath, socket, session)

	// up after the crash: reboots the substrate and clears the stale binding.
	// The reborn session's initial pane reuses the old pane id.
	// Without the booted-session binding-clear, the strand would look falsely live.
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

// TestSmokeClaudeResumeRecallsCodeword is the end-to-end proof of reed's Claude-adjacent duty:
// env hygiene on the server spawn, plus the opaque resumeCmd replay.
// Without it, a claude launched inside a Claude Code session treats itself as a nested child,
// so it silently stops persisting its transcript.
// The test launches a real claude in a strand with a codeword prompt.
// It kills the whole tmux server out from under it.
// It then resumes via the stored `claude --continue` and asserts the codeword comes back.
// That is possible only if the transcript was persisted and found again.
// It needs a logged-in claude CLI and runs a real subscription session, which takes minutes.
func TestSmokeClaudeResumeRecallsCodeword(t *testing.T) {
	tiergate.Require(t, 3)
	tmuxPath := tmuxBinaryPath(t)
	claudePath := llmkit.Claude(t, "LYM_REED_CLAUDE")

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	deferHubRelease(t, h.PrimeWorktree())
	tmuxkit.KillOnCleanup(t, tmuxPath, reedengine.ServerName(h.Path))
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}

	codeword := fmt.Sprintf("zebra-%d", time.Now().UnixNano()%1000000)
	prompt := fmt.Sprintf("Remember the codeword %s. Reply with exactly: STORED %s", codeword, codeword)
	// --model is pinned to the cheapest model on both lines, launch and resume.
	// Nothing this test asserts is model-specific.
	// It checks that a transcript persisted and that `--continue` found it again.
	// That is reed's env-hygiene and opaque-replay contract, not a model capability.
	// A bare `-run Smoke` sweep matches this test's name.
	// Without the pin, it would start a real subscription session on the operator's default model.
	launch := smokeInvokeLine(claudePath, "--model", smokeClaudeModel, prompt)
	resume := smokeInvokeLine(claudePath, "--continue", "--model", smokeClaudeModel)

	// Scope the transcript watch to this test's claude project directory, derived from the fixture hub (the pane's cwd).
	// Snapshot what is already there before the launch.
	// Phase 1 then matches only the transcript this test's own claude produces.
	// Each suite has a unique temp hub, hence a unique project dir,
	// so a concurrent sibling suite's transcript is never matched.
	projectDir := claudeProjectDir(t, h.PrimeWorktree())
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

	// dismissTrust answers claude's one-time "do you trust this folder?" gate whenever that screen is visible.
	// It sends Enter, which selects claude's default "yes".
	// A fresh fixture dir triggers the gate; it is operator setup, not the contract under test.
	// The caller invokes it on every poll iteration.
	// A single early Enter can land before the prompt is interactive and be dropped, so a one-time call is not enough.
	dismissTrust := func(paneID string) {
		content := capturePane(t, tmuxPath, socket, paneID)
		if strings.Contains(content, "trust") && strings.Contains(content, "folder") {
			_ = exec.Command(tmuxPath, "-L", socket, "send-keys", "-t", paneID, "Enter").Run()
		}
	}

	// Phase 1: let claude receive the codeword and persist a transcript before the crash.
	// The persistence gate is the transcript file itself, not a TUI idle marker.
	// The "? for shortcuts" hint is on screen even while claude is still starting or thinking,
	// so a marker-based wait can fire before the first transcript flush.
	// The crash would then truncate the session before anything reaches disk.
	// A resume then reports "No conversation found", which is a test artifact, not a reed defect.
	// Waiting for the .jsonl to appear and stop growing is the direct proof that env hygiene let claude persist.
	paneID := readPane()
	transcript := waitTranscriptStable(t, projectDir, transcriptsBefore, dismissTrust, paneID, 180*time.Second)
	t.Logf("phase 1 transcript persisted: %s", transcript)

	// Phase 2: crash the whole server, then resume. The stored resumeCmd is
	// `claude --continue`, which reopens the most recent conversation for
	// this directory — it only finds one because the transcript above
	// persisted.
	if err := exec.Command(tmuxPath, "-L", socket, "kill-server").Run(); err != nil {
		t.Fatalf("kill-server: %v", err)
	}
	waitServerGone(t, tmuxPath, socket, session)

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

	// Phase 3: the codeword must come back in the resumed pane.
	// The resume command line is `claude --continue`, which carries no codeword,
	// so the codeword can only come from the persisted transcript being reloaded.
	// Match the codeword token alone: it has no internal spaces,
	// so it survives capture-pane's space-stripping of claude's rendered response boxes.
	// `--continue` re-renders the prior turn, so the codeword typically returns on its own.
	// If a future TUI hides history, ask for it explicitly; the question carries no codeword, so it cannot false-match.
	resumedPane := readPane()
	dismissTrust(resumedPane)
	if paneEventuallyContains(t, tmuxPath, socket, resumedPane, codeword, 30*time.Second) {
		return
	}
	sendKeysLine(t, tmuxPath, socket, resumedPane, "What was the codeword I gave you? Reply with only that word.")
	pollPaneContains(t, tmuxPath, socket, resumedPane, codeword, 120*time.Second)
}
