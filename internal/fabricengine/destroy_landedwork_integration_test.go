//go:build integration

// destroy_landedwork_integration_test.go drives the pair-warp ownership kind and the unlanded-work
// dirtiness kind against real throwaway git repositories (hermetic git via testmain_test.go, empty
// branch_prefix so the warp branch is the bare slug). No hub is involved: the gate reads only the
// repository named by repoDir.

package fabricengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

func landedWorkGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitexec.Run(args, dir)
	if err != nil {
		t.Fatalf("git %v in %s: %v", args, dir, err)
	}
	return out
}

// landedWorkRepo returns a repository whose main branch holds one commit, plus a bare remote it
// is wired to.
func landedWorkRepo(t *testing.T) (repo, remote string) {
	t.Helper()
	remote = t.TempDir()
	landedWorkGit(t, remote, "init", "--bare", "-b", "main")
	repo = t.TempDir()
	landedWorkGit(t, repo, "init", "-b", "main")
	landedWorkGit(t, repo, "config", "user.email", "t@example.com")
	landedWorkGit(t, repo, "config", "user.name", "t")
	landedWorkGit(t, repo, "remote", "add", "origin", remote)
	landedWorkGit(t, repo, "commit", "--allow-empty", "-m", "base")
	return repo, remote
}

func landedWorkCommitFile(t *testing.T, repo, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, name), []byte(name), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	landedWorkGit(t, repo, "add", name)
	landedWorkGit(t, repo, "commit", "-m", name)
}

func landedWorkCheck(repo, branch string) error {
	return checkBranchRequest(branchRequest{
		what:      "test delete warp branch",
		repoDir:   repo,
		branch:    branch,
		ownership: ownedPairWarpBranch(branch, "main"),
		dirtiness: dirtyUnlandedWork("main"),
		force:     true,
	})
}

func TestLandedWork_PushedBranchPasses(t *testing.T) {
	repo, _ := landedWorkRepo(t)
	landedWorkGit(t, repo, "switch", "-c", "task")
	landedWorkGit(t, repo, "commit", "--allow-empty", "-m", "work")
	landedWorkGit(t, repo, "push", "origin", "task")
	landedWorkGit(t, repo, "switch", "main")

	if err := landedWorkCheck(repo, "task"); err != nil {
		t.Fatalf("pushed branch refused: %v", err)
	}
}

func TestLandedWork_SquashLandedBranchPasses(t *testing.T) {
	repo, _ := landedWorkRepo(t)
	landedWorkGit(t, repo, "switch", "-c", "task")
	landedWorkCommitFile(t, repo, "work.txt")
	landedWorkGit(t, repo, "switch", "main")
	landedWorkGit(t, repo, "merge", "--squash", "task")
	landedWorkGit(t, repo, "commit", "-m", "landed")

	if err := landedWorkCheck(repo, "task"); err != nil {
		t.Fatalf("squash-landed branch refused: %v", err)
	}
}

func TestLandedWork_UnlandedBranchRefusedEvenWithForce(t *testing.T) {
	repo, _ := landedWorkRepo(t)
	landedWorkGit(t, repo, "switch", "-c", "task")
	landedWorkCommitFile(t, repo, "one.txt")
	landedWorkCommitFile(t, repo, "two.txt")
	landedWorkGit(t, repo, "switch", "main")

	err := landedWorkCheck(repo, "task")
	assertRefusalCheck(t, err, CheckDirtiness)
	msg := err.Error()
	if !strings.Contains(msg, "2 commit(s)") || !strings.Contains(msg, "git branch -D task") {
		t.Fatalf("refusal = %q; want the commit count 2 and the hand remedy", msg)
	}
}

func TestLandedWork_CheckedOutBranchRefused(t *testing.T) {
	repo, _ := landedWorkRepo(t)
	landedWorkGit(t, repo, "switch", "-c", "task")
	landedWorkGit(t, repo, "push", "origin", "task")

	err := landedWorkCheck(repo, "task")
	assertRefusalCheck(t, err, CheckDirtiness)
	if !strings.Contains(err.Error(), "checked out") {
		t.Fatalf("refusal = %q; want a checked-out reason", err.Error())
	}
}
