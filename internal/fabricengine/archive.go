// archive.go archives the tips a teardown deletes, under archive/<slug>/ tags pushed to origin.
// It tags and pushes the local tip, and also an origin tip the local tip does not contain.
// The tags keep the run records on those tips reachable after their branch is gone.
// Tag creation and push destroy nothing, so they stay out of destroy.go.

package fabricengine

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// ArchiveTagSHALen is how many leading hex digits of the tip SHA the archive tag name carries.
const ArchiveTagSHALen = 12

// archiveTagPushHook is nil in production.
// When set, it runs before each attempt of an archive-tag push with the tag's full ref.
// An error it returns is that attempt's failure.
var archiveTagPushHook func(tagRef string) error

// archiveTagPushDelays returns the sleeps between attempts of an archive-tag push the remote refuses retryably.
// The delays double from 2 s to 32 s, six attempts in all.
// A teardown archives while it holds the pair's teardown lock exclusively.
// The pair's pushes, an Add and a discard of the slug, contend for that lock too.
// So the schedule waits out a remote hiccup of about a minute; a plain push's shorter schedule would fail the row.
func archiveTagPushDelays() []time.Duration {
	return []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second}
}

// ArchiveTagPrefix is the short-name prefix every archive tag carries.
const ArchiveTagPrefix = "archive/"

// ArchiveTagSlug returns the slug of the short tag name archive/<slug>/<sha>.
// The bool is false unless name has the archive prefix, a non-empty slug segment, and a non-empty final segment.
func ArchiveTagSlug(name string) (string, bool) {
	rest, ok := strings.CutPrefix(name, ArchiveTagPrefix)
	if !ok {
		return "", false
	}
	slug, tip, ok := strings.Cut(rest, "/")
	if !ok || slug == "" || tip == "" {
		return "", false
	}
	return slug, true
}

// ErrArchiveFailed is the sentinel every archive error wraps.
// A teardown caller tests for it with errors.Is to tell a failed archive from other refusals.
// A failed archive leaves the pair untouched, so a plain re-run retries it.
// The message names neither side of the pair, for callers outside the fabric vocabulary.
var ErrArchiveFailed = errors.New("archiving the pair's records before teardown failed")

// weftArchive records the tags and tips of one weft-branch archive.
// The zero value says nothing was archived.
type weftArchive struct {
	// localTag is the archive tag at the local weft tip.
	localTag string
	// originTag is the archive tag at origin's weft tip, set only when that tip is neither the local tip nor an ancestor of it.
	originTag string
	// localTip is the local weft tip, or origin's when the branch exists only there.
	localTip string
	// originTip is origin's weft tip after any branch push, empty when origin has no weft branch.
	originTip string
	// leaseTip is the origin tip a later delete of the weft branch on origin is leased on.
	// It is covered by localTag or originTag.
	leaseTip string
}

// ArchiveTagName returns the archive tag name of a tip: archive/<slug>/<first 12 hex of tip>.
// The tip must hold at least ArchiveTagSHALen characters.
func ArchiveTagName(slug, tip string) string {
	return ArchiveTagPrefix + slug + "/" + tip[:ArchiveTagSHALen]
}

