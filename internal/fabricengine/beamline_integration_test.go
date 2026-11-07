// beamline_integration_test.go drives BeamLineCommits and BeamCommitsPast against a real fabric pair and real git.
// The commits they list are the beam branch's own first-parent line.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestBeamLineCommits pins what a beam's line lists:
// each commit on the beam's first-parent line that the parent lacks, oldest first.
// A merge from the parent is included; the commits a merge brought in are left out.
// Each commit carries its full message and commit time,
// and a commit that changes only lym's records is marked RecordsOnly.
func TestBeamLineCommits(t *testing.T) {
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
	commitIndexedRecord(t, worktree, "_lym/landing/beam-ready.json", "{}\n", "beam: bookkeeping")

	commits, err := fabricengine.BeamLineCommits(location, parentBranch)
	if err != nil {
		t.Fatalf("BeamLineCommits() error = %v; want nil", err)
	}
	var subjects []string
	var recordsOnly []bool
	for _, commit := range commits {
		subject, _, _ := strings.Cut(commit.Message, "\n")
		subjects = append(subjects, subject)
		recordsOnly = append(recordsOnly, commit.RecordsOnly)
		if commit.SHA == "" || commit.Committed.IsZero() {
			t.Errorf("BeamLineCommits() entry %+v; want its object name and commit time", commit)
		}
	}
	if want := []string{"Add widgets (#12)", "Merge the member", "Merge the parent", "beam: bookkeeping"}; !slices.Equal(subjects, want) {
		t.Errorf("BeamLineCommits() subjects = %q; want %q", subjects, want)
	}
	if want := []bool{false, false, false, true}; !slices.Equal(recordsOnly, want) {
		t.Errorf("BeamLineCommits() RecordsOnly = %v; want %v", recordsOnly, want)
	}
	if len(commits) > 0 && !strings.Contains(commits[0].Message, "Adds the widgets.") {
		t.Errorf("BeamLineCommits() first message = %q; want it to carry the body", commits[0].Message)
	}

	if _, err := fabricengine.BeamLineCommits(location, ""); err == nil || !strings.Contains(err.Error(), "lym beam create") {
		t.Errorf("BeamLineCommits(empty parent) error = %v; want one naming `lym beam create`", err)
	}
}

// TestBeamCommitsPast pins what a merged pull request lacks: the beam's commits past the merged head.
// A merge that only brought the parent in, and a commit that only records lym's own state, are left out.
func TestBeamCommitsPast(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "beampast"
	hubforge.AddPair(t, hub, slug)
	location, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree error = %v; want nil", err)
	}
	worktree := hub.PairCodeWorktree(slug)
	prime := hub.PrimeWorktree()
	parentBranch := gitkit.CurrentBranch(t, prime)

	gitkit.CommitFile(t, worktree, "widgets.txt", "widgets\n", "Add widgets")
	mergedHead := gitkit.Git(t, worktree, "rev-parse", "HEAD")
	parentTip := gitkit.CommitFile(t, prime, "parent.txt", "parent\n", "parent: progress")
	gitkit.Git(t, worktree, "merge", "--no-ff", "-m", "Merge the parent", parentTip)
	commitIndexedRecord(t, worktree, "_lym/landing/beam-ready.json", "{}\n", "beam: bookkeeping")

	lacking, err := fabricengine.BeamCommitsPast(location, parentBranch, mergedHead)
	if err != nil || len(lacking) != 0 {
		t.Errorf("BeamCommitsPast() = %q, %v for a beam past the merged head only by a merge of the parent and a records-only commit; want none", lacking, err)
	}

	gitkit.CommitFile(t, worktree, "gadgets.txt", "gadgets\n", "Add gadgets")
	lacking, err = fabricengine.BeamCommitsPast(location, parentBranch, mergedHead)
	if err != nil || len(lacking) != 1 || !strings.HasSuffix(lacking[0], " Add gadgets") {
		t.Errorf("BeamCommitsPast() = %q, %v after a beam commit; want the one commit named by its subject", lacking, err)
	}
}

// commitIndexedRecord commits content at rel through the index alone.
// The worktree holds rel beyond a directory link that git refuses to stage through.
func commitIndexedRecord(t *testing.T, dir, rel, content, msg string) {
	t.Helper()

	source := filepath.Join(t.TempDir(), "blob")
	if err := os.WriteFile(source, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", source, err)
	}
	blob := gitkit.Git(t, dir, "hash-object", "-w", source)
	gitkit.Git(t, dir, "update-index", "--add", "--cacheinfo", "100644,"+blob+","+rel)
	gitkit.Git(t, dir, "commit", "-m", msg)
}
