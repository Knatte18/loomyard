// push_destination_integration_test.go covers where a push lands against real git repositories.
// It covers the remote a default push picks from the branch's and repository's settings, and the pushes that record no remote-tracking ref.

package gitrepo_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// featureClone returns a clone of a published bare remote, checked out on a new branch "feature" with one unpushed commit, and that commit.
func featureClone(t *testing.T) (clone, origin, commit string) {
	t.Helper()
	clone, origin = publishedClone(t)
	gitkit.Git(t, clone, "checkout", "-b", "feature")
	return clone, origin, gitkit.CommitFile(t, clone, "feature.txt", "feature", "feature commit")
}

func TestPushRebaseFree_ADefaultPushGoesToTheRemoteTheSettingsPick(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	tests := []struct {
		name string
		// config is the `git config` arguments run in the clone before the push.
		config     [][]string
		wantRemote string
	}{
		{"BranchPushRemote", [][]string{{"branch.feature.pushRemote", "second"}}, "second"},
		{"BranchPushRemoteOverPushDefault", [][]string{{"branch.feature.pushRemote", "second"}, {"remote.pushDefault", "origin"}}, "second"},
		{"PushDefault", [][]string{{"remote.pushDefault", "second"}}, "second"},
		{"PushDefaultOverUpstreamRemote", [][]string{{"remote.pushDefault", "second"}, {"branch.feature.remote", "origin"}, {"branch.feature.merge", "refs/heads/feature"}}, "second"},
		{"UpstreamRemote", [][]string{{"branch.feature.remote", "second"}, {"branch.feature.merge", "refs/heads/feature"}}, "second"},
		{"OriginWhenNothingIsSet", nil, "origin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			clone, origin, commit := featureClone(t)
			second := newBareRemote(t, t.TempDir())
			gitkit.Git(t, clone, "remote", "add", "second", second)
			for _, setting := range tt.config {
				gitkit.Git(t, clone, append([]string{"config"}, setting...)...)
			}

			if err := gitrepo.New(clone).PushRebaseFree(); err != nil {
				t.Fatalf("PushRebaseFree() error = %v; want nil", err)
			}

			bares := map[string]string{"origin": origin, "second": second}
			for remote, bare := range bares {
				wantCommit := ""
				if remote == tt.wantRemote {
					wantCommit = commit
				}
				if got := branchSHA(t, bare, "refs/heads/feature"); got != wantCommit {
					t.Errorf("%s's feature = %q; want %q", remote, got, wantCommit)
				}
				if got := branchSHA(t, clone, "refs/remotes/"+remote+"/feature"); got != wantCommit {
					t.Errorf("refs/remotes/%s/feature = %q; want %q", remote, got, wantCommit)
				}
			}
		})
	}
}

func TestPush_AFailedRecordAfterALandedTransferStillSucceeds(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	tests := []struct {
		name   string
		branch string
		// prepare readies the clone and returns the push to run and the commit origin's branch holds afterwards, empty for a deleted branch.
		prepare func(t *testing.T, clone string, repo *gitrepo.Repo) (call func() error, want string)
	}{
		{"PushBranch", "feature", func(t *testing.T, clone string, repo *gitrepo.Repo) (func() error, string) {
			gitkit.Git(t, clone, "checkout", "-b", "feature")
			want := gitkit.CommitFile(t, clone, "feature.txt", "feature", "feature commit")
			return func() error { return repo.PushBranch("origin", "feature") }, want
		}},
		{"DeleteRemoteBranch", "doomed", func(t *testing.T, clone string, repo *gitrepo.Repo) (func() error, string) {
			gitkit.Git(t, clone, "push", "origin", "main:doomed")
			return func() error {
				deleted, err := repo.DeleteRemoteBranch("origin", "doomed")
				if err == nil && !deleted {
					return fmt.Errorf("DeleteRemoteBranch() deleted = false; want true")
				}
				return err
			}, ""
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			clone, origin := publishedClone(t)
			repo := gitrepo.New(clone)
			call, want := tt.prepare(t, clone, repo)
			commonDir, err := repo.CommonDir()
			if err != nil {
				t.Fatalf("CommonDir() error = %v", err)
			}
			// A leftover lock file makes git refuse to write the tracking ref, so the record step fails after the transfer landed.
			lockFile := filepath.Join(commonDir, "refs", "remotes", "origin", tt.branch+".lock")
			if err := os.WriteFile(lockFile, nil, 0o644); err != nil {
				t.Fatalf("write %s: %v", lockFile, err)
			}

			if err := call(); err != nil {
				t.Errorf("%s error = %v; want nil, since the transfer landed", tt.name, err)
			}
			if got := branchSHA(t, origin, "refs/heads/"+tt.branch); got != want {
				t.Errorf("origin's %s = %q; want %q", tt.branch, got, want)
			}
		})
	}
}

func TestPushBranch_APushWithNoTrackingRefToWriteLandsAndRecordsNothing(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	tests := []struct {
		name string
		// remoteFor readies the clone and returns the remote argument of the push.
		remoteFor func(t *testing.T, clone, bare string) string
	}{
		{"RemoteGivenAsAPath", func(t *testing.T, clone, bare string) string { return bare }},
		{"RemoteWhoseFetchRefspecMapsOnlyAnotherBranch", func(t *testing.T, clone, bare string) string {
			gitkit.Git(t, clone, "remote", "add", "-t", "other", "second", bare)
			return "second"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			clone, _, commit := featureClone(t)
			second := newBareRemote(t, t.TempDir())
			remote := tt.remoteFor(t, clone, second)
			trackingBefore := gitkit.Git(t, clone, "for-each-ref", "refs/remotes")

			if err := gitrepo.New(clone).PushBranch(remote, "feature"); err != nil {
				t.Fatalf("PushBranch(%q, feature) error = %v; want nil", remote, err)
			}

			if got := branchSHA(t, second, "refs/heads/feature"); got != commit {
				t.Errorf("the pushed-to remote's feature = %q; want %q", got, commit)
			}
			if got := gitkit.Git(t, clone, "for-each-ref", "refs/remotes"); got != trackingBefore {
				t.Errorf("remote-tracking refs after the push = %q; want them unchanged at %q", got, trackingBefore)
			}
		})
	}
}
