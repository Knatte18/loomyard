// remote.go implements RemoteURL, a go-git-backed local config read of a named remote's configured URL.

package gitrepo

import (
	"errors"
	"fmt"

	"github.com/go-git/go-git/v5"
)

// errRemoteHasNoURLs is wrapped into RemoteURL's error when the named remote exists but its
// configured URL list is empty.
var errRemoteHasNoURLs = errors.New("remote has no configured URL")

// ErrRemoteNotConfigured is wrapped into RemoteURL's error when the repository has no usable remote of that name.
// The remote is absent, or it carries no URL.
// Any other RemoteURL error is a failure to read the repository's configuration.
var ErrRemoteNotConfigured = errors.New("remote is not configured")

// RemoteURL returns the configured URL of the remote named name, read via go-git's local config.
// It resolves state already on disk and spawns no git process,
// so it is not among the methods that `PATTERN-gitrepo-client-boundary` pins to run or runChecked.
// A remote that exists but carries no configured URL is an error, never an index panic.
func (r *Repo) RemoteURL(name string) (string, error) {
	return readGoGit(r, func(repo *git.Repository) (string, error) {
		remote, err := repo.Remote(name)
		if errors.Is(err, git.ErrRemoteNotFound) {
			return "", fmt.Errorf("gitrepo: read remote %q URL in %s: %w: %w", name, r.path, ErrRemoteNotConfigured, err)
		}
		if err != nil {
			return "", fmt.Errorf("gitrepo: read remote %q URL in %s: %w", name, r.path, err)
		}

		urls := remote.Config().URLs
		if len(urls) == 0 {
			return "", fmt.Errorf("gitrepo: read remote %q URL in %s: %w: %w", name, r.path, ErrRemoteNotConfigured, errRemoteHasNoURLs)
		}
		return urls[0], nil
	})
}
