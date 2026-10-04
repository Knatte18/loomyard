// selfreport.go contains the go-github client seam and CreateIssue domain function for the
// selfreportengine package.
// It holds everything from the selfreport module that does not belong to the cobra command layer:
// the NewGitHubClient seam, CreateIssue and the helpers every call shares.

// Package selfreportengine provides the domain kernel for filing GitHub issues, and for listing, fetching, commenting on and closing them for board intake, via githubclient's authenticated go-github client.
// It exposes CreateIssue for filing, the calls in inbox.go for intake, and NewGitHubClient as a swappable factory seam for testing, keeping targetRepo unexported.
package selfreportengine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/logger"
)

// targetRepo is the hardcoded "owner/repo" GitHub repository that all issues are filed against and that the intake calls in inbox.go read from, so it serves both directions, filing and intake.
// It is a constant so that tests can verify the exact owner and repo arguments passed to the Issues calls without any config-file or environment-variable indirection.
// githubclient resolves neither owner nor repo itself -- repoClient splits this constant and every call passes both as parameters.
const targetRepo = "Knatte18/loomyard"

// defaultLabel is the label applied when a caller supplies no explicit
// labels. It is unexported so DefaultLabels is the single way to observe it.
const defaultLabel = "bug"

// callTimeout bounds each call this package makes to GitHub, including a 401-triggered credential re-resolution and replay performed internally by githubclient's transport.
// It matches the 30s budget githubclient.New's returned client already carries on its underlying http.Client, so no call relies solely on a caller-supplied context.Background() to keep an autonomous run from hanging.
const callTimeout = 30 * time.Second

// NewGitHubClient is the seam through which CreateIssue obtains an authenticated *github.Client,
// swappable for testing.
var NewGitHubClient = githubclient.New

// DefaultLabels is the single owner of the label default shared by both the
// automatic self-report path and the manual "selfreport create" path.
// It returns a fresh []string{"bug"} on every call, never a shared
// package-level slice, so a caller that appends to or mutates the returned
// slice cannot corrupt the default seen by the next caller.
func DefaultLabels() []string {
	return []string{defaultLabel}
}

// CreateIssue files a GitHub issue with the given title, optional body, and labels.
// It returns the issue's HTML URL and issue number on success.
// Error handling distinguishes token-resolution failures (errors.Is(err,
// githubclient.ErrTokenUnresolvable)), network failures (*github.ErrorResponse absent), and API
// rejections (*github.ErrorResponse present).
func CreateIssue(title string, body *string, labels []string) (url string, number int, err error) {
	client, owner, repo, err := repoClient()
	if err != nil {
		return "", 0, err
	}

	// Bound the whole call, including a possible 401 re-resolution and
	// replay performed internally by githubclient's transport, so a stalled
	// connection can never hang an autonomous lyx run indefinitely.
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
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
		return "", 0, classifyCallError("create issue", "github issue create failed", owner, repo, createErr)
	}

	return issue.GetHTMLURL(), issue.GetNumber(), nil
}

// repoClient returns an authenticated client and the owner and repo halves of targetRepo, the common start of every call this package makes.
func repoClient() (client *github.Client, owner, repo string, err error) {
	client, err = NewGitHubClient()
	if err != nil {
		// A factory failure is the same operator-facing case as an unresolvable token:
		// there is no authenticated client to even attempt the request with, so surface it the same way rather than risking a nil-client panic in the caller.
		logger.Warn("selfreportengine: github call failed", "action", "new github client", "cause", err)
		return nil, "", "", fmt.Errorf("github client unavailable: %w", err)
	}

	owner, repo, ok := strings.Cut(targetRepo, "/")
	if !ok {
		// Unreachable given targetRepo's fixed "owner/repo" literal.
		// Kept as a defensive guard so a future edit to the constant fails loudly instead of silently calling a malformed repo path.
		return nil, "", "", fmt.Errorf("selfreportengine: targetRepo %q is not \"owner/repo\" shaped", targetRepo)
	}
	return client, owner, repo, nil
}

// classifyCallError logs a failed GitHub call and returns its operator-facing error.
// action is the log's action field and apiFailure the prefix of an API rejection's message.
func classifyCallError(action, apiFailure, owner, repo string, callErr error) error {
	// A single Warn covers all three classified returns below, rather than one per branch, since they share the same action/owner/repo context and only the cause differs.
	logger.Warn("selfreportengine: github call failed", "action", action, "owner", owner, "repo", repo, "cause", callErr)

	// A token that could not be resolved (env, cache, and gh CLI all exhausted) surfaces distinctly from a generic network problem, so the operator knows to fix credentials rather than investigate connectivity.
	if errors.Is(callErr, githubclient.ErrTokenUnresolvable) {
		return fmt.Errorf("github token not resolvable: %w", callErr)
	}

	// A non-2xx GitHub response decodes into *github.ErrorResponse;
	// surfacing its Message directly is the closest equivalent to the old "gh issue create failed: <stderr>" text.
	var ghErr *github.ErrorResponse
	if errors.As(callErr, &ghErr) {
		return fmt.Errorf("%s: %s", apiFailure, strings.TrimSpace(ghErr.Message))
	}

	return fmt.Errorf("failed to reach GitHub: %w", callErr)
}
