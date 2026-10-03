// openprheads_test.go covers the head filtering of the open pull request lookup over a stubbed PR list,
// and exports the lookup seam for the package's external integration tests.

package fabriccli

import (
	"context"
	"testing"

	"github.com/google/go-github/v75/github"
)

// SetOpenPRHeadsForTest replaces the open-PR lookup with lookup for the test's lifetime.
func SetOpenPRHeadsForTest(t *testing.T, lookup func(ctx context.Context, remoteURL string) (map[string]bool, error)) {
	t.Helper()
	prev := listOpenPRHeads
	listOpenPRHeads = lookup
	t.Cleanup(func() { listOpenPRHeads = prev })
}

// testPR builds a pull request whose head is ref in the repository fullName.
func testPR(fullName, ref string) *github.PullRequest {
	return &github.PullRequest{Head: &github.PullRequestBranch{
		Ref:  github.Ptr(ref),
		Repo: &github.Repository{FullName: github.Ptr(fullName)},
	}}
}

func TestSameRepoHeads_ExcludesForkHeads(t *testing.T) {
	prs := []*github.PullRequest{
		testPR("owner/repo", "task-a"),
		testPR("someone/repo", "task-b"),
		testPR("Owner/Repo", "task-c"),
		{},
	}

	got := sameRepoHeads(prs, "owner", "repo")

	if !got["task-a"] || !got["task-c"] {
		t.Errorf("same-repo heads missing: got %v; want task-a and task-c", got)
	}
	if got["task-b"] {
		t.Errorf("fork head task-b included: got %v", got)
	}
	if len(got) != 2 {
		t.Errorf("got %d heads (%v); want 2", len(got), got)
	}
}

func TestSameRepoHeads_NoPullRequestsIsEmptyNonNil(t *testing.T) {
	got := sameRepoHeads(nil, "owner", "repo")
	if got == nil || len(got) != 0 {
		t.Errorf("sameRepoHeads(nil) = %#v; want an empty non-nil map", got)
	}
}
