// beamline.go reads the commits on a beam pair's first-parent line beyond its parent branch.
// It also lists the part of that line that a merged pull request lacks.

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
	// Parents are the commit's parent object names, first parent first,
	// so a caller tells a merge from the commit it brought in.
	Parents []string
	// Committed is the commit's committer time.
	Committed time.Time
	// RecordsOnly reports whether the commit changes, against its first parent, only paths under the pair's lym directory.
	// Such a commit is lym's own bookkeeping and never a landing of work.
	RecordsOnly bool
}

// BeamLineCommits lists the commits on the beam pair's first-parent line that its merge base with parentBranch lacks.
// They are oldest first.
// The first-parent line holds what landed on the beam branch itself:
// a squash or merge of each member, each merge from the parent, and each commit made on the beam directly.
// It never holds the commits a merge brought in.
// An empty parentBranch, meaning a pair with no origin record, is refused.
// The error names the way forward: make the beam through `lym beam create`.
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

	// The first-parent line leaves the merge base's history at its first commit the parent already holds,
	// so it is at most as long as added.
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

// BeamCommitsPast lists the commits a landing of a pull request merged at mergedHead lacks, oldest first.
// Each entry is a short object name and its subject; the beam pair at l has parent branch parentBranch.
// The candidates are the commits past mergedHead on the beam's first-parent line.
// When mergedHead is not on that line, the candidates are the whole line.
// Commits that only record lym's own state are left out.
// So are merges whose second parent parentBranch already holds.
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
