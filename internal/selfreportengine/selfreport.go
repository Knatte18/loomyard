// selfreport.go holds the go-github client seam, CreateIssue, and the helpers every call shares.
// Code for the cobra command layer is not in this file.

// Package selfreportengine files LoomYard bugs and enhancements as GitHub issues on LoomYard's own repository.
// It files to that repository whatever hub lym runs in, and only there.
// An issue about another repository goes to that repository's own issues, never through selfreport.
// It uses githubclient's authenticated go-github client.
// It exposes CreateIssue and NewGitHubClient as a swappable factory seam for testing, keeping targetRepo unexported.
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

// targetRepo is the hardcoded "owner/repo" GitHub repository, LoomYard's own, that all issues are filed against.
// It is a constant with no config-file or environment-variable indirection.
// That lets tests verify the exact owner and repo arguments passed to the Issues calls.
// githubclient resolves neither owner nor repo itself, so this constant is split into both before each call.
const targetRepo = "Knatte18/loomyard"

// defaultLabel is the label applied when a caller supplies no explicit labels.
// It is unexported so that DefaultLabels is the only way to observe it.
const defaultLabel = "bug"

// callTimeout bounds each request this package makes to GitHub.
// The bound covers a 401-triggered credential re-resolution and replay performed inside githubclient's transport.
// It matches the 30s budget that transport applies from the end of a pause hold.
// So no call relies on a bare context.Background() alone to keep an autonomous run from hanging.
const callTimeout = 30 * time.Second

// NewGitHubClient builds the authenticated *github.Client that CreateIssue uses.
// It is a package variable so that tests can swap it.
var NewGitHubClient = githubclient.New

// DefaultLabels returns the default label set for both the automatic self-report path and "selfreport create".
// It returns a fresh []string{"bug"} on every call, never a shared slice,
// so a caller that appends to or mutates the returned slice cannot corrupt the default seen by the next caller.
func DefaultLabels() []string {
	return []string{defaultLabel}
}

// CreateIssue files a GitHub issue with the given title, optional body, and labels.
// It returns the issue's HTML URL and issue number on success.
// A failure to resolve a token matches errors.Is(err, githubclient.ErrTokenUnresolvable).
// A network failure carries no *github.ErrorResponse; an API rejection carries one.
func CreateIssue(title string, body *string, labels []string) (url string, number int, err error) {
	client, owner, repo, err := repoClient()
	if err != nil {
		return "", 0, err
	}

	// Bound the whole call, including a possible 401 re-resolution and replay inside githubclient's transport.
	// A stalled connection must not hang an autonomous lym run indefinitely.
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

// repoClient returns an authenticated client and the owner and repo halves of targetRepo.
// Every call this package makes starts here.
func repoClient() (client *github.Client, owner, repo string, err error) {
	client, err = NewGitHubClient()
	if err != nil {
		// A factory failure is the same operator-facing case as an unresolvable token.
		// There is no authenticated client to attempt the request with.
		// Surface it the same way rather than risk a nil-client panic in the caller.
		logger.Warn("selfreportengine: github call failed", "action", "new github client", "cause", err)
		return nil, "", "", fmt.Errorf("github client unavailable: %w", err)
	}

	owner, repo, ok := strings.Cut(targetRepo, "/")
	if !ok {
		// Unreachable given targetRepo's fixed "owner/repo" literal.
		// The guard stays so that a future edit to the constant fails loudly rather than silently calling a malformed repo path.
		return nil, "", "", fmt.Errorf("selfreportengine: targetRepo %q is not \"owner/repo\" shaped", targetRepo)
	}
	return client, owner, repo, nil
}

// classifyCallError logs a failed GitHub call and returns its operator-facing error.
// action is the log's action field and apiFailure the prefix of an API rejection's message.
func classifyCallError(action, apiFailure, owner, repo string, callErr error) error {
	// One Warn covers all three returns below, not one per branch.
	// They share the action, owner and repo context, and only the cause differs.
	logger.Warn("selfreportengine: github call failed", "action", action, "owner", owner, "repo", repo, "cause", callErr)

	// An unresolvable token (env, cache and gh CLI all exhausted) surfaces distinctly from a network problem.
	// The operator then knows to fix credentials rather than investigate connectivity.
	if errors.Is(callErr, githubclient.ErrTokenUnresolvable) {
		return fmt.Errorf("github token not resolvable: %w", callErr)
	}

	// A non-2xx GitHub response decodes into *github.ErrorResponse.
	// Its Message is surfaced directly as the rejection's detail.
	var ghErr *github.ErrorResponse
	if errors.As(callErr, &ghErr) {
		return fmt.Errorf("%s: %s", apiFailure, strings.TrimSpace(ghErr.Message))
	}

	return fmt.Errorf("failed to reach GitHub: %w", callErr)
}
