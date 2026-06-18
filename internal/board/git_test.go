// git_test.go — unit tests for the git plumbing (git.go).
//
// Pull / CommitPush behavior.

package board_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/board"
)

func TestPull(t *testing.T) {
	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}

	tmpDir := t.TempDir()
	bareRepoPath := filepath.Join(tmpDir, "bare.git")
	clonePath := filepath.Join(tmpDir, "clone")

	// Create bare repo
	cmd := exec.Command("git", "init", "--bare", bareRepoPath)
	if err := cmd.Run(); err != nil {
		t.Fatalf("init bare repo failed: %v", err)
	}

	// Clone it
	cmd = exec.Command("git", "clone", bareRepoPath, clonePath)
	if err := cmd.Run(); err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	// Configure clone
	cmd = exec.Command("git", "-C", clonePath, "config", "user.email", "test@example.com")
	if err := cmd.Run(); err != nil {
		t.Fatalf("config user.email failed: %v", err)
	}
	cmd = exec.Command("git", "-C", clonePath, "config", "user.name", "Test User")
	if err := cmd.Run(); err != nil {
		t.Fatalf("config user.name failed: %v", err)
	}

	// Create an initial commit to have something to pull
	testFile := filepath.Join(clonePath, "README.md")
	if err := os.WriteFile(testFile, []byte("initial"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	cmd = exec.Command("git", "-C", clonePath, "add", "README.md")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	cmd = exec.Command("git", "-C", clonePath, "commit", "-m", "initial commit")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}
	cmd = exec.Command("git", "-C", clonePath, "push", "-u", "origin", "master")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git push failed: %v", err)
	}

	// Pull when nothing to pull should return updated=false
	updated, err := board.Pull(clonePath)
	if err != nil {
		t.Fatalf("Pull failed: %v", err)
	}
	if updated {
		t.Errorf("Pull returned updated=true, want false when nothing to pull")
	}
}

