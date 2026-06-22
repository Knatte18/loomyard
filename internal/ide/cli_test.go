//go:build integration

// cli_test.go covers the ide CLI router: spawn dispatch with a stubbed launcher,
// the unknown-subcommand and missing-slug error envelopes, and usage on no args.

package ide

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestRunCLISpawnDispatch tests that spawn subcommand dispatches correctly with stubbed launcher.
func TestRunCLISpawnDispatch(t *testing.T) {
	// Create a real git repo to test dispatch
	gitRepo := lyxtest.CopyHostHub(t).Hub

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(gitRepo)

	// Stub codeLauncher
	originalLauncher := codeLauncher
	defer func() { codeLauncher = originalLauncher }()
	codeLauncher = func(dir string) error { return nil }

	var out bytes.Buffer
	code := RunCLI(&out, []string{"spawn", "child"})

	// spawn should succeed (or fail for a different reason, not layout resolution)
	// We're testing that the dispatch path is reached, not the entire spawn flow
	if code != 0 && !strings.Contains(out.String(), "spawn failed") {
		t.Fatalf("unexpected error during dispatch; output: %s", out.String())
	}
}

// TestRunCLIUnknownSubcommand tests unknown subcommand error handling.
func TestRunCLIUnknownSubcommand(t *testing.T) {
	gitRepo := lyxtest.CopyHostHub(t).Hub

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(gitRepo)

	var out bytes.Buffer
	code := RunCLI(&out, []string{"unknown"})

	// Should fail with unknown subcommand error
	if code != 1 {
		t.Fatalf("expected exit 1 for unknown subcommand, got %d; output: %s", code, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "unknown subcommand") {
		t.Fatalf("expected 'unknown subcommand' error, got: %q", output)
	}
}

// TestRunCLIMissingSlug tests missing slug error for spawn.
func TestRunCLIMissingSlug(t *testing.T) {
	gitRepo := lyxtest.CopyHostHub(t).Hub

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(gitRepo)

	var out bytes.Buffer
	code := RunCLI(&out, []string{"spawn"})

	// Should fail with missing slug error
	if code != 1 {
		t.Fatalf("expected exit 1 for missing slug, got %d; output: %s", code, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "spawn") {
		t.Fatalf("expected spawn error, got: %q", output)
	}
}

// TestRunCLINoArgs tests no-args usage error.
func TestRunCLINoArgs(t *testing.T) {
	gitRepo := lyxtest.CopyHostHub(t).Hub

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(gitRepo)

	var out bytes.Buffer
	code := RunCLI(&out, []string{})

	// Should fail with usage error
	if code != 1 {
		t.Fatalf("expected exit 1 for no args, got %d; output: %s", code, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "usage") {
		t.Fatalf("expected usage error, got: %q", output)
	}

	// Verify output is JSON
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v; output: %s", err, out.String())
	}

	if ok, _ := result["ok"].(bool); ok {
		t.Fatalf("expected ok=false, got %v", result)
	}
}
