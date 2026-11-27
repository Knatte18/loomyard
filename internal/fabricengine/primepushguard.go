// primepushguard.go holds the guard a one-repository prime's records push runs before it pushes.
// The prime's branch is also the operator's code branch.
// So a records push of it must never carry the operator's own commits.

package fabricengine

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/dotgit"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
)

// PushFailureOperatorUnpushed means the prime's branch holds commits of the operator's own that lym never pushes.
// The prime's records stay unpushed until the operator pushes those commits.
const PushFailureOperatorUnpushed PushFailureClass = "operator_unpushed"

// isOneRepositoryPrimeAt reports whether path is the main worktree of a one-repository hub's repository.
func isOneRepositoryPrimeAt(path string) bool {
	if !oneRepositoryAt(path) {
		return false
	}
	geometry, err := dotgit.Read(path)
	if err != nil {
		return false
	}
	return sameDirectory(path, filepath.Dir(geometry.CommonDir))
}

// unpushedCommitsOfPrime returns the commits of branch at prime that origin's ref of the branch lacks, newest first.
// A branch origin holds no ref of lists every commit it reaches.
func unpushedCommitsOfPrime(prime, branch string) ([]string, error) {
	args := []string{"rev-list", "refs/heads/" + branch}
	originRef := "refs/remotes/" + originRemoteName + "/" + branch
	if _, err := gitexec.Run([]string{"rev-parse", "--verify", "--quiet", originRef}, prime); err == nil {
		args = append(args, "^"+originRef)
	} else if !exitedWithStatusOne(err) {
		return nil, fmt.Errorf("fabricengine: look up %s in %s: %w", originRef, prime, err)
	}
	listed, err := gitexec.Run(args, prime)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: list the unpushed commits of %s in %s: %w", branch, prime, err)
	}
	return strings.Fields(listed), nil
}

// originHoldsCommit reports whether some remote-tracking ref of origin in the repository at repoDir contains sha.
func originHoldsCommit(repoDir, sha string) (bool, error) {
	holders, err := gitexec.Run([]string{"for-each-ref", "--contains", sha, "--format=%(refname)", "refs/remotes/" + originRemoteName}, repoDir)
	if err != nil {
		return false, fmt.Errorf("fabricengine: list the origin refs containing %s in %s: %w", sha, repoDir, err)
	}
	return strings.TrimSpace(holders) != "", nil
}

// primePushOperatorCommits returns the commits of branch at prime that a records push must not carry, newest first.
// They are the commits origin's ref of the branch lacks that are neither records-only nor a fabric merge commit.
// A commit reachable from a second parent of such a merge is no operator commit while some origin ref holds that parent.
// A commit message alone admits nothing, so a code commit carrying the merge trailer stays an operator commit.
func primePushOperatorCommits(prime, branch string) ([]string, error) {
	unpushed, err := unpushedCommitsOfPrime(prime, branch)
	if err != nil {
		return nil, err
	}
	var candidates, mergedSides []string
	for _, sha := range unpushed {
		recordsOnly, err := IsRecordsOnlyCommit(prime, sha)
		if err != nil {
			return nil, err
		}
		if recordsOnly {
			continue
		}
		fabricMerge, err := IsFabricMergeCommit(prime, sha)
		if err != nil {
			return nil, err
		}
		if !fabricMerge {
			candidates = append(candidates, sha)
			continue
		}
		parents, err := parentsOf(prime, sha)
		if err != nil {
			return nil, err
		}
		held, err := originHoldsCommit(prime, parents[1])
		if err != nil {
			return nil, err
		}
		if held {
			mergedSides = append(mergedSides, parents[1])
		}
	}

	var operatorCommits []string
	for _, candidate := range candidates {
		merged := false
		for _, side := range mergedSides {
			reached, err := isAncestorOrEqual(prime, candidate, side)
			if err != nil {
				return nil, err
			}
			if reached {
				merged = true
				break
			}
		}
		if !merged {
			operatorCommits = append(operatorCommits, candidate)
		}
	}
	return operatorCommits, nil
}

// primeRunsCarried returns the slugs the Run-Transition trailers of the unpushed commits of branch at prime name.
// The slugs come newest commit first, each once.
func primeRunsCarried(prime, branch string) ([]string, error) {
	unpushed, err := unpushedCommitsOfPrime(prime, branch)
	if err != nil {
		return nil, err
	}
	var runs []string
	for _, sha := range unpushed {
		message, err := gitexec.Run([]string{"show", "-s", "--format=%B", sha}, prime)
		if err != nil {
			return nil, fmt.Errorf("fabricengine: read the message of %s in %s: %w", sha, prime, err)
		}
		slug := trailerValue(message, TransitionTrailerKey)
		if slug != "" && !slices.Contains(runs, slug) {
			runs = append(runs, slug)
		}
	}
	return runs, nil
}

// holdPrimePushForOperator is the guard every records push of a one-repository prime runs before it pushes.
// It returns nil for a worktree that is not such a prime.
// It also returns nil for a prime whose unpushed commits are records or fabric merges.
// Otherwise it returns a *PushFailedError the caller returns without pushing.
// The failed push is recorded for the prime's branch as class PushFailureOperatorUnpushed.
// The record names the operator's commits and the runs the unpushed commits carry.
// Its notice is mailed once.
// A prime that already holds a protected failed-push record for the branch records it as protected instead.
func holdPrimePushForOperator(hubPath, prime string, now time.Time) error {
	if !isOneRepositoryPrimeAt(prime) {
		return nil
	}
	branch, err := gitrepo.New(prime).CurrentBranch()
	if err != nil {
		return fmt.Errorf("fabricengine: read the branch of the prime at %s: %w", prime, err)
	}
	operatorCommits, err := primePushOperatorCommits(prime, branch)
	if err != nil {
		return err
	}
	if len(operatorCommits) == 0 {
		return nil
	}
	runs, err := primeRunsCarried(prime, branch)
	if err != nil {
		return err
	}
	class := PushFailureOperatorUnpushed
	standing, found, err := readFailedPush(hubPath, failedPushSideRecords, branch)
	if err != nil {
		return err
	}
	if found && standing.Class == PushFailureProtected {
		class = PushFailureProtected
	}

	cause := fmt.Errorf("the prime's branch %s holds commits that are neither records nor a fabric merge (%s), so its records were not pushed",
		branch, strings.Join(operatorCommits, " "))
	record := FailedPush{
		Repo:         failedPushSideRecords,
		RepoPath:     prime,
		CodeWorktree: prime,
		Branch:       branch,
		Class:        class,
		Error:        cause.Error(),
		Commits:      operatorCommits,
		Runs:         runs,
	}
	if err := recordPushFailure(hubPath, record, now, installedNotifier()); err != nil {
		logger.Warn("fabricengine: failed-push record not written", "side", failedPushSideRecords, "branch", branch, "error", err)
	}
	return &PushFailedError{Record: record, Err: cause}
}
