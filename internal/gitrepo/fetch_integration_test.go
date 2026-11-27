// fetch_integration_test.go covers Repo.Fetch and the remote-failure paths of Fetch and Pull.
// Its tests run against real git repositories.
// Its fixtures are `newBareRemote`, `newRepoWithRemote` and `cloneFromBare` from push_test.go.

package gitrepo_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestFetch_RemoteAdvanced_UpdatesTrackingRefWithoutMovingHEAD checks that Fetch advances the remote-tracking ref.
// After a second clone pushes a commit the first clone lacks, Fetch moves the first clone's `@{u}` ref to the new tip.
// Fetch leaves local HEAD unchanged.
func TestFetch_RemoteAdvanced_UpdatesTrackingRefWithoutMovingHEAD(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	cloneAPath, repoA := newRepoWithRemote(t, container, "cloneA", bareRemote)
	writeFile(t, cloneAPath, "a.txt", "from A")
	commitAll(t, cloneAPath, "commit from A")
	if err := repoA.Push(); err != nil {
		t.Fatalf("Push() (establish upstream) error = %v; want nil", err)
	}
	headBefore := requireCurrentSHA(t, repoA)

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

	if headAfter := requireCurrentSHA(t, repoA); headAfter != headBefore {
		t.Errorf("local HEAD after Fetch() = %q; want unchanged %q (Fetch merges nothing)", headAfter, headBefore)
	}
}

// TestRemoteFailurePaths covers how Fetch and Pull report a repository whose remote is missing or unreachable.
// Bare `git fetch` with no remote configured exits 0 having done nothing, because it never needs a merge target.
// Fetch's error path is exercised with a remote whose URL cannot be reached.
// Each error must name the repo path and must never leak git's raw "fatal:"-prefixed stderr.
func TestRemoteFailurePaths(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// setup receives the repo directory after its first commit.
		setup   func(t *testing.T, dir string)
		call    func(repo *gitrepo.Repo) error
		wantErr bool
	}{
		{
			name:    "Fetch with no remote configured is a no-op",
			call:    (*gitrepo.Repo).Fetch,
			wantErr: false,
		},
		{
			name: "Fetch against an unreachable remote errors",
			setup: func(t *testing.T, dir string) {
				gitkit.MustRun(t, dir, "git", "remote", "add", "origin", filepath.Join(dir, "does-not-exist.git"))
			},
			call:    (*gitrepo.Repo).Fetch,
			wantErr: true,
		},
		{
			name:    "Pull with no remote configured errors",
			call:    (*gitrepo.Repo).Pull,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir, repo := newRepo(t)
			writeFile(t, dir, "a.txt", "content")
			commitAll(t, dir, "init")
			if tt.setup != nil {
				tt.setup(t, dir)
			}

			err := tt.call(repo)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("call error = %v; want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("call error = nil; want an error")
			}
			if !strings.Contains(err.Error(), dir) {
				t.Errorf("error = %q; want it to name the repo path %q", err, dir)
			}
			if strings.Contains(err.Error(), "fatal:") {
				t.Errorf("error = %q; must not leak raw git stderr", err)
			}
		})
	}
}
