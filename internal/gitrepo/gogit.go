// gogit.go implements the go-git handle infrastructure every migrated read in
// later batches builds on: goGit, the lazily-opened and cached *git.Repository
// accessor. Nothing in this file changes any existing method's backend — see
// gitrepo.go's Repo struct doc and this file's own godoc for the locking
// discipline it establishes.

package gitrepo

import (
	"fmt"

	"github.com/go-git/go-git/v5"
)

// goGit returns this Repo's go-git handle, opening it on first use via
// git.PlainOpenWithOptions and caching it for every later call — but only
// once the open has succeeded. A failed open is never cached: New's
// documented posture is that the checkout need not exist yet (fabricengine
// may create the worktree at r.path only after a Repo already wraps it), so
// caching a failure would make such a Repo permanently broken instead of
// merely early.
//
// # Why EnableDotGitCommonDir, and why not PlainOpen or DetectDotGit
//
// The open is git.PlainOpenWithOptions(r.path, &git.PlainOpenOptions{
// EnableDotGitCommonDir: true}) — never git.PlainOpen, and DetectDotGit is
// never set. This is measured, not cautious (see
// .scratch/gogit-worktree-probe-report.md): against a linked worktree (this
// checkout's own shape — fabricengine's host and weft worktrees, reached
// through internal/fslink junctions), PlainOpen returns NO error and hands
// back a handle that cannot read HEAD, cannot read any object, and reports
// every existing refs/loomyard/snapshot/* key as absent — forever, with no
// error anywhere. EnableDotGitCommonDir defaults to false, so the
// convenience call is the wrong one. DetectDotGit is worse than merely
// wrong: pointed at a path that is not itself a repository, it walks *up*
// the directory tree and silently opens whatever ancestor repository it
// finds — proven, in the probe, to escape a fixture directory and open this
// very loomyard checkout. gitrepo's callers always pass a worktree root, so
// DetectDotGit buys nothing and risks operating on the wrong repository
// entirely. KeepDescriptors is left at its default (false) deliberately:
// with it true, the common dir's packfiles stay open and lock against `git
// worktree remove`/`git gc` on Windows; false is what keeps a long-lived
// cached handle from blocking fabricengine's topology verbs.
//
// A successful open is deliberately not treated as full validation beyond
// what PlainOpenWithOptions itself checks — an extensions.worktreeConfig
// repository is refused by go-git outright (git.ErrUnsupportedExtensionRepositoryFormatVersion,
// checkable via errors.Is on the returned error), and an incomplete linked
// worktree whose commondir file points nowhere fails with the typed
// git.ErrRepositoryIncomplete — both wrapped below so a caller sees a
// gitrepo-owned error naming this package instead of a bare go-git message,
// while remaining checkable against go-git's own sentinels through the %w
// wrap.
//
// # The caller's locking obligation — goGit cannot enforce this itself
//
// goGit's own write lock (r.goGitMu.Lock, below) covers only the
// cache-check-and-open step in this method's body: the lock is acquired and
// released (via defer) entirely within this call, so by the time goGit
// returns, the *git.Repository it hands back is completely unprotected.
// go-git's *filesystem.Storage builds its object index lazily on first read
// and is not safe for unsynchronized concurrent use — this is shared
// mutable state reached from every goroutine that calls goGit on the same
// Repo.
//
// Every caller — every migrated read in batches 3 and 4 — must therefore
// hold r.goGitMu for the WHOLE DURATION of its own use of the returned
// handle, not merely across the call to goGit() itself: wrapping only
// `r.goGit()` in a lock protects nothing, since goGit's internal lock has
// already been released by the time it returns. The package's locking
// discipline, stated once here because every migrating card works from it:
//
//   - A plain ref read (Head, an unresolved Reference — CurrentSHA,
//     CurrentBranch, remoteName, and both snapshot ref reads) acquires
//     r.goGitMu.RLock for the duration of the go-git call it makes with the
//     handle goGit returned, then releases it. go-git never caches refs, so
//     there is no lazy-index mutation to protect against on this path —
//     only the concurrent-map-style safety a plain read needs.
//   - An object lookup (a commit, tree, or blob resolution — SHAExists,
//     ChangedFilesSince, isStrictDescendant, hasUnpushed, and
//     SetSnapshotSHA's ^{commit} canonicalization) never locks around the
//     handle itself; it calls the pack-fingerprint-gated reindex-and-retry
//     lookup helper instead (added alongside this method in the same
//     package), which acquires r.goGitMu.Lock (not RLock) for its entire
//     attempt-check-reindex-retry sequence as one unit.
//
// This is the single point of truth for that discipline; it exists in code,
// here, rather than only in the plan, precisely so a future implementer
// reads it before writing the next migrated call site.
//
// Exercised by gogit_test.go; no migrated read calls goGit yet in this batch
// (that starts in batch 3), so golangci-lint's default (untagged) build sees
// no caller, matching gitnativepoc/read.go's identical hasUnpushed
// precedent.
//
//nolint:unused // only exercised by the //go:build integration-tagged gogit_test.go
func (r *Repo) goGit() (*git.Repository, error) {
	r.goGitMu.Lock()
	defer r.goGitMu.Unlock()

	if r.goGitOK {
		return r.goGitRepo, nil
	}

	repo, err := git.PlainOpenWithOptions(r.path, &git.PlainOpenOptions{
		EnableDotGitCommonDir: true,
	})
	if err != nil {
		return nil, fmt.Errorf("gitrepo: open go-git handle at %s: %w", r.path, err)
	}

	r.goGitRepo = repo
	r.goGitOK = true
	return repo, nil
}
