//go:build integration

// deleteremotebranch_integration_test.go covers Repo.DeleteRemoteBranch and DeleteRemoteBranchLeased against a real
// bare remote and clones, reusing push_test.go's bare-remote/clone fixtures (newBareRemote,
// newRepoWithRemote, cloneFromBare) exactly as fetch_integration_test.go does.

package gitrepo_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// pushFeatureBranch creates branch in the clone with one commit, pushes it to origin, and returns the pushed tip SHA.
func pushFeatureBranch(t *testing.T, clonePath, branch string) string {
	t.Helper()

	gitkit.Git(t, clonePath, "checkout", "-b", branch)
	gitkit.CommitFile(t, clonePath, "feature.txt", "from "+branch, "commit on "+branch)
	gitkit.Git(t, clonePath, "push", "origin", branch)
	return gitkit.RevParse(t, clonePath, "HEAD")
}

// remoteHeads returns `git ls-remote --heads` output for the bare remote.
func remoteHeads(t *testing.T, container, bareRemote string) string {
	t.Helper()

	return gitkit.Git(t, container, "ls-remote", "--heads", bareRemote)
}

// TestDeleteRemoteBranch drives DeleteRemoteBranch and DeleteRemoteBranchLeased against one bare
// remote with two clones.
// The steps run serially in one order and share the remote: each pushes its own branch from clone A,
// and the unreachable-remote step runs last because it repoints clone A's origin at a missing path.
// The top-level test calls t.Parallel; no step does, because the steps share the fixture.
func TestDeleteRemoteBranch(t *testing.T) {
	t.Parallel()

	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, cloneAPath, "a.txt", "from A")
	commitAll(t, cloneAPath, "commit from A")
	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
	}

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		// The first of DeleteRemoteBranch's three named outcomes: deleting a branch that exists on
		// the remote returns (true, nil), and the remote no longer lists it afterward.
		{"an existing branch is deleted and reported true", func(t *testing.T) {
			const branch = "feature-x"
			pushFeatureBranch(t, cloneAPath, branch)

			// Confirm the bare remote holds the branch before the call, so the assertion below
			// proves deletion rather than absence.
			// The bare remote path is passed as ls-remote's explicit target since a bare repo has
			// no "origin" of its own to default to.
			if lsOut := remoteHeads(t, container, bareRemote); !strings.Contains(lsOut, "refs/heads/"+branch) {
				t.Fatalf("bare remote heads before delete = %q; want it to contain refs/heads/%s", lsOut, branch)
			}

			deleted, err := repoA.DeleteRemoteBranch("origin", branch)
			if err != nil {
				t.Fatalf("DeleteRemoteBranch(%q) error = %v; want nil", branch, err)
			}
			if !deleted {
				t.Errorf("DeleteRemoteBranch(%q) deleted = false; want true", branch)
			}

			if lsOut := remoteHeads(t, container, bareRemote); strings.Contains(lsOut, "refs/heads/"+branch) {
				t.Errorf("bare remote heads after delete = %q; want it to no longer contain refs/heads/%s", lsOut, branch)
			}
		}},
		// The idempotence contract: deleting a branch that is already absent from the remote
		// returns (false, nil).
		// This runs a real `git push --delete` against a ref that genuinely is not there, so it
		// observes git's own stderr rather than a fixture, and is the tripwire on the single pinned
		// substring "remote ref does not exist": a future git rewording makes this step fail
		// loudly instead of silently reclassifying the common case as an error.
		{"an absent branch is reported false with a nil error", func(t *testing.T) {
			const branch = "gone-branch"
			deleted, err := repoA.DeleteRemoteBranch("origin", branch)
			if err != nil {
				t.Fatalf("DeleteRemoteBranch(%q) error = %v; want nil (absent ref is idempotent success)", branch, err)
			}
			if deleted {
				t.Errorf("DeleteRemoteBranch(%q) deleted = true; want false (nothing was there to delete)", branch)
			}
		}},
		{"a lease at the remote tip deletes the branch", func(t *testing.T) {
			const branch = "leased-tip"
			tip := pushFeatureBranch(t, cloneAPath, branch)

			if err := repoA.DeleteRemoteBranchLeased("origin", branch, tip); err != nil {
				t.Fatalf("DeleteRemoteBranchLeased(%q, %s) error = %v; want nil", branch, tip, err)
			}
			if heads := remoteHeads(t, container, bareRemote); strings.Contains(heads, "refs/heads/"+branch) {
				t.Errorf("bare remote heads after leased delete = %q; want no refs/heads/%s", heads, branch)
			}
		}},
		// A lease whose SHA the remote branch has since moved past fails and leaves the branch at
		// its advanced tip.
		{"a stale lease errors and keeps the branch", func(t *testing.T) {
			const branch = "leased-stale"
			staleTip := pushFeatureBranch(t, cloneAPath, branch)

			cloneBPath, _ := cloneFromBare(t, container, "cloneB", bareRemote)
			gitkit.Git(t, cloneBPath, "checkout", branch)
			advanced := gitkit.CommitFile(t, cloneBPath, "advance.txt", "advanced", "advance "+branch)
			gitkit.Git(t, cloneBPath, "push", "origin", branch)

			if err := repoA.DeleteRemoteBranchLeased("origin", branch, staleTip); err == nil {
				t.Fatal("DeleteRemoteBranchLeased() with a stale lease error = nil; want an error")
			}
			heads := remoteHeads(t, container, bareRemote)
			if !strings.Contains(heads, advanced+"\trefs/heads/"+branch) {
				t.Errorf("bare remote heads after failed lease = %q; want %s at %s", heads, branch, advanced)
			}
		}},
		// An absent remote branch is a failed lease, not an idempotent success.
		{"a leased delete of an absent branch errors", func(t *testing.T) {
			tip := gitkit.RevParse(t, cloneAPath, "HEAD")

			if err := repoA.DeleteRemoteBranchLeased("origin", "gone-leased-branch", tip); err == nil {
				t.Fatal("DeleteRemoteBranchLeased() against an absent branch error = nil; want an error")
			}
		}},
		// A malformed expectSHA is rejected before the remote is touched.
		{"a malformed lease sha returns ErrInvalidSHA and touches nothing", func(t *testing.T) {
			const branch = "leased-malformed"
			pushFeatureBranch(t, cloneAPath, branch)

			err := repoA.DeleteRemoteBranchLeased("origin", branch, "not-a-sha")
			if !errors.Is(err, gitrepo.ErrInvalidSHA) {
				t.Fatalf("DeleteRemoteBranchLeased() error = %v; want ErrInvalidSHA", err)
			}
			if heads := remoteHeads(t, container, bareRemote); !strings.Contains(heads, "refs/heads/"+branch) {
				t.Errorf("bare remote heads = %q; want refs/heads/%s untouched", heads, branch)
			}
		}},
		// The failure outcome: a genuine failure returns a non-nil error and deleted == false.
		// The failure is induced without a network by pointing the clone's remote at a filesystem
		// path that does not exist.
		// The exact wording of git's failure is not pinned by any decision, so this deliberately
		// does not assert on it.
		// Runs last: it leaves origin pointing at the missing path.
		{"an unreachable remote returns an error", func(t *testing.T) {
			missing := filepath.Join(container, "does-not-exist.git")
			gitkit.Git(t, cloneAPath, "remote", "set-url", "origin", missing)

			deleted, err := repoA.DeleteRemoteBranch("origin", "feature-x")
			if err == nil {
				t.Fatal("DeleteRemoteBranch() against an unreachable remote error = nil; want an error")
			}
			if deleted {
				t.Errorf("DeleteRemoteBranch() against an unreachable remote deleted = true; want false")
			}
		}},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			return
		}
	}
}
