//go:build integration

// pull_test.go covers Repo.Pull against real git repositories, reusing
// push_test.go's bare-remote/clone fixtures (newBareRemote, newRepoWithRemote,
// cloneFromBare) since a fast-forward pull needs the same
// bare-remote-plus-clones shape the push rebase-retry tests already build.

package gitrepo_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPull_RemoteAdvanced_FastForwards asserts the ordinary case: the
// remote has commits this clone lacks, and Pull() fast-forwards the local
// branch to include them, with the file content landing on disk.
func TestPull_RemoteAdvanced_FastForwards(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, cloneAPath, "a.txt", "from A")
	commitAll(t, cloneAPath, "commit from A")
	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
	}

	cloneBPath, repoB := cloneFromBare(t, container, "cloneB", bareRemote)
	writeFile(t, cloneBPath, "b.txt", "from B")
	commitAll(t, cloneBPath, "commit from B")
	if err := repoB.Push(); err != nil {
		t.Fatalf("Push() from clone B error = %v; want nil", err)
	}

	// Clone A is now behind the remote; Pull() must fast-forward it to
	// include B's commit.
	if err := repoA.Pull(); err != nil {
		t.Fatalf("Pull() error = %v; want nil", err)
	}

	got, err := os.ReadFile(filepath.Join(cloneAPath, "b.txt"))
	if err != nil {
		t.Fatalf("read b.txt after Pull() error = %v; want the file fast-forwarded in", err)
	}
	if string(got) != "from B" {
		t.Errorf("b.txt content after Pull() = %q; want %q", got, "from B")
	}
}

// TestPull_LocalDiverged_ReturnsError asserts the fast-forward-only
// contract: a local branch with its own unpushed commit, pulling from a
// remote that has diverged from underneath it, must be refused with an
// error rather than folded into a merge commit — and local history must be
// left untouched.
func TestPull_LocalDiverged_ReturnsError(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, cloneAPath, "shared.txt", "base\n")
	commitAll(t, cloneAPath, "init")
	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
	}

	cloneBPath, repoB := cloneFromBare(t, container, "cloneB", bareRemote)
	writeFile(t, cloneBPath, "shared.txt", "from B\n")
	commitAll(t, cloneBPath, "commit from B")
	if err := repoB.Push(); err != nil {
		t.Fatalf("Push() from clone B error = %v; want nil", err)
	}

	// Clone A commits its own conflicting local change without pushing,
	// diverging from the remote it is about to try to Pull from.
	writeFile(t, cloneAPath, "shared.txt", "from A\n")
	commitAll(t, cloneAPath, "commit from A")
	localHead, err := repoA.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	err = repoA.Pull()
	if err == nil {
		t.Fatal("Pull() on a diverged branch error = nil; want an error (fast-forward-only refuses to merge)")
	}
	if strings.Contains(err.Error(), "fatal:") {
		t.Errorf("Pull() error = %q; must not leak raw git stderr", err)
	}

	headAfter, err := repoA.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	if headAfter != localHead {
		t.Errorf("HEAD after refused Pull() = %q; want unchanged %q (local history must not move)", headAfter, localHead)
	}
}

// TestPull_NoRemoteConfigured_ErrorNamesRepoPath asserts that a repo with no
// remote at all still fails loudly, and that the error names the repo path
// (per Pull's documented error style) without leaking git's raw stderr.
func TestPull_NoRemoteConfigured_ErrorNamesRepoPath(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "content")
	commitAll(t, dir, "init")

	err := repo.Pull()
	if err == nil {
		t.Fatal("Pull() with no remote configured error = nil; want an error")
	}
	if !strings.Contains(err.Error(), dir) {
		t.Errorf("Pull() error = %q; want it to name the repo path %q", err, dir)
	}
	if strings.Contains(err.Error(), "fatal:") {
		t.Errorf("Pull() error = %q; must not leak raw git stderr", err)
	}
}
