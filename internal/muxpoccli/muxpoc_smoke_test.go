//go:build smoke

package muxpoccli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// TestSmokeFullLifecycle verifies the complete muxpoc end-to-end lifecycle:
// up (fresh), status, review, kill-server (crash), up again (cold recover), down.
// Uses a cheap placeholder command instead of real claude.
func TestSmokeFullLifecycle(t *testing.T) {
	// Skip if not on Windows (psmux is Windows-only)
	if runtime.GOOS != "windows" {
		t.Skip("smoke test requires Windows psmux")
	}

	// Build config with cheap placeholder commands
	cfg := Config{
		PsmuxPath:    `C:\Code\tools\bin\psmux.exe`,
		PwshPath:     `C:\Code\tools\powershell7\pwsh.exe`,
		ClaudePath:   "",
		LaunchTpl:    "Write-Host ready; Read-Host",
		ResumeTpl:    "Write-Host resumed; Read-Host",
		Width:        220,
		Height:       50,
		Interval:     2 * time.Second,
		WorktreeRoot: "", // Will set after chdir
	}

	// Skip if psmux not found
	if _, err := os.Stat(cfg.PsmuxPath); err != nil {
		t.Skipf("psmux not found at %s", cfg.PsmuxPath)
	}

	// Run in an isolated temp dir so the test never writes .lyx/ into the
	// package directory. The cmd* functions derive state path and socket from
	// cfg.WorktreeRoot, so chdir into the temp dir for the duration of the test.
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	cwd := t.TempDir()
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir to temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	// Initialize temp dir as a git repo so paths.Resolve succeeds
	initCmd := exec.Command("git", "init", "-b", "main")
	initCmd.Dir = cwd
	if err := initCmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	// Set git user
	configUserCmd := exec.Command("git", "config", "user.email", "test@example.com")
	configUserCmd.Dir = cwd
	if err := configUserCmd.Run(); err != nil {
		t.Fatalf("git config user.email: %v", err)
	}

	configNameCmd := exec.Command("git", "config", "user.name", "Test User")
	configNameCmd.Dir = cwd
	if err := configNameCmd.Run(); err != nil {
		t.Fatalf("git config user.name: %v", err)
	}

	// Create initial commit so we have a valid repo
	commitCmd := exec.Command("git", "commit", "--allow-empty", "-m", "initial commit")
	commitCmd.Dir = cwd
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	// Set cfg.WorktreeRoot to the temp dir (same raw string for consistency)
	cfg.WorktreeRoot = cwd

	// Cleanup: ensure down is called even if test fails
	t.Cleanup(func() {
		buf := &bytes.Buffer{}
		cmdDown(buf, cfg)
	})

	// ========== UP (fresh) ==========
	buf := &bytes.Buffer{}
	exitCode := cmdUp(buf, cfg)
	if exitCode != 0 {
		t.Fatalf("cmdUp failed with exit code %d, output: %s", exitCode, buf.String())
	}

	var upResult map[string]any
	if err := json.Unmarshal(buf.Bytes(), &upResult); err != nil {
		t.Fatalf("parse up result: %v", err)
	}

	if !upResult["ok"].(bool) {
		t.Fatalf("up returned ok=false: %v", upResult)
	}

	sessionID, ok := upResult["session_id"].(string)
	if !ok || sessionID == "" {
		t.Fatalf("up missing or empty session_id")
	}

	socket, ok := upResult["socket"].(string)
	if !ok || socket == "" {
		t.Fatalf("up missing or empty socket")
	}

	strippedEnv, ok := upResult["stripped_env"].([]any)
	if !ok {
		t.Fatalf("up stripped_env not an array")
	}

	// When Claude-Code env vars are present, verify they were stripped
	hasClaudeCodeEnv := os.Getenv("CLAUDECODE") != "" ||
		(len(os.Environ()) > 0 && func() bool {
			for _, e := range os.Environ() {
				if len(e) > 11 && e[:11] == "CLAUDE_CODE" {
					return true
				}
			}
			return false
		}())
	if hasClaudeCodeEnv && len(strippedEnv) == 0 {
		t.Fatalf("CLAUDECODE or CLAUDE_CODE_* found in test env but stripped_env is empty")
	}

	// ========== STATUS ==========
	buf = &bytes.Buffer{}
	exitCode = cmdStatus(buf, cfg)
	if exitCode != 0 {
		t.Fatalf("cmdStatus failed with exit code %d, output: %s", exitCode, buf.String())
	}

	var statusResult map[string]any
	if err := json.Unmarshal(buf.Bytes(), &statusResult); err != nil {
		t.Fatalf("parse status result: %v", err)
	}

	if !statusResult["ok"].(bool) {
		t.Fatalf("status returned ok=false: %v", statusResult)
	}

	// Verify all seven required fields
	requiredFields := []string{"have_state", "server_up", "session", "socket", "stripped_env", "state_panes", "live_panes"}
	for _, field := range requiredFields {
		if _, ok := statusResult[field]; !ok {
			t.Fatalf("status missing required field: %s", field)
		}
	}

	if !statusResult["server_up"].(bool) {
		t.Fatalf("status reports server_up=false")
	}

	// ========== REVIEW ==========
	buf = &bytes.Buffer{}
	exitCode = cmdReview(buf, cfg)
	if exitCode != 0 {
		t.Fatalf("cmdReview failed with exit code %d, output: %s", exitCode, buf.String())
	}

	var reviewResult map[string]any
	if err := json.Unmarshal(buf.Bytes(), &reviewResult); err != nil {
		t.Fatalf("parse review result: %v", err)
	}

	if !reviewResult["ok"].(bool) {
		t.Fatalf("review returned ok=false: %v", reviewResult)
	}

	if _, ok := reviewResult["session_id"].(string); !ok {
		t.Fatalf("review missing session_id")
	}

	// Status again to verify live_panes has 2 entries
	buf = &bytes.Buffer{}
	exitCode = cmdStatus(buf, cfg)
	if exitCode != 0 {
		t.Fatalf("cmdStatus (after review) failed with exit code %d", exitCode)
	}

	if err := json.Unmarshal(buf.Bytes(), &statusResult); err != nil {
		t.Fatalf("parse status result (after review): %v", err)
	}

	livePanes, ok := statusResult["live_panes"].([]any)
	if !ok {
		t.Fatalf("status live_panes not an array")
	}
	if len(livePanes) < 2 {
		t.Fatalf("status live_panes has %d entries, expected >= 2", len(livePanes))
	}

	// ========== KILL-SERVER (simulate crash) ==========
	killCmd := exec.Command(cfg.PsmuxPath, "-L", socket, "kill-server")
	if err := killCmd.Run(); err != nil {
		t.Logf("kill-server failed (may be OK if server already dead): %v", err)
	}

	// Verify state file still exists after crash
	_, err = LoadState(cfg.WorktreeRoot)
	if err != nil {
		t.Fatalf("state should exist after crash: %v", err)
	}

	// ========== UP AGAIN (cold recover) ==========
	buf = &bytes.Buffer{}
	exitCode = cmdUp(buf, cfg)
	if exitCode != 0 {
		t.Fatalf("cmdUp (recovery) failed with exit code %d, output: %s", exitCode, buf.String())
	}

	var recoverResult map[string]any
	if err := json.Unmarshal(buf.Bytes(), &recoverResult); err != nil {
		t.Fatalf("parse recovery result: %v", err)
	}

	if !recoverResult["ok"].(bool) {
		t.Fatalf("recovery returned ok=false: %v", recoverResult)
	}

	message, ok := recoverResult["message"].(string)
	if !ok || message != "cold-recovered" {
		t.Fatalf("recovery message is %q, expected 'cold-recovered'", message)
	}

	recoveredPanes, ok := recoverResult["recovered_panes"].(float64)
	if !ok || recoveredPanes < 1 {
		t.Fatalf("recovery recovered_panes is %v, expected >= 1", recoveredPanes)
	}

	// ========== DOWN ==========
	buf = &bytes.Buffer{}
	exitCode = cmdDown(buf, cfg)
	if exitCode != 0 {
		t.Fatalf("cmdDown failed with exit code %d, output: %s", exitCode, buf.String())
	}

	// Status again to verify have_state is false
	buf = &bytes.Buffer{}
	exitCode = cmdStatus(buf, cfg)
	if exitCode != 0 {
		t.Fatalf("cmdStatus (after down) failed with exit code %d", exitCode)
	}

	if err := json.Unmarshal(buf.Bytes(), &statusResult); err != nil {
		t.Fatalf("parse status result (after down): %v", err)
	}

	haveState, ok := statusResult["have_state"].(bool)
	if !ok || haveState {
		t.Fatalf("status have_state is %v, expected false", haveState)
	}
}