// archiveWeftTip archives the tip of weftBranch under an archive/<slug>/<first 12 hex of the tip SHA> tag.
// It pushes the tag to the weft repo's origin.
// When tip is empty, the tip is the local refs/heads/<weftBranch> when present, else origin's copy, which is fetched.
// A caller that read the tip before deleting the branch passes it as tip.
// The tag name is deterministic from slug and tip, so a resumed teardown on the same tip finds its own tag and succeeds.
// A new tip archives under a new name.
//
// With no origin remote, it archives nothing and returns a skippedReason and no error.
// A branch present neither locally nor on origin also archives nothing, with no reason and no error.
// There is nothing to archive.
//
// With a branch on both sides, it reads origin's tip and fetches it by SHA when it differs from the local tip,
// so both tips are present locally to compare.
// With pushBranch set and origin lacking the local tip, it first pushes the local tip alone to the weft branch:
// origin lacks the local tip when it has no branch, or its tip is neither the local tip nor a descendant of it.
// The push rebases nothing and does not push the code branch.
// It writes the tracking ref under the remote-refs lock and needs no local branch.
// A refused branch push is logged at Warn and never fails the archive.
// It then tags and pushes the local tip.
// When origin's tip after that push is neither the local tip nor an ancestor of it, it tags and pushes that tip too.
// The returned weftArchive names the tags and the origin tip a later delete is leased on.
//
// An existing tag of that name pointing elsewhere, and any tag push failure, are returned errors.
// A push refused with a bare "(failed)", "cannot lock ref" or "missing necessary objects" is retried.
// Retries wait on the archive push's delay schedule, sleeping through sleep, and a nil sleep sleeps for real.
// Every returned error wraps ErrArchiveFailed.
// KindTagPushed is recorded on rec only after the push observably succeeded.
func archiveWeftTip(rec *Mutations, l *lymcwd.Location, slug, weftBranch, tip string, pushBranch bool, sleep func(time.Duration)) (archive weftArchive, skippedReason string, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", ErrArchiveFailed, err)
		}
	}()

	weftRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return weftArchive{}, "", fmt.Errorf("archive weft tip of %q: resolve weft repo: %w", weftBranch, err)
	}

	if _, urlErr := gitrepo.New(weftRoot).RemoteURL(originRemoteName); urlErr != nil {
		return weftArchive{}, fmt.Sprintf(
			"no archive tag pushed: the weft repo has no %q remote configured: %v",
			originRemoteName, urlErr), nil
	}

	if tip == "" {
		tip, err = resolveArchiveTip(weftRoot, weftBranch)
		if err != nil {
			return weftArchive{}, "", err
		}
	}
	if tip == "" {
		return weftArchive{}, "", nil
	}

	originTip, err := remoteHeadTip(weftRoot, weftBranch)
	if err != nil {
		return weftArchive{}, "", fmt.Errorf("push archive tag %q to %q: read the tip of weft branch %q there: %w", ArchiveTagName(slug, tip), originRemoteName, weftBranch, err)
	}
	if originTip != "" && originTip != tip {
		if _, err := gitrepo.New(weftRoot).FetchWithArgs(originRemoteName, originTip); err != nil {
			return weftArchive{}, "", fmt.Errorf("fetch the tip %s of weft branch %q from %q: %w", originTip, weftBranch, originRemoteName, err)
		}
	}

	if pushBranch {
		originHoldsTip := originTip == tip
		if originTip != "" && !originHoldsTip {
			originHoldsTip, err = isAncestorOrEqual(weftRoot, tip, originTip)
			if err != nil {
				return weftArchive{}, "", fmt.Errorf("compare the local and origin tips of weft branch %q: %w", weftBranch, err)
			}
		}
		if !originHoldsTip {
			if pushErr := gitrepo.New(weftRoot).PushCommit(originRemoteName, tip, weftBranch); pushErr != nil {
				logger.Warn("fabricengine: pushing the weft branch before its archive tag failed; the tag still holds the local tip", "branch", weftBranch, "error", pushErr)
			} else {
				originTip = tip
			}
		}
	}

	localTag, err := tagAndPushArchiveTip(rec, weftRoot, slug, tip, "weft", sleep)
	if err != nil {
		return weftArchive{}, "", err
	}
	archive = weftArchive{localTag: localTag, localTip: tip, originTip: originTip, leaseTip: originTip}

	if originTip != "" && originTip != tip {
		originBehind, err := isAncestorOrEqual(weftRoot, originTip, tip)
		if err != nil {
			return weftArchive{}, "", fmt.Errorf("compare the origin and local tips of weft branch %q: %w", weftBranch, err)
		}
		if !originBehind {
			archive.originTag, err = tagAndPushArchiveTip(rec, weftRoot, slug, originTip, "weft", sleep)
			if err != nil {
				return weftArchive{}, "", err
			}
		}
	}
	return archive, "", nil
}

// archiveTaskTip archives tip, a commit of the task branch of slug, under its archive tag in the repo at repoDir.
// The tag is archive/<slug>/<first 12 hex of tip>, and it is pushed to the repo's origin.
// A tag already at tip is reused, and a tag of that name elsewhere is refused.
// A push the remote refuses retryably is retried on the archive push's delay schedule, sleeping through sleep.
// A nil sleep sleeps for real.
// Every returned error wraps ErrArchiveFailed.
// KindTagPushed is recorded on rec only after the push observably succeeded.
func archiveTaskTip(rec *Mutations, repoDir, slug, tip string, sleep func(time.Duration)) (string, error) {
	tag, err := tagAndPushArchiveTip(rec, repoDir, slug, tip, "task", sleep)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrArchiveFailed, err)
	}
	return tag, nil
}

// tagAndPushArchiveTip tags tip under its archive name in the repo at repoDir and pushes that tag to origin.
// The name is archive/<slug>/<first 12 hex of tip>, and a tag already at tip is reused.
// side names the branch's side, weft or task, in the refusal for a tag of that name sitting elsewhere.
// A push refused with a bare "(failed)", "cannot lock ref" or "missing necessary objects" is retried.
// Retries wait on the archive push's delay schedule, sleeping through sleep.
func tagAndPushArchiveTip(rec *Mutations, repoDir, slug, tip, side string, sleep func(time.Duration)) (string, error) {
	tag, err := createArchiveTag(repoDir, slug, tip, side)
	if err != nil {
		return "", err
	}
	tagRef := "refs/tags/" + tag

	pushTag := func() error {
		if archiveTagPushHook != nil {
			if err := archiveTagPushHook(tagRef); err != nil {
				return err
			}
		}
		_, err := gitexec.Run([]string{"push", originRemoteName, tagRef}, repoDir)
		return err
	}
	if err := gitrepo.PushWithRetryReasons(pushTag, tagRef, archiveTagPushDelays(), sleep, []string{"missing necessary objects"}); err != nil {
		return "", fmt.Errorf("push archive tag %q to %q: %w", tag, originRemoteName, err)
	}
	rec.Append(KindTagPushed, repoDir, tag)
	return tag, nil
}

