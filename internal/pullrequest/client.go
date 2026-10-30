// client.go holds Client, the GitHub calls on one repository's pull requests, and the API interface the row, Finalize and pair teardown hold.

package pullrequest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/v75/github"

	"github.com/Knatte18/loomyard/internal/githubclient"
)

// listPageSize is the page size the list calls request, GitHub's maximum.
const listPageSize = 100

// State is where a pull request stands on GitHub.
type State string

const (
	// StateOpen is a pull request still under review.
	StateOpen State = "open"
	// StateClosed is a pull request closed without merging.
	StateClosed State = "closed"
	// StateMerged is a merged pull request, which GitHub also lists as closed.
	StateMerged State = "merged"
)

// Status is what Client.Status reads about a pull request.
type Status struct {
	State State
	// HeadSHA is the pull request's current head commit.
	HeadSHA string
	// Approved reports whether a reviewer's standing review approves HeadSHA.
	Approved bool
	// ChangesRequested reports whether any reviewer's standing review requests changes.
	ChangesRequested bool
}

// Ref names a pull request.
type Ref struct {
	Number int
	URL    string
}

// RefusalKind classifies why GitHub refused a merge.
type RefusalKind string

const (
	// RefusalMethod means the repository does not allow the merge method asked for.
	RefusalMethod RefusalKind = "method"
	// RefusalPending means the base branch moved or GitHub is still computing mergeability, so a retry may succeed.
	RefusalPending RefusalKind = "pending"
	// RefusalOther covers every other refusal: a moved head, a missing approval, a required check or a conflict, none of which a retry clears.
	RefusalOther RefusalKind = "other"
)

// MergeRefusal is the error of a merge GitHub refused with HTTP 405 or 409.
type MergeRefusal struct {
	// Kind classifies the refusal from GitHub's message.
	Kind RefusalKind
	// Message is GitHub's own message.
	Message string
}

// Error returns GitHub's message prefixed with the refusal's source.
func (r *MergeRefusal) Error() string {
	return "pullrequest: github refused the merge: " + r.Message
}

// API is the set of GitHub calls a caller makes on its run's pull request, so a test fakes GitHub behind it.
type API interface {
	// FindOpen returns the first open pull request from head into base, and false when there is none.
	FindOpen(ctx context.Context, head, base string) (Ref, bool, error)
	// Create opens a pull request from head into base.
	Create(ctx context.Context, head, base, title, body string) (Ref, error)
	// Edit sets the title and body of an open pull request.
	Edit(ctx context.Context, number int, title, body string) error
	// Status reads a pull request's state, head and standing reviews.
	Status(ctx context.Context, number int) (Status, error)
	// Close comments on a pull request, then closes it without merging.
	Close(ctx context.Context, number int, comment string) error
	// Merge merges a pull request whose head is expectedHead, squashing when squash is set.
	// A merge GitHub refuses returns a *MergeRefusal.
	Merge(ctx context.Context, number int, expectedHead string, squash bool, title, message string) error
}

// Client is the pull requests of one GitHub repository, reached through an authenticated client.
// It is safe for concurrent use.
type Client struct {
	gh    *github.Client
	owner string
	repo  string
}

var _ API = (*Client)(nil)

// Open returns the Client of the GitHub repository originURL names, authenticated through githubclient.
// It refuses a URL that does not name a GitHub repository, naming the URL.
func Open(originURL string) (*Client, error) {
	owner, repo, err := githubclient.ParseOwnerRepo(originURL)
	if err != nil {
		return nil, fmt.Errorf("the origin %s is not a GitHub repository, so it has no pull requests: %w", originURL, err)
	}
	gh, err := githubclient.New()
	if err != nil {
		return nil, fmt.Errorf("github client unavailable: %w", err)
	}
	return NewWithClient(gh, owner, repo), nil
}

// NewWithClient returns the Client of owner/repo reached through gh.
func NewWithClient(gh *github.Client, owner, repo string) *Client {
	return &Client{gh: gh, owner: owner, repo: repo}
}

// FindOpen returns the first open pull request from head into base, and false when there is none.
func (c *Client) FindOpen(ctx context.Context, head, base string) (Ref, bool, error) {
	opts := &github.PullRequestListOptions{
		State:       "open",
		Head:        c.owner + ":" + head,
		Base:        base,
		ListOptions: github.ListOptions{PerPage: listPageSize},
	}
	found, _, err := c.gh.PullRequests.List(ctx, c.owner, c.repo, opts)
	if err != nil {
		return Ref{}, false, fmt.Errorf("pullrequest: list open pull requests of %s into %s: %w", head, base, err)
	}
	if len(found) == 0 {
		return Ref{}, false, nil
	}
	return Ref{Number: found[0].GetNumber(), URL: found[0].GetHTMLURL()}, true, nil
}

// Create opens a pull request from head into base with the told title and body.
func (c *Client) Create(ctx context.Context, head, base, title, body string) (Ref, error) {
	created, _, err := c.gh.PullRequests.Create(ctx, c.owner, c.repo, &github.NewPullRequest{
		Title: &title,
		Head:  &head,
		Base:  &base,
		Body:  &body,
	})
	if err != nil {
		return Ref{}, fmt.Errorf("pullrequest: open a pull request from %s into %s: %w", head, base, err)
	}
	return Ref{Number: created.GetNumber(), URL: created.GetHTMLURL()}, nil
}

