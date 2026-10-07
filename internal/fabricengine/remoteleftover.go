// remoteleftover.go holds Add's read-only pre-flight probes of both origins.
// A branch on the weft or warp origin either makes the pair live or is a removed pair's leftover.
// Add adopts a live pair's branch;
// a leftover is proven replaceable or refused before Add's first mutation,
// so the refusal never arrives mid-Add as a rejected push.
// Every probe is read-only git through gitexec;
// the fetches (the weft archive and fast-forward probes) write FETCH_HEAD only and create no branch, remote-tracking ref or tag.

package fabricengine

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// leftoverKind is the closed set of reasons ErrRemoteLeftover refuses.
// A remote weft branch no archive tag covers is not one of them: it makes the pair live,
// and Add adopts it.
type leftoverKind int

const (
	// leftoverDivergedAdoptedWeft is a remote weft branch that diverges from the local one Add would adopt.
	leftoverDivergedAdoptedWeft leftoverKind = iota
	// leftoverDivergedWarp is a remote warp branch that is neither the new branch's base nor an ancestor of it, on a pair that is not live.
	leftoverDivergedWarp
)

// ErrRemoteLeftover is Add's refusal when a remote branch cannot be adopted or proven replaceable: a remote weft branch that diverges from the local one, or a remote warp branch left by a removed pair.
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

// weftLeftover is probeWeftLeftover's answer.
// live is true when the pair is live: Add adopts the weft branch, from the local copy or from origin, instead of forking one.
// fastForwardTo is the origin tip a live pair's behind local weft branch advances to, empty when it needs no advance.
// tip and tag are the remote weft tip and the archive tag covering it, set only for an archived leftover Add replaces.
// A zero answer means there is nothing on origin and the pair is not live.
type weftLeftover struct {
	live          bool
	fastForwardTo string
	tip           string
	tag           string
}

// probeWeftLeftover inspects the weft origin for weftBranch, run in the weft repo against originRemoteName.
// With localExists the pair is live:
// an origin tip that equals or is an ancestor of the local tip needs nothing,
// a local tip that is a strict ancestor of the origin tip becomes fastForwardTo,
// and a true divergence is refused.
// Without it, an absent remote branch returns the zero answer,
// an origin tip covered by an archive/<slug>/* tag whose target equals or descends from it is a replaceable leftover (tip and tag),
// and any other origin tip makes the pair live.
func probeWeftLeftover(l *lyxcwd.Location, slug, weftBranch string, localExists bool) (weftLeftover, error) {
	weftRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return weftLeftover{}, fmt.Errorf("resolve weft repo root: %w", err)
	}
	tip, err := remoteHeadTip(weftRoot, weftBranch)
	if err != nil {
		return weftLeftover{}, err
	}
	if tip == "" {
		return weftLeftover{live: localExists}, nil
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
	localTip, err := gitexec.Run([]string{"rev-parse", "refs/heads/" + weftBranch}, weftRoot)
	if err != nil {
		return weftLeftover{}, fmt.Errorf("resolve local weft branch %q: %w", weftBranch, err)
	}
	ahead, err := isAncestorOrEqual(weftRoot, strings.TrimSpace(localTip), originTip)
	if err != nil {
		return weftLeftover{}, err
	}
	if ahead {
		return weftLeftover{live: true, fastForwardTo: originTip}, nil
	}
	return weftLeftover{}, &ErrRemoteLeftover{Slug: slug, Branch: weftBranch, RemoteTip: originTip, kind: leftoverDivergedAdoptedWeft}
}

// warpOriginState is what Add's pre-flight learned about the warp branch on origin.
type warpOriginState int

const (
	// warpOriginNotProbed means no probe ran (SkipGit or SkipPush), so nothing is known about origin.
	warpOriginNotProbed warpOriginState = iota
	// warpOriginAbsent means origin has no such branch, so any branch there later is this Add's own push.
	warpOriginAbsent
	// warpOriginPresent means origin already holds the branch: adopted, or a leftover at or behind HEAD.
	warpOriginPresent
)

// probeWarpLeftover inspects the warp origin for warpBranch, run in the warp worktree.
// It returns the origin tip Add adopts, or "" when it forks from HEAD, together with whether origin held the branch at all.
// For a live pair a present origin branch is adopted whatever its relation to HEAD.
// For a pair that is not live it returns "" when the branch is absent or its tip equals or is an ancestor of the HEAD the new branch forks from, and refuses otherwise.
// It never fetches.
func probeWarpLeftover(l *lyxcwd.Location, slug, warpBranch string, live bool) (adoptTip string, origin warpOriginState, err error) {
	dir := l.WorktreePath()
	tip, err := remoteHeadTip(dir, warpBranch)
	if err != nil {
		return "", warpOriginNotProbed, err
	}
	if tip == "" {
		return "", warpOriginAbsent, nil
	}
	if live {
		return tip, warpOriginPresent, nil
	}
	ok, err := isAncestorOrEqual(dir, tip, "HEAD")
	if err != nil {
		return "", warpOriginNotProbed, err
	}
	if ok {
		return "", warpOriginPresent, nil
	}
	return "", warpOriginNotProbed, &ErrRemoteLeftover{Slug: slug, Branch: warpBranch, RemoteTip: tip, kind: leftoverDivergedWarp}
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
	prefix := "refs/tags/archive/" + slug + "/"
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
