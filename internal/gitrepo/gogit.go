// gogit.go implements the go-git handle infrastructure every migrated read in
// later batches builds on: goGit, the lazily-opened and cached *git.Repository
// accessor, and lookupObjectRetrying, the pack-fingerprint-gated
// reindex-and-retry helper every migrated object lookup (commit, tree, or
// blob resolution) must route through. Nothing in this file changes any
// existing method's backend — see gitrepo.go's Repo struct doc and this
// file's own godoc for the locking discipline both pieces establish.

package gitrepo

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/filesystem"
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
//     handle itself; it calls lookupObjectRetrying instead, which acquires
//     r.goGitMu.Lock (not RLock) for its entire attempt-check-reindex-retry
//     sequence as one unit — see that function's doc for why a read-only
//     lock is not enough there.
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

// lookupObjectRetrying is the shared object-lookup helper every migrated
// read that resolves a commit, tree, or blob must route through — never
// calling the storer directly. It calls lookup once; if lookup succeeds, or
// fails with anything other than plumbing.ErrObjectNotFound, that result is
// returned unchanged. On an object-not-found miss, it computes the current
// pack fingerprint (packFingerprint, below) of repo's common-dir packfiles
// and compares it against the fingerprint recorded at this Repo's last index
// build (r.lastPackFingerprint): only when the two differ does it call
// Reindex() on the underlying *filesystem.Storage, record the new
// fingerprint, and retry lookup exactly once more. When the fingerprint is
// unchanged, the original not-found is returned as truth, with no rescan —
// this is the gate that keeps a genuinely-absent object from paying a
// reindex cost on every call.
//
// # Why the gate is on-disk state, not a per-Repo call counter
//
// One physical checkout is addressed concurrently by several live *Repo
// values in this process and by a separate OS process entirely —
// internal/fabricengine/fabric.go's cached Warp/Weft handles,
// internal/fabricengine/weftgit.go's PushWeftAt, internal/websterengine/gitwrap.go,
// and internal/fabriccli/spawn.go's detached child all address the same
// worktree independently. A counter kept on one *Repo would never see a
// write made through any of the others, so it could never detect that a
// reindex is needed. The pack fingerprint is shared ground truth that any
// of them can observe.
//
// # Why the whole sequence needs the write lock, not merely the reindex
//
// go-git's *filesystem.Storage builds its object index lazily on first
// read (storage/filesystem/object.go's requireIndex), so even two
// concurrent reindex-free FIRST reads mutate the same shared index map —
// guarding only the Reindex() call would not deliver the safety this
// package claims. lookupObjectRetrying therefore holds r.goGitMu.Lock (a
// full write lock, not RLock) across the initial lookup attempt, the
// fingerprint check, the conditional reindex, and the retry, as one
// indivisible unit. The fingerprint field itself is read and written only
// inside this locked sequence and needs no atomicity of its own as a
// result.
//
// # A narrow, documented residual staleness
//
// A concurrent external repack landing between this call's not-found miss
// and its fingerprint read can still yield one stale answer: the
// fingerprint read might observe the pre-repack pack set even though the
// repack has already replaced it on disk, in which case this call reports
// not-found once more and a later call (whose fingerprint read lands after
// the repack completes) reindexes and finds the object. This is strictly
// narrower than the CLI behaviour it replaces, where every call reads
// packfiles fresh and this window does not exist at all — but it is not
// eliminated, only bounded to one stale read per repack race.
//
// Exercised by gogit_test.go's concurrency and reindex-retry coverage; no
// migrated read calls it yet in this batch (that starts in batch 3), so
// golangci-lint's default (untagged) build sees no caller, matching
// gitnativepoc/read.go's identical hasUnpushed precedent.
//
//nolint:unused // only exercised by the //go:build integration-tagged gogit_test.go
func lookupObjectRetrying[T any](r *Repo, repo *git.Repository, lookup func() (T, error)) (T, error) {
	r.goGitMu.Lock()
	defer r.goGitMu.Unlock()

	result, err := lookup()
	if err == nil || !errors.Is(err, plumbing.ErrObjectNotFound) {
		return result, err
	}

	storer, ok := repo.Storer.(*filesystem.Storage)
	if !ok {
		// Not a filesystem-backed storer (never true for a handle goGit
		// opened, but the type assertion is the honest way to express the
		// dependency) — nothing to reindex; the miss stands.
		return result, err
	}

	fingerprint, fpErr := packFingerprint(storer)
	if fpErr != nil {
		// Could not even read the pack directory to compare; return the
		// original not-found rather than risk masking it with a fingerprint
		// error the caller never asked about.
		return result, err
	}
	if fingerprint == r.lastPackFingerprint {
		// The on-disk pack set has not changed since the last index build —
		// the miss is truth, not staleness. No rescan.
		return result, err
	}

	storer.Reindex()
	r.lastPackFingerprint = fingerprint
	return lookup()
}

// packFingerprint computes the pack fingerprint lookupObjectRetrying gates
// its reindex decision on: the sorted (name, size) list of every *.idx file
// in storer's routed objects/pack directory, joined into one comparable
// string. Because storer's Filesystem() was built with
// EnableDotGitCommonDir set (see goGit), "objects/pack" resolves to the
// shared common dir even when repo is a linked worktree — the fingerprint is
// therefore shared ground truth across every *Repo and OS process addressing
// the same checkout, not a per-handle notion. A missing objects/pack
// directory (no packfiles have ever been written) reports the empty
// fingerprint rather than an error, since that is a normal, valid state, not
// a failure to read the pack directory.
//
// Called only from lookupObjectRetrying, which is itself only reachable from
// gogit_test.go's integration-tagged coverage in this batch — see that
// function's doc.
//
//nolint:unused // only exercised (transitively) by the //go:build integration-tagged gogit_test.go
func packFingerprint(storer *filesystem.Storage) (string, error) {
	entries, err := storer.Filesystem().ReadDir("objects/pack")
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	type idxEntry struct {
		name string
		size int64
	}
	var idxFiles []idxEntry
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".idx") {
			continue
		}
		idxFiles = append(idxFiles, idxEntry{name: entry.Name(), size: entry.Size()})
	}
	sort.Slice(idxFiles, func(i, j int) bool { return idxFiles[i].name < idxFiles[j].name })

	var b strings.Builder
	for _, f := range idxFiles {
		fmt.Fprintf(&b, "%s:%d;", f.name, f.size)
	}
	return b.String(), nil
}
