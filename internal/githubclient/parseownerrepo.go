// parseownerrepo.go implements ParseOwnerRepo, a pure standard-library parser.
// It splits a GitHub remote URL into its owner and repo segments.

package githubclient

import (
	"fmt"
	"strings"
)

// ParseOwnerRepo splits remoteURL into its owner and repo segments.
// It accepts four shapes, each with an optional ".git" suffix and an optional single trailing slash.
// The SSH form is git@github.com:owner/repo.git; the ssh:// form is ssh://git@github.com/owner/repo.git.
// The HTTPS form is https://github.com/owner/repo.git.
// The HTTPS form with user info is https://user@github.com/owner/repo.git.
// It makes no network call and no go-github call.
// A non-GitHub host, a URL without an owner/repo pair, and an unrecognized shape each yield a distinct wrapped error.
// A caller can report which failure it hit.
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

// ownerRepoPath strips remoteURL's GitHub-host prefix and returns the remaining owner/repo path.
// The prefixes are "git@github.com:", "ssh://git@github.com/", "https://github.com/" and "https://<user>@github.com/".
// It returns an error naming which shape failed to match.
func ownerRepoPath(remoteURL string) (string, error) {
	if path, ok := userInfoHTTPSPath(remoteURL); ok {
		return path, nil
	}

	switch {
	case strings.HasPrefix(remoteURL, "git@github.com:"):
		return strings.TrimPrefix(remoteURL, "git@github.com:"), nil
	case strings.HasPrefix(remoteURL, "ssh://git@github.com/"):
		return strings.TrimPrefix(remoteURL, "ssh://git@github.com/"), nil
	case strings.HasPrefix(remoteURL, "https://github.com/"):
		return strings.TrimPrefix(remoteURL, "https://github.com/"), nil
	case strings.Contains(remoteURL, "://") || strings.Contains(remoteURL, "@"):
		return "", fmt.Errorf("githubclient: parse owner/repo from %q: not a github.com remote URL", remoteURL)
	default:
		return "", fmt.Errorf("githubclient: parse owner/repo from %q: unrecognized remote URL shape", remoteURL)
	}
}

// userInfoHTTPSPath returns the owner/repo path of an "https://<user>@github.com/" URL.
// It also reports whether remoteURL has that shape.
// <user> is one non-empty run of characters other than "/", "@" and ":".
func userInfoHTTPSPath(remoteURL string) (string, bool) {
	rest, ok := strings.CutPrefix(remoteURL, "https://")
	if !ok {
		return "", false
	}
	user, path, ok := strings.Cut(rest, "@github.com/")
	if !ok || user == "" || strings.ContainsAny(user, "/@:") {
		return "", false
	}
	return path, true
}
