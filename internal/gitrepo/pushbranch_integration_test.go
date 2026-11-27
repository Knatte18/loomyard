// pushbranch_integration_test.go covers PushBranch, DropRemoteTracking and the trace line of a landed push or delete.
// It runs them against a real bare remote and clones, and reuses push_test.go's fixtures.

package gitrepo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestLandedPush_LeavesTraceLine is not parallel, nor are its subtests:
// each points the process-global logger at its own buffer.
func TestLandedPush_LeavesTraceLine(t *testing.T) {
	tiergate.Require(t, 1)
	tests := []struct {
		name string
		// onOrigin pushes the branch with plain git before act, which leaves no trace line.
		onOrigin bool
		act      func(repo *gitrepo.Repo, tip string) error
		// wantTip returns the tip field the line must hold for a branch whose local tip is tip.
		wantTip func(tip string) string
	}{
		{
			name: "PushBranch names the new tip",
			act: func(repo *gitrepo.Repo, _ string) error {
				return repo.PushBranch("origin", "feat", gitrepo.RefNamespace{})
			},
			wantTip: func(tip string) string { return "tip=" + tip },
		},
		{
			name:     "DeleteRemoteBranch names an empty tip",
			onOrigin: true,
			act: func(repo *gitrepo.Repo, _ string) error {
				_, err := repo.DeleteRemoteBranch("origin", "feat", gitrepo.RefNamespace{})
				return err
			},
			wantTip: func(string) string { return `tip=""` },
		},
		{
			name:     "DeleteRemoteBranchLeased names an empty tip",
			onOrigin: true,
			act: func(repo *gitrepo.Repo, tip string) error {
				return repo.DeleteRemoteBranchLeased("origin", "feat", tip, gitrepo.RefNamespace{})
			},
			wantTip: func(string) string { return `tip=""` },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.CaptureVerbose(t)
			_, _, clonePath, repo := newPushedMainClone(t)
			tip := commitOnUncheckedBranch(t, clonePath, "feat")
			if tt.onOrigin {
				gitkit.Git(t, clonePath, "push", "origin", "feat")
			}

			if err := tt.act(repo, tip); err != nil {
				t.Fatalf("act error = %v; want nil", err)
			}
			want := tt.wantTip(tip)
			for _, line := range strings.Split(logs.String(), "\n") {
				if strings.Contains(line, "gitrepo: push landed") && strings.Contains(line, "ref=refs/heads/feat") {
					if !strings.Contains(line, "remote=origin") || !strings.Contains(line, want) {
						t.Errorf("push trace line = %q; want it to name remote=origin and %s", line, want)
					}
					return
				}
			}
			t.Errorf("logs = %q; want a push landed line naming ref=refs/heads/feat", logs.String())
		})
	}
}

// newPushedMainClone returns a clone whose main is pushed to a fresh bare remote, with main checked out.
func newPushedMainClone(t *testing.T) (container, bareRemote, clonePath string, repo *gitrepo.Repo) {
	t.Helper()

	container = t.TempDir()
	bareRemote = newBareRemote(t, container)
	clonePath, repo = newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, clonePath, "a.txt", "from A")
	commitAll(t, clonePath, "commit from A")
	if err := repo.Push(gitrepo.RefNamespace{}); err != nil {
		t.Fatalf("Push() (establish main) error = %v; want nil", err)
	}
	return container, bareRemote, clonePath, repo
}

// commitOnUncheckedBranch creates branch from main with one commit and leaves main checked out, returning the branch tip.
func commitOnUncheckedBranch(t *testing.T, clonePath, branch string) string {
	t.Helper()

	gitkit.Git(t, clonePath, "checkout", "-b", branch)
	tip := gitkit.CommitFile(t, clonePath, branch+".txt", branch, "commit on "+branch)
	gitkit.Git(t, clonePath, "checkout", "main")
	return tip
}

func TestPushBranch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("an unchecked-out branch ahead of origin lands and moves its tracking ref", func(t *testing.T) {
		t.Parallel()

		_, bareRemote, clonePath, repo := newPushedMainClone(t)
		tip := commitOnUncheckedBranch(t, clonePath, "feat")

		if err := repo.PushBranch("origin", "feat", gitrepo.RefNamespace{}); err != nil {
			t.Fatalf("PushBranch() error = %v; want nil", err)
		}
		if got := remoteBranchSHA(t, bareRemote, "feat"); got != tip {
			t.Errorf("origin feat = %s; want %s", got, tip)
		}
		if got := gitkit.RevParse(t, clonePath, "refs/remotes/origin/feat"); got != tip {
			t.Errorf("tracking ref origin/feat = %s; want %s", got, tip)
		}
	})

	t.Run("a branch origin has diverged from is rejected and origin keeps its tip", func(t *testing.T) {
		t.Parallel()

		container, bareRemote, clonePath, repo := newPushedMainClone(t)
		commitOnUncheckedBranch(t, clonePath, "feat")
		if err := repo.PushBranch("origin", "feat", gitrepo.RefNamespace{}); err != nil {
			t.Fatalf("PushBranch() (first push) error = %v; want nil", err)
		}

		cloneBPath, _ := cloneFromBare(t, container, "cloneB", bareRemote)
		gitkit.Git(t, cloneBPath, "checkout", "feat")
		advanced := gitkit.CommitFile(t, cloneBPath, "advance.txt", "advanced", "advance feat")
		gitkit.Git(t, cloneBPath, "push", "origin", "feat")

		gitkit.CommitFileOnBranch(t, clonePath, "feat", "local.txt", "local", "local commit on feat")
		gitkit.Git(t, clonePath, "checkout", "main")

		err := repo.PushBranch("origin", "feat", gitrepo.RefNamespace{})
		if !errors.Is(err, gitrepo.ErrPushRejected) {
			t.Fatalf("PushBranch() error = %v; want it to match ErrPushRejected", err)
		}
		if got := remoteBranchSHA(t, bareRemote, "feat"); got != advanced {
			t.Errorf("origin feat = %s; want it unchanged at %s", got, advanced)
		}
	})
}

func TestDropRemoteTracking(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	_, _, clonePath, repo := newPushedMainClone(t)
	commitOnUncheckedBranch(t, clonePath, "feat")
	gitkit.Git(t, clonePath, "push", "-u", "origin", "feat")
	if _, _, code, _ := runGit(t, clonePath, "config", "--get", "branch.feat.merge"); code != 0 {
		t.Fatalf("branch.feat.merge is unset before the drop; want it configured by push -u")
	}

	for _, call := range []string{"a configured branch", "the same branch again"} {
		if err := repo.DropRemoteTracking("origin", "feat"); err != nil {
			t.Fatalf("DropRemoteTracking() on %s error = %v; want nil", call, err)
		}
		if _, _, code, _ := runGit(t, clonePath, "show-ref", "--verify", "--quiet", "refs/remotes/origin/feat"); code == 0 {
			t.Errorf("refs/remotes/origin/feat still exists after the drop on %s", call)
		}
		for _, key := range []string{"branch.feat.remote", "branch.feat.merge"} {
			if _, _, code, _ := runGit(t, clonePath, "config", "--get", key); code == 0 {
				t.Errorf("%s still set after the drop on %s", key, call)
			}
		}
	}
}
