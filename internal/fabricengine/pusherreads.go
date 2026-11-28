// pusherreads.go holds the local reads that judge whether a worktree's commits still owe origin a push.
// Each reads only local refs and makes no network call.

package fabricengine

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// branchQuestion is the ancestry answer a caller needs of a branch against its tracking ref.
type branchQuestion int

const (
	// askAhead asks whether the tracking ref lacks a commit of the tip.
	askAhead branchQuestion = iota
	// askHeld asks whether the tracking ref holds the tip.
	askHeld
	// askCeiling asks whether the tracking ref lacks a given commit.
	askCeiling
)

// localBranch is the checked-out branch of one worktree against its tracking ref, as last fetched.
type localBranch struct {
	fabriccore.BranchReading
	repo        *gitrepo.Repo
	upstreamTip string
}

// readLocalBranch reads the checked-out branch of the worktree at path against its tracking ref.
// A code worktree with a detach marker is read at the marker's branch, whatever its HEAD holds.
// It also reads the one ancestry answer question asks for.
// ceiling is the commit askCeiling asks about, and empty for the other questions.
// The ancestry is read only of a tracked upstream.
func readLocalBranch(path string, question branchQuestion, ceiling string) (localBranch, error) {
	branch := localBranch{repo: gitrepo.New(path)}
	branch.Upstream, branch.Ceiling = fabriccore.PushUpstreamNone, ceiling
	l, marked, stands, err := markedWorktree(path)
	if err != nil {
		return localBranch{}, err
	}
	var tip, upstreamTip string
	if stands {
		tip, err = markedBranchTip(l, marked)
	} else {
		tip, err = branch.repo.CurrentSHA()
	}
	if errors.Is(err, gitrepo.ErrNoCommits) {
		return branch, nil
	}
	if err != nil {
		return localBranch{}, fmt.Errorf("fabricengine: read the tip of %s: %w", path, err)
	}
	branch.Tip = tip
	if stands {
		// The upstream ref is named even when it is pruned, which reading the ref then tells.
		var ref string
		ref, err = gitexec.Run([]string{"for-each-ref", "--format=%(upstream)", "refs/heads/" + marked}, path)
		switch ref = strings.TrimSpace(ref); {
		case err != nil:
		case ref == "":
			err = gitrepo.ErrNoUpstream
		default:
			upstreamTip, err = branch.repo.RefSHA(ref)
		}
	} else {
		upstreamTip, err = branch.repo.UpstreamSHA()
	}
	switch {
	case errors.Is(err, gitrepo.ErrNoUpstream):
		return branch, nil
	case errors.Is(err, gitrepo.ErrRefNotFound):
		branch.Upstream = fabriccore.PushUpstreamPruned
		return branch, nil
	case err != nil:
		return localBranch{}, fmt.Errorf("fabricengine: read the upstream of %s: %w", path, err)
	}
	branch.Upstream, branch.upstreamTip = fabriccore.PushUpstreamTracked, upstreamTip

	switch question {
	case askAhead:
		var notOnOrigin []string
		notOnOrigin, err = branch.repo.CommitsNotIn(branch.Tip, branch.upstreamTip)
		branch.LacksCommit = len(notOnOrigin) > 0
	case askHeld:
		branch.HoldsTip, err = branch.repo.IsAncestor(branch.Tip, branch.upstreamTip)
	case askCeiling:
		var onOrigin bool
		onOrigin, err = branch.repo.IsAncestor(ceiling, branch.upstreamTip)
		branch.LacksCeiling = !onOrigin
	}
	if err != nil {
		return localBranch{}, err
	}
	return branch, nil
}

// heldCeiling returns the first parent of the held commit hold in the repository at path.
// It is the newest commit a push may send while hold is held.
func heldCeiling(path, hold string) (string, error) {
	parents, err := gitrepo.New(path).CommitParents(hold)
	if err != nil {
		return "", fmt.Errorf("fabricengine: read the held commit %s: %w", hold, err)
	}
	if len(parents) == 0 {
		return "", fmt.Errorf("fabricengine: the held commit %s in %s has no parent to push up to", hold, path)
	}
	return parents[0], nil
}

// RunCommitsAhead reports whether the worktree l holds commits origin lacks, reading local refs only.
// l is the hub's prime or a task pair's code worktree, and its records worktree is the one RecordsWorktree names.
// A branch is ahead when it holds a commit its tracking ref lacks, or has no upstream and holds a commit.
// A branch whose upstream is set and whose tracking ref is absent reads level: origin ended it.
// The prime is ahead when its records branch is, since its code branch is the operator's.
// A pair is ahead when either of its branches is.
// A records commit of the pair's run may be held back from every push.
// The records branch then counts only up to that commit's first parent.
func RunCommitsAhead(l *lymcwd.Location) (bool, error) {
	prime, err := IsPrimeWorktree(l)
	if err != nil {
		return false, err
	}
	question, ceiling := askAhead, ""
	if !prime {
		hold, held, err := recordsPushCeiling(l.HubPath, l.WorktreeName)
		if err != nil {
			return false, err
		}
		if held {
			question = askCeiling
			if ceiling, err = heldCeiling(RecordsWorktree(l), hold); err != nil {
				return false, err
			}
		}
	}
	records, err := readLocalBranch(RecordsWorktree(l), question, ceiling)
	if err != nil {
		return false, err
	}
	// The code branch is read only when the records branch alone does not answer.
	ahead := fabriccore.RunAhead(prime, records.BranchReading, fabriccore.BranchReading{})
	if ahead || prime {
		return ahead, nil
	}
	code, err := readLocalBranch(l.WorktreePath(), askAhead, "")
	if err != nil {
		return false, err
	}
	return fabriccore.RunAhead(prime, records.BranchReading, code.BranchReading), nil
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
		marked, stands, err := ReadDetachMarker(l)
		if err != nil {
			return false, err
		}
		if codeBranch = marked; !stands {
			codeBranch, err = gitrepo.New(l.WorktreePath()).CurrentBranch()
			if err != nil {
				return false, fmt.Errorf("fabricengine: read the code branch of %s: %w", l.WorktreePath(), err)
			}
		}
	}
	records, err := ListFailedPushes(l.HubPath)
	if err != nil {
		return false, err
	}
	return fabriccore.RefusedPushStands(records, recordsBranch, codeBranch), nil
}

// TipsOnOrigin reports whether origin's tracking refs hold the local records tip of l.
// For a pair they must also hold its local code tip.
// A tracking ref holds a tip when it is that tip or a descendant of it, read from local refs with no network call.
// A branch with no upstream fails it.
// A branch whose upstream is set and whose tracking ref is absent reads held: origin ended it, so it has nothing to push.
func TipsOnOrigin(l *lymcwd.Location) (bool, error) {
	records, err := readLocalBranch(RecordsWorktree(l), askHeld, "")
	if err != nil || !fabriccore.BranchHeldOnOrigin(records.BranchReading) {
		return false, err
	}
	prime, err := IsPrimeWorktree(l)
	if err != nil || prime {
		return true, err
	}
	code, err := readLocalBranch(l.WorktreePath(), askHeld, "")
	if err != nil {
		return false, err
	}
	return fabriccore.TipsHeldOnOrigin(prime, records.BranchReading, code.BranchReading), nil
}
