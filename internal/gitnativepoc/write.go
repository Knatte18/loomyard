// write.go implements gitnativepoc's write-surface: go-git-backed counterparts
// to internal/gitrepo's mutating methods (StageAndCommit, StageAllAndCommit,
// Push, SetSnapshotSHA), each built directly on go-git's worktree/index and
// transport model rather than by shelling out to git. Every method here is
// exercised against the matching gitrepo.Repo method as the differential
// oracle in write_test.go, per the plan's differential-oracle Shared
// Decision; the MIGRATE/CLI-BOUND verdict for each surface is recorded as a
// comment on its test, not here, except where the verdict itself needs the
// method's own doc comment to explain the mechanism it depends on.

package gitnativepoc

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// gitSignature returns the fixed test identity every commit method in this
// file passes explicitly to go-git's Worktree.Commit. go-git v5 does not
// honor GIT_CONFIG_GLOBAL/GIT_CONFIG_NOSYSTEM the way gitexec's hermetic
// wrapper relies on — it resolves global config straight from $HOME/XDG,
// bypassing the env vars lyxtest.HermeticGitEnv sets — so reading identity
// "via go-git config" would either find nothing (Worktree.Commit's
// ErrMissingAuthor) or silently leak the operator's real ~/.gitconfig into a
// supposedly-hermetic test. Passing an explicit signature sidesteps go-git's
// config resolution entirely; the name/email mirror the neutral identity the
// harness's fixture builders configure via `git config user.name/user.email`
// so a real commit's author matches what the CLI oracle side would produce.
func gitSignature() *object.Signature {
	return &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()}
}

// StageAndCommit stages exactly the given files and commits exactly those
// files with msg, mirroring gitrepo.StageAndCommit's pathspec-scoped
// contract: an empty files list stages nothing and returns ("", false, nil)
// with no work; when none of the listed files differ from HEAD's committed
// content, it returns ("", false, nil) rather than an error; a real commit
// returns the new HEAD SHA with committed=true. MIGRATE, but not via
// go-git's high-level Worktree.Commit directly: CommitOptions has no
// pathspec field (unlike `git commit -- <files>`), so Worktree.Commit always
// builds its tree from the *entire* on-disk index — there is no first-class
// go-git API for a partial commit. The mechanism used here instead: stage
// files via Worktree.Add (identical to `git add -- <files>`), then swap in a
// synthetic index — HEAD's tree flattened, with only the listed files'
// entries overlaid from the real post-Add index — call Worktree.Commit
// against that synthetic index (go-git's own buildTreeHelper then produces
// exactly the tree a scoped CLI commit would), and finally restore the real
// index. Every other staged entry the real index carried is never read by
// the synthetic-index commit and is put back afterward untouched, matching
// gitrepo leaving files staged outside the pathspec staged-but-uncommitted.
func (r *Repo) StageAndCommit(msg string, files []string) (sha string, committed bool, err error) {
	// An empty list stages nothing, so there is never anything of the
	// caller's to commit; return the documented no-op signal before touching
	// go-git at all, mirroring gitrepo's early return before any git spawn.
	if len(files) == 0 {
		return "", false, nil
	}

	wt, err := r.repo.Worktree()
	if err != nil {
		return "", false, err
	}

	// Stage each listed path individually: Worktree.Add mirrors `git add --
	// <file>` per path, including honoring a working-tree deletion by
	// removing the path from the index, matching gitrepo's `git add --
	// <files>` step exactly.
	for _, f := range files {
		if _, err := wt.Add(f); err != nil {
			return "", false, fmt.Errorf("gitnativepoc: stage %s: %w", f, err)
		}
	}

	// The real on-disk index now carries our staged files' state plus
	// whatever a caller staged outside this call. Capture it before building
	// the scoped commit below so it can be restored afterward — see the
	// method doc comment for why a synthetic index is the only way to get a
	// pathspec-scoped tree out of Worktree.Commit.
	realIndex, err := r.repo.Storer.Index()
	if err != nil {
		return "", false, err
	}

	scopedIndex, changed, err := r.buildScopedIndex(realIndex, files)
	if err != nil {
		return "", false, err
	}
	if !changed {
		// None of the listed files differ from HEAD's committed content —
		// the scoped equivalent of gitrepo's `diff --cached --quiet --
		// <files>` reporting exit 0. The real index captured above is left
		// exactly as the Add calls above staged it, matching gitrepo leaving
		// those files staged even though there is nothing new to commit.
		return "", false, nil
	}

	if err := r.repo.Storer.SetIndex(scopedIndex); err != nil {
		return "", false, err
	}
	hash, commitErr := wt.Commit(msg, &git.CommitOptions{Author: gitSignature()})
	// Restore the real index regardless of outcome: a scoped commit must
	// never leave the checkout's staged state reflecting our synthetic
	// HEAD-plus-scoped-files snapshot instead of what the caller actually
	// staged.
	if restoreErr := r.repo.Storer.SetIndex(realIndex); restoreErr != nil {
		return "", false, restoreErr
	}
	if commitErr != nil {
		if errors.Is(commitErr, git.ErrEmptyCommit) {
			// buildScopedIndex's own diff already ruled this out above, so in
			// practice this path is a safety net rather than the common
			// case; it stays semantically identical to the changed=false
			// return above.
			return "", false, nil
		}
		return "", false, commitErr
	}

	return hash.String(), true, nil
}

