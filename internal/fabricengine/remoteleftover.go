// remoteleftover.go holds the read-only probes that Add runs against both origins before its first mutation.
// A branch on the weft or warp origin either makes the pair live or is the leftover of a removed pair.
// A live pair's branch is adopted;
// a leftover is proven replaceable or refused before the first mutation,
// so the refusal never arrives later as a rejected push of the new pair.
// Every probe is read-only git through gitexec.
// The fetches write FETCH_HEAD only and create no branch, remote-tracking ref or tag.

package fabricengine

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// leftoverKind is the closed set of reasons a remote branch is refused as a leftover.
// A remote weft branch that no archive tag covers is not one of them: it makes the pair live,
// and Add adopts it.
type leftoverKind int

const (
	// leftoverDivergedAdoptedWeft is a remote weft branch that diverges from the local one Add would adopt.
	leftoverDivergedAdoptedWeft leftoverKind = iota
	// leftoverDivergedWarp is a remote warp branch that is neither the new branch's base nor an ancestor of it, on a pair that is not live.
	leftoverDivergedWarp
)

// ErrRemoteLeftover is the refusal of Add when a remote branch can be neither adopted nor proven replaceable.
// It covers a remote weft branch that diverges from the local one, and a remote warp branch left by a removed pair.
type ErrRemoteLeftover struct {
	// Slug is the slug Add was asked to create.
	Slug string
	// Branch is the leftover remote branch, prefix and suffix included.
	Branch string
	// RemoteTip is the full SHA the leftover branch sits at on origin.
	RemoteTip string

	kind leftoverKind
}

// Error implements the error interface, naming the branch, its remote tip and the remedies for the kind.
func (e *ErrRemoteLeftover) Error() string {
	switch e.kind {
	case leftoverDivergedAdoptedWeft:
		return fmt.Sprintf(
			"weft branch %q on origin (at %s) diverges from the local %q this add would adopt; delete it with \"git push origin --delete %s\" once its content is disposable, or use a different slug",
			e.Branch, e.RemoteTip, e.Branch, e.Branch)
	default:
		return fmt.Sprintf(
			"warp branch %q already exists on origin at %s and is not an ancestor of this worktree's HEAD; delete it with \"git push origin --delete %s\" once its work is landed, or use a different slug",
			e.Branch, e.RemoteTip, e.Branch)
	}
}

// weftLeftover is the outcome of probing the weft origin for a slug's branch.
// live is true when the pair is live: the weft branch is then taken from the local copy or from origin, not forked.
// fastForwardTo is the origin tip that a live pair's local weft branch advances to;
// it is empty when no advance is needed.
// tip and tag are the remote weft tip and the archive tag covering it, set only for a replaceable archived leftover.
// originAbsent is true when origin holds no such weft branch, whether or not a local copy makes the pair live.
// When no probe runs, only live is set from the local branch, so originAbsent stays false and nothing is dropped.
type weftLeftover struct {
	live          bool
	fastForwardTo string
	tip           string
	tag           string
	originAbsent  bool
}

// probeWeftLeftover inspects the weft origin for weftBranch, run in the weft repo against originRemoteName.
// With localExists the pair is live:
// an origin tip that equals or is an ancestor of the local tip needs nothing,
// a local tip that is a strict ancestor of the origin tip becomes fastForwardTo,
// and a true divergence is refused.
// Without localExists, an absent remote branch returns the answer with only originAbsent set.
// An origin tip is a replaceable leftover when an archive/<slug>/* tag's target equals or descends from it.
// Such a leftover is reported with tip and tag set.
// Any other origin tip makes the pair live.
func probeWeftLeftover(l *lymcwd.Location, slug, weftBranch string, localExists bool) (weftLeftover, error) {
	weftRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return weftLeftover{}, fmt.Errorf("resolve weft repo root: %w", err)
	}
	tip, err := remoteHeadTip(weftRoot, weftBranch)
	if err != nil {
		return weftLeftover{}, err
	}
	if tip == "" {
		return weftLeftover{live: localExists, originAbsent: true}, nil
	}

	if localExists {
		return probeLiveLocalWeft(weftRoot, slug, weftBranch, tip)
	}

	tags, err := remoteArchiveTags(weftRoot, slug)
	if err != nil {
		return weftLeftover{}, err
	}
	for name, target := range tags {
		if target == tip {
			return weftLeftover{tip: tip, tag: name}, nil
		}
	}
	if len(tags) == 0 {
		return weftLeftover{live: true}, nil
	}

	// FETCH_HEAD only: no local branch, remote-tracking ref or tag is created.
	args := []string{"fetch", "--no-tags", "--refmap=", originRemoteName, "refs/heads/" + weftBranch}
	for name := range tags {
		args = append(args, "refs/tags/"+name)
	}
	if _, err := gitexec.Run(args, weftRoot); err != nil {
		return weftLeftover{}, fmt.Errorf("fetch weft branch %q and its archive tags from %q: %w", weftBranch, originRemoteName, err)
	}
	for name, target := range tags {
		ok, err := isAncestorOrEqual(weftRoot, tip, target)
		if err != nil {
			return weftLeftover{}, err
		}
		if ok {
			return weftLeftover{tip: tip, tag: name}, nil
		}
	}
	return weftLeftover{live: true}, nil
}

