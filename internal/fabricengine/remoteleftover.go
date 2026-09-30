// remoteleftover.go holds Add's read-only pre-flight probes of both origins.
// A branch a removed pair left on the warp or weft origin is either proven replaceable or refused
// before Add's first mutation, so the refusal never arrives mid-Add as a rejected push.
// Every probe is read-only git through gitexec; the one fetch (the weft archive probe) writes
// FETCH_HEAD only and creates no branch, remote-tracking ref or tag.

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
type leftoverKind int

const (
	// leftoverUnarchivedWeft is a remote weft branch no archive tag covers, with no local weft branch.
	leftoverUnarchivedWeft leftoverKind = iota
	// leftoverDivergedAdoptedWeft is a remote weft branch that diverges from the local one Add would adopt.
	leftoverDivergedAdoptedWeft
	// leftoverDivergedWarp is a remote warp branch that is neither the new branch's base nor an ancestor of it.
	leftoverDivergedWarp
)

// ErrRemoteLeftover is Add's refusal when a leftover remote branch from a removed pair cannot be proven replaceable.
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
	case leftoverUnarchivedWeft:
		return fmt.Sprintf(
			"weft branch %q already exists on origin at %s and no archive tag covers it; run \"lyx fabric remove %s\" on the hub that holds a live pair (it archives the branch), or delete it with \"git push origin --delete %s\" once no hub holds the pair and its content is disposable, or use a different slug",
			e.Branch, e.RemoteTip, e.Slug, e.Branch)
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

// weftLeftover is probeWeftLeftover's answer: the remote weft tip and the archive tag covering it.
// Both are empty when there is nothing to replace.
type weftLeftover struct {
	tip string
	tag string
}

// probeWeftLeftover inspects the weft origin for a leftover weftBranch, run in the weft repo against originRemoteName.
// An absent remote branch returns the zero answer.
// With localExists (Add adopts the local branch), the remote tip must equal or be an ancestor of the local tip.
// Without it, the tip must be covered by an archive/<slug>/* tag whose target equals or descends from it.
func probeWeftLeftover(l *lyxcwd.Location, slug, weftBranch string, localExists bool) (weftLeftover, error) {
	weftRoot, err := WeftRepoRoot(l)
	if err != nil {
		return weftLeftover{}, fmt.Errorf("resolve weft repo root: %w", err)
	}
	tip, err := remoteHeadTip(weftRoot, weftBranch)
	if err != nil {
		return weftLeftover{}, err
	}
	if tip == "" {
		return weftLeftover{}, nil
	}

	if localExists {
		ok, err := isAncestorOrEqual(weftRoot, tip, "refs/heads/"+weftBranch)
		if err != nil {
			return weftLeftover{}, err
		}
		if ok {
			return weftLeftover{}, nil
		}
		return weftLeftover{}, &ErrRemoteLeftover{Slug: slug, Branch: weftBranch, RemoteTip: tip, kind: leftoverDivergedAdoptedWeft}
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
	refused := &ErrRemoteLeftover{Slug: slug, Branch: weftBranch, RemoteTip: tip, kind: leftoverUnarchivedWeft}
	if len(tags) == 0 {
		return weftLeftover{}, refused
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
	return weftLeftover{}, refused
}

// probeWarpLeftover inspects the warp origin for a leftover warpBranch, run in the warp worktree.
// It returns nil when the branch is absent or its tip equals or is an ancestor of the HEAD the new branch forks from.
// It never fetches.
func probeWarpLeftover(l *lyxcwd.Location, warpBranch string) error {
	dir := l.WorktreePath()
	tip, err := remoteHeadTip(dir, warpBranch)
	if err != nil {
		return err
	}
	if tip == "" {
		return nil
	}
	ok, err := isAncestorOrEqual(dir, tip, "HEAD")
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return &ErrRemoteLeftover{Branch: warpBranch, RemoteTip: tip, kind: leftoverDivergedWarp}
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
