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

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// smokeClaudeBin returns the claude binary path, skipping the calling test
// when it is not on PATH — the same self-skip discipline builder's own
// smoke_test.go and muxengine's integration tests apply.
func smokeClaudeBin(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("claude not found on PATH; skipping live webster smoke test")
	}
	return path
}

// realForkSettingsPath composes the PRODUCTION settings.json a webster Master
// spawn would run under (ForkSubagents true, the agent-deny on) via the real
// claudeengine.Prepare, and returns the path to it — so the guard tests
// exercise exactly the hook document production emits, never a hand-built
// approximation.
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

// bashGuardCommand extracts the PreToolUse(Bash) hook command string from a
// settings.json document — the fork-context webster-verb guard's own shell
// command, so the payload-replay test drives the exact production artifact.
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

// runHook runs the guard's own shell command with payload on stdin and returns
// its stdout — the mechanical deny channel (a non-empty deny JSON denies the
// tool call; empty stdout allows it). It asserts the hook always exits 0, the
// invariant that keeps a non-deny path from surfacing as a spurious hook error.
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

// TestSmoke_ForkGuardHookDeniesForkPayload drives the PRODUCTION guard hook
// command against the exact payload shapes Claude Code 2.1.205 emits (captured
// live: a fork's PreToolUse payload carries a top-level agent_id; a top-level
// Master call does not). It is the deterministic regression anchor for R2-a —
// no LLM in the loop, so it never flakes — proving the two AND-ed predicates:
// deny only when fork-context AND a lyx webster command, allow otherwise.
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

// runClaudeFork launches a real headless claude session in dir under
// settingsPath with the fork-subagent capability enabled, blocking until it
// finishes or ctx expires. sessionID pins the transcript identity so a caller
// can locate the on-disk transcripts afterward. It is the shared substrate
// driver for the three live tests below.
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

// tailBytes returns the last n bytes of b as a string, for compact failure
// diagnostics without dumping a whole session's output.
func tailBytes(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[len(b)-n:])
}

// pollExists reports whether path comes to exist before the deadline, checking
// on a fixed short tick — the deterministic substrate wait (never a fixed
// sleep) the whole file uses.
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

// mintSessionID returns a fresh random UUID-v4 string for a claude
// --session-id, so parallel runs never collide on the same project-dir
// transcript set. crypto/rand keeps it collision-free without a shared seed.
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

