//go:build integration

// cli_test.go — tests for weft CLI routing.

package weft

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

func TestRunCLI_UnknownSubcommand(t *testing.T) {
	// Use a real weft fixture so paths.Resolve succeeds and the CLI reaches the
	// dispatch table; a bare temp dir causes ErrNotAGitRepo before dispatch.
	fixture := lyxtest.CopyWeft(t)
	t.Chdir(fixture.WeftPath)

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"unknown"})

	if exitCode != 1 {
		t.Errorf("RunCLI with unknown subcommand returned %d; want 1", exitCode)
	}
}

func TestRunCLI_WeftPathPushOnly(t *testing.T) {
	tmpDir := t.TempDir()

	var out bytes.Buffer
	// Call with --weft-path and a non-push subcommand
	exitCode := RunCLI(&out, []string{"--weft-path", tmpDir, "status"})

	if exitCode != 1 {
		t.Errorf("RunCLI --weft-path with non-push returned %d; want 1", exitCode)
	}

	// Check that the error message is about worktree context
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

func TestRunCLI_StatusWithMinimalFixture(t *testing.T) {
	// Serial test: uses t.Chdir to test cwd-resolution entry point
	fixture := lyxtest.CopyPaired(t)

	// Seed the weft-prime fixture with the weft config template needed for RunCLI.
	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
		"weft": ConfigTemplate(),
	})

	// Change to the host repo to test cwd resolution
	t.Chdir(fixture.Hub)

	// Call status
	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"status"})

	if exitCode != 0 {
		t.Errorf("RunCLI status returned %d; want 0", exitCode)
		t.Logf("output: %s", out.String())
	}

	// Parse JSON output
	var jsonOut map[string]any
	if err := json.Unmarshal(out.Bytes(), &jsonOut); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v", err)
	}

	if ok, _ := jsonOut["ok"].(bool); !ok {
		t.Errorf("ok should be true; got false. Error: %v", jsonOut["error"])
	}

	// Junction reporting has moved to warp status; weft status exposes only content-sync fields.
	// Assert the content fields are present in the output.
	if _, hasWorktree := jsonOut["weft_worktree"]; !hasWorktree {
		t.Errorf("weft_worktree field missing from status output")
	}
	if _, hasBranch := jsonOut["branch"]; !hasBranch {
		t.Errorf("branch field missing from status output")
	}
}