// Edit sets the title and body of a pull request.
func (c *Client) Edit(ctx context.Context, number int, title, body string) error {
	_, _, err := c.gh.PullRequests.Edit(ctx, c.owner, c.repo, number, &github.PullRequest{Title: &title, Body: &body})
	if err != nil {
		return fmt.Errorf("pullrequest: edit pull request #%d: %w", number, err)
	}
	return nil
}

// Status reads a pull request's state, with merged winning over closed, its head commit, and the standing verdict of every page of its reviews.
func (c *Client) Status(ctx context.Context, number int) (Status, error) {
	pull, _, err := c.gh.PullRequests.Get(ctx, c.owner, c.repo, number)
	if err != nil {
		return Status{}, fmt.Errorf("pullrequest: read pull request #%d: %w", number, err)
	}
	status := Status{State: StateOpen, HeadSHA: pull.GetHead().GetSHA()}
	switch {
	case pull.GetMerged():
		status.State = StateMerged
	case pull.GetState() == "closed":
		status.State = StateClosed
	}

	var reviews []*github.PullRequestReview
	opts := &github.ListOptions{PerPage: listPageSize}
	for {
		page, resp, err := c.gh.PullRequests.ListReviews(ctx, c.owner, c.repo, number, opts)
		if err != nil {
			return Status{}, fmt.Errorf("pullrequest: list reviews of pull request #%d: %w", number, err)
		}
		reviews = append(reviews, page...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	status.Approved, status.ChangesRequested = standingReviews(reviews, status.HeadSHA)
	return status, nil
}

// Close posts one comment on a pull request, then closes it without merging.
// When the comment posts and the close fails, the error says so, since a rerun posts a second comment.
func (c *Client) Close(ctx context.Context, number int, comment string) error {
	if _, _, err := c.gh.Issues.CreateComment(ctx, c.owner, c.repo, number, &github.IssueComment{Body: &comment}); err != nil {
		return fmt.Errorf("pullrequest: comment on pull request #%d: %w", number, err)
	}
	closed := "closed"
	if _, _, err := c.gh.PullRequests.Edit(ctx, c.owner, c.repo, number, &github.PullRequest{State: &closed}); err != nil {
		return fmt.Errorf("pullrequest: comment posted on pull request #%d but closing it failed, so rerunning posts a second comment: %w", number, err)
	}
	return nil
}

// Merge merges a pull request whose head must equal expectedHead, squashing when squash is set, with the told commit title and message.
// Only HTTP 405 and 409 are read as a refusal; any other failure is returned wrapped so githubclient.IsTransient still judges it.
func (c *Client) Merge(ctx context.Context, number int, expectedHead string, squash bool, title, message string) error {
	method := "merge"
	if squash {
		method = "squash"
	}
	_, _, err := c.gh.PullRequests.Merge(ctx, c.owner, c.repo, number, message, &github.PullRequestOptions{
		CommitTitle: title,
		SHA:         expectedHead,
		MergeMethod: method,
	})
	if err == nil {
		return nil
	}
	var ghErr *github.ErrorResponse
	if errors.As(err, &ghErr) && ghErr.Response != nil &&
		(ghErr.Response.StatusCode == http.StatusMethodNotAllowed || ghErr.Response.StatusCode == http.StatusConflict) {
		return &MergeRefusal{Kind: classifyRefusal(ghErr.Message), Message: strings.TrimSpace(ghErr.Message)}
	}
	return fmt.Errorf("pullrequest: merge pull request #%d: %w", number, err)
}

// classifyRefusal reads GitHub's refusal message as one RefusalKind.
func classifyRefusal(message string) RefusalKind {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "not allowed"):
		return RefusalMethod
	case strings.Contains(lower, "base branch was modified"), strings.Contains(lower, "mergeability"), strings.Contains(lower, "still being computed"):
		return RefusalPending
	default:
		return RefusalOther
	}
}

// standingReviews reads each reviewer's latest APPROVED, CHANGES_REQUESTED or DISMISSED review, ignoring COMMENTED and PENDING.
// approved is true when at least one standing review is APPROVED with a commit id equal to head.
// changesRequested is true when any standing review is CHANGES_REQUESTED.
// reviews are in the order GitHub lists them, oldest first.
func standingReviews(reviews []*github.PullRequestReview, head string) (approved, changesRequested bool) {
	standing := map[string]*github.PullRequestReview{}
	for _, review := range reviews {
		switch review.GetState() {
		case "APPROVED", "CHANGES_REQUESTED", "DISMISSED":
			standing[review.GetUser().GetLogin()] = review
		}
	}
	for _, review := range standing {
		switch review.GetState() {
		case "APPROVED":
			approved = approved || review.GetCommitID() == head
		case "CHANGES_REQUESTED":
			changesRequested = true
		}
	}
	return approved, changesRequested
}