// probeLiveLocalWeft decides what a live pair's existing local weftBranch needs given the origin tip it found there.
// The origin tip is fetched into FETCH_HEAD only when it is not already an ancestor of the local tip.
func probeLiveLocalWeft(weftRoot, slug, weftBranch, originTip string) (weftLeftover, error) {
	behindOrEqual, err := isAncestorOrEqual(weftRoot, originTip, "refs/heads/"+weftBranch)
	if err != nil {
		return weftLeftover{}, err
	}
	if behindOrEqual {
		return weftLeftover{live: true}, nil
	}

	if _, err := gitexec.Run([]string{"fetch", "--no-tags", "--refmap=", originRemoteName, "refs/heads/" + weftBranch}, weftRoot); err != nil {
		return weftLeftover{}, fmt.Errorf("fetch weft branch %q from %q: %w", weftBranch, originRemoteName, err)
	}
	localTip, err := gitrepo.New(weftRoot).RefSHA("refs/heads/" + weftBranch)
	if err != nil {
		return weftLeftover{}, fmt.Errorf("resolve local weft branch %q: %w", weftBranch, err)
	}
	ahead, err := isAncestorOrEqual(weftRoot, localTip, originTip)
	if err != nil {
		return weftLeftover{}, err
	}
	if ahead {
		return weftLeftover{live: true, fastForwardTo: originTip}, nil
	}
	return weftLeftover{}, &ErrRemoteLeftover{Slug: slug, Branch: weftBranch, RemoteTip: originTip, kind: leftoverDivergedAdoptedWeft}
}

// judgeWarpLeftover judges tip, the origin tip of warpBranch as the caller read it; "" means origin lacks the branch.
// It returns the origin tip to adopt, or "" to fork from HEAD, and onOrigin, whether origin holds the branch at all.
// For a live pair a present origin branch is adopted whatever its relation to HEAD.
// For a pair that is not live it returns "" when the branch is absent or its tip equals or is an ancestor of HEAD;
// otherwise it refuses.
// It touches no remote.
func judgeWarpLeftover(l *lymcwd.Location, slug, warpBranch, tip string, live bool) (adoptTip string, onOrigin bool, err error) {
	dir := l.WorktreePath()
	if tip == "" {
		return "", false, nil
	}
	if live {
		return tip, true, nil
	}
	ok, err := isAncestorOrEqual(dir, tip, "HEAD")
	if err != nil || ok {
		return "", true, err
	}
	return "", true, &ErrRemoteLeftover{Slug: slug, Branch: warpBranch, RemoteTip: tip, kind: leftoverDivergedWarp}
}

// dropStaleOriginTracking drops the remote-tracking ref and upstream setting of the weft branch when weftAbsent;
// it does the same for the warp branch when warpAbsent.
// Each drop runs in its own repository; each flag is a probe's answer that origin holds no such branch.
// A removed pair can leave such a ref behind for a branch that origin lacks.
// Kept, it would make a later reconcile read the new pair's unpushed branch as gone from origin.
// A failed drop is returned naming the branch.
func dropStaleOriginTracking(l *lymcwd.Location, weftBranch string, weftAbsent bool, warpBranch string, warpAbsent bool) error {
	if weftAbsent {
		weftRoot, err := RecordsRepoRoot(l)
		if err != nil {
			return fmt.Errorf("resolve weft repo root: %w", err)
		}
		if err := gitrepo.New(weftRoot).DropRemoteTracking(originRemoteName, weftBranch); err != nil {
			return fmt.Errorf("drop the stale remote-tracking ref of weft branch %q: %w", weftBranch, err)
		}
	}
	if warpAbsent {
		if err := gitrepo.New(l.WorktreePath()).DropRemoteTracking(originRemoteName, warpBranch); err != nil {
			return fmt.Errorf("drop the stale remote-tracking ref of warp branch %q: %w", warpBranch, err)
		}
	}
	return nil
}

// remoteHeadTip returns the SHA of branch on originRemoteName as seen from dir, or "" when it is absent.
func remoteHeadTip(dir, branch string) (string, error) {
	out, err := gitexec.Run([]string{"ls-remote", "--heads", originRemoteName, "refs/heads/" + branch}, dir)
	if err != nil {
		return "", fmt.Errorf("look up branch %q on %q: %w", branch, originRemoteName, err)
	}
	for _, line := range strings.Split(out, "\n") {
		sha, ref, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if ok && ref == "refs/heads/"+branch {
			return sha, nil
		}
	}
	return "", nil
}

// remoteArchiveTags maps each archive/<slug>/* tag name on origin to its commit target, preferring a peeled ^{} line.
func remoteArchiveTags(weftRoot, slug string) (map[string]string, error) {
	prefix := "refs/tags/" + ArchiveTagPrefix + slug + "/"
	out, err := gitexec.Run([]string{"ls-remote", "--tags", originRemoteName, prefix + "*"}, weftRoot)
	if err != nil {
		return nil, fmt.Errorf("list archive tags of %q on %q: %w", slug, originRemoteName, err)
	}
	plain := map[string]string{}
	peeled := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		sha, ref, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok || !strings.HasPrefix(ref, prefix) {
			continue
		}
		if name, isPeeled := strings.CutSuffix(ref, "^{}"); isPeeled {
			peeled[strings.TrimPrefix(name, "refs/tags/")] = sha
		} else {
			plain[strings.TrimPrefix(ref, "refs/tags/")] = sha
		}
	}
	for name, sha := range peeled {
		plain[name] = sha
	}
	return plain, nil
}

// isAncestorOrEqual reports whether sha equals or is an ancestor of ref in the repo at dir.
// A commit missing locally counts as not an ancestor.
func isAncestorOrEqual(dir, sha, ref string) (bool, error) {
	if _, err := gitexec.Run([]string{"cat-file", "-e", sha + "^{commit}"}, dir); err != nil {
		var gitErr *gitexec.GitError
		if errors.As(err, &gitErr) {
			return false, nil
		}
		return false, err
	}
	return gitrepo.New(dir).IsAncestor(sha, ref)
}
