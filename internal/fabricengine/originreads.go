// originreads.go reads a run's state from origin's branches as the last fetch left them, with no network call.
// It covers the parent branch, a run's child status record and a task branch.

package fabricengine

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
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
// status is the record's bytes, which the caller decodes.
// found is false when the ref or the record is absent.
// The record is read under the anchor the branch records, from the slug's run directory.
// It refuses a process with no run-records paths installed.
func OriginRunStatus(prime *lymcwd.Location, slug string) (status []byte, found bool, err error) {
	if err := refuseUnsuppliedSeams(fabriccore.SeamRunRecordPaths); err != nil {
		return nil, false, err
	}
	cfg, err := LoadConfig(ConfigDirs(prime))
	if err != nil {
		return nil, false, err
	}
	recordsRoot, err := RecordsRepoRoot(prime)
	if err != nil {
		return nil, false, fmt.Errorf("resolve the records repository: %w", err)
	}
	repo := gitrepo.New(recordsRoot)
	for _, name := range pairBranchNames(cfg, slug) {
		tip, err := repo.RefSHA(originRefPrefix + fabriclayout.RecordsBranchName(name))
		if errors.Is(err, gitrepo.ErrRefNotFound) {
			continue
		}
		if err != nil {
			return nil, false, err
		}
		status, _, found, err = originStatusBytesAtTip(prime, recordsRoot, tip, slug)
		return status, found, err
	}
	return nil, false, nil
}

// originStatusBytesAtTip reads the bytes of slug's child run record in the records repository at recordsRoot.
// It reads them from the tree of the commit tip, and returns the record's path in that tree with them.
// found is false when the tip holds no record.
// It refuses a process with no run-records paths installed.
func originStatusBytesAtTip(prime *lymcwd.Location, recordsRoot, tip, slug string) (data []byte, statusPath string, found bool, err error) {
	if err := refuseUnsuppliedSeams(fabriccore.SeamRunRecordPaths); err != nil {
		return nil, "", false, err
	}
	names, err := RepoWiredNames(prime)
	if err != nil {
		return nil, "", false, fmt.Errorf("load the fabric config: %w", err)
	}
	anchor, err := treeAnchor(recordsRoot, tip, names, prime.AnchorRel)
	if err != nil {
		return nil, "", false, err
	}
	statusRel, err := installedRunRecordPaths.Load().TreeStatusRel(slug)
	if err != nil {
		return nil, "", false, err
	}
	statusPath = path.Join(filepath.ToSlash(anchor), statusRel)
	data, err = showFile(recordsRoot, tip, statusPath)
	if err != nil {
		return nil, "", false, fmt.Errorf("read %s at %s in %s: %w", statusPath, tip, recordsRoot, err)
	}
	if data == nil {
		return nil, statusPath, false, nil
	}
	return data, statusPath, true, nil
}

// OriginCodeTip reads the code tip the last transition of slug on origin names, with no network call.
// It reads origin's fetched records branch of the slug for the transition.
// It reads origin's fetched code branch for whether that branch holds the tip.
// A tip this repository lacks counts as missing from origin, and so does an absent code branch.
// A slug with no records branch on origin reads the zero CodeTipReading, not missing.
// It refuses a process with no run-records paths or no status codec installed.
func OriginCodeTip(prime *lymcwd.Location, slug string) (fabriclayout.CodeTipReading, error) {
	if err := refuseUnsuppliedSeams(fabriccore.SeamRunRecordPaths, fabriccore.SeamStatusCodec); err != nil {
		return fabriclayout.CodeTipReading{}, err
	}
	cfg, err := LoadConfig(ConfigDirs(prime))
	if err != nil {
		return fabriclayout.CodeTipReading{}, err
	}
	recordsRoot, err := RecordsRepoRoot(prime)
	if err != nil {
		return fabriclayout.CodeTipReading{}, fmt.Errorf("resolve the records repository: %w", err)
	}
	for _, name := range pairBranchNames(cfg, slug) {
		recordsTip, err := gitrepo.New(recordsRoot).RefSHA(originRefPrefix + fabriclayout.RecordsBranchName(name))
		if errors.Is(err, gitrepo.ErrRefNotFound) {
			continue
		}
		if err != nil {
			return fabriclayout.CodeTipReading{}, err
		}
		return readCodeTipAt(prime, recordsRoot, recordsTip, name, slug)
	}
	return fabriclayout.CodeTipReading{}, nil
}

// readCodeTipAt reads the code tip of slug's last transition on the records branch tip recordsTip.
// branch is the slug's code branch, whose fetched origin ref says whether origin holds the tip.
func readCodeTipAt(prime *lymcwd.Location, recordsRoot, recordsTip, branch, slug string) (fabriclayout.CodeTipReading, error) {
	commit, found, err := lastTransitionCommit(recordsRoot, recordsTip, slug)
	if err != nil || !found {
		return fabriclayout.CodeTipReading{}, err
	}
	tip := trailerValue(commit.Message, fabriclayout.WarpSHATrailerKey)
	if tip == "" {
		return fabriclayout.CodeTipReading{}, nil
	}
	reading := fabriclayout.CodeTipReading{Tip: tip, At: commit.Committed}

	status, statusPath, statusFound, err := originStatusBytesAtTip(prime, recordsRoot, commit.SHA, slug)
	if err != nil {
		return fabriclayout.CodeTipReading{}, err
	}
	if statusFound {
		reading.Machine, err = installedStatusCodec.Load().LastMachine(status)
		if err != nil {
			return fabriclayout.CodeTipReading{}, fmt.Errorf("parse %s at %s in %s: %w", statusPath, commit.SHA, recordsRoot, err)
		}
	}

	codeDir := prime.WorktreePath()
	originTip, err := gitrepo.New(codeDir).RefSHA(originRefPrefix + branch)
	switch {
	case errors.Is(err, gitrepo.ErrRefNotFound):
		reading.Missing = true
	case err != nil:
		return fabriclayout.CodeTipReading{}, err
	default:
		held, err := isAncestorOrEqual(codeDir, tip, originTip)
		if err != nil {
			return fabriclayout.CodeTipReading{}, err
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
