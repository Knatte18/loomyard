//go:build integration

// cli_integration_test.go holds the muxcli tests that seed a real paired
// git-repo fixture (lyxtest.CopyPaired) with mux config resolution against a
// real fixture hub, so this file is integration-tagged per the Test Tier
// Purity Invariant.

package muxcli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/muxengine"
)

// TestRunCLI_ResolvesLayoutAndConfig seeds a real mux.yaml into a fixture
// hub's _lyx/config/ (mux config is anchored at layout.Cwd, unlike weft's
// weft-sibling-anchored config) and verifies PersistentPreRunE reaches the
// engine call rather than aborting on Getwd/Resolve/LoadConfig. Status then
// fails because no psmux server is running under this fixture's socket name
// — that failure is the point: it proves config resolution itself
// succeeded, exercising a domain-error path distinct from the no-git-repo
// path TestRunCLI_NotAGitRepo covers.
func TestRunCLI_ResolvesLayoutAndConfig(t *testing.T) {
	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})

	t.Chdir(fixture.Hub)

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"status"})

	if exitCode != 1 {
		t.Errorf("RunCLI(status) = %d; want 1 (no live psmux session)", exitCode)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI(status) output is not valid JSON: %v; got: %q", err, out.String())
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("RunCLI(status) ok = true; want false (no psmux session up)")
	}

	// Guard against the wrong failure: this must NOT be a config-resolution
	// error (those paths are covered by TestRunCLI_NotAGitRepo and the
	// "not initialized" case) — this test's whole point is that config
	// resolution succeeded and the engine's own psmux check is what failed.
	errMsg, _ := env["error"].(string)
	if strings.Contains(errMsg, "not initialized") || strings.Contains(errMsg, "not a git repository") {
		t.Errorf("RunCLI(status) error = %q; want a psmux/session error, not a config-resolution error", errMsg)
	}
}

// TestRunCLI_AddNotUp_FriendlyError verifies that running `add` before `up`
// surfaces the same friendly "no mux session" error Status has always given,
// rather than a raw psmux error bubbling up from launchStrandLocked's first
// unguarded psmux call (orch_04 finding #3).
func TestRunCLI_AddNotUp_FriendlyError(t *testing.T) {
	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	t.Chdir(fixture.Hub)

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"add", "--cmd", "pwsh -NoExit -Command Write-Host ready"})

	if exitCode != 1 {
		t.Errorf("RunCLI(add) before up = %d; want 1 (no live psmux session)", exitCode)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI(add) output is not valid JSON: %v; got: %q", err, out.String())
	}
	wantErr := `no mux session; run "lyx mux up"`
	if errMsg, _ := env["error"].(string); errMsg != wantErr {
		t.Errorf("RunCLI(add) before up error = %q; want %q", errMsg, wantErr)
	}
}

// TestRunCLI_RemoveNotUp_FriendlyError verifies that running `remove` before
// `up` surfaces the same friendly "no mux session" error, rather than a raw
// psmux error bubbling up from reconcileApplyPersistLocked's first unguarded
// listPanes call (orch_04 finding #3). The guid is a placeholder — the
// pre-flight session check must fail before the table is even consulted.
func TestRunCLI_RemoveNotUp_FriendlyError(t *testing.T) {
	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	t.Chdir(fixture.Hub)

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"remove", "does-not-exist"})

	if exitCode != 1 {
		t.Errorf("RunCLI(remove) before up = %d; want 1 (no live psmux session)", exitCode)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI(remove) output is not valid JSON: %v; got: %q", err, out.String())
	}
	wantErr := `no mux session; run "lyx mux up"`
	if errMsg, _ := env["error"].(string); errMsg != wantErr {
		t.Errorf("RunCLI(remove) before up error = %q; want %q", errMsg, wantErr)
	}
}
