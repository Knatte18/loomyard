//go:build integration

// cli_test.go covers the ide CLI cobra surface: spawn dispatch with a stubbed
// launcher, the unknown-subcommand cobra error, and the no-arg listing path.

package ide

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestRunCLISpawnDispatch tests that spawn subcommand dispatches correctly with stubbed launcher.
func TestRunCLISpawnDispatch(t *testing.T) {
	// Create a real git repo so paths.Resolve succeeds inside the PersistentPreRunE.
	gitRepo := lyxtest.CopyHostHub(t).Hub

	t.Chdir(gitRepo)

	// Stub codeLauncher so the test does not open VS Code.
	originalLauncher := codeLauncher
	defer func() { codeLauncher = originalLauncher }()
	codeLauncher = func(dir string) error { return nil }

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

// TestRunCLI_UnknownSubcommand verifies that an unknown subcommand produces the
// cobra "unknown command" message and exits 1.
func TestRunCLI_UnknownSubcommand(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	code := RunCLI(&out, []string{"unknown"})

	if code != 1 {
		t.Errorf("RunCLI(unknown) = %d; want 1", code)
	}
	if !strings.Contains(out.String(), "unknown command") {
		t.Errorf("RunCLI(unknown) output missing \"unknown command\"; got: %q", out.String())
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
