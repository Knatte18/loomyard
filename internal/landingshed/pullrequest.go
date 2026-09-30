// pullrequest.go implements the lookup of the pull request from a task branch to its parent.

package landingshed

import (
	"context"
	"fmt"

	"github.com/google/go-github/v75/github"
)

// FindPullRequest returns the newest pull request from head to base in owner/repo, whatever its
// state, or nil with a nil error when none exists, so every landing step resolves "the pull request
// from the task branch to the parent" the same way. The query is bounded by the landing GitHub
// timeout, and a GitHub error is returned as-is.
func FindPullRequest(ctx context.Context, client *github.Client, owner, repo, head, base string) (*github.PullRequest, error) {
	queryCtx, cancel := context.WithTimeout(ctx, publishGitHubTimeout)
	defer cancel()

	prs, _, err := client.PullRequests.List(queryCtx, owner, repo, &github.PullRequestListOptions{
		State:     "all",
		Head:      fmt.Sprintf("%s:%s", owner, head),
		Base:      base,
		Sort:      "created",
		Direction: "desc",
	})
	if err != nil {
		return nil, err
	}
	if len(prs) == 0 {
		return nil, nil
	}
	return prs[0], nil
}