// TestSmoke_ForkContextGuardDeniesLiveFork proves the guard end-to-end against
// a REAL Agent-tool fork (R2-a): the fork's own `lyx webster` call is refused
// by the PreToolUse hook (its sentinel never lands) while a non-webster fork
// command runs (its control sentinel lands) and the parent's own `lyx webster`
// call passes the guard (its sentinel lands). This is the true substrate walk
// behind TestSmoke_ForkGuardHookDeniesForkPayload's deterministic anchor.
func TestSmoke_ForkContextGuardDeniesLiveFork(t *testing.T) {
	bin := smokeClaudeBin(t)
	settingsPath := realForkSettingsPath(t)
	dir := t.TempDir()
	forkControl := filepath.Join(dir, "fork_control")
	forkWebster := filepath.Join(dir, "fork_webster_ran")
	parentWebster := filepath.Join(dir, "parent_webster_ran")

	// The `;` (not `&&`) after each lyx webster call is deliberate: an ALLOWED
	// call that merely errors still lets the trailing touch run, so a present
	// sentinel unambiguously means "the guard let the call through", and an
	// absent one means "the PreToolUse hook blocked the whole tool call".
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

// TestSmoke_ForkTranscriptAuditCountsOneNoNestedAgent proves a real fork's
// transcript audit (F2): a single in-session fork yields exactly one fork
// transcript, and the parent's own spawning Agent tool_use — replayed as the
// fork transcript's inherited-context boundary line — is NOT miscounted as the
// fork nesting an Agent call (the false ClassNestedAgent that wedged every
// record-batch before F2).
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

	// The fork's transcript flush is asynchronous from the run's return; poll
	// the audit on a deadline until exactly one fork transcript is present.
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

// smokeGitRepo initializes dir as a git repo with one base commit and returns
// that commit's SHA — the StartSHA a RecordBatch drift computation diffs
// against. Identity is set repo-locally so the hermetic git env needs no
// global config.
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

// TestSmoke_RecordBatchConsumesCrashedSessionReport proves the crash/resume
// seam fixed in round fable-r3 (FR3-2) against the REAL transcript layout: a
// fork spawned under one session writes its batch report; RecordBatch is then
// driven with a DIFFERENT current Master session (the resumed run's fresh
// session) but a begin record stamped with the original session — and must
// find the crashed session's fork transcript on disk, audit it, and consume
// the report as a terminal done digest. Before the fix this exact state
// wedged every resume of the report-landed-but-unrecorded crash window across
// all three bracket verbs (live-reproduced: the resumed Master declared a
// finished batch stuck).
func TestSmoke_RecordBatchConsumesCrashedSessionReport(t *testing.T) {
	bin := smokeClaudeBin(t)
	settingsPath := realForkSettingsPath(t)
	dir := t.TempDir()
	startSHA := smokeGitRepo(t, dir)
	reportsDir := filepath.Join(dir, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatalf("mkdir reports dir: %v", err)
	}
	reportPath := filepath.Join(reportsDir, builderengine.BatchReportFileName(1, "alpha"))
	crashedSession := mintSessionID(t)

	prompt := "Spawn exactly one Agent-tool subagent with subagent_type set to \"fork\" and NO name. " +
		"Instruct the fork to create the file " + reportPath + " with exactly this content and nothing else:\n" +
		"batch: 01-alpha\nstatus: done\ntests: green\nstuck_reason: null\n" +
		"After the fork returns, stop."

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	runClaudeFork(t, ctx, bin, dir, settingsPath, crashedSession, prompt)

	// The fork's transcript flush is asynchronous from the run's return; wait
	// for both the report and the transcript before driving RecordBatch, so
	// the test asserts the session-keying behavior, not flush timing.
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

	// The resumed run's state: a FRESH Master session, with the batch's begin
	// record still stamped by the crashed session that actually forked it.
	layout := &hubgeometry.Layout{Cwd: dir, WorktreeRoot: dir, Hub: filepath.Dir(dir)}
	state := &websterengine.State{
		MasterSessionID: mintSessionID(t),
		CurrentBatch:    1,
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "alpha", StartSHA: startSHA, Kind: "fork", SessionID: crashedSession},
		},
	}
	deps := websterengine.RecordDeps{
		Plan:         &builderengine.Plan{Batches: []builderengine.PlanBatch{{Number: 1, Slug: "alpha"}}},
		State:        state,
		Engine:       eng,
		Layout:       layout,
		WorktreeRoot: dir,
		ReportsDir:   reportsDir,
		OutcomePath:  filepath.Join(dir, "outcome.yaml"),
		SummaryPath:  filepath.Join(dir, "summary.md"),
		Sleeper:      realSleeper{},
	}

	result, err := websterengine.RecordBatch(deps, 1)
	if err != nil {
		t.Fatalf("RecordBatch() error = %v; want the crashed session's report consumed", err)
	}
	if result.Digest == nil || result.Digest.Status != builderengine.DigestStatusDone {
		t.Fatalf("RecordBatch() digest = %+v; want a terminal done digest", result.Digest)
	}
	if !state.Batches[1].Terminal {
		t.Error("batch 1 not marked Terminal after the late record")
	}
}

// TestSmoke_AwaitBatchSeesForkWrittenReport proves the await-batch poll loop
// reaching a fork-written report (F1): with the claude fork run launched in the
// background, AwaitBatch — pointed at the batch report path a fork is about to
// write — long-polls and returns ReportPresent as soon as the backgrounded
// fork's report lands, which is exactly how Master stays inside its turn across
// a backgrounded fork.
func TestSmoke_AwaitBatchSeesForkWrittenReport(t *testing.T) {
	bin := smokeClaudeBin(t)
	settingsPath := realForkSettingsPath(t)
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatalf("mkdir reports dir: %v", err)
	}
	reportPath := filepath.Join(reportsDir, builderengine.BatchReportFileName(1, "alpha"))

	prompt := "Spawn exactly one Agent-tool subagent with subagent_type set to \"fork\" and NO name. " +
		"Instruct the fork to create the file " + reportPath + " with exactly this content and nothing else:\n" +
		"batch: 01-alpha\nstatus: done\ntests: green\nstuck_reason: null\n" +
		"After the fork returns, stop."

	plan := &builderengine.Plan{Batches: []builderengine.PlanBatch{{Number: 1, Slug: "alpha"}}}

	// Launch the fork run in the background so AwaitBatch genuinely polls a
	// report that does not yet exist when the poll begins — the real
	// backgrounded-fork timing, not a report already on disk.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	runErrCh := make(chan struct{})
	go func() {
		defer close(runErrCh)
		runClaudeFork(t, ctx, bin, dir, settingsPath, mintSessionID(t), prompt)
	}()

	// AwaitBatch blocks up to its wait budget, returning the instant the report
	// appears; a generous budget keeps a slow real fork from timing the poll
	// out before it writes.
	result, err := websterengine.AwaitBatch(plan, reportsDir, 1, 5*time.Minute, recoverRealClock{})
	if err != nil {
		t.Fatalf("AwaitBatch: %v", err)
	}
	if !result.ReportPresent {
		t.Fatalf("AwaitBatch returned ReportPresent=false after %ds; the fork never wrote %s", result.ElapsedS, reportPath)
	}
	<-runErrCh
}
