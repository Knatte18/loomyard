// originreads.go reads a run's state from origin's branches as the last fetch left them, with no network call.
// It covers the parent branch, a run's child status record and a task branch.

package fabricengine

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"

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
