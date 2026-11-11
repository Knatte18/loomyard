// ownerarchive.go archives the local tips a machine keeps of a subject another machine now owns.
// Each tip origin lacks is tagged archive-owner/<slug>/<sha12>, and the tag is pushed to origin with no branch push,
// so the new owner can reach the commits it cannot see.

package fabricengine

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// OwnerArchiveTagPrefix is the short-name prefix every owner archive tag carries.
// No reader of ArchiveTagPrefix tags matches it,
// so the owner archive of a live run or the prime never reads as an archived run.
const OwnerArchiveTagPrefix = "archive-owner/"

// OwnerArchiveTagName returns the owner archive tag name of a tip: archive-owner/<slug>/<first 12 hex of tip>.
// The tip must hold at least ArchiveTagSHALen characters.
func OwnerArchiveTagName(slug, tip string) string {
	return OwnerArchiveTagPrefix + slug + "/" + tip[:ArchiveTagSHALen]
}

// OwnerArchivePair is one pair whose local tips an owner archive covers.
type OwnerArchivePair struct {
	// Slug names the pair's tags and its teardown lock; the prime's pair is `root`.
	Slug string
	// Pair is the pair's code worktree.
	Pair *lymcwd.Location
}

// ArchiveOwnerTips archives each local tip of pairs that origin lacks, for the machine that now owns them.
// A pair's tips are the branches its code worktree and its records worktree have checked out.
// Origin lacks a tip when its copy of that branch is absent, or is neither the tip nor a descendant of it.
// Such a tip is tagged with its owner archive tag in its own repository,
// and the tag is pushed to that repository's origin; no branch is pushed.
// Each pair is archived under its slug's teardown lock, taken exclusive and waited on at most StatusPushLockWait.
// A worktree that is not there, a repository with no origin remote and a branch with no local tip archive nothing.
// The tag names follow from the tips, so a re-run reuses the tags an earlier run made and pushes what is missing.
// It returns the tags it pushed.
// Every pair is tried, and the error joins each pair's failure, each wrapping ErrArchiveFailed.
func ArchiveOwnerTips(hubPath string, pairs []OwnerArchivePair) ([]string, error) {
	rec := NewMutations(hubPath)
	var tags []string
	var failures []error
	for _, pair := range pairs {
		pushed, err := archiveOwnerPair(rec, hubPath, pair)
		tags = append(tags, pushed...)
		if err != nil {
			failures = append(failures, fmt.Errorf("%w: pair %s: %w", ErrArchiveFailed, pair.Slug, err))
		}
	}
	return tags, errors.Join(failures...)
}

// archiveOwnerPair archives the code and records tips of pair under its slug's teardown lock.
// It returns the tags it pushed.
func archiveOwnerPair(rec *Mutations, hubPath string, pair OwnerArchivePair) ([]string, error) {
	if err := ensureTeardownLockDir(hubPath, pair.Slug); err != nil {
		return nil, err
	}
	held, acquired, err := lock.AcquireWriteLockWithin(TeardownLockPath(hubPath, pair.Slug), StatusPushLockWait)
	if err != nil {
		return nil, fmt.Errorf("take the teardown lock: %w", err)
	}
	if !acquired {
		return nil, fmt.Errorf("a push or removal of the pair still holds its teardown lock: %w", ErrPairPushRunning)
	}
	defer func() { _ = held.Release() }()

	var tags []string
	for _, side := range []struct{ name, dir string }{
		{"code", pair.Pair.WorktreePath()},
		{"records", RecordsWorktree(pair.Pair)},
	} {
		tag, err := archiveOwnerTip(rec, side.dir, pair.Slug, side.name)
		if err != nil {
			return tags, fmt.Errorf("archive the %s tip: %w", side.name, err)
		}
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags, nil
}

// archiveOwnerTip tags and pushes the tip of the branch checked out at dir when origin lacks it, and returns the tag.
// It returns no tag for a directory that is not there, a repository with no origin remote and a tip origin holds.
// A detached checkout is an error.
// side names the branch's side, code or records, in the refusal for a tag of that name sitting elsewhere.
func archiveOwnerTip(rec *Mutations, dir, slug, side string) (string, error) {
	if !pathPresent(dir) {
		return "", nil
	}
	repo := gitrepo.New(dir)
	if _, err := repo.RemoteURL(originRemoteName); err != nil {
		return "", nil
	}
	branch, err := repo.CurrentBranch()
	if err != nil {
		return "", fmt.Errorf("read the branch checked out at %s: %w", dir, err)
	}
	tip, err := localBranchTip(dir, branch)
	if err != nil || tip == "" {
		return "", err
	}
	originTip, err := remoteHeadTip(dir, branch)
	if err != nil {
		return "", err
	}
	if originTip == tip {
		return "", nil
	}
	if originTip != "" {
		if _, err := repo.FetchWithArgs(originRemoteName, originTip); err != nil {
			return "", fmt.Errorf("fetch the tip %s of branch %q from %q: %w", originTip, branch, originRemoteName, err)
		}
		originHolds, err := isAncestorOrEqual(dir, tip, originTip)
		if err != nil {
			return "", fmt.Errorf("compare the local and origin tips of branch %q: %w", branch, err)
		}
		if originHolds {
			return "", nil
		}
	}
	tag := OwnerArchiveTagName(slug, tip)
	if err := createTagAt(dir, tag, tip, side); err != nil {
		return "", err
	}
	if err := pushArchiveTag(rec, dir, tag, nil); err != nil {
		return "", err
	}
	return tag, nil
}
