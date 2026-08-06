//go:build integration

// cli_integration_test.go holds the reedcli tests that seed a real paired
// git-repo fixture (lyxtest.CopyPaired) with reed config resolution against a
// real fixture hub, so this file is integration-tagged per the Test Tier
// Purity Invariant.

package reedcli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// TestRunCLI_ResolvesLayoutAndConfig seeds a real reed.yaml into a fixture hub and verifies config resolution succeeds.
func TestRunCLI_ResolvesLayoutAndConfig(t *testing.T) {
	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"reed": reedengine.ConfigTemplate(),
	})

	t.Chdir(fixture.Hub)

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"status"})

	if exitCode != 1 {
		t.Errorf("RunCLI(status) = %d; want 1 (no live tmux session)", exitCode)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI(status) output is not valid JSON: %v; got: %q", err, out.String())
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("RunCLI(status) ok = true; want false (no tmux session up)")
	}

	errMsg, _ := env["error"].(string)
	if strings.Contains(errMsg, "not initialized") || strings.Contains(errMsg, "not a git repository") {
		t.Errorf("RunCLI(status) error = %q; want a tmux/session error, not a config-resolution error", errMsg)
	}
}

// TestRunCLI_AddNotUp_FriendlyError verifies that running `add` before `up` surfaces the friendly "no reed session" error.
func TestRunCLI_AddNotUp_FriendlyError(t *testing.T) {
	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"reed": reedengine.ConfigTemplate(),
	})
	t.Chdir(fixture.Hub)

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"add", "--cmd", "pwsh -NoExit -Command Write-Host ready"})

	if exitCode != 1 {
		t.Errorf("RunCLI(add) before up = %d; want 1 (no live tmux session)", exitCode)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI(add) output is not valid JSON: %v; got: %q", err, out.String())
	}
	wantErr := `no reed session; run "lyx reed up"`
	if errMsg, _ := env["error"].(string); errMsg != wantErr {
		t.Errorf("RunCLI(add) before up error = %q; want %q", errMsg, wantErr)
	}
}

// TestRunCLI_RemoveNotUp_FriendlyError verifies that running `remove` before `up` surfaces the friendly "no reed session" error.
func TestRunCLI_RemoveNotUp_FriendlyError(t *testing.T) {
	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"reed": reedengine.ConfigTemplate(),
	})
	t.Chdir(fixture.Hub)

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"remove", "does-not-exist"})

	if exitCode != 1 {
		t.Errorf("RunCLI(remove) before up = %d; want 1 (no live tmux session)", exitCode)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI(remove) output is not valid JSON: %v; got: %q", err, out.String())
	}
	wantErr := `no reed session; run "lyx reed up"`
	if errMsg, _ := env["error"].(string); errMsg != wantErr {
		t.Errorf("RunCLI(remove) before up error = %q; want %q", errMsg, wantErr)
	}
}

// TestRunCLI_StatusNotUp_EnrichedResumeHint verifies that running `status` before `up` with persisted strands surfaces the enriched "lyx reed resume" message.
func TestRunCLI_StatusNotUp_EnrichedResumeHint(t *testing.T) {
	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"reed": reedengine.ConfigTemplate(),
	})

	st := &reedengine.ReedState{
		Socket:  "test-socket",
		Session: "test-session",
		Strands: []reedengine.Strand{
			{GUID: "strand-one", Name: "one", Worktree: fixture.Layout.WorktreePath(), Cmd: "true"},
			{GUID: "strand-two", Name: "two", Worktree: fixture.Layout.WorktreePath(), Cmd: "true"},
		},
	}
	if err := reedengine.SaveState(filepath.Join(fixture.Layout.WorktreePath(), ".lyx"), st); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	t.Chdir(fixture.Hub)

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"status"})

	if exitCode != 1 {
		t.Errorf("RunCLI(status) before up = %d; want 1 (no live tmux session)", exitCode)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI(status) output is not valid JSON: %v; got: %q", err, out.String())
	}
	wantErr := `no reed session (2 strands persisted); run "lyx reed resume" to rebuild, or "lyx reed up" for a bare substrate`
	if errMsg, _ := env["error"].(string); errMsg != wantErr {
		t.Errorf("RunCLI(status) before up error = %q; want %q", errMsg, wantErr)
	}
}
