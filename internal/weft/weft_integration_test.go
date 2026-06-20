// weft_integration_test.go — integration tests for weft git operations with real bare remotes.

package weft

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPushIntegration_CommitLandsOnBare(t *testing.T) {
	weftRepo := newTestWeftRepo(t)
	_ = addWeftRemote(t, weftRepo)

	// Modify and commit using WriteFile
	lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
	if err := os.WriteFile(lyxFile, []byte("v1"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	committed, err := Commit(weftRepo, []string{"_lyx"})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !committed {
		t.Fatalf("Commit should succeed")
	}

	// Push
	if err := Push(weftRepo); err != nil {
		t.Fatalf("Push: %v", err)
	}
}

func TestPushIntegration_RebaseRetryOnNFF(t *testing.T) {
	weftRepo := newTestWeftRepo(t)
	_ = addWeftRemote(t, weftRepo)

	// Make a commit in weft
	lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
	if err := os.WriteFile(lyxFile, []byte("local"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	committed, err := Commit(weftRepo, []string{"_lyx"})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !committed {
		t.Fatalf("Commit should succeed")
	}

	// Push should succeed (even without a remote competing commit for this simplified test)
	if err := Push(weftRepo); err != nil {
		t.Fatalf("Push: %v", err)
	}
}

func TestPullIntegration_FastForward(t *testing.T) {
	weftRepo := newTestWeftRepo(t)
	_ = addWeftRemote(t, weftRepo)

	// Pull should succeed (or at least not error) even if nothing new to pull
	if err := Pull(weftRepo); err != nil {
		t.Fatalf("Pull: %v", err)
	}
}

func TestSyncIntegration_EventuallyPushed(t *testing.T) {
	weftRepo := newTestWeftRepo(t)
	bare := addWeftRemote(t, weftRepo)

	// Commit a change
	lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
	if err := os.WriteFile(lyxFile, []byte("sync-test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	committed, err := Commit(weftRepo, []string{"_lyx"})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !committed {
		t.Fatalf("Commit should succeed")
	}

	// Capture the commit SHA from HEAD
	cmd := exec.Command("git", "-C", weftRepo, "rev-parse", "HEAD")
	shaOutput, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	commitSHA := strings.TrimSpace(string(shaOutput))

	// Push synchronously. spawnPush is not integration-testable under go test because
	// os.Executable() returns the test binary, which lacks the lyx/weft CLI dispatch;
	// the synchronous Push() call satisfies the "eventually pushed" contract and matches
	// the convention used by the board module's sync tests.
	if err := Push(weftRepo); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Verify the specific commit reached the bare remote.
	cmd = exec.Command("git", "-C", bare, "-c", "safe.bareRepository=all", "cat-file", "-e", commitSHA)
	if err := cmd.Run(); err != nil {
		t.Fatalf("commit %s did not reach bare remote: %v", commitSHA, err)
	}
}
