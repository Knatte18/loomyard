//go:build integration

// cli_test.go covers the ide CLI cobra surface: spawn dispatch with a stubbed
// launcher, the unknown-subcommand cobra error, and the no-arg listing path.

package idecli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/ideengine"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestRunCLISpawnDispatch tests that spawn subcommand dispatches correctly with stubbed launcher.
func TestRunCLISpawnDispatch(t *testing.T) {
	// Create a real git repo so paths.Resolve succeeds inside the PersistentPreRunE.
	gitRepo := lyxtest.CopyHostHub(t).Hub

	t.Chdir(gitRepo)

	// Stub ideengine.CodeLauncher so the test does not open VS Code.
	originalLauncher := ideengine.CodeLauncher
	defer func() { ideengine.CodeLauncher = originalLauncher }()
	ideengine.CodeLauncher = func(dir string) error { return nil }

	var out bytes.Buffer
	code := RunCLI(&out, []string{"spawn", "child"})

	// spawn should succeed or fail for a handler reason, not layout resolution.
	if code != 0 && !strings.Contains(out.String(), "spawn failed") {
		t.Fatalf("unexpected error during dispatch; output: %s", out.String())
	}
}

// TestRunCLI_NoArgs verifies that "lyx ide" with no subcommand prints the subcommand
// listing and exits 0 — layout resolution is never attempted, so no git repo is needed.
func TestRunCLI_NoArgs(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	code := RunCLI(&out, []string{})

	if code != 0 {
		t.Errorf("RunCLI() = %d; want 0 for no-arg listing", code)
	}
	// cobra prints "Usage:" or lists available commands; assert at least one subcommand name.
	got := out.String()
	if !strings.Contains(got, "spawn") && !strings.Contains(got, "menu") {
		t.Errorf("RunCLI() no-arg output missing subcommand listing; got: %q", got)
	}
}

// TestRunCLI_UnknownSubcommand verifies that an unknown subcommand exits 1 and
// emits a JSON error envelope with ok=false.
func TestRunCLI_UnknownSubcommand(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	code := RunCLI(&out, []string{"unknown"})

	if code != 1 {
		t.Errorf("RunCLI(unknown) = %d; want 1", code)
	}

	// GroupRunE wraps the error in a JSON envelope; parse and assert ok=false.
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI(unknown) output is not valid JSON: %v; got: %q", err, out.String())
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("RunCLI(unknown) ok = true; want false")
	}
	// The error text contains "unknown" (GroupRunE produces "unknown subcommand").
	if errMsg, _ := env["error"].(string); !strings.Contains(errMsg, "unknown") {
		t.Errorf("RunCLI(unknown) error = %q; want \"unknown\" substring", errMsg)
	}
}

// TestRunCLI_MissingSlug verifies that "lyx ide spawn" with no slug errors appropriately.
func TestRunCLI_MissingSlug(t *testing.T) {
	// Requires a git repo so the PersistentPreRunE can resolve layout.
	gitRepo := lyxtest.CopyHostHub(t).Hub
	t.Chdir(gitRepo)

	var out bytes.Buffer
	code := RunCLI(&out, []string{"spawn"})

	if code != 1 {
		t.Errorf("RunCLI(spawn) with no slug = %d; want 1", code)
	}
	if !strings.Contains(out.String(), "spawn") {
		t.Errorf("RunCLI(spawn) output missing \"spawn\"; got: %q", out.String())
	}
}
