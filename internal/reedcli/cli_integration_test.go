//go:build integration

// cli_integration_test.go holds the reedcli tests that seed a real paired
// git-repo fixture (lyxtest.CopyPaired) with reed config resolution against a
// real fixture hub, so this file is integration-tagged per the Test Tier
// Purity Invariant.

package reedcli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// TestRunCLI_ResolvesLayoutAndConfig seeds a real reed.yaml into a fixture
// hub's _lyx/config/ (reed config is anchored at layout.Cwd, unlike weft's
// weft-sibling-anchored config) and verifies PersistentPreRunE reaches the
// engine call rather than aborting on Getwd/Resolve/LoadConfig. Status then
// fails because no tmux server is running under this fixture's socket name
// — that failure is the point: it proves config resolution itself
// succeeded, exercising a domain-error path distinct from the no-git-repo
// path TestRunCLI_NotAGitRepo covers.
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

	// Guard against the wrong failure: this must NOT be a config-resolution
	// error (those paths are covered by TestRunCLI_NotAGitRepo and the
	// "not initialized" case) — this test's whole point is that config
	// resolution succeeded and the engine's own tmux check is what failed.
	errMsg, _ := env["error"].(string)
	if strings.Contains(errMsg, "not initialized") || strings.Contains(errMsg, "not a git repository") {
		t.Errorf("RunCLI(status) error = %q; want a tmux/session error, not a config-resolution error", errMsg)
	}
}

// TestRunCLI_AddNotUp_FriendlyError verifies that running `add` before `up`
// surfaces the same friendly "no reed session" error Status has always given,
// rather than a raw tmux error bubbling up from launchStrandLocked's first
// unguarded tmux call (orch_04 finding #3).
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
	// Fresh CopyPaired fixture, no reed.json seeded: zero strands persisted,
	// so requireSessionLocked/noSessionMessage keeps today's short message
	// rather than the resume-hint enrichment (that case is covered by
	// TestRunCLI_StatusNotUp_EnrichedResumeHint).
	wantErr := `no reed session; run "lyx reed up"`
	if errMsg, _ := env["error"].(string); errMsg != wantErr {
		t.Errorf("RunCLI(add) before up error = %q; want %q", errMsg, wantErr)
	}
}

// TestRunCLI_RemoveNotUp_FriendlyError verifies that running `remove` before
// `up` surfaces the same friendly "no reed session" error, rather than a raw
// tmux error bubbling up from reconcileApplyPersistLocked's first unguarded
// listPanes call (orch_04 finding #3). The guid is a placeholder — the
// pre-flight session check must fail before the table is even consulted.
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
	// Fresh CopyPaired fixture, no reed.json seeded: zero strands persisted,
	// so requireSessionLocked/noSessionMessage keeps today's short message
	// rather than the resume-hint enrichment (that case is covered by
	// TestRunCLI_StatusNotUp_EnrichedResumeHint).
	wantErr := `no reed session; run "lyx reed up"`
	if errMsg, _ := env["error"].(string); errMsg != wantErr {
		t.Errorf("RunCLI(remove) before up error = %q; want %q", errMsg, wantErr)
	}
}

// TestRunCLI_StatusNotUp_EnrichedResumeHint verifies that running `status`
// before `up`, with persisted strands already sitting in reed.json (an
// unexplained-server-death scenario: the server is gone but the strand table
// survived), surfaces the enriched "lyx reed resume" pointer instead of the
// bare "lyx reed up" message the zero-strand before-up tests above assert —
// requireSessionLocked/noSessionMessage's persisted-state branch (Shared
// Decision enriched-no-session-error).
func TestRunCLI_StatusNotUp_EnrichedResumeHint(t *testing.T) {
	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"reed": reedengine.ConfigTemplate(),
	})

	// Seed reed.json with two persisted strand records directly, bypassing a
	// live add (there is no server up yet) — this is exactly the state an
	// unexplained server death leaves behind: the strand table survives on
	// disk even though the session is gone.
	st := &reedengine.ReedState{
		Socket:  "test-socket",
		Session: "test-session",
		Strands: []reedengine.Strand{
			{GUID: "strand-one", Name: "one", Worktree: fixture.Layout.WorktreeRoot, Cmd: "true"},
			{GUID: "strand-two", Name: "two", Worktree: fixture.Layout.WorktreeRoot, Cmd: "true"},
		},
	}
	if err := reedengine.SaveState(fixture.Layout.DotLyxDir(), st); err != nil {
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
