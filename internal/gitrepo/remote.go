// remote.go implements RemoteURL, a go-git-backed local config read of a named remote's configured
// URL.

package gitrepo

import (
	"errors"
	"fmt"
)

// errRemoteHasNoURLs is wrapped into RemoteURL's error when the named remote exists but its
// configured URL list is empty.
var errRemoteHasNoURLs = errors.New("remote has no configured URL")

// RemoteURL returns the configured URL of the remote named name, read via go-git's local config
// rather than a git subprocess -- it resolves state already on disk and spawns no process, so it
// stays outside the gitrepo Client Boundary Invariant's pinned run/runChecked method list (see
// CONSTRAINTS.md).
// A remote that exists but carries no configured URL is an error, never an index panic.
func (r *Repo) RemoteURL(name string) (string, error) {
	repo, err := r.goGit()
	if err != nil {
		return "", err
	}

	r.goGitMu.RLock()
	defer r.goGitMu.RUnlock()

	remote, err := repo.Remote(name)
	if err != nil {
		return "", fmt.Errorf("gitrepo: read remote %q URL in %s: %w", name, r.path, err)
	}

	urls := remote.Config().URLs
	if len(urls) == 0 {
		return "", fmt.Errorf("gitrepo: read remote %q URL in %s: %w", name, r.path, errRemoteHasNoURLs)
	}
	return urls[0], nil
}