func TestCommitPush(t *testing.T) {
	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}

	t.Run("commits and logs with BOARD_SKIP_PUSH", func(t *testing.T) {
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")

		// Initialize repo
		cmd := exec.Command("git", "init", repoPath)
		if err := cmd.Run(); err != nil {
			t.Fatalf("git init failed: %v", err)
		}

		// Configure repo
		cmd = exec.Command("git", "-C", repoPath, "config", "user.email", "test@example.com")
		if err := cmd.Run(); err != nil {
			t.Fatalf("config user.email failed: %v", err)
		}
		cmd = exec.Command("git", "-C", repoPath, "config", "user.name", "Test User")
		if err := cmd.Run(); err != nil {
			t.Fatalf("config user.name failed: %v", err)
		}

		// Write a file
		testFile := filepath.Join(repoPath, "test.txt")
		if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		// Set env to skip push
		t.Setenv("BOARD_SKIP_PUSH", "1")

		// Commit via commitPush
		err := board.CommitPush(repoPath, []string{"test.txt"}, "test commit")
		if err != nil {
			t.Fatalf("CommitPush failed: %v", err)
		}

		// Verify commit exists in log
		cmd = exec.Command("git", "-C", repoPath, "log", "--oneline")
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("git log failed: %v", err)
		}
		if !strings.Contains(string(output), "test commit") {
			t.Errorf("commit not found in log: %s", string(output))
		}
	})

	t.Run("idempotent with no changes", func(t *testing.T) {
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "repo")

		// Initialize repo
		cmd := exec.Command("git", "init", repoPath)
		if err := cmd.Run(); err != nil {
			t.Fatalf("git init failed: %v", err)
		}

		cmd = exec.Command("git", "-C", repoPath, "config", "user.email", "test@example.com")
		if err := cmd.Run(); err != nil {
			t.Fatalf("config user.email failed: %v", err)
		}
		cmd = exec.Command("git", "-C", repoPath, "config", "user.name", "Test User")
		if err := cmd.Run(); err != nil {
			t.Fatalf("config user.name failed: %v", err)
		}

		// Write and commit a file
		testFile := filepath.Join(repoPath, "test.txt")
		if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		t.Setenv("BOARD_SKIP_PUSH", "1")

		err := board.CommitPush(repoPath, []string{"test.txt"}, "first commit")
		if err != nil {
			t.Fatalf("CommitPush failed: %v", err)
		}

		// Get commit count
		cmd = exec.Command("git", "-C", repoPath, "rev-list", "--count", "HEAD")
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("git rev-list failed: %v", err)
		}
		firstCount := strings.TrimSpace(string(output))

		// Call commitPush again with no changes - should be idempotent
		err = board.CommitPush(repoPath, []string{"test.txt"}, "second commit")
		if err != nil {
			t.Fatalf("CommitPush second call failed: %v", err)
		}

		// Get commit count again - should be the same
		cmd = exec.Command("git", "-C", repoPath, "rev-list", "--count", "HEAD")
		output, err = cmd.Output()
		if err != nil {
			t.Fatalf("git rev-list failed: %v", err)
		}
		secondCount := strings.TrimSpace(string(output))

		if firstCount != secondCount {
			t.Errorf("commit count changed: first=%s, second=%s", firstCount, secondCount)
		}
	})

	t.Run("rebase retry on non-fast-forward", func(t *testing.T) {
		tmpDir := t.TempDir()
		bareRepoPath := filepath.Join(tmpDir, "bare.git")
		cloneAPath := filepath.Join(tmpDir, "cloneA")
		cloneBPath := filepath.Join(tmpDir, "cloneB")

		// Create bare repo
		cmd := exec.Command("git", "init", "--bare", bareRepoPath)
		if err := cmd.Run(); err != nil {
			t.Fatalf("init bare repo failed: %v", err)
		}

		// Clone twice
		cmd = exec.Command("git", "clone", bareRepoPath, cloneAPath)
		if err := cmd.Run(); err != nil {
			t.Fatalf("clone A failed: %v", err)
		}

		cmd = exec.Command("git", "clone", bareRepoPath, cloneBPath)
		if err := cmd.Run(); err != nil {
			t.Fatalf("clone B failed: %v", err)
		}

		// Configure both clones
		for _, path := range []string{cloneAPath, cloneBPath} {
			cmd = exec.Command("git", "-C", path, "config", "user.email", "test@example.com")
			if err := cmd.Run(); err != nil {
				t.Fatalf("config user.email failed: %v", err)
			}
			cmd = exec.Command("git", "-C", path, "config", "user.name", "Test User")
			if err := cmd.Run(); err != nil {
				t.Fatalf("config user.name failed: %v", err)
			}
		}

		// Push a commit from clone B
		fileB := filepath.Join(cloneBPath, "fileB.txt")
		if err := os.WriteFile(fileB, []byte("from B"), 0o644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		t.Setenv("BOARD_SKIP_PUSH", "")
		err := board.CommitPush(cloneBPath, []string{"fileB.txt"}, "commit from B")
		if err != nil {
			t.Fatalf("CommitPush on B failed: %v", err)
		}

		// Now push a commit from clone A (which doesn't have B's commit)
		fileA := filepath.Join(cloneAPath, "fileA.txt")
		if err := os.WriteFile(fileA, []byte("from A"), 0o644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		// This should succeed via rebase retry
		err = board.CommitPush(cloneAPath, []string{"fileA.txt"}, "commit from A")
		if err != nil {
			t.Fatalf("CommitPush on A failed (should have succeeded via rebase): %v", err)
		}

		// Verify both commits are in the log
		cmd = exec.Command("git", "-C", cloneAPath, "log", "--oneline")
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("git log failed: %v", err)
		}
		logStr := string(output)
		if !strings.Contains(logStr, "commit from A") {
			t.Errorf("commit from A not found in log")
		}
		if !strings.Contains(logStr, "commit from B") {
			t.Errorf("commit from B not found in log")
		}
	})
}
