//go:build integration

// cli_test.go covers the weft CLI cobra surface: unknown-subcommand cobra error,
// --weft-path push-only gate, no-arg listing, and status with a minimal fixture.

package weft

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestRunCLI_NoArgs verifies that "lyx weft" with no subcommand prints the
// subcommand listing and exits 0 — no git repo is needed.
func TestRunCLI_NoArgs(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{})

	if exitCode != 0 {
		t.Errorf("RunCLI() = %d; want 0 for no-arg listing", exitCode)
	}
	// cobra lists available commands; assert at least one subcommand name is present.
	got := out.String()
	if !strings.Contains(got, "status") && !strings.Contains(got, "commit") {
		t.Errorf("RunCLI() no-arg output missing subcommand listing; got: %q", got)
	}
}

// TestRunCLI_UnknownSubcommand verifies that an unknown subcommand produces the
// cobra "unknown command" message and exits 1.
func TestRunCLI_UnknownSubcommand(t *testing.T) {
	// Use a real weft fixture so paths.Resolve succeeds in the PersistentPreRunE
	// and the cobra dispatch table is reached before any resolution is done.
	// (cobra routes unknown commands before invoking PersistentPreRunE, so the
	// fixture is only needed for safety; the test would pass in a bare dir too.)
	fixture := lyxtest.CopyWeft(t)
	t.Chdir(fixture.WeftPath)

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"unknown"})

	if exitCode != 1 {
		t.Errorf("RunCLI with unknown subcommand returned %d; want 1", exitCode)
	}
	if !strings.Contains(out.String(), "unknown command") {
		t.Errorf("RunCLI(unknown) output missing \"unknown command\"; got: %q", out.String())
	}
}

// TestRunCLI_WeftPathPushOnly verifies that --weft-path with a non-push subcommand
// returns exit 1 and the JSON error envelope {"ok":false,"error":"subcommand requires
// a worktree context"}. This path is preserved via the PersistentPreRunE abort.
func TestRunCLI_WeftPathPushOnly(t *testing.T) {
	tmpDir := t.TempDir()

	var out bytes.Buffer
	// Call with --weft-path and a non-push subcommand.
	exitCode := RunCLI(&out, []string{"--weft-path", tmpDir, "status"})

	if exitCode != 1 {
		t.Errorf("RunCLI --weft-path with non-push returned %d; want 1", exitCode)
	}

	// The error must be the JSON envelope written by output.Err.
	output := out.String()
	var jsonOut map[string]any
	if err := json.Unmarshal([]byte(output), &jsonOut); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v", err)
	}

	if ok, _ := jsonOut["ok"].(bool); ok {
		t.Errorf("ok should be false for error; got true")
	}

	if errMsg, ok := jsonOut["error"].(string); ok {
		if errMsg != "subcommand requires a worktree context" {
			t.Errorf("error message = %q; want %q", errMsg, "subcommand requires a worktree context")
		}
	} else {
		t.Errorf("error field missing or not a string")
	}
}

// TestRunCLI_StatusWithMinimalFixture tests the status subcommand via cwd resolution.
func TestRunCLI_StatusWithMinimalFixture(t *testing.T) {
	// Serial test: uses t.Chdir to test cwd-resolution entry point.
	fixture := lyxtest.CopyPaired(t)

	// Seed the weft-prime fixture with the weft config template needed for RunCLI.
	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
		"weft": ConfigTemplate(),
	})

	// Change to the host repo to test cwd resolution.
	t.Chdir(fixture.Hub)

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"status"})

	if exitCode != 0 {
		t.Errorf("RunCLI status returned %d; want 0", exitCode)
		t.Logf("output: %s", out.String())
	}

	// Parse JSON output.
	var jsonOut map[string]any
	if err := json.Unmarshal(out.Bytes(), &jsonOut); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v", err)
	}

	if ok, _ := jsonOut["ok"].(bool); !ok {
		t.Errorf("ok should be true; got false. Error: %v", jsonOut["error"])
	}

	// Junction reporting has moved to warp status; weft status exposes only content-sync fields.
	if _, hasWorktree := jsonOut["weft_worktree"]; !hasWorktree {
		t.Errorf("weft_worktree field missing from status output")
	}
	if _, hasBranch := jsonOut["branch"]; !hasBranch {
		t.Errorf("branch field missing from status output")
	}
}
