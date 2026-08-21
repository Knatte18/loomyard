//go:build smoke

// smoke_test.go walks the live-only webster behaviors the hermetic tests can
// never exercise, against the REAL substrate (a real logged-in `claude`): the
// fork-context guard that deterministically closes the fork-loop deadlock
// (round opus-r2's R2-a — a fork's `lyx webster` call is refused while
// Master's own passes), a real Agent-tool fork's transcript audit (round
// fable-r1's F2 — exactly one transcript, and the parent's own spawn replay is
// NOT miscounted as a nested-Agent violation), and the await-batch poll loop
// reaching a fork-written report (round fable-r1's F1 — forks are backgrounded,
// so Master long-polls await-batch until the report lands). Every test
// self-skips when its substrate is absent, and every substrate wait polls on a
// deadline — never a fixed sleep — since substrate state transitions are
// asynchronous by contract.

package webstercli

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// smokeClaudeBin returns the claude binary path, skipping the test if not on PATH.
func smokeClaudeBin(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("claude not found on PATH; skipping live webster smoke test")
	}
	return path
}

// realForkSettingsPath composes the production settings.json a Master spawn would use via claudeengine.Prepare.
func realForkSettingsPath(t *testing.T) string {
	t.Helper()
	runDir := t.TempDir()
	spec := shuttleengine.Spec{Prompt: "smoke", Interactive: false, ForkSubagents: true}
	cfg := shuttleengine.Config{ClaudeDenyAgentTool: true, ClaudeDenyAskUserQuestion: true}
	if _, err := claudeengine.New().Prepare(runDir, spec, cfg); err != nil {
		t.Fatalf("Prepare fork-mode settings: %v", err)
	}
	return filepath.Join(runDir, "settings.json")
}

// bashGuardCommand extracts the PreToolUse(Bash) hook command string from a settings.json document.
func bashGuardCommand(t *testing.T, settingsPath string) string {
	t.Helper()
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var doc struct {
		Hooks struct {
			PreToolUse []struct {
				Matcher string `json:"matcher"`
				Hooks   []struct {
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"PreToolUse"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal settings.json: %v", err)
	}
	for _, e := range doc.Hooks.PreToolUse {
		if e.Matcher == "Bash" && len(e.Hooks) > 0 {
			return e.Hooks[0].Command
		}
	}
	t.Fatalf("no PreToolUse(Bash) guard hook in %s: %s", settingsPath, data)
	return ""
}

// runHook runs the guard's shell command with payload on stdin and returns its stdout.
func runHook(t *testing.T, command, payload string) string {
	t.Helper()
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = strings.NewReader(payload)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("guard hook exited non-zero (want 0 always): %v; stdout=%q", err, out)
	}
	return string(out)
}

func TestSmoke_ForkGuardHookDeniesForkPayload(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not found on PATH; skipping guard payload-replay")
	}
	command := bashGuardCommand(t, realForkSettingsPath(t))

	const forkWebster = `{"agent_id":"a055698cb6ea49469","agent_type":"fork","tool_name":"Bash","tool_input":{"command":"lyx webster await-batch 1"}}`
	const parentWebster = `{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"lyx webster await-batch 1"}}`
	const forkNonWebster = `{"agent_id":"a055698cb6ea49469","agent_type":"fork","tool_name":"Bash","tool_input":{"command":"git commit -m done"}}`

	tests := []struct {
		name       string
		payload    string
		wantDenied bool
	}{
		{"fork_webster_denied", forkWebster, true},
		{"parent_webster_allowed", parentWebster, false},
		{"fork_non_webster_allowed", forkNonWebster, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := runHook(t, command, tt.payload)
			denied := strings.Contains(out, `"permissionDecision":"deny"`)
			if denied != tt.wantDenied {
				t.Errorf("guard hook denied = %v; want %v (stdout=%q)", denied, tt.wantDenied, out)
			}
		})
	}
}

// runClaudeFork launches a real headless claude session with fork-subagent capability enabled.
func runClaudeFork(t *testing.T, ctx context.Context, bin, dir, settingsPath, sessionID, prompt string) {
	t.Helper()
	cmd := exec.CommandContext(ctx, bin, "-p",
		"--dangerously-skip-permissions",
		"--settings", settingsPath,
		"--session-id", sessionID,
		prompt,
	)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CLAUDE_CODE_FORK_SUBAGENT=1")
	out, err := cmd.CombinedOutput()
	if err != nil && ctx.Err() != nil {
		t.Fatalf("claude fork run timed out: %v; output tail: %s", err, tailBytes(out, 800))
	}
	if err != nil {
		t.Fatalf("claude fork run failed: %v; output tail: %s", err, tailBytes(out, 800))
	}
}

