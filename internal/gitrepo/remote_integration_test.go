// remote_integration_test.go covers RemoteURL and RemoteFetchRefspecs.
// It runs them against a real git repository with a configured origin remote.

package gitrepo_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRemoteFetchRefspecs covers RemoteFetchRefspecs against a repository whose origin keeps git's default refspec.
// It also covers one configured with the namespaced refspecs, and a remote name the repository does not have.
func TestRemoteFetchRefspecs(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name    string
		remote  string
		config  [][]string
		want    []string
		wantErr bool
	}{
		{name: "a default clone answers git's refspec", remote: "origin", want: []string{"+refs/heads/*:refs/remotes/origin/*"}},
		{
			name:   "a namespaced store answers both refspecs",
			remote: "origin",
			config: [][]string{
				{"--replace-all", "remote.origin.fetch", "+refs/lym/heads/*:refs/remotes/origin/*"},
				{"--add", "remote.origin.fetch", "+refs/lym/tags/*:refs/tags/*"},
			},
			want: []string{"+refs/lym/heads/*:refs/remotes/origin/*", "+refs/lym/tags/*:refs/tags/*"},
		},
		{name: "an unconfigured remote returns ErrRemoteNotConfigured", remote: "upstream", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			container := t.TempDir()
			repoPath, repo := newRepoWithRemote(t, container, "clone", newBareRemote(t, container))
			for _, setting := range tt.config {
				gitkit.Git(t, repoPath, append([]string{"config"}, setting...)...)
			}

			got, err := repo.RemoteFetchRefspecs(tt.remote)
			if tt.wantErr {
				if !errors.Is(err, gitrepo.ErrRemoteNotConfigured) {
					t.Fatalf("RemoteFetchRefspecs(%s) error = %v; want one wrapping ErrRemoteNotConfigured", tt.remote, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("RemoteFetchRefspecs(%s) error = %v; want nil", tt.remote, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RemoteFetchRefspecs(%s) = %v; want %v", tt.remote, got, tt.want)
			}
		})
	}
}

// TestRemoteURL covers RemoteURL against a repository whose only remote is origin.
// The configured origin's URL comes back exactly as git reports it.
// A requested remote the repository does not have returns an empty string and an error naming it.
// It does not silently resolve to a different configured remote.
func TestRemoteURL(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	repoPath, repo := newRepoWithRemote(t, container, "clone", bareRemote)
	writeFile(t, repoPath, "a.txt", "content")
	commitAll(t, repoPath, "init")

	tests := []struct {
		name    string
		remote  string
		want    string
		wantErr bool
	}{
		{name: "configured origin returns the URL verbatim", remote: "origin", want: bareRemote},
		{name: "misspelled remote returns an error naming it", remote: "upstream", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := repo.RemoteURL(tt.remote)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RemoteURL(%s) = (%q, %v); wantErr %v", tt.remote, got, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("RemoteURL(%s) = %q; want %q", tt.remote, got, tt.want)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.remote) {
				t.Errorf("RemoteURL(%s) error = %q; want it to name the requested remote %q", tt.remote, err.Error(), tt.remote)
			}
			if tt.wantErr && !errors.Is(err, gitrepo.ErrRemoteNotConfigured) {
				t.Errorf("RemoteURL(%s) error = %v; want one wrapping ErrRemoteNotConfigured", tt.remote, err)
			}
		})
	}
}
