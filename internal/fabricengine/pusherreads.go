// pusherreads.go holds the local reads that judge whether a worktree's commits still owe origin a push.
// Each reads only local refs and makes no network call.

package fabricengine

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// upstreamState is how a branch's upstream stands in the local repository.
type upstreamState int

const (
	// upstreamUnset means the branch has no upstream configured.
	upstreamUnset upstreamState = iota
	// upstreamPruned means the upstream is set but its tracking ref is absent, as after a fetch pruned it.
	upstreamPruned
	// upstreamTracked means the upstream is set and its tracking ref holds upstreamTip.
	upstreamTracked
)

// localBranch is the checked-out branch of one worktree against its tracking ref, as last fetched.
// tip is empty for a branch with no commits.
type localBranch struct {
	repo        *gitrepo.Repo
	tip         string
	state       upstreamState
	upstreamTip string
}

// readLocalBranch reads the checked-out branch of the worktree at path against its tracking ref.
func readLocalBranch(path string) (localBranch, error) {
	repo := gitrepo.New(path)
	tip, err := repo.CurrentSHA()
	if errors.Is(err, gitrepo.ErrNoCommits) {
		return localBranch{repo: repo}, nil
	}
	if err != nil {
		return localBranch{}, fmt.Errorf("fabricengine: read the tip of %s: %w", path, err)
	}
	branch := localBranch{repo: repo, tip: tip}
	upstreamTip, err := repo.UpstreamSHA()
	switch {
	case errors.Is(err, gitrepo.ErrNoUpstream):
		branch.state = upstreamUnset
	case errors.Is(err, gitrepo.ErrRefNotFound):
		branch.state = upstreamPruned
	case err != nil:
		return localBranch{}, fmt.Errorf("fabricengine: read the upstream of %s: %w", path, err)
	default:
		branch.state = upstreamTracked
		branch.upstreamTip = upstreamTip
	}
	return branch, nil
}

// ahead reports whether the branch holds a commit its tracking ref lacks.
// A branch with an upstream unset and a commit is ahead; one whose tracking ref is pruned reads level.
func (b localBranch) ahead() (bool, error) {
	switch {
	case b.tip == "":
		return false, nil
	case b.state == upstreamUnset:
		return true, nil
	case b.state == upstreamPruned:
		return false, nil
	}
	notOnOrigin, err := b.repo.CommitsNotIn(b.tip, b.upstreamTip)
	return len(notOnOrigin) > 0, err
}

// lacks reports whether the branch's tracking ref lacks the commit sha.
// An upstream unset lacks every commit; a pruned tracking ref reads level, so it lacks none.
func (b localBranch) lacks(sha string) (bool, error) {
	switch b.state {
	case upstreamUnset:
		return true, nil
	case upstreamPruned:
		return false, nil
	}
	onOrigin, err := b.repo.IsAncestor(sha, b.upstreamTip)
	return !onOrigin, err
}

// heldOnOrigin reports whether the branch's tracking ref is the local tip or a descendant of it.
// A branch with no commits is held, and so is one whose tracking ref is pruned: origin ended it.
func (b localBranch) heldOnOrigin() (bool, error) {
	switch {
	case b.tip == "", b.state == upstreamPruned:
		return true, nil
	case b.state == upstreamUnset:
		return false, nil
	}
	return b.repo.IsAncestor(b.tip, b.upstreamTip)
}

// RunCommitsAhead reports whether the worktree l holds commits origin lacks, reading local refs only.
// l is the hub's prime or a task pair's code worktree, and its records worktree is the one RecordsWorktree names.
// A branch is ahead when it holds a commit its tracking ref lacks, or has no upstream and holds a commit.
// A branch whose upstream is set and whose tracking ref is absent reads level: origin ended it.
// The prime is ahead when its records branch is, since its code branch is the operator's.
// A pair is also ahead when the Warp-SHA trailer of its records tip names a code tip.
// That tip must be on its code branch, and the branch's tracking ref must lack it.
// Code commits past the named tip are not counted.
// A records tip with no Warp-SHA trailer counts no code side.
func RunCommitsAhead(l *lymcwd.Location) (bool, error) {
	records, err := readLocalBranch(RecordsWorktree(l))
	if err != nil {
		return false, err
	}
	ahead, err := records.ahead()
	if err != nil || ahead {
		return ahead, err
	}
	prime, err := IsPrimeWorktree(l)
	if err != nil || prime || records.tip == "" {
		return false, err
	}
	detail, err := records.repo.CommitDetail(records.tip)
	if err != nil {
		return false, fmt.Errorf("fabricengine: read the records tip %s: %w", records.tip, err)
	}
	namedTip := trailerValue(detail.Message, fabriclayout.WarpSHATrailerKey)
	if namedTip == "" {
		return false, nil
	}
	codePath := l.WorktreePath()
	held, err := holdsCommit(codePath, namedTip)
	if err != nil || !held {
		return false, err
	}
	contained, err := HeadContains(codePath, namedTip)
	if err != nil || !contained {
		return false, err
	}
	code, err := readLocalBranch(codePath)
	if err != nil {
		return false, err
	}
	return code.lacks(namedTip)
}

// RefusedPushStands reports whether a refused-push record stands for a branch of l.
// The branches are its records branch and, for a pair, its code branch.
// A refused-push record is a failed-push record of a class other than transport, deferred and teardown_lock.
// Records are keyed by repo side and branch.
// So a refused push of the prime's records branch stands for every run whose commits that branch carries.
func RefusedPushStands(l *lymcwd.Location) (bool, error) {
	recordsBranch, err := gitrepo.New(RecordsWorktree(l)).CurrentBranch()
	if err != nil {
		return false, fmt.Errorf("fabricengine: read the records branch of %s: %w", l.WorktreePath(), err)
	}
	prime, err := IsPrimeWorktree(l)
	if err != nil {
		return false, err
	}
	codeBranch := ""
	if !prime {
		codeBranch, err = gitrepo.New(l.WorktreePath()).CurrentBranch()
		if err != nil {
			return false, fmt.Errorf("fabricengine: read the code branch of %s: %w", l.WorktreePath(), err)
		}
	}
	records, err := ListFailedPushes(l.HubPath)
	if err != nil {
		return false, err
	}
	for _, record := range records {
		if isOwedClass(record.Class) {
			continue
		}
		if (record.Repo == fabriclayout.RepoRecords && record.Branch == recordsBranch) ||
			(codeBranch != "" && record.Repo == fabriclayout.RepoCode && record.Branch == codeBranch) {
			return true, nil
		}
	}
	return false, nil
}

// TipsOnOrigin reports whether origin's tracking refs hold the local records tip of l.
// For a pair they must also hold its local code tip.
// A tracking ref holds a tip when it is that tip or a descendant of it, read from local refs with no network call.
// A branch with no upstream fails it.
// A branch whose upstream is set and whose tracking ref is absent reads held: origin ended it, so it has nothing to push.
func TipsOnOrigin(l *lymcwd.Location) (bool, error) {
	records, err := readLocalBranch(RecordsWorktree(l))
	if err != nil {
		return false, err
	}
	held, err := records.heldOnOrigin()
	if err != nil || !held {
		return false, err
	}
	prime, err := IsPrimeWorktree(l)
	if err != nil || prime {
		return true, err
	}
	code, err := readLocalBranch(l.WorktreePath())
	if err != nil {
		return false, err
	}
	return code.heldOnOrigin()
}