// tailBytes returns the last n bytes of b as a string.
func tailBytes(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[len(b)-n:])
}

// pollExists reports whether path comes to exist before the deadline.
func pollExists(path string, deadline time.Time) bool {
	for {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// mintSessionID returns a fresh random UUID-v4 string for a claude --session-id.
func mintSessionID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("mint session id: %v", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC-4122 variant
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func TestSmoke_ForkContextGuardDeniesLiveFork(t *testing.T) {
	bin := smokeClaudeBin(t)
	settingsPath := realForkSettingsPath(t)
	dir := t.TempDir()
	forkControl := filepath.Join(dir, "fork_control")
	forkWebster := filepath.Join(dir, "fork_webster_ran")
	parentWebster := filepath.Join(dir, "parent_webster_ran")

	prompt := "Spawn exactly one Agent-tool subagent with subagent_type set to \"fork\" and NO name. " +
		"Instruct the fork to run these two Bash commands: " +
		"first `touch " + forkControl + "`, " +
		"then `lyx webster await-batch 1 ; touch " + forkWebster + "`. " +
		"After the fork returns, you (the parent) run one Bash command: " +
		"`lyx webster await-batch 1 ; touch " + parentWebster + "`. Then stop."

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	runClaudeFork(t, ctx, bin, dir, settingsPath, mintSessionID(t), prompt)

	deadline := time.Now().Add(20 * time.Second)
	if !pollExists(forkControl, deadline) {
		t.Fatal("fork control sentinel never appeared — the fork did not run its non-webster command, so the guard result is inconclusive")
	}
	if !pollExists(parentWebster, deadline) {
		t.Error("parent webster sentinel never appeared — Master's OWN lyx webster call was blocked; the guard must let the parent through")
	}
	if _, err := os.Stat(forkWebster); err == nil {
		t.Error("fork webster sentinel present — a fork's lyx webster call was NOT denied; the fork-context guard failed")
	}
}

func TestSmoke_ForkTranscriptAuditCountsOneNoNestedAgent(t *testing.T) {
	bin := smokeClaudeBin(t)
	settingsPath := realForkSettingsPath(t)
	dir := t.TempDir()
	sessionID := mintSessionID(t)
	done := filepath.Join(dir, "fork_done")

	prompt := "Spawn exactly one Agent-tool subagent with subagent_type set to \"fork\" and NO name. " +
		"Instruct the fork to run exactly one Bash command: `touch " + done + "`. " +
		"After the fork returns, stop."

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	runClaudeFork(t, ctx, bin, dir, settingsPath, sessionID, prompt)

	eng := claudeengine.New()
	deadline := time.Now().Add(30 * time.Second)
	var audit shuttleengine.ForkAudit
	for {
		var err error
		audit, err = eng.AuditForks(sessionID, dir)
		if err != nil {
			t.Fatalf("AuditForks: %v", err)
		}
		if len(audit.Forks) >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no fork transcript audited within deadline (session %s, dir %s); the fork never ran or never flushed", sessionID, dir)
		}
		time.Sleep(250 * time.Millisecond)
	}

	if len(audit.Forks) != 1 {
		t.Errorf("audited %d fork transcripts; want exactly 1", len(audit.Forks))
	}
	if audit.Forks[0].AgentCalls != 0 {
		t.Errorf("fork AgentCalls = %d; want 0 — the parent's own spawn replay must not be miscounted as a nested Agent call (F2)", audit.Forks[0].AgentCalls)
	}
}

// smokeGitRepo initializes dir as a git repo with one base commit and returns that commit's SHA.
func smokeGitRepo(t *testing.T, dir string) string {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "smoke@test"},
		{"config", "user.name", "smoke"},
		{"commit", "-q", "--allow-empty", "-m", "base"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v; output: %s", args, err, out)
		}
	}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func TestSmoke_RecordBatchConsumesCrashedSessionReport(t *testing.T) {
	bin := smokeClaudeBin(t)
	settingsPath := realForkSettingsPath(t)
	dir := t.TempDir()
	startSHA := smokeGitRepo(t, dir)
	reportsDir := filepath.Join(dir, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatalf("mkdir reports dir: %v", err)
	}
	reportPath := filepath.Join(reportsDir, websterengine.ReportFileName(1, "alpha"))
	crashedSession := mintSessionID(t)

	prompt := "Spawn exactly one Agent-tool subagent with subagent_type set to \"fork\" and NO name. " +
		"Instruct the fork to create the file " + reportPath + " with exactly this content and nothing else:\n" +
		"status: OK\nhead_sha: " + startSHA + "\n" +
		"After the fork returns, stop."

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	runClaudeFork(t, ctx, bin, dir, settingsPath, crashedSession, prompt)

	deadline := time.Now().Add(30 * time.Second)
	if !pollExists(reportPath, deadline) {
		t.Fatalf("the fork never wrote %s", reportPath)
	}
	eng := claudeengine.New()
	for {
		audit, err := eng.AuditForks(crashedSession, dir)
		if err != nil {
			t.Fatalf("AuditForks: %v", err)
		}
		if len(audit.Forks) >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no fork transcript flushed for session %s", crashedSession)
		}
		time.Sleep(250 * time.Millisecond)
	}

	state := &websterengine.State{
		MasterSessionID: mintSessionID(t),
		CurrentBatch:    1,
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "alpha", StartSHA: startSHA, Kind: "fork", SessionID: crashedSession},
		},
	}
	// Only WorktreeRoot and ReportsDir are read on this path (the head-SHA capture, the
	// dirty-worktree check, the fork audit's workdir, and the batch-report join); the remaining
	// Geometry fields stay zero rather than being invented, so a future read of one fails loudly
	// here instead of silently resolving against a plausible-looking temp path.
	// NeverMatches is the pinned supplier for a mode with no fabric repo -- this fixture is a bare
	// git repo, never a wired hub -- and the field must be non-nil either way, since CheckParent
	// and CheckFork call Matches unguarded.
	deps := websterengine.RecordDeps{
		Batches:     []batcher.Batch{{Cards: []planparser.Card{{Number: 1, Slug: "alpha"}}}},
		State:       state,
		Engine:      eng,
		Geom:        websterengine.Geometry{WorktreeRoot: dir, ReportsDir: reportsDir},
		RefMatcher:  websterengine.NeverMatches{},
		OutcomePath: filepath.Join(dir, "outcome.yaml"),
		SummaryPath: filepath.Join(dir, "summary.md"),
		Sleeper:     realSleeper{},
	}

	result, err := websterengine.RecordBatch(deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want the crashed session's report consumed", err)
	}
	if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
		t.Fatalf("RecordBatch() digest = %+v; want a terminal done digest", result.Digest)
	}
	if !state.Batches[1].Terminal {
		t.Error("batch 1 not marked Terminal after the late record")
	}
}

