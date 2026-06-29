//go:build integration

// git_test.go — unit tests for the git plumbing (git.go).
//
// Pull / CommitPush behavior.

package boardtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

func TestPull(t *testing.T) {
	t.Parallel()
	// Use CopyWeft to get a working clone of main with upstream tracking already
	// established — no per-test git init/clone/config/commit/push spawns needed.
	fixture := lyxtest.CopyWeft(t)
	clonePath := fixture.WeftPath

	// Pull when nothing to pull should return updated=false.
	updated, err := boardengine.Pull(clonePath)
	if err != nil {
		t.Fatalf("Pull failed: %v", err)
	}
	if updated {
		t.Errorf("Pull returned updated=true, want false when nothing to pull")
	}
}

func TestCommitPush(t *testing.T) {
	t.Run("commits and logs with skipPush=true", func(t *testing.T) {
		t.Parallel()
		// CopyHostHub provides a local repo with an initial commit and no upstream
		// push needed, matching the skipPush=true scenario.
		repoPath := lyxtest.CopyHostHub(t).Hub

		// Write a file to stage for CommitPush.
		testFile := filepath.Join(repoPath, "test.txt")
		if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		err := boardengine.CommitPush(repoPath, []string{"test.txt"}, "test commit", true)
		if err != nil {
			t.Fatalf("CommitPush failed: %v", err)
		}

		// Verify the commit appears in the log.
		cmd := exec.Command("git", "-C", repoPath, "log", "--oneline")
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("git log failed: %v", err)
		}
		if !strings.Contains(string(output), "test commit") {
			t.Errorf("commit not found in log: %s", string(output))
		}
	})

	t.Run("idempotent with no changes", func(t *testing.T) {
		t.Parallel()
		// CopyHostHub provides a local repo with an initial commit and no upstream
		// push needed, matching the skipPush=true scenario.
		repoPath := lyxtest.CopyHostHub(t).Hub

		// Write a file and make the first CommitPush call.
		testFile := filepath.Join(repoPath, "test.txt")
		if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		err := boardengine.CommitPush(repoPath, []string{"test.txt"}, "first commit", true)
		if err != nil {
			t.Fatalf("CommitPush failed: %v", err)
		}

		// Capture commit count before the idempotency check.
		cmd := exec.Command("git", "-C", repoPath, "rev-list", "--count", "HEAD")
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("git rev-list failed: %v", err)
		}
		firstCount := strings.TrimSpace(string(output))

		// A second CommitPush with no new changes must not create an extra commit.
		err = boardengine.CommitPush(repoPath, []string{"test.txt"}, "second commit", true)
		if err != nil {
			t.Fatalf("CommitPush second call failed: %v", err)
		}

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
		t.Parallel()
		// CopyWeft gives cloneA a working tree with upstream tracking established
		// on main. Clone B is derived from the same bare so both share the remote.
		fixtureA := lyxtest.CopyWeft(t)
		cloneAPath := fixtureA.WeftPath
		bareRepoPath := fixtureA.Bare

		// Clone the same bare to produce clone B (one spawn; fixture-build spawns
		// — init, config, initial commit — are eliminated by CopyWeft).
		// Specify -b main because the bare's HEAD symref defaults to master while
		// the only branch is main; without -b, git clone checks out nothing.
		cloneBPath := filepath.Join(t.TempDir(), "cloneB")
		cmd := exec.Command("git", "clone", "-b", "main", bareRepoPath, cloneBPath)
		if err := cmd.Run(); err != nil {
			t.Fatalf("clone B failed: %v", err)
		}

		// Configure clone B's identity so git commit succeeds.
		for _, arg := range [][2]string{
			{"user.email", "test@example.com"},
			{"user.name", "Test User"},
		} {
			cmd = exec.Command("git", "-C", cloneBPath, "config", arg[0], arg[1])
			if err := cmd.Run(); err != nil {
				t.Fatalf("config %s failed: %v", arg[0], err)
			}
		}

		// Push a commit from clone B to make clone A's push a non-fast-forward.
		fileB := filepath.Join(cloneBPath, "fileB.txt")
		if err := os.WriteFile(fileB, []byte("from B"), 0o644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		err := boardengine.CommitPush(cloneBPath, []string{"fileB.txt"}, "commit from B", false)
		if err != nil {
			t.Fatalf("CommitPush on B failed: %v", err)
		}

		// Push from clone A (behind B by one commit); CommitPush must rebase and retry.
		fileA := filepath.Join(cloneAPath, "fileA.txt")
		if err := os.WriteFile(fileA, []byte("from A"), 0o644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		err = boardengine.CommitPush(cloneAPath, []string{"fileA.txt"}, "commit from A", false)
		if err != nil {
			t.Fatalf("CommitPush on A failed (should have succeeded via rebase): %v", err)
		}

		// Verify both commits appear in clone A's log after the rebase.
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
