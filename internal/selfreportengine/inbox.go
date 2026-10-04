// inbox.go holds the GitHub calls `lyx board intake` makes against the inbox repository:
// listing open issues, fetching one, and commenting on and closing one.
// They share targetRepo, the NewGitHubClient seam and the error classification with CreateIssue.

package selfreportengine

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/v75/github"
)

// listPageSize is the page size ListOpenIssues requests, GitHub's maximum.
const listPageSize = 100

// Issue is the view of an inbox issue that intake prints and decides on.
type Issue struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	Labels      []string  `json:"labels"`
	URL         string    `json:"url"`
	CreatedAt   time.Time `json:"created_at"`
	State       string    `json:"state"`
	StateReason string    `json:"state_reason"`
	PullRequest bool      `json:"pull_request"`
}

// ListOpenIssues returns every open issue of the inbox repository, following pagination to the last page.
// Pull requests are excluded, although GitHub's issue list includes them.
func ListOpenIssues() ([]Issue, error) {
	client, owner, repo, err := repoClient()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()

	opts := &github.IssueListByRepoOptions{
		State:       "open",
		ListOptions: github.ListOptions{PerPage: listPageSize},
	}
	var issues []Issue
	for {
		page, resp, listErr := client.Issues.ListByRepo(ctx, owner, repo, opts)
		if listErr != nil {
			return nil, classifyCallError("list issues", "github issue list failed", owner, repo, listErr)
		}
		for _, raw := range page {
			if raw.IsPullRequest() {
				continue
			}
			issues = append(issues, toIssue(raw))
		}
		if resp.NextPage == 0 {
			return issues, nil
		}
		opts.ListOptions.Page = resp.NextPage
	}
}

// GetIssue returns one issue or pull request of the inbox repository, open or closed.
func GetIssue(number int) (Issue, error) {
	client, owner, repo, err := repoClient()
	if err != nil {
		return Issue{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()

	raw, _, getErr := client.Issues.Get(ctx, owner, repo, number)
	if getErr != nil {
		return Issue{}, classifyCallError("get issue", "github issue get failed", owner, repo, getErr)
	}
	return toIssue(raw), nil
}

// CommentAndClose posts comment on the issue, then closes it with state reason completed when completed is set and not_planned otherwise.
// When the comment posts and the close fails, the error says the comment was posted, so a rerun is understood to post a second comment.
func CommentAndClose(number int, comment string, completed bool) error {
	client, owner, repo, err := repoClient()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()

	_, _, commentErr := client.Issues.CreateComment(ctx, owner, repo, number, &github.IssueComment{Body: &comment})
	if commentErr != nil {
		return classifyCallError("comment on issue", "github issue comment failed", owner, repo, commentErr)
	}

	state, reason := "closed", "not_planned"
	if completed {
		reason = "completed"
	}
	_, _, closeErr := client.Issues.Edit(ctx, owner, repo, number, &github.IssueRequest{State: &state, StateReason: &reason})
	if closeErr != nil {
		return fmt.Errorf("comment posted on issue #%d but closing it failed, so rerunning posts a second comment: %w",
			number, classifyCallError("close issue", "github issue close failed", owner, repo, closeErr))
	}
	return nil
}

// toIssue converts a go-github issue to the Issue view.
func toIssue(raw *github.Issue) Issue {
	labels := make([]string, 0, len(raw.Labels))
	for _, label := range raw.Labels {
		labels = append(labels, label.GetName())
	}
	return Issue{
		Number:      raw.GetNumber(),
		Title:       raw.GetTitle(),
		Body:        raw.GetBody(),
		Labels:      labels,
		URL:         raw.GetHTMLURL(),
		CreatedAt:   raw.GetCreatedAt().Time,
		State:       raw.GetState(),
		StateReason: raw.GetStateReason(),
		PullRequest: raw.IsPullRequest(),
	}
}
