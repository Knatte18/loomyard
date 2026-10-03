//go:build integration

// destroy_landedwork_integration_test.go drives the pair-warp ownership kind and the unlanded-work
// dirtiness kind against real throwaway git repositories (hermetic git via testmain_test.go, empty
// branch_prefix so the warp branch is the bare slug). No hub is involved: the gate reads only the
// repository named by repoDir.

package fabricengine

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
)

// landedWorkRepo returns a repository whose main branch holds one commit, plus a bare remote it
// is wired to.
func landedWorkRepo(t *testing.T) (repo, remote string) {
	t.Helper()
	remote = t.TempDir()
	gitkit.Git(t, remote, "init", "--bare", "-b", "main")
	repo = t.TempDir()
	gitkit.Git(t, repo, "init", "-b", "main")
	gitkit.Git(t, repo, "config", "user.email", "t@example.com")
	gitkit.Git(t, repo, "config", "user.name", "t")
	gitkit.Git(t, repo, "remote", "add", "origin", remote)
	gitkit.Git(t, repo, "commit", "--allow-empty", "-m", "base")
	return repo, remote
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
	gitkit.Git(t, repo, "switch", "-c", "task")
	gitkit.Git(t, repo, "commit", "--allow-empty", "-m", "work")
	gitkit.Git(t, repo, "push", "origin", "task")
	gitkit.Git(t, repo, "switch", "main")

	if err := landedWorkCheck(repo, "task"); err != nil {
		t.Fatalf("pushed branch refused: %v", err)
	}
}

func TestLandedWork_SquashLandedBranchPasses(t *testing.T) {
	repo, _ := landedWorkRepo(t)
	gitkit.Git(t, repo, "switch", "-c", "task")
	gitkit.CommitFile(t, repo, "work.txt", "work.txt", "work.txt")
	gitkit.Git(t, repo, "switch", "main")
	gitkit.Git(t, repo, "merge", "--squash", "task")
	gitkit.Git(t, repo, "commit", "-m", "landed")

	if err := landedWorkCheck(repo, "task"); err != nil {
		t.Fatalf("squash-landed branch refused: %v", err)
	}
}

func TestLandedWork_UnlandedBranchRefusedEvenWithForce(t *testing.T) {
	repo, _ := landedWorkRepo(t)
	gitkit.Git(t, repo, "switch", "-c", "task")
	gitkit.CommitFile(t, repo, "one.txt", "one.txt", "one.txt")
	gitkit.CommitFile(t, repo, "two.txt", "two.txt", "two.txt")
	gitkit.Git(t, repo, "switch", "main")

	err := landedWorkCheck(repo, "task")
	assertRefusalCheck(t, err, CheckDirtiness)
	msg := err.Error()
	if !strings.Contains(msg, "2 commit(s)") || !strings.Contains(msg, "git branch -D task") {
		t.Fatalf("refusal = %q; want the commit count 2 and the hand remedy", msg)
	}
}

func TestLandedWork_CheckedOutBranchRefused(t *testing.T) {
	repo, _ := landedWorkRepo(t)
	gitkit.Git(t, repo, "switch", "-c", "task")
	gitkit.Git(t, repo, "push", "origin", "task")

	err := landedWorkCheck(repo, "task")
	assertRefusalCheck(t, err, CheckDirtiness)
	if !strings.Contains(err.Error(), "checked out") {
		t.Fatalf("refusal = %q; want a checked-out reason", err.Error())
	}
}
