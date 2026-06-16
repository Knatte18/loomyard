// integration_test.go — git-backed integration tests (integration-gated).
//
// Exercises the real git plumbing (AtomicWrite + CommitPush + Pull) against the
// dummy remote at testRepoURL: clones it, writes and pushes a commit, then
// re-clones to confirm the commit landed. Hits the network, so it is behind the
// integration build tag. testRepoURL is shared with bench_git_test.go.

//go:build integration

package boardtest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/board"
)

const testRepoURL = "https://github.com/Knatte18/loomyard-test.git"

func setupIntegrationRepo(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "repo")

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s", args, out)
		}
	}

	run("git", "clone", testRepoURL, repoPath)
	run("git", "-C", repoPath, "config", "user.email", "test@mhgo.dev")
	run("git", "-C", repoPath, "config", "user.name", "MHGo Integration Test")

	// If repo is empty (no commits), push an initial commit to establish the branch.
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD")
	if err := cmd.Run(); err != nil {
		initFile := filepath.Join(repoPath, ".gitkeep")
		if err := os.WriteFile(initFile, []byte(""), 0o644); err != nil {
			t.Fatalf("write .gitkeep: %v", err)
		}
		run("git", "-C", repoPath, "add", ".gitkeep")
		run("git", "-C", repoPath, "commit", "-m", "chore: init test repo")
		run("git", "-C", repoPath, "push", "--set-upstream", "origin", "HEAD")
	}

	return repoPath
}

func TestIntegrationCommitPush(t *testing.T) {
	repoPath := setupIntegrationRepo(t)

	filename := fmt.Sprintf("test-%d.txt", time.Now().Unix())
	content := fmt.Sprintf("integration test run at %s", time.Now().Format(time.RFC3339))

	if err := board.AtomicWrite(repoPath, filename, content); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	if err := board.CommitPush(repoPath, []string{filename}, "test: integration CommitPush"); err != nil {
		t.Fatalf("CommitPush: %v", err)
	}

	// Verify commit is visible on remote by cloning fresh and checking log.
	tmpDir := t.TempDir()
	verifyPath := filepath.Join(tmpDir, "verify")
	cmd := exec.Command("git", "clone", testRepoURL, verifyPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("verify clone: %s", out)
	}

	logOut, err := exec.Command("git", "-C", verifyPath, "log", "--oneline").Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if !strings.Contains(string(logOut), "integration CommitPush") {
		t.Errorf("commit not found on remote.\nLog:\n%s", logOut)
	}
}

func TestIntegrationPull(t *testing.T) {
	repoPath := setupIntegrationRepo(t)

	updated, err := board.Pull(repoPath)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	// Nothing pushed since clone — should not be updated.
	if updated {
		t.Errorf("Pull returned updated=true immediately after clone")
	}
}
