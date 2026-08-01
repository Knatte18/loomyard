//go:build integration

// fetch_integration_test.go covers Repo.Fetch against real git repositories,
// reusing push_test.go's bare-remote/clone fixtures (newBareRemote,
// newRepoWithRemote, cloneFromBare) since Fetch needs the same
// bare-remote-plus-clones shape those tests already build.

package gitrepo_test

import (
	"strings"
	"testing"
)

// TestFetch_RemoteAdvanced_UpdatesTrackingRefWithoutMovingHEAD asserts
// Fetch's whole point: after a second clone pushes a commit the first clone
// lacks, calling Fetch() on the first clone advances its remote-tracking ref
// (`@{u}`) to the new tip while leaving local HEAD completely untouched —
// unlike Pull, which would fast-forward HEAD itself.
func TestFetch_RemoteAdvanced_UpdatesTrackingRefWithoutMovingHEAD(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, cloneAPath, "a.txt", "from A")
	commitAll(t, cloneAPath, "commit from A")
	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
	}
	headBefore, err := repoA.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	cloneBPath, repoB := cloneFromBare(t, container, "cloneB", bareRemote)
	writeFile(t, cloneBPath, "b.txt", "from B")
	commitAll(t, cloneBPath, "commit from B")
	if err := repoB.Push(); err != nil {
		t.Fatalf("Push() from clone B error = %v; want nil", err)
	}
	remoteHead, stderr, code, err := runGit(t, bareRemote, "rev-parse", "main")
	if err != nil {
		t.Fatalf("git rev-parse main (bare) error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git rev-parse main (bare) exited %d: %s", code, stderr)
	}
	wantTrackingRef := strings.TrimSpace(remoteHead)

	if err := repoA.Fetch(); err != nil {
		t.Fatalf("Fetch() error = %v; want nil", err)
	}

	trackingRef, stderr, code, err := runGit(t, cloneAPath, "rev-parse", "@{u}")
	if err != nil {
		t.Fatalf("git rev-parse @{u} error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git rev-parse @{u} exited %d: %s", code, stderr)
	}
	if got := strings.TrimSpace(trackingRef); got != wantTrackingRef {
		t.Errorf("remote-tracking ref after Fetch() = %q; want it to advance to the new tip %q", got, wantTrackingRef)
	}

	headAfter, err := repoA.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	if headAfter != headBefore {
		t.Errorf("local HEAD after Fetch() = %q; want unchanged %q (Fetch merges nothing)", headAfter, headBefore)
	}
}

// TestFetch_NoRemoteConfigured_ErrorNamesRepoPath mirrors
// TestPull_NoRemoteConfigured_ErrorNamesRepoPath: a repo with no remote at
// all must still fail loudly, with an error naming the repo path and never
// leaking git's raw "fatal:"-prefixed stderr.
func TestFetch_NoRemoteConfigured_ErrorNamesRepoPath(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "content")
	commitAll(t, dir, "init")

	err := repo.Fetch()
	if err == nil {
		t.Fatal("Fetch() with no remote configured error = nil; want an error")
	}
	if !strings.Contains(err.Error(), dir) {
		t.Errorf("Fetch() error = %q; want it to name the repo path %q", err, dir)
	}
	if strings.Contains(err.Error(), "fatal:") {
		t.Errorf("Fetch() error = %q; must not leak raw git stderr", err)
	}
}
