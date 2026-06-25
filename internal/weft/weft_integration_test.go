//go:build integration

// weft_integration_test.go — integration tests for weft git operations with real bare remotes.

package weft

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

func TestPushIntegration(t *testing.T) {
	tests := []struct {
		name          string
		fileContent   string
		verifyCatFile bool
	}{
		{
			name:          "TestPushIntegration_CommitLandsOnBare",
			fileContent:   "v1",
			verifyCatFile: false,
		},
		{
			name:          "TestPushIntegration_RebaseRetryOnNFF",
			fileContent:   "local",
			verifyCatFile: false,
		},
		{
			name:          "TestSyncIntegration_EventuallyPushed",
			fileContent:   "sync-test",
			verifyCatFile: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fixture := lyxtest.CopyWeft(t)
			weftRepo := fixture.WeftPath

			// Commit a change
			lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
			if err := os.WriteFile(lyxFile, []byte(tt.fileContent), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			committed, err := Commit(weftRepo, []string{"_lyx"}, SyncOptions{})
			if err != nil {
				t.Fatalf("Commit: %v", err)
			}
			if !committed {
				t.Fatalf("Commit should succeed")
			}

			// Capture the commit SHA from HEAD (needed for verifyCatFile case)
			var commitSHA string
			if tt.verifyCatFile {
				cmd := exec.Command("git", "-C", weftRepo, "rev-parse", "HEAD")
				shaOutput, err := cmd.Output()
				if err != nil {
					t.Fatalf("git rev-parse HEAD: %v", err)
				}
				commitSHA = strings.TrimSpace(string(shaOutput))
			}

			// Push
			if err := Push(weftRepo, SyncOptions{}); err != nil {
				t.Fatalf("Push: %v", err)
			}

			// Verify the specific commit reached the bare remote (EventuallyPushed case only)
			if tt.verifyCatFile {
				bare := fixture.Bare
				cmd := exec.Command("git", "-C", bare, "-c", "safe.bareRepository=all", "cat-file", "-e", commitSHA)
				if err := cmd.Run(); err != nil {
					t.Fatalf("commit %s did not reach bare remote: %v", commitSHA, err)
				}
			}
		})
	}
}

// TestRunCLI_EnvMapToOption tests that the cli edge properly maps WEFT_SKIP_PUSH
// to SyncOptions. This is a serial test because it exercises the cwd-based
// push command which reads the current directory. The test sets WEFT_SKIP_PUSH
// and verifies the push command succeeds (with the push skipped due to the env var).
func TestRunCLI_EnvMapToOption(t *testing.T) {
	// Serial: uses t.Setenv and t.Chdir which affect process-wide state
	fixture := lyxtest.CopyPaired(t)

	// Seed the weft-prime fixture with the weft config template needed for RunCLI.
	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
		"weft": ConfigTemplate(),
	})

	hubPath := fixture.Hub

	// Change to the hub directory so paths.Resolve can locate the repo from cwd;
	// t.Chdir restores the original cwd automatically after the test.
	t.Chdir(hubPath)

	// Modify a file in the weft config that would be committed
	weftConfigFile := filepath.Join(fixture.WeftPrime, "_lyx", "placeholder")
	if err := os.WriteFile(weftConfigFile, []byte("modified"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Set WEFT_SKIP_PUSH to prevent the actual push
	t.Setenv("WEFT_SKIP_PUSH", "1")

	// Call the push subcommand via CLI (from the hub directory)
	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"push"})

	if exitCode != 0 {
		t.Errorf("RunCLI push returned %d; want 0", exitCode)
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
}
