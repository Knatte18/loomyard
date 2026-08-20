// parseownerrepo.go implements ParseOwnerRepo, a pure standard-library parser splitting a GitHub
// remote URL into its owner and repo segments.

package githubclient

import (
	"fmt"
	"strings"
)

// ParseOwnerRepo splits remoteURL into its owner and repo segments.
// It accepts exactly two shapes, each with an optional ".git" suffix and an optional single
// trailing slash: the SSH form (git@github.com:owner/repo.git) and the HTTPS form
// (https://github.com/owner/repo.git).
// This parser lives here, rather than in a generic URL-handling package, because this package owns
// GitHub knowledge; it does no network call and no go-github call, staying inside this package's own
// leaf allowlist (stdlib, go-github, golang.org/x/sys, internal/proc).
// A non-GitHub host, a URL with no owner/repo segment pair, and unparseable garbage each produce a
// distinct wrapped error so a caller can report which failure it hit.
func ParseOwnerRepo(remoteURL string) (owner, repo string, err error) {
	path, parseErr := ownerRepoPath(remoteURL)
	if parseErr != nil {
		return "", "", parseErr
	}

	path = strings.TrimSuffix(path, "/")
	path = strings.TrimSuffix(path, ".git")

	segments := strings.Split(path, "/")
	if len(segments) != 2 || segments[0] == "" || segments[1] == "" {
		return "", "", fmt.Errorf("githubclient: parse owner/repo from %q: no owner/repo segment pair found", remoteURL)
	}

	return segments[0], segments[1], nil
}

// ownerRepoPath strips remoteURL's GitHub-host prefix (either the SSH "git@github.com:" form or the
// HTTPS "https://github.com/" form) and returns the remaining owner/repo path, or an error naming
// which shape failed to match.
func ownerRepoPath(remoteURL string) (string, error) {
	switch {
	case strings.HasPrefix(remoteURL, "git@github.com:"):
		return strings.TrimPrefix(remoteURL, "git@github.com:"), nil
	case strings.HasPrefix(remoteURL, "https://github.com/"):
		return strings.TrimPrefix(remoteURL, "https://github.com/"), nil
	case strings.Contains(remoteURL, "://") || strings.Contains(remoteURL, "@"):
		return "", fmt.Errorf("githubclient: parse owner/repo from %q: not a github.com remote URL", remoteURL)
	default:
		return "", fmt.Errorf("githubclient: parse owner/repo from %q: unrecognized remote URL shape", remoteURL)
	}
}
