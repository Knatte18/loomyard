//go:build tier0

// openprheads_test.go covers the head filtering of the open pull request lookup over a stubbed PR list.

package fabriccli

import (
	"testing"

	"github.com/google/go-github/v75/github"
)

// testPR builds a pull request whose head is ref in the repository fullName.
func testPR(fullName, ref string) *github.PullRequest {
	return &github.PullRequest{Head: &github.PullRequestBranch{
		Ref:  github.Ptr(ref),
		Repo: &github.Repository{FullName: github.Ptr(fullName)},
	}}
}

func TestSameRepoHeads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		prs       []*github.PullRequest
		wantHeads []string
	}{
		{
			name: "ExcludesForkHeads",
			prs: []*github.PullRequest{
				testPR("owner/repo", "task-a"),
				testPR("someone/repo", "task-b"),
				testPR("Owner/Repo", "task-c"),
				{},
			},
			wantHeads: []string{"task-a", "task-c"},
		},
		{name: "NoPullRequestsIsEmptyNonNil", prs: nil, wantHeads: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := sameRepoHeads(tt.prs, "owner", "repo")

			if got == nil {
				t.Fatalf("sameRepoHeads = nil; want a non-nil map")
			}
			if len(got) != len(tt.wantHeads) {
				t.Errorf("got %d heads (%v); want %v", len(got), got, tt.wantHeads)
			}
			for _, head := range tt.wantHeads {
				if !got[head] {
					t.Errorf("same-repo head %q missing: got %v", head, got)
				}
			}
		})
	}
}