// tagWeftTipForArchive tags the local tip of weftBranch before a teardown deletes the branch.
// The tag is archive/<slug>/<first 12 hex of the tip>, and the function returns the tip and the tag.
// A weft repo with no origin remote tags nothing and returns the reason no archive tag is pushed,
// and a branch with no local copy returns an empty tip and tag.
// Every returned error wraps ErrArchiveFailed.
func tagWeftTipForArchive(l *lymcwd.Location, slug, weftBranch string) (tip, tag, skippedReason string, err error) {
	weftRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return "", "", "", fmt.Errorf("%w: archive weft tip of %q: resolve weft repo: %w", ErrArchiveFailed, weftBranch, err)
	}
	if _, urlErr := gitrepo.New(weftRoot).RemoteURL(originRemoteName); urlErr != nil {
		return "", "", fmt.Sprintf("no archive tag pushed: the weft repo has no %q remote configured: %v", originRemoteName, urlErr), nil
	}
	tip, err = localBranchTip(weftRoot, weftBranch)
	if err != nil || tip == "" {
		if err != nil {
			err = fmt.Errorf("%w: %w", ErrArchiveFailed, err)
		}
		return "", "", "", err
	}
	tag, err = createArchiveTag(weftRoot, slug, tip, "weft")
	if err != nil {
		return "", "", "", fmt.Errorf("%w: %w", ErrArchiveFailed, err)
	}
	return tip, tag, "", nil
}

// createArchiveTag tags tip as archive/<slug>/<first 12 hex of tip> in the repo at repoDir.
// It touches no remote and returns the tag name.
// A tag already at tip is reused.
// A tag of that name at another commit is refused, and the error names side as the branch's side.
// A teardown takes it before it deletes a branch locally,
// so the tip stays reachable while the tag's push is still to come.
func createArchiveTag(repoDir, slug, tip, side string) (string, error) {
	tag := ArchiveTagName(slug, tip)
	existing, err := gitexec.Run([]string{"for-each-ref", "--format=%(objectname)", "refs/tags/" + tag}, repoDir)
	if err != nil {
		return "", fmt.Errorf("look up archive tag %q: %w", tag, err)
	}
	switch existing = strings.TrimSpace(existing); {
	case existing == "":
		if _, err := gitexec.Run([]string{"tag", tag, tip}, repoDir); err != nil {
			return "", fmt.Errorf("create archive tag %q at %s: %w", tag, tip, err)
		}
	case existing != tip:
		return "", fmt.Errorf("archive tag %q already exists at %s, not the %s tip %s", tag, existing, side, tip)
	}
	return tag, nil
}

// localBranchTip returns the full SHA of refs/heads/<branch> in the repo at repoDir.
// It returns an empty SHA when the branch does not exist.
func localBranchTip(repoDir, branch string) (string, error) {
	out, err := gitexec.Run([]string{"for-each-ref", "--format=%(objectname)", "refs/heads/" + branch}, repoDir)
	if err != nil {
		return "", fmt.Errorf("resolve branch %q: %w", branch, err)
	}
	return strings.TrimSpace(out), nil
}

// resolveArchiveTip returns the full SHA of weftBranch in the weft repo at weftRoot.
// It is the local branch's SHA when present; otherwise it is the origin's copy, fetched into the repo.
// It returns an empty SHA and no error when the branch exists in neither place.
// It returns an error when the origin cannot be read to find out.
func resolveArchiveTip(weftRoot, weftBranch string) (string, error) {
	if tip, err := localBranchTip(weftRoot, weftBranch); err != nil || tip != "" {
		return tip, err
	}

	listed, err := gitexec.Run([]string{"ls-remote", "--heads", originRemoteName, "refs/heads/" + weftBranch}, weftRoot)
	if err != nil {
		var gitErr *gitexec.GitError
		if errors.As(err, &gitErr) {
			return "", fmt.Errorf("look up weft branch %q on %q: %w", weftBranch, originRemoteName, err)
		}
		return "", err
	}
	// ls-remote matches a pattern by its tail, so only the line naming exactly this branch is its tip.
	tip := ""
	for _, line := range strings.Split(listed, "\n") {
		sha, ref, found := strings.Cut(strings.TrimSpace(line), "\t")
		if found && ref == "refs/heads/"+weftBranch {
			tip = sha
		}
	}
	if tip == "" {
		return "", nil
	}
	// The tip is fetched by SHA, so the answer never rests on FETCH_HEAD.
	// Another fetch of the worktree can rewrite FETCH_HEAD once the lock is released.
	if _, err := gitrepo.New(weftRoot).FetchWithArgs(originRemoteName, tip); err != nil {
		return "", fmt.Errorf("fetch weft branch %q from %q: %w", weftBranch, originRemoteName, err)
	}
	return tip, nil
}
