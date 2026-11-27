// ownerarchive.go archives the local tips a machine keeps of a subject another machine now owns.
// Each tip origin lacks is tagged archive-owner/<slug>/<sha12>, and the tag is pushed to origin with no branch push,
// so the new owner can reach the commits it cannot see.
// It also holds the new owner's read of those tags against each slug's branch on origin.

package fabricengine

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

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
		if errors.Is(err, gitrepo.ErrRemoteNotConfigured) {
			return "", nil
		}
		return "", fmt.Errorf("read the %q remote of %s: %w", originRemoteName, dir, err)
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

// The repository sides an owner archive tag sits in.
const (
	OwnerArchiveSideCode    = "code"
	OwnerArchiveSideRecords = "records"
)

// OwnerArchive is one owner archive tag on origin whose commit the slug's branch on that origin does not contain.
type OwnerArchive struct {
	// Slug is the pair the tag archives.
	Slug string `json:"slug"`
	// Side is the repository the tag sits in, OwnerArchiveSideCode or OwnerArchiveSideRecords.
	Side string `json:"side"`
	Tag  string `json:"tag"`
	// Commit is the full hex name of the commit the tag names.
	Commit string `json:"commit"`
	// Branch is the slug's branch on that side's origin the commit was judged against.
	Branch string `json:"branch"`
	// Dir is the local repository the tag was fetched into, where `git log <tag>` shows it.
	Dir string `json:"dir"`
}

// ReadOwnerArchives reads the owner archive tags on the code and records origins of the hub of prime.
// It returns those of the slugs keep admits whose commit the slug's branch on that side's origin does not contain.
// The result is sorted by slug, side and tag.
// primeSlug names the prime's pair in the tags, and its code branch is the branch the prime has checked out.
// Any other slug's code branch is its pair's task branch, read locally first and then from origin.
// A records tag is judged against the records branch paired with that code branch,
// and a branch origin lacks contains no commit.
// Each tag it returns is fetched into its side's repository as a local tag, so `git log <tag>` shows it there;
// it writes nothing else but the fetched objects.
// A repository with no origin remote holds no tag to read, and an origin that does not answer is an error.
func (t *Topology) ReadOwnerArchives(prime *lymcwd.Location, primeSlug string, keep func(slug string) bool) ([]OwnerArchive, error) {
	recordsDir, err := RecordsRepoRoot(prime)
	if err != nil {
		return nil, fmt.Errorf("resolve the records repository: %w", err)
	}
	codeBranches := map[string]string{}
	codeBranchOf := func(slug string) (string, error) {
		if branch, read := codeBranches[slug]; read {
			return branch, nil
		}
		var branch string
		var err error
		if slug == primeSlug {
			branch, err = gitrepo.New(prime.WorktreePath()).CurrentBranch()
		} else {
			branch, err = t.PairTaskBranchOrOrigin(prime, slug)
		}
		if err != nil {
			return "", fmt.Errorf("read the code branch of %s: %w", slug, err)
		}
		codeBranches[slug] = branch
		return branch, nil
	}

	var archives []OwnerArchive
	for _, side := range []struct {
		name, dir string
		branchOf  func(codeBranch string) string
	}{
		{OwnerArchiveSideCode, prime.WorktreePath(), func(codeBranch string) string { return codeBranch }},
		{OwnerArchiveSideRecords, recordsDir, RecordsBranchName},
	} {
		branchOf := func(slug string) (string, error) {
			codeBranch, err := codeBranchOf(slug)
			if err != nil {
				return "", err
			}
			return side.branchOf(codeBranch), nil
		}
		found, err := readSideOwnerArchives(side.name, side.dir, keep, branchOf)
		if err != nil {
			return nil, fmt.Errorf("read the %s owner archives: %w", side.name, err)
		}
		archives = append(archives, found...)
	}
	slices.SortFunc(archives, func(a, b OwnerArchive) int {
		return cmp.Or(strings.Compare(a.Slug, b.Slug), strings.Compare(a.Side, b.Side), strings.Compare(a.Tag, b.Tag))
	})
	return archives, nil
}

// readSideOwnerArchives returns the owner archive tags on the origin of the repository at dir, on side.
// It returns those of the slugs keep admits whose commit the branch branchOf names for the slug does not contain.
// It fetches each tag it returns as a local tag of dir.
func readSideOwnerArchives(side, dir string, keep func(slug string) bool, branchOf func(slug string) (string, error)) ([]OwnerArchive, error) {
	if !pathPresent(dir) {
		return nil, nil
	}
	repo := gitrepo.New(dir)
	if _, err := repo.RemoteURL(originRemoteName); err != nil {
		if errors.Is(err, gitrepo.ErrRemoteNotConfigured) {
			return nil, nil
		}
		return nil, fmt.Errorf("read the %q remote of %s: %w", originRemoteName, dir, err)
	}
	tags, err := remoteTagsUnder(dir, OwnerArchiveTagPrefix)
	if err != nil {
		return nil, fmt.Errorf("list the owner archive tags on %q: %w", originRemoteName, err)
	}
	var archives []OwnerArchive
	for tag, commit := range tags {
		slug, named := ownerArchiveSlug(tag)
		if !named || !keep(slug) {
			continue
		}
		branch, err := branchOf(slug)
		if err != nil {
			return nil, err
		}
		branchTip, err := remoteHeadTip(dir, branch)
		if err != nil {
			return nil, err
		}
		fetch := []string{originRemoteName, "refs/tags/" + tag}
		if branchTip != "" {
			fetch = append(fetch, branchTip)
		}
		if _, err := repo.FetchWithArgs(fetch...); err != nil {
			return nil, fmt.Errorf("fetch the tag %s from %q: %w", tag, originRemoteName, err)
		}
		if branchTip != "" {
			contained, err := isAncestorOrEqual(dir, commit, branchTip)
			if err != nil {
				return nil, fmt.Errorf("compare the tag %s with branch %q on %q: %w", tag, branch, originRemoteName, err)
			}
			if contained {
				continue
			}
		}
		if err := createTagAt(dir, tag, commit, side); err != nil {
			return nil, err
		}
		archives = append(archives, OwnerArchive{Slug: slug, Side: side, Tag: tag, Commit: commit, Branch: branch, Dir: dir})
	}
	return archives, nil
}

// ownerArchiveSlug returns the slug an owner archive tag name carries, reporting false for a name of another shape.
func ownerArchiveSlug(tag string) (string, bool) {
	rest, prefixed := strings.CutPrefix(tag, OwnerArchiveTagPrefix)
	if !prefixed {
		return "", false
	}
	cut := strings.LastIndex(rest, "/")
	if cut <= 0 {
		return "", false
	}
	return rest[:cut], true
}
