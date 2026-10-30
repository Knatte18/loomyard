// parseownerrepo.go implements ParseOwnerRepo, a pure standard-library parser splitting a GitHub
// remote URL into its owner and repo segments.

package githubclient

import (
	"fmt"
	"strings"
)

// ParseOwnerRepo splits remoteURL into its owner and repo segments.
// It accepts exactly four shapes, each with an optional ".git" suffix and an optional single trailing slash.
// They are the SSH form (git@github.com:owner/repo.git) and the ssh:// form (ssh://git@github.com/owner/repo.git).
// The other two are the HTTPS form (https://github.com/owner/repo.git) and the HTTPS form with user info (https://user@github.com/owner/repo.git).
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

// userInfoHTTPSPath returns the owner/repo path of an "https://<user>@github.com/" URL and reports whether remoteURL has that shape.
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
