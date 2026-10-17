// pullrequest.go implements the lookup of the pull request from a task branch to its parent.

package landingshed

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/v75/github"
)

// FindPullRequest returns as pr the newest pull request from head to base in owner/repo created at or after since, whatever its state, or nil when none exists.
// Every landing step resolves "the pull request from the task branch to the parent" through it, so they all resolve it the same way.
// The run's start is the bound: a pull request created before since belongs to an earlier run of the same slug and reads as none.
// It also returns as olderOpen the newest open pull request created before since, nil when there is none.
// A zero since filters nothing: pr is the newest pull request and olderOpen is nil.
// The query is bounded by the landing GitHub timeout, and a GitHub error is returned as-is with two nils.
func FindPullRequest(ctx context.Context, client *github.Client, owner, repo, head, base string, since time.Time) (pr, olderOpen *github.PullRequest, err error) {
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
		return nil, nil, err
	}
	for _, candidate := range prs {
		if !candidate.GetCreatedAt().Time.Before(since) {
			if pr == nil {
				pr = candidate
			}
			continue
		}
		if olderOpen == nil && candidate.GetState() == "open" {
			olderOpen = candidate
		}
	}
	return pr, olderOpen, nil
}
