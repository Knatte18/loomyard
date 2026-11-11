// destroy_landedwork_integration_test.go drives the pair-warp ownership kind.
// It also drives the untracked-tracked-at ownership kind and the unlanded-work dirtiness kind.
// It runs them against real throwaway git repositories, with hermetic git from testmain_test.go.
// It uses an empty branch_prefix, so the warp branch is the bare slug.
// No hub is involved: the gate reads only the repository named by repoDir.

package fabricengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// landedWorkRepo returns a repository whose main branch holds one commit, plus a bare remote it is wired to.
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

func TestOwnedUntrackedTrackedAt_RefusesATargetTheIndexTracksOrTheCommitDoesNot(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	repo, _ := landedWorkRepo(t)
	gitkit.CommitFile(t, repo, "indexed.txt", "indexed", "indexed")
	gitkit.Git(t, repo, "switch", "-c", "other")
	otherTip := gitkit.CommitFile(t, repo, "theirs.txt", "theirs", "theirs")
	gitkit.Git(t, repo, "switch", "main")
	for _, name := range []string{"theirs.txt", "stray.txt"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte("local\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	tests := []struct {
		name       string
		target     string
		wantOwned  bool
		wantReason string
	}{
		{"UntrackedFileTheCommitTracksIsOwned", "theirs.txt", true, ""},
		{"FileTheIndexTracksIsRefused", "indexed.txt", false, "is tracked in"},
		{"UntrackedFileTheCommitDoesNotTrackIsRefused", "stray.txt", false, "is not tracked at"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			owned, reason := resolvePathOwnership(ownedUntrackedTrackedAt(repo, otherTip), filepath.Join(repo, tt.target))
			if owned != tt.wantOwned || !strings.Contains(reason, tt.wantReason) {
				t.Errorf("resolvePathOwnership(%s) = %v, %q; want %v with a reason containing %q", tt.target, owned, reason, tt.wantOwned, tt.wantReason)
			}
		})
	}
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

func TestLandedWork_SquashLandedBranchPasses(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

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

func TestLandedWork_CheckedOutBranchRefused(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	repo, _ := landedWorkRepo(t)
	gitkit.Git(t, repo, "switch", "-c", "task")
	gitkit.Git(t, repo, "push", "origin", "task")

	err := landedWorkCheck(repo, "task")
	assertRefusalCheck(t, err, CheckDirtiness)
	if !strings.Contains(err.Error(), "checked out") {
		t.Fatalf("refusal = %q; want a checked-out reason", err.Error())
	}
}
