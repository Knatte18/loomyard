// beamline.go implements BeamLineCommits, the read of the commits a beam pair's own first-parent line holds beyond its parent branch, and BeamCommitsPast, the part of that line a merged pull request lacks.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// LineCommit is one commit on a beam pair's first-parent line.
type LineCommit struct {
	// SHA is the commit's full hex object name.
	SHA string
	// Message is the commit's full message: its subject line, then its body.
	Message string
	// Parents are the commit's parent object names, first parent first, so a caller tells a merge from the commit it brought in.
	Parents []string
	// Committed is the commit's committer time.
	Committed time.Time
	// RecordsOnly reports whether every path the commit changes against its first parent lies under the pair's lym directory.
	// Such a commit is lym's own bookkeeping and never a landing of work.
	RecordsOnly bool
}

// BeamLineCommits lists the commits on the first-parent line of the beam pair at l that its merge base with parentBranch lacks, oldest first.
// The first-parent line holds what landed on the beam branch itself: a squash or merge of each member, each merge from the parent and each commit made on the beam directly, and never the commits a merge brought in.
// An empty parentBranch, a pair with no origin record, is refused with the way forward of making the beam through `lym beam create`.
func BeamLineCommits(l *lymcwd.Location, parentBranch string) ([]LineCommit, error) {
	if parentBranch == "" {
		return nil, errors.New("the beam's pair records no parent branch to measure its commits from; way forward: make the beam through \"lym beam create\", which records its parent branch")
	}
	repo := gitrepo.New(l.WorktreePath())
	head, err := repo.CurrentSHA()
	if err != nil {
		return nil, err
	}
	parentTip, err := repo.ResolveSHA(parentBranch)
	if err != nil {
		return nil, err
	}
	base, err := repo.MergeBase(head, parentTip)
	if err != nil {
		return nil, err
	}
	added, err := repo.CommitsNotIn(head, base)
	if err != nil || len(added) == 0 {
		return nil, err
	}
	beamOwn := make(map[string]bool, len(added))
	for _, sha := range added {
		beamOwn[sha] = true
	}

	// The first-parent line leaves the merge base's history at its first commit the parent already holds, so it is at most as long as added.
	line, err := repo.Log(head, gitrepo.LogQuery{FirstParent: true, Limit: len(added)})
	if err != nil {
		return nil, err
	}
	lymPrefix := filepath.ToSlash(filepath.Join(l.AnchorRel, lymdirs.LymDirName)) + "/"
	var commits []LineCommit
	for _, commit := range line {
		if !beamOwn[commit.SHA] {
			break
		}
		detail, err := repo.CommitDetail(commit.SHA)
		if err != nil {
			return nil, err
		}
		commits = append(commits, LineCommit{
			SHA:         commit.SHA,
			Message:     commit.Message,
			Parents:     commit.Parents,
			Committed:   commit.Committed,
			RecordsOnly: len(detail.Paths) > 0 && !slices.ContainsFunc(detail.Paths, func(path string) bool { return !strings.HasPrefix(path, lymPrefix) }),
		})
	}
	// Log answers newest first.
	slices.Reverse(commits)
	return commits, nil
}

// BeamCommitsPast lists the commits a landing of a pull request merged at mergedHead would lack, from the beam pair at l whose parent branch is parentBranch, oldest first, each as a short object name and its subject.
// They are the commits of the beam's first-parent line past mergedHead, or the whole line when mergedHead is not on it, bar the commits that only record lym's own state and the merges whose second parent parentBranch already holds.
func BeamCommitsPast(l *lymcwd.Location, parentBranch, mergedHead string) ([]string, error) {
	line, err := BeamLineCommits(l, parentBranch)
	if err != nil {
		return nil, err
	}
	pair, err := Open(l)
	if err != nil {
		return nil, err
	}
	past := line
	for i, commit := range line {
		if commit.SHA == mergedHead {
			past = line[i+1:]
			break
		}
	}
	var lacking []string
	for _, commit := range past {
		if commit.RecordsOnly {
			continue
		}
		if len(commit.Parents) > 1 {
			fromParent, err := pair.IsAncestor(commit.Parents[1], "refs/heads/"+parentBranch)
			if err != nil {
				return nil, err
			}
			if fromParent {
				continue
			}
		}
		subject, _, _ := strings.Cut(commit.Message, "\n")
		lacking = append(lacking, fmt.Sprintf("%.7s %s", commit.SHA, subject))
	}
	return lacking, nil
}
