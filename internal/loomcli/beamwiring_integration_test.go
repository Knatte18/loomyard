// beamwiring_integration_test.go drives beamCommits against a real fabric pair and real git, since the commits it lists are the beam branch's own first-parent line.

package loomcli

import (
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestBeamCommits_ListsTheBeamsOwnLine pins what Beam-Describe lists: each commit on the beam's first-parent line that the parent lacks, oldest first, a merge from the parent included and the commits a merge brought in left out, each with its full message and commit time.
func TestBeamCommits_ListsTheBeamsOwnLine(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "beamcommits"
	hubforge.AddPair(t, hub, slug)
	location, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree error = %v; want nil", err)
	}
	worktree := hub.PairCodeWorktree(slug)
	prime := hub.PrimeWorktree()
	parentBranch := gitkit.CurrentBranch(t, prime)
	beamBranch := gitkit.CurrentBranch(t, worktree)

	gitkit.CommitFile(t, worktree, "widgets.txt", "widgets\n", "Add widgets (#12)\n\nAdds the widgets.")
	gitkit.Git(t, worktree, "checkout", "-b", "member")
	gitkit.CommitFile(t, worktree, "member.txt", "member\n", "member: inner work")
	gitkit.Git(t, worktree, "checkout", beamBranch)
	gitkit.Git(t, worktree, "merge", "--no-ff", "-m", "Merge the member", "member")
	parentTip := gitkit.CommitFile(t, prime, "parent.txt", "parent\n", "parent: progress")
	gitkit.Git(t, worktree, "merge", "--no-ff", "-m", "Merge the parent", parentTip)

	commits, err := beamCommits(location, parentBranch)
	if err != nil {
		t.Fatalf("beamCommits() error = %v; want nil", err)
	}
	var subjects []string
	for _, commit := range commits {
		subject, _, _ := strings.Cut(commit.Message, "\n")
		subjects = append(subjects, subject)
		if commit.SHA == "" || commit.Committed.IsZero() {
			t.Errorf("beamCommits() entry %+v; want its object name and commit time", commit)
		}
	}
	if want := []string{"Add widgets (#12)", "Merge the member", "Merge the parent"}; !slices.Equal(subjects, want) {
		t.Errorf("beamCommits() subjects = %q; want %q", subjects, want)
	}
	if len(commits) > 0 && !strings.Contains(commits[0].Message, "Adds the widgets.") {
		t.Errorf("beamCommits() first message = %q; want it to carry the body", commits[0].Message)
	}
}
