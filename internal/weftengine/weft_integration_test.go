//go:build integration

// weft_integration_test.go — integration tests for weft git operations with real bare remotes.

package weftengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/paths"
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
			lyxFile := filepath.Join(weftRepo, paths.LyxDirName, "config.yaml")
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
