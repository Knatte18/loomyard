//go:build integration

// deleteremotebranch_integration_test.go covers Repo.DeleteRemoteBranch against real git
// repositories, reusing push_test.go's bare-remote/clone fixtures (newBareRemote,
// newRepoWithRemote, cloneFromBare) exactly as fetch_integration_test.go does.

package gitrepo_test

import (
	"path/filepath"
	"strings"
	"testing"
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
