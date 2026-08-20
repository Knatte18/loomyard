//go:build integration

// remote_integration_test.go covers RemoteURL against real git repositories, reusing
// gitrepo_test.go's fixture helpers (newRepo, writeFile, commitAll) and push_test.go's
// newBareRemote/newRepoWithRemote helpers for a repo carrying a configured origin remote.

package gitrepo_test

import (
	"strings"
	"testing"
)

// TestRemoteURL_ConfiguredOrigin_ReturnsURLVerbatim asserts RemoteURL returns a configured origin
// remote's URL exactly as git reports it.
func TestRemoteURL_ConfiguredOrigin_ReturnsURLVerbatim(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	repoPath, repo := newRepoWithRemote(t, container, "clone", bareRemote)
	writeFile(t, repoPath, "a.txt", "content")
	commitAll(t, repoPath, "init")

	got, err := repo.RemoteURL("origin")
	if err != nil {
		t.Fatalf("RemoteURL(origin) error = %v; want nil", err)
	}
	if got != bareRemote {
		t.Errorf("RemoteURL(origin) = %q; want %q", got, bareRemote)
	}
}

// TestRemoteURL_NoSuchRemote_ReturnsError asserts RemoteURL returns an error, not an empty string,
// when the repo has no remote at all.
func TestRemoteURL_NoSuchRemote_ReturnsError(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "content")
	commitAll(t, dir, "init")

	got, err := repo.RemoteURL("origin")
	if err == nil {
		t.Fatalf("RemoteURL(origin) on a repo with no remotes = (%q, nil); want an error", got)
	}
	if got != "" {
		t.Errorf("RemoteURL(origin) on a repo with no remotes = %q; want empty string alongside the error", got)
	}
}

// TestRemoteURL_MisspelledRemoteName_ReturnsErrorNamingIt asserts RemoteURL returns an error naming
// the requested remote when the caller misspells it, rather than silently resolving to a different
// configured remote.
func TestRemoteURL_MisspelledRemoteName_ReturnsErrorNamingIt(t *testing.T) {
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)

	repoPath, repo := newRepoWithRemote(t, container, "clone", bareRemote)
	writeFile(t, repoPath, "a.txt", "content")
	commitAll(t, repoPath, "init")

	got, err := repo.RemoteURL("upstream")
	if err == nil {
		t.Fatalf("RemoteURL(upstream) on a repo with only origin configured = (%q, nil); want an error", got)
	}
	if got != "" {
		t.Errorf("RemoteURL(upstream) = %q; want empty string alongside the error", got)
	}
	if !strings.Contains(err.Error(), "upstream") {
		t.Errorf("RemoteURL(upstream) error = %q; want it to name the requested remote %q", err.Error(), "upstream")
	}
}