func TestSmoke_AwaitBatchSeesForkWrittenReport(t *testing.T) {
	bin := smokeClaudeBin(t)
	settingsPath := realForkSettingsPath(t)
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatalf("mkdir reports dir: %v", err)
	}
	reportPath := filepath.Join(reportsDir, websterengine.ReportFileName(1, "alpha"))

	prompt := "Spawn exactly one Agent-tool subagent with subagent_type set to \"fork\" and NO name. " +
		"Instruct the fork to create the file " + reportPath + " with exactly this content and nothing else:\n" +
		"status: OK\nhead_sha: deadbeefdeadbeefdeadbeefdeadbeefdeadbeef\n" +
		"After the fork returns, stop."

	batches := []batcher.Batch{{Cards: []planparser.Card{{Number: 1, Slug: "alpha"}}}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	runErrCh := make(chan struct{})
	go func() {
		defer close(runErrCh)
		runClaudeFork(t, ctx, bin, dir, settingsPath, mintSessionID(t), prompt)
	}()

	result, err := websterengine.AwaitBatch(batches, reportsDir, 1, 5*time.Minute, recoverRealClock{})
	if err != nil {
		t.Fatalf("AwaitBatch: %v", err)
	}
	if !result.ReportPresent {
		t.Fatalf("AwaitBatch returned ReportPresent=false after %ds; the fork never wrote %s", result.ElapsedS, reportPath)
	}
	<-runErrCh
}
