//go:build integration

// deleteremotebranch_integration_test.go covers Repo.DeleteRemoteBranch against real git
// repositories, reusing push_test.go's bare-remote/clone fixtures (newBareRemote,
// newRepoWithRemote, cloneFromBare) exactly as fetch_integration_test.go does.

package gitrepo_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// TestDeleteRemoteBranch_ExistingBranch_DeletesAndReportsTrue asserts the first of
// DeleteRemoteBranch's three named outcomes: deleting a branch that exists on the remote returns
// (true, nil), and the remote no longer lists it afterward.
func TestDeleteRemoteBranch_ExistingBranch_DeletesAndReportsTrue(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, cloneAPath, "a.txt", "from A")
	commitAll(t, cloneAPath, "commit from A")
	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
	}

	const branch = "feature-x"
	if _, _, code, err := runGit(t, cloneAPath, "checkout", "-b", branch); err != nil || code != 0 {
		t.Fatalf("git checkout -b %s error = %v, code = %d", branch, err, code)
	}
	writeFile(t, cloneAPath, "feature.txt", "from feature-x")
	commitAll(t, cloneAPath, "commit on feature-x")
	if _, _, code, err := runGit(t, cloneAPath, "push", "origin", branch); err != nil || code != 0 {
		t.Fatalf("git push origin %s error = %v, code = %d", branch, err, code)
	}

	// Confirm the bare remote holds the branch before the call, so the
	// assertion below actually proves deletion rather than absence. The bare
	// remote path is passed as ls-remote's explicit target since a bare repo
	// has no "origin" of its own to default to.
	lsOut, _, code, err := runGit(t, container, "ls-remote", "--heads", bareRemote)
	if err != nil {
		t.Fatalf("git ls-remote --heads error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git ls-remote --heads exited %d", code)
	}
	if !strings.Contains(lsOut, "refs/heads/"+branch) {
		t.Fatalf("bare remote heads before delete = %q; want it to contain refs/heads/%s", lsOut, branch)
	}

	deleted, err := repoA.DeleteRemoteBranch("origin", branch)
	if err != nil {
		t.Fatalf("DeleteRemoteBranch(%q) error = %v; want nil", branch, err)
	}
	if !deleted {
		t.Errorf("DeleteRemoteBranch(%q) deleted = false; want true", branch)
	}

	lsOut, _, code, err = runGit(t, container, "ls-remote", "--heads", bareRemote)
	if err != nil {
		t.Fatalf("git ls-remote --heads error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git ls-remote --heads exited %d", code)
	}
	if strings.Contains(lsOut, "refs/heads/"+branch) {
		t.Errorf("bare remote heads after delete = %q; want it to no longer contain refs/heads/%s", lsOut, branch)
	}
}

// TestDeleteRemoteBranch_AbsentBranch_ReportsFalseWithNilError asserts DeleteRemoteBranch's
// idempotence contract: deleting a branch that is already absent from the remote returns (false,
// nil).
// This runs a real `git push --delete` against a ref that genuinely is not there, so the test
// observes git's own stderr rather than a fixture, and is the tripwire on the single pinned
// substring "remote ref does not exist": a future git rewording makes this test fail loudly instead
// of silently reclassifying the common case as an error.
func TestDeleteRemoteBranch_AbsentBranch_ReportsFalseWithNilError(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, cloneAPath, "a.txt", "from A")
	commitAll(t, cloneAPath, "commit from A")
	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
	}

	const branch = "gone-branch"
	deleted, err := repoA.DeleteRemoteBranch("origin", branch)
	if err != nil {
		t.Fatalf("DeleteRemoteBranch(%q) error = %v; want nil (absent ref is idempotent success)", branch, err)
	}
	if deleted {
		t.Errorf("DeleteRemoteBranch(%q) deleted = true; want false (nothing was there to delete)", branch)
	}
}

// TestDeleteRemoteBranch_UnreachableRemote_ReturnsError asserts DeleteRemoteBranch's failure
// outcome: a genuine failure returns a non-nil error and deleted == false.
// The failure is induced without a network by pointing the clone's remote at a filesystem path that
// does not exist. The exact wording of git's failure is not pinned by any decision, so this
// deliberately does not assert on it.
func TestDeleteRemoteBranch_UnreachableRemote_ReturnsError(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, cloneAPath, "a.txt", "from A")
	commitAll(t, cloneAPath, "commit from A")
	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
	}

	missing := filepath.Join(container, "does-not-exist.git")
	if _, _, code, err := runGit(t, cloneAPath, "remote", "set-url", "origin", missing); err != nil || code != 0 {
		t.Fatalf("git remote set-url origin error = %v, code = %d", err, code)
	}

	deleted, err := repoA.DeleteRemoteBranch("origin", "feature-x")
	if err == nil {
		t.Fatal("DeleteRemoteBranch() against an unreachable remote error = nil; want an error")
	}
	if deleted {
		t.Errorf("DeleteRemoteBranch() against an unreachable remote deleted = true; want false")
	}
}

