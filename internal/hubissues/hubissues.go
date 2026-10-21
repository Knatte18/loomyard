// hubissues.go holds Repo and its calls: listing open issues, fetching one, listing one's comments, and commenting on and closing one.

package hubissues

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

// callTimeout bounds each request to GitHub, including a 401-triggered credential re-resolution and replay inside githubclient's transport.
// It matches the 30s budget githubclient's transport applies from the end of a pause hold, so no call hangs an autonomous run.
const callTimeout = 30 * time.Second

// listPageSize is the page size the list calls request, GitHub's maximum.
const listPageSize = 100

// Repo is one GitHub repository's issues, reached through an authenticated client.
// It is safe for concurrent use.
type Repo struct {
	client *github.Client
	owner  string
	name   string
	// perRequest is the deadline of each request.
	perRequest time.Duration
}

// Issue is the view of an issue that intake prints and decides on.
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
	// Comments is the issue's comment count as GitHub reports it on the issue itself.
	Comments int `json:"comments"`
}

// IssueComment is one comment on an issue, its author not recorded.
type IssueComment struct {
	Body      string
	CreatedAt time.Time
}

// Open returns the Repo of the GitHub repository remoteURL names, authenticated through githubclient.
// It refuses a URL that does not name a GitHub repository, naming the URL.
func Open(remoteURL string) (*Repo, error) {
	owner, name, err := githubclient.ParseOwnerRepo(remoteURL)
	if err != nil {
		return nil, fmt.Errorf("the hub's repository %s is not a GitHub repository, so it has no issues to take in: %w", remoteURL, err)
	}
	client, err := githubclient.New()
	if err != nil {
		logger.Warn("hubissues: github call failed", "action", "new github client", "cause", err)
		return nil, fmt.Errorf("github client unavailable: %w", err)
	}
	return New(client, owner, name), nil
}

// New returns the Repo owner/name reached through client, each request bounded by the package's call timeout.
func New(client *github.Client, owner, name string) *Repo {
	return &Repo{client: client, owner: owner, name: name, perRequest: callTimeout}
}

// FullName returns the repository as "owner/name".
func (r *Repo) FullName() string {
	return r.owner + "/" + r.name
}

// ListOpenIssues returns every open issue of the repository, following pagination to the last page.
// Pull requests are excluded, although GitHub's issue list includes them.
func (r *Repo) ListOpenIssues() ([]Issue, error) {
	opts := &github.IssueListByRepoOptions{
		State:       "open",
		ListOptions: github.ListOptions{PerPage: listPageSize},
	}
	var issues []Issue
	for {
		ctx, cancel := context.WithTimeout(context.Background(), r.perRequest)
		page, resp, err := r.client.Issues.ListByRepo(ctx, r.owner, r.name, opts)
		cancel()
		if err != nil {
			return nil, r.classifyCallError("list issues", "github issue list failed", err)
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

// GetIssue returns one issue or pull request of the repository, open or closed.
func (r *Repo) GetIssue(number int) (Issue, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.perRequest)
	defer cancel()

	raw, _, err := r.client.Issues.Get(ctx, r.owner, r.name, number)
	if err != nil {
		return Issue{}, r.classifyCallError("get issue", "github issue get failed", err)
	}
	return toIssue(raw), nil
}

// ListIssueComments returns every comment on the issue numbered number, oldest first, following pagination to the last page.
func (r *Repo) ListIssueComments(number int) ([]IssueComment, error) {
	sort, direction := "created", "asc"
	opts := &github.IssueListCommentsOptions{
		Sort:        &sort,
		Direction:   &direction,
		ListOptions: github.ListOptions{PerPage: listPageSize},
	}
	var comments []IssueComment
	for {
		ctx, cancel := context.WithTimeout(context.Background(), r.perRequest)
		page, resp, err := r.client.Issues.ListComments(ctx, r.owner, r.name, number, opts)
		cancel()
		if err != nil {
			return nil, r.classifyCallError("list issue comments", "github issue comment list failed", err)
		}
		for _, raw := range page {
			comments = append(comments, IssueComment{Body: raw.GetBody(), CreatedAt: raw.GetCreatedAt().Time})
		}
		if resp.NextPage == 0 {
			return comments, nil
		}
		opts.ListOptions.Page = resp.NextPage
	}
}

// CommentAndClose posts comment on the issue, then closes it with state reason completed when completed is set and not_planned otherwise.
// When the comment posts and the close fails, the error says the comment was posted, so a rerun is understood to post a second comment.
func (r *Repo) CommentAndClose(number int, comment string, completed bool) error {
	commentCtx, cancelComment := context.WithTimeout(context.Background(), r.perRequest)
	_, _, commentErr := r.client.Issues.CreateComment(commentCtx, r.owner, r.name, number, &github.IssueComment{Body: &comment})
	cancelComment()
	if commentErr != nil {
		return r.classifyCallError("comment on issue", "github issue comment failed", commentErr)
	}

	state, reason := "closed", "not_planned"
	if completed {
		reason = "completed"
	}
	closeCtx, cancelClose := context.WithTimeout(context.Background(), r.perRequest)
	defer cancelClose()
	_, _, closeErr := r.client.Issues.Edit(closeCtx, r.owner, r.name, number, &github.IssueRequest{State: &state, StateReason: &reason})
	if closeErr != nil {
		return fmt.Errorf("comment posted on issue #%d but closing it failed, so rerunning posts a second comment: %w",
			number, r.classifyCallError("close issue", "github issue close failed", closeErr))
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
		Comments:    raw.GetComments(),
	}
}

// classifyCallError logs a failed GitHub call and returns its operator-facing error.
// action is the log's action field and apiFailure the prefix of an API rejection's message.
func (r *Repo) classifyCallError(action, apiFailure string, callErr error) error {
	logger.Warn("hubissues: github call failed", "action", action, "owner", r.owner, "repo", r.name, "cause", callErr)

	// An unresolvable token surfaces apart from a network problem, so the operator fixes credentials rather than connectivity.
	if errors.Is(callErr, githubclient.ErrTokenUnresolvable) {
		return fmt.Errorf("github token not resolvable: %w", callErr)
	}

	var ghErr *github.ErrorResponse
	if errors.As(callErr, &ghErr) {
		return fmt.Errorf("%s on %s: %s", apiFailure, r.FullName(), strings.TrimSpace(ghErr.Message))
	}

	return fmt.Errorf("failed to reach GitHub: %w", callErr)
}
