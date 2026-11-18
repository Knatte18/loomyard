// originreads.go reads a run's state from origin's branches as the last fetch left them, with no network call.
// It covers the parent branch, a run's child status record and a task branch.

package fabricengine

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// OriginParentHoldsTaskWork reports whether origin's parent branch holds the pair's task work.
// It reads the branch as this machine last fetched it.
// That marks a landing run past its parent-side merge.
// A parent branch with no remote-tracking ref reads false.
// It returns an error when the pair has no origin record naming its parent branch or a ref cannot be read.
func OriginParentHoldsTaskWork(l, pairLocation *lymcwd.Location, slug string) (bool, error) {
	return parentHoldsTaskWorkAt(l, pairLocation, slug, originRefPrefix)
}

// OriginRunStatus reads the child run record of slug from the tree of its records branch's remote-tracking ref.
// It serves a machine that holds no pair for the run.
// found is false when the ref or the record is absent.
// The record is read under the anchor the branch records, from the slug's run directory or the legacy `self` directory.
func OriginRunStatus(prime *lymcwd.Location, slug string) (status shedengine.Status, found bool, err error) {
	cfg, err := LoadConfig(ConfigDirs(prime))
	if err != nil {
		return shedengine.Status{}, false, err
	}
	recordsRoot, err := RecordsRepoRoot(prime)
	if err != nil {
		return shedengine.Status{}, false, fmt.Errorf("resolve the records repository: %w", err)
	}
	repo := gitrepo.New(recordsRoot)
	for _, name := range pairBranchNames(cfg, slug) {
		tip, err := repo.RefSHA(originRefPrefix + RecordsBranchName(name))
		if errors.Is(err, gitrepo.ErrRefNotFound) {
			continue
		}
		if err != nil {
			return shedengine.Status{}, false, err
		}
		return originStatusAtTip(prime, recordsRoot, tip, slug)
	}
	return shedengine.Status{}, false, nil
}

// originStatusAtTip reads the child run record of slug at the commit tip of the records repository at recordsRoot.
func originStatusAtTip(prime *lymcwd.Location, recordsRoot, tip, slug string) (shedengine.Status, bool, error) {
	names, err := RepoWiredNames(prime)
	if err != nil {
		return shedengine.Status{}, false, fmt.Errorf("load the fabric config: %w", err)
	}
	anchor, err := treeAnchor(recordsRoot, tip, names, prime.AnchorRel)
	if err != nil {
		return shedengine.Status{}, false, err
	}
	for _, runID := range []string{slug, shedrun.SelfRunID} {
		_, statusRel, err := shedrun.TreeRunFileRels(runID)
		if err != nil {
			return shedengine.Status{}, false, err
		}
		statusPath := path.Join(filepath.ToSlash(anchor), statusRel)
		data, err := showFile(recordsRoot, tip, statusPath)
		if err != nil {
			return shedengine.Status{}, false, fmt.Errorf("read %s at %s in %s: %w", statusPath, tip, recordsRoot, err)
		}
		if data == nil {
			continue
		}
		status, err := shedengine.ParseStatusTolerant(data)
		if err != nil {
			return shedengine.Status{}, false, fmt.Errorf("parse %s at %s in %s: %w", statusPath, tip, recordsRoot, err)
		}
		return status, true, nil
	}
	return shedengine.Status{}, false, nil
}

// CodeTipReading is what OriginCodeTip read of a run's code tip.
type CodeTipReading struct {
	// Tip is the code tip the run's last transition on origin names, empty when origin holds no such transition.
	Tip string
	// Missing reports whether origin's fetched code branch lacks Tip.
	Missing bool
	// Machine is the machine of the last history entry of the run's child record at that transition.
	Machine string
	// At is the commit time of that transition.
	At time.Time
}

// OriginCodeTip reads the code tip the last transition of slug on origin names, with no network call.
// It reads origin's fetched records branch of the slug for the transition.
// It reads origin's fetched code branch for whether that branch holds the tip.
// A tip this repository lacks counts as missing from origin, and so does an absent code branch.
// A slug with no records branch on origin reads the zero CodeTipReading, not missing.
func OriginCodeTip(prime *lymcwd.Location, slug string) (CodeTipReading, error) {
	cfg, err := LoadConfig(ConfigDirs(prime))
	if err != nil {
		return CodeTipReading{}, err
	}
	recordsRoot, err := RecordsRepoRoot(prime)
	if err != nil {
		return CodeTipReading{}, fmt.Errorf("resolve the records repository: %w", err)
	}
	for _, name := range pairBranchNames(cfg, slug) {
		recordsTip, err := gitrepo.New(recordsRoot).RefSHA(originRefPrefix + RecordsBranchName(name))
		if errors.Is(err, gitrepo.ErrRefNotFound) {
			continue
		}
		if err != nil {
			return CodeTipReading{}, err
		}
		return readCodeTipAt(prime, recordsRoot, recordsTip, name, slug)
	}
	return CodeTipReading{}, nil
}

// readCodeTipAt reads the code tip of slug's last transition on the records branch tip recordsTip.
// branch is the slug's code branch, whose fetched origin ref says whether origin holds the tip.
func readCodeTipAt(prime *lymcwd.Location, recordsRoot, recordsTip, branch, slug string) (CodeTipReading, error) {
	commit, found, err := lastTransitionCommit(recordsRoot, recordsTip, slug)
	if err != nil || !found {
		return CodeTipReading{}, err
	}
	tip := trailerValue(commit.Message, WarpSHATrailerKey)
	if tip == "" {
		return CodeTipReading{}, nil
	}
	reading := CodeTipReading{Tip: tip, At: commit.Committed}

	status, statusFound, err := originStatusAtTip(prime, recordsRoot, commit.SHA, slug)
	if err != nil {
		return CodeTipReading{}, err
	}
	if statusFound && len(status.History) > 0 {
		reading.Machine = status.History[len(status.History)-1].Machine
	}

	codeDir := prime.WorktreePath()
	originTip, err := gitrepo.New(codeDir).RefSHA(originRefPrefix + branch)
	switch {
	case errors.Is(err, gitrepo.ErrRefNotFound):
		reading.Missing = true
	case err != nil:
		return CodeTipReading{}, err
	default:
		held, err := isAncestorOrEqual(codeDir, tip, originTip)
		if err != nil {
			return CodeTipReading{}, err
		}
		reading.Missing = !held
	}
	return reading, nil
}

// FetchedTaskBranchOnOrigin reports whether slug's task branch has a remote-tracking ref under origin.
// The ref is looked up in the prime's warp repository.
// It matches the branch under the current branch prefix, or the bare slug of a pair made before the prefix was set.
// It reads the refs the last fetch left and makes no network call,
// so a branch deleted on origin since that fetch still reads present.
func FetchedTaskBranchOnOrigin(prime *lymcwd.Location, slug string) (bool, error) {
	cfg, err := LoadConfig(ConfigDirs(prime))
	if err != nil {
		return false, err
	}
	branches, err := gitrepo.New(prime.WorktreePath()).BranchNames(originRemoteName)
	if err != nil {
		return false, err
	}
	names := pairBranchNames(cfg, slug)
	for _, branch := range branches {
		for _, name := range names {
			if branch == name {
				return true, nil
			}
		}
	}
	return false, nil
}
