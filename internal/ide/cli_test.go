//go:build integration

// cli_test.go covers the ide CLI router: spawn dispatch with a stubbed launcher,
// the unknown-subcommand and missing-slug error envelopes, and usage on no args.

package ide

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestRunCLISpawnDispatch tests that spawn subcommand dispatches correctly with stubbed launcher.
func TestRunCLISpawnDispatch(t *testing.T) {
	// Create a real git repo to test dispatch
	gitRepo := lyxtest.CopyHostHub(t).Hub

	t.Chdir(gitRepo)

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

// TestRunCLIErrors covers error-envelope paths: unknown subcommand, missing slug, and no args.
// All tests use os.Chdir, so they run serially.
func TestRunCLIErrors(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantSubstring string
	}{
		{
			name:          "TestRunCLIUnknownSubcommand",
			args:          []string{"unknown"},
			wantSubstring: "unknown subcommand",
		},
		{
			name:          "TestRunCLIMissingSlug",
			args:          []string{"spawn"},
			wantSubstring: "spawn",
		},
		{
			name:          "TestRunCLINoArgs",
			args:          []string{},
			wantSubstring: "usage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Each test runs in its own temporary repo
			gitRepo := lyxtest.CopyHostHub(t).Hub

			t.Chdir(gitRepo)

			var out bytes.Buffer
			code := RunCLI(&out, tt.args)

			// All error cases return exit 1
			if code != 1 {
				t.Errorf("RunCLI() = %d; want 1; output: %s", code, out.String())
			}

			// Check for expected substring in output
			if !strings.Contains(out.String(), tt.wantSubstring) {
				t.Errorf("RunCLI() output missing %q; got: %q", tt.wantSubstring, out.String())
			}

			// For TestRunCLINoArgs, also verify JSON ok=false assertion
			if tt.name == "TestRunCLINoArgs" {
				var result map[string]any
				if err := json.Unmarshal(out.Bytes(), &result); err != nil {
					t.Errorf("failed to parse JSON output: %v; output: %s", err, out.String())
				}
				if ok, _ := result["ok"].(bool); ok {
					t.Errorf("expected ok=false, got %v", result)
				}
			}
		})
	}
}