// buildScopedIndex builds the synthetic index StageAndCommit swaps in before
// calling Worktree.Commit: HEAD's tree flattened into index entries, with
// only the paths in files overlaid by their entry in afterAdd (the real
// index just captured after staging) — or removed, when a file was staged as
// a deletion. It also reports whether any of files actually differs from
// HEAD, the pathspec-scoped equivalent of `git diff --cached --quiet --
// <files>`'s exit code; when false the returned index is nil and must not be
// used.
func (r *Repo) buildScopedIndex(afterAdd *index.Index, files []string) (*index.Index, bool, error) {
	entries, err := r.headTreeEntries()
	if err != nil {
		return nil, false, err
	}

	staged := make(map[string]*index.Entry, len(afterAdd.Entries))
	for _, e := range afterAdd.Entries {
		staged[e.Name] = e
	}

	changed := false
	for _, f := range files {
		before, hadBefore := entries[f]
		after, hasAfter := staged[f]
		switch {
		case hasAfter && (!hadBefore || before.Mode != after.Mode || before.Hash != after.Hash):
			entries[f] = &object.TreeEntry{Name: f, Mode: after.Mode, Hash: after.Hash}
			changed = true
		case !hasAfter && hadBefore:
			// Staged as a deletion (or the working-tree file is simply gone
			// and was never re-added): drop it from the scoped tree so the
			// commit reflects the removal, matching `git commit -- <files>`
			// committing a deletion within the pathspec.
			delete(entries, f)
			changed = true
		}
	}
	if !changed {
		return nil, false, nil
	}

	scoped := &index.Index{Version: 2}
	for _, e := range entries {
		scoped.Entries = append(scoped.Entries, &index.Entry{Name: e.Name, Mode: e.Mode, Hash: e.Hash})
	}
	return scoped, true, nil
}