// pushFeatureBranch creates branch in the clone with one commit, pushes it to origin, and returns the pushed tip SHA.
func pushFeatureBranch(t *testing.T, clonePath, branch string) string {
	t.Helper()

	if _, _, code, err := runGit(t, clonePath, "checkout", "-b", branch); err != nil || code != 0 {
		t.Fatalf("git checkout -b %s error = %v, code = %d", branch, err, code)
	}
	writeFile(t, clonePath, "feature.txt", "from "+branch)
	commitAll(t, clonePath, "commit on "+branch)
	if _, _, code, err := runGit(t, clonePath, "push", "origin", branch); err != nil || code != 0 {
		t.Fatalf("git push origin %s error = %v, code = %d", branch, err, code)
	}
	stdout, _, code, err := runGit(t, clonePath, "rev-parse", "HEAD")
	if err != nil || code != 0 {
		t.Fatalf("git rev-parse HEAD error = %v, code = %d", err, code)
	}
	return strings.TrimSpace(stdout)
}

// remoteHeads returns `git ls-remote --heads` output for the bare remote.
func remoteHeads(t *testing.T, container, bareRemote string) string {
	t.Helper()

	out, _, code, err := runGit(t, container, "ls-remote", "--heads", bareRemote)
	if err != nil || code != 0 {
		t.Fatalf("git ls-remote --heads error = %v, code = %d", err, code)
	}
	return out
}

// TestDeleteRemoteBranchLeased_LeaseAtTip_Deletes asserts a lease at the remote's current tip deletes the branch.
func TestDeleteRemoteBranchLeased_LeaseAtTip_Deletes(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, cloneAPath, "a.txt", "from A")
	commitAll(t, cloneAPath, "commit from A")
	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
	}

	const branch = "feature-x"
	tip := pushFeatureBranch(t, cloneAPath, branch)

	if err := repoA.DeleteRemoteBranchLeased("origin", branch, tip); err != nil {
		t.Fatalf("DeleteRemoteBranchLeased(%q, %s) error = %v; want nil", branch, tip, err)
	}
	if heads := remoteHeads(t, container, bareRemote); strings.Contains(heads, "refs/heads/"+branch) {
		t.Errorf("bare remote heads after leased delete = %q; want no refs/heads/%s", heads, branch)
	}
}

// TestDeleteRemoteBranchLeased_StaleLease_ErrorsAndKeepsBranch asserts a lease whose SHA the remote branch has since moved past fails and leaves the branch at its advanced tip.
func TestDeleteRemoteBranchLeased_StaleLease_ErrorsAndKeepsBranch(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, cloneAPath, "a.txt", "from A")
	commitAll(t, cloneAPath, "commit from A")
	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
	}

	const branch = "feature-x"
	staleTip := pushFeatureBranch(t, cloneAPath, branch)

	cloneBPath, _ := cloneFromBare(t, container, "cloneB", bareRemote)
	if _, _, code, err := runGit(t, cloneBPath, "checkout", branch); err != nil || code != 0 {
		t.Fatalf("git checkout %s in cloneB error = %v, code = %d", branch, err, code)
	}
	writeFile(t, cloneBPath, "advance.txt", "advanced")
	commitAll(t, cloneBPath, "advance "+branch)
	if _, _, code, err := runGit(t, cloneBPath, "push", "origin", branch); err != nil || code != 0 {
		t.Fatalf("git push origin %s from cloneB error = %v, code = %d", branch, err, code)
	}
	advancedOut, _, _, _ := runGit(t, cloneBPath, "rev-parse", "HEAD")
	advanced := strings.TrimSpace(advancedOut)

	if err := repoA.DeleteRemoteBranchLeased("origin", branch, staleTip); err == nil {
		t.Fatal("DeleteRemoteBranchLeased() with a stale lease error = nil; want an error")
	}
	heads := remoteHeads(t, container, bareRemote)
	if !strings.Contains(heads, advanced+"\trefs/heads/"+branch) {
		t.Errorf("bare remote heads after failed lease = %q; want %s at %s", heads, branch, advanced)
	}
}

// TestDeleteRemoteBranchLeased_AbsentBranch_ReturnsError asserts an absent remote branch is a failed lease, not an idempotent success.
func TestDeleteRemoteBranchLeased_AbsentBranch_ReturnsError(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, cloneAPath, "a.txt", "from A")
	commitAll(t, cloneAPath, "commit from A")
	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
	}
	tipOut, _, _, _ := runGit(t, cloneAPath, "rev-parse", "HEAD")

	if err := repoA.DeleteRemoteBranchLeased("origin", "gone-branch", strings.TrimSpace(tipOut)); err == nil {
		t.Fatal("DeleteRemoteBranchLeased() against an absent branch error = nil; want an error")
	}
}

// TestDeleteRemoteBranchLeased_MalformedSHA_ReturnsErrInvalidSHA asserts a malformed expectSHA is rejected before the remote is touched.
func TestDeleteRemoteBranchLeased_MalformedSHA_ReturnsErrInvalidSHA(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, cloneAPath, "a.txt", "from A")
	commitAll(t, cloneAPath, "commit from A")
	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
	}

	const branch = "feature-x"
	pushFeatureBranch(t, cloneAPath, branch)

	err := repoA.DeleteRemoteBranchLeased("origin", branch, "not-a-sha")
	if !errors.Is(err, gitrepo.ErrInvalidSHA) {
		t.Fatalf("DeleteRemoteBranchLeased() error = %v; want ErrInvalidSHA", err)
	}
	if heads := remoteHeads(t, container, bareRemote); !strings.Contains(heads, "refs/heads/"+branch) {
		t.Errorf("bare remote heads = %q; want refs/heads/%s untouched", heads, branch)
	}
}
