// remote_integration_test.go covers RemoteURL against a real git repository with a configured origin remote.

package gitrepo_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

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
		})
	}
}