// headTreeEntries flattens HEAD's tree into a map keyed by full repo-relative
// path, the base buildScopedIndex overlays the listed files onto. An unborn
// HEAD (no commits yet) reports an empty map rather than an error — the same
// "nothing committed yet" state gitrepo's scoped commit would build its tree
// from from scratch.
func (r *Repo) headTreeEntries() (map[string]*object.TreeEntry, error) {
	entries := map[string]*object.TreeEntry{}

	head, err := r.repo.Head()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return entries, nil
		}
		return nil, err
	}
	commit, err := r.repo.CommitObject(head.Hash())
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	iter := tree.Files()
	defer iter.Close()
	err = iter.ForEach(func(f *object.File) error {
		entries[f.Name] = &object.TreeEntry{Name: f.Name, Mode: f.Mode, Hash: f.Hash}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// StageAllAndCommit stages every working-tree change via go-git's `AddOptions{All:
// true}` (the `git add -A` equivalent) and commits whatever lands in the
// index with msg, mirroring gitrepo.StageAllAndCommit's wildcard sibling of
// StageAndCommit. Return semantics mirror StageAndCommit exactly: when
// nothing is staged after the add, it returns ("", false, nil); on a real
// commit it returns the new HEAD SHA with committed=true. MIGRATE: unlike the
// pathspec-scoped case, the wildcard stage-everything commit needs no
// synthetic index — Worktree.Commit's whole-index tree build is exactly what
// `git add -A && git commit` produces.
func (r *Repo) StageAllAndCommit(msg string) (sha string, committed bool, err error) {
	wt, err := r.repo.Worktree()
	if err != nil {
		return "", false, err
	}

	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return "", false, fmt.Errorf("gitnativepoc: stage all: %w", err)
	}

	hash, err := wt.Commit(msg, &git.CommitOptions{Author: gitSignature()})
	if err != nil {
		if errors.Is(err, git.ErrEmptyCommit) {
			// A clean working tree after the wildcard add — nothing landed
			// in the index that differs from HEAD — mirrors gitrepo's `diff
			// --cached --quiet` reporting exit 0 as a plain no-op signal,
			// not a failure.
			return "", false, nil
		}
		return "", false, err
	}
	return hash.String(), true, nil
}

// ErrPushCLIBound is the documented sentinel Push returns when a
// non-fast-forward rejection occurs: go-git v5.19.1 ships no rebase
// implementation whatsoever (confirmed by grepping the entire module source
// for any Rebase type or function — none exists), so unlike
// gitrepo.pushWithRebaseRetry's `git pull --rebase` recovery, this poc method
// cannot even attempt a rebase-replay retry — it can only detect the
// rejection and report the limitation. This is the pivotal CLI-BOUND finding
// the write-surface batch exists to establish; see write_test.go's
// TestPush_RebaseRetryOnNonFastForward for the evidence.
var ErrPushCLIBound = errors.New("gitnativepoc: push rebase-retry recovery is CLI-BOUND (go-git has no rebase support)")

// Push runs a single go-git push of the current branch to its configured
// remote (go-git's zero-value PushOptions default to "origin" and the
// standard refs/heads/*:refs/heads/* refspec, matching a plain `git push`
// against an already-tracking branch). CLI-BOUND for the rebase-retry
// recovery path specifically: on a non-fast-forward rejection this method
// returns ErrPushCLIBound (wrapping go-git's own rejection error) rather than
// attempting gitrepo.pushWithRebaseRetry's pull-rebase-then-retry recovery —
// see ErrPushCLIBound's doc comment for why go-git cannot do that recovery at
// all. A plain, non-rejected push (including "nothing to push") is MIGRATE:
// go-git's Repository.Push computes fast-forward-ness against the live
// remote refs it just fetched during the same push handshake, not a
// possibly-stale local remote-tracking ref, so the non-retry push path is
// exactly as accurate as the CLI's.
func (r *Repo) Push() error {
	err := r.repo.Push(&git.PushOptions{})
	if err == nil || errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	if !isNonFastForwardRejection(err) {
		return fmt.Errorf("gitnativepoc: push: %w", err)
	}
	return fmt.Errorf("%w: %v", ErrPushCLIBound, err)
}

// isNonFastForwardRejection reports whether err is go-git's non-fast-forward
// push rejection. go-git offers no typed sentinel or error type for this
// rejection (checkFastForwardUpdate in go-git's remote.go builds it via a
// plain fmt.Errorf with no wrapped target), so detection here is a
// stderr-substring-style string match on the error text — the same posture
// gitrepo's rebaseRetryTriggers stderr sniff uses, just against go-git's
// error string instead of git CLI's stderr. This is itself a MIGRATE-vs-CLI
// data point: go-git's rejection is no more strongly typed than the CLI's.
func isNonFastForwardRejection(err error) bool {
	return err != nil && strings.Contains(err.Error(), "non-fast-forward update")
}

// SetSnapshotSHA records sha as the value for key, advancing
// refs/loomyard/snapshot/<key> locally and pushing it to the repo's remote
// fast-forward-only, mirroring gitrepo.SetSnapshotSHA's adopt-on-conflict
// contract: on a rejected push, fetch-and-adopt the remote's current value
// into the local ref, and if that adopted value is a strict ancestor of sha
// (isStrictDescendant, reused unchanged from read.go/card 7), the caller is
// genuinely further along than a transient contention loss — re-advance and
// retry, bounded by snapshotPushMaxAttempts. validSnapshotKey and validSHA
// (read.go/card 6) gate key and sha exactly as gitrepo's SetSnapshotSHA does,
// before any ref is touched. MIGRATE: go-git's Repository.Push already
// evaluates fast-forward-ness against the live remote (see Push's doc
// comment), and Repository.Fetch with a force refspec performs the adopt
// step natively — both the local ref advance and the adopt round-trip work
// exactly as gitrepo's CLI-backed equivalents do. One documented deviation:
// gitrepo canonicalizes sha to its full spelling only best-effort (an
// unresolvable sha is passed through unchanged for `update-ref` to reject);
// go-git's reference API takes a concrete plumbing.Hash rather than a
// string, so there is nothing to "pass through" — an unresolvable sha is a
// genuine error here rather than a deferred one. Also, per the
// cli-bound-is-a-recorded-outcome Shared Decision's ask to call out rejection
// typing: go-git offers no typed rejection sentinel either (see
// isNonFastForwardRejection) — a MIGRATE data point, not a CLI-BOUND one,
// since detection still works correctly via the same string-match posture.
func (r *Repo) SetSnapshotSHA(key, sha string) error {
	if !validSnapshotKey(key) {
		return ErrInvalidSnapshotKey
	}
	if !validSHA(sha) {
		return ErrInvalidSHA
	}

	resolved, err := r.repo.ResolveRevision(plumbing.Revision(sha))
	if err != nil {
		return fmt.Errorf("gitnativepoc: resolve %s: %w", sha, err)
	}
	hash := *resolved
	canonicalSHA := hash.String()

	ref := snapshotRefPrefix + key
	remote := r.remoteName()

	for attempt := 0; attempt < snapshotPushMaxAttempts; attempt++ {
		rejected, err := r.advanceAndPushSnapshotRef(remote, ref, hash)
		if err != nil || !rejected {
			return err
		}

		// The push was rejected; adopt the remote's current value so the
		// local ref converges — and so we can tell a genuine advance past
		// sha from transient contention that merely raced us.
		if err := r.adoptSnapshotRef(remote, ref); err != nil {
			return err
		}

		adoptedRef, err := r.repo.Reference(plumbing.ReferenceName(ref), true)
		if err != nil {
			if errors.Is(err, plumbing.ErrReferenceNotFound) {
				// The adopt-fetch found nothing to bring in — the remote's
				// rejection was not backed by an actual ref value we can
				// compare against; stop rather than loop forever.
				return nil
			}
			return fmt.Errorf("gitnativepoc: resolve adopted %s: %w", ref, err)
		}

		// If the adopted value is not a strict ancestor of sha, another
		// writer genuinely processed at least as far as us: the monotonic
		// line says their value is the correct one to keep, so stop with it
		// adopted.
		if !r.isStrictDescendant(adoptedRef.Hash().String(), canonicalSHA) {
			return nil
		}
		// Otherwise sha strictly descends from the adopted value — we lost
		// only transient contention while actually being further along — so
		// loop and retry the push rather than silently dropping the
		// strictly-newer value. hash is unchanged: it already names sha, the
		// same value the next attempt must re-advance the local ref to.
	}
	return nil
}

// snapshotPushMaxAttempts mirrors gitrepo's constant of the same name: it
// bounds SetSnapshotSHA's adopt-on-conflict retry loop so it always
// terminates, per the same monotonic-advance-in-practice-needs-few-attempts
// reasoning gitrepo's copy documents.
const snapshotPushMaxAttempts = 8

// advanceAndPushSnapshotRef sets ref to hash locally and pushes it to remote
// fast-forward-only (no force refspec), mirroring gitrepo's method of the
// same name. It reports rejected=true when the push failed with go-git's
// non-fast-forward rejection (see isNonFastForwardRejection — this single
// check covers both a genuine non-fast-forward and a remote-side creation
// race, since go-git's client-side check produces the same error shape for
// both, unlike gitrepo's three-substring CLI stderr sniff); any other
// failure is returned as an error.
func (r *Repo) advanceAndPushSnapshotRef(remote, ref string, hash plumbing.Hash) (rejected bool, err error) {
	if err := r.repo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName(ref), hash)); err != nil {
		return false, fmt.Errorf("gitnativepoc: set local ref %s: %w", ref, err)
	}

	err = r.repo.Push(&git.PushOptions{
		RemoteName: remote,
		RefSpecs:   []config.RefSpec{config.RefSpec(ref + ":" + ref)},
	})
	if err == nil || errors.Is(err, git.NoErrAlreadyUpToDate) {
		return false, nil
	}
	if isNonFastForwardRejection(err) {
		return true, nil
	}
	return false, fmt.Errorf("gitnativepoc: push %s %s: %w", remote, ref, err)
}

// adoptSnapshotRef force-fetches ref's current value on remote into the
// local ref — the adopt half of SetSnapshotSHA's adopt-on-conflict protocol,
// mirroring gitrepo's method of the same name via go-git's Repository.Fetch
// with a forced (`+`-prefixed) refspec instead of `git fetch remote
// +ref:ref`.
func (r *Repo) adoptSnapshotRef(remote, ref string) error {
	err := r.repo.Fetch(&git.FetchOptions{
		RemoteName: remote,
		RefSpecs:   []config.RefSpec{config.RefSpec("+" + ref + ":" + ref)},
		Force:      true,
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("gitnativepoc: fetch %s +%s:%s (adopt-on-conflict): %w", remote, ref, ref, err)
	}
	return nil
}
