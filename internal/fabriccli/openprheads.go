// openprheads.go is the open pull request lookup `lyx fabric cleanup --remote` hands to fabricengine.Topology.CleanupRemoteWarp, which keeps every branch an open pull request still names.
// It lives here rather than in fabricengine because the engine imports no GitHub client.

package fabriccli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/google/go-github/v75/github"
)

// openPRListTimeout bounds the whole open-PR listing, every page included.
const openPRListTimeout = 30 * time.Second

// openPRListPageSize is the page size of the open-PR listing, GitHub's maximum.
const openPRListPageSize = 100

// listOpenPRHeads returns the head branch of every open pull request on the repository behind remoteURL whose head lives in that same repository;
// the map is non-nil, and empty when none is open.
// Any failure (a non-GitHub origin, no token, a network error, the timeout) returns an error, so the caller keeps every branch.
// It is a package-level variable so tests can substitute the GitHub call.
var listOpenPRHeads = fetchOpenPRHeads

// fetchOpenPRHeads is listOpenPRHeads' production implementation.
func fetchOpenPRHeads(ctx context.Context, remoteURL string) (map[string]bool, error) {
	owner, repo, err := githubclient.ParseOwnerRepo(remoteURL)
	if err != nil {
		return nil, err
	}
	client, err := githubclient.New()
	if err != nil {
		return nil, err
	}

	listCtx, cancel := context.WithTimeout(ctx, openPRListTimeout)
	defer cancel()

	var prs []*github.PullRequest
	opts := &github.PullRequestListOptions{
		State:       "open",
		ListOptions: github.ListOptions{PerPage: openPRListPageSize},
	}
	for {
		page, resp, err := client.PullRequests.List(listCtx, owner, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list open pull requests of %s/%s: %w", owner, repo, err)
		}
		prs = append(prs, page...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return sameRepoHeads(prs, owner, repo), nil
}

// sameRepoHeads collects the head ref of every pull request in prs whose head repository is owner/repo.
// A fork's head ref is excluded: it names a branch in another repository, not one on this origin.
func sameRepoHeads(prs []*github.PullRequest, owner, repo string) map[string]bool {
	heads := map[string]bool{}
	want := owner + "/" + repo
	for _, pr := range prs {
		head := pr.GetHead()
		if !strings.EqualFold(head.GetRepo().GetFullName(), want) {
			continue
		}
		if ref := head.GetRef(); ref != "" {
			heads[ref] = true
		}
	}
	return heads
}
