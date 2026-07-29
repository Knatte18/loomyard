// selfreport.go contains the go-github client seam and CreateIssue domain
// function for the selfreportengine package. It holds everything from the
// selfreport module that does not belong to the cobra command layer:
// the NewGitHubClient seam and CreateIssue.

// Package selfreportengine provides the domain kernel for filing GitHub issues
// via githubclient's authenticated go-github client. It exposes CreateIssue as
// the single entry point and NewGitHubClient as a swappable factory seam for
// testing, keeping targetRepo unexported.
package selfreportengine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/githubclient"
)

// targetRepo is the hardcoded "owner/repo" GitHub repository that all issues
// are filed against. It is a constant so that tests can verify the exact
// owner and repo arguments passed to Issues.Create without any config-file or
// environment-variable indirection. githubclient resolves neither owner nor
// repo itself -- CreateIssue splits this constant and passes both as
// parameters.
const targetRepo = "Knatte18/loomyard"

// createIssueTimeout bounds CreateIssue's whole call to Issues.Create,
// including a 401-triggered credential re-resolution and replay performed
// internally by githubclient's transport. It matches the 30s budget
// githubclient.New's returned client already carries on its underlying
// http.Client, so CreateIssue never relies solely on a caller-supplied
// context.Background() to keep an autonomous run from hanging.
const createIssueTimeout = 30 * time.Second

// NewGitHubClient is the seam through which CreateIssue obtains an
// authenticated *github.Client. Tests replace it with a closure that returns
// a real go-github client pointed at an httptest server, so no test ever
// reaches real token resolution or the real GitHub API.
var NewGitHubClient = githubclient.New

// CreateIssue files a GitHub issue with the given title, optional body, and
// labels via go-github's typed Issues.Create call against the hardcoded
// targetRepo. It returns the issue's HTML URL and issue number on success.
//
// Error handling distinguishes three cases, carried over in spirit from the
// gh-CLI transport this replaces:
//   - the token cannot be resolved (formerly: gh binary not on PATH) --
//     surfaces as errors.Is(err, githubclient.ErrTokenUnresolvable), reached
//     either through NewGitHubClient itself or through the first request's
//     401 handling
//   - a transport or network failure (formerly: generic exec failure) --
//     any other error Issues.Create returns that is not an *github.ErrorResponse
//   - GitHub rejected the request with a non-2xx response (formerly: non-zero
//     gh exit) -- an *github.ErrorResponse, whose Message is surfaced directly
//
// These three remain distinguishable because a network failure and an API
// rejection call for different operator action: one is worth retrying, the
// other names a rejected request that will not succeed on retry alone.
//
// The returned number is 0 only when go-github's Issue.Number itself comes
// back nil. In practice a successful (2xx) response from Issues.Create always
// carries a typed issue number, so this is a defensive fallback rather than
// an expected parse outcome the way it was under the old URL-suffix parse --
// callers must not read a live "issue number omitted" case into it.
func CreateIssue(title string, body *string, labels []string) (url string, number int, err error) {
	client, err := NewGitHubClient()
	if err != nil {
		// A factory failure is the same operator-facing case as an
		// unresolvable token: there is no authenticated client to even
		// attempt the request with, so surface it the same way rather than
		// risking a nil-client panic below.
		return "", 0, fmt.Errorf("github client unavailable: %w", err)
	}

	owner, repo, ok := strings.Cut(targetRepo, "/")
	if !ok {
		// Unreachable given targetRepo's fixed "owner/repo" literal; kept as
		// a defensive guard so a future edit to the constant fails loudly
		// instead of silently filing against a malformed repo path.
		return "", 0, fmt.Errorf("selfreportengine: targetRepo %q is not \"owner/repo\" shaped", targetRepo)
	}

	// Bound the whole call, including a possible 401 re-resolution and
	// replay performed internally by githubclient's transport, so a stalled
	// connection can never hang an autonomous lyx run indefinitely.
	ctx, cancel := context.WithTimeout(context.Background(), createIssueTimeout)
	defer cancel()

	req := &github.IssueRequest{Title: &title}
	if body != nil {
		req.Body = body
	}
	if len(labels) > 0 {
		req.Labels = &labels
	}

	issue, _, createErr := client.Issues.Create(ctx, owner, repo, req)
	if createErr != nil {
		// A token that could not be resolved (env, cache, and gh CLI all
		// exhausted) surfaces distinctly from a generic network problem so
		// the operator knows to fix credentials rather than investigate
		// connectivity.
		if errors.Is(createErr, githubclient.ErrTokenUnresolvable) {
			return "", 0, fmt.Errorf("github token not resolvable: %w", createErr)
		}

		// A non-2xx GitHub response decodes into *github.ErrorResponse;
		// surfacing its Message directly is the closest equivalent to the
		// old "gh issue create failed: <stderr>" text.
		var ghErr *github.ErrorResponse
		if errors.As(createErr, &ghErr) {
			return "", 0, fmt.Errorf("github issue create failed: %s", strings.TrimSpace(ghErr.Message))
		}

		return "", 0, fmt.Errorf("failed to reach GitHub: %w", createErr)
	}

	return issue.GetHTMLURL(), issue.GetNumber(), nil
}
