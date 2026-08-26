// commitweftpaths.go implements CommitWeftPaths, a narrow positive-pathspec, no-push weft-commit
// helper: the third weft-commit shape alongside commitWeft and commitWeftAt, added for callers
// (Topology.Add, a later batch) that hold no *Fabric and must commit only a small, explicit set of
// paths.

package fabricengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// weftCommitPathspec returns the pathspec entries for relPaths, scoped under anchorRel.
// It is the single place the anchor join happens for CommitWeftPaths, so no caller ever joins
// AnchorRel or names the durable lyx directory in a pathspec itself.
func weftCommitPathspec(anchorRel string, relPaths []string) []string {
	return ScopedPathspec(anchorRel, relPaths)
}

// CommitWeftPaths stages and commits exactly relPaths (scoped under anchorRel) in the weft
// worktree at weftPath, taking the weft write lock itself and never pushing.
//
// Its body order is load-bearing:
//  1. opts.SkipGit returns ("", false, nil) with no lock taken, nothing staged, and nothing
//     recorded, exactly as commitWeftAt does, so an existing SkipGit test path can thread its opts
//     straight through.
//  2. An empty relPaths returns ("", false, nil), likewise recording nothing — nothing staged is
//     never reported as a commit.
//  3. The weft write lock is acquired and released via a deferred call, wrapping an acquisition
//     failure as "fabricengine: acquire weft write lock: %w".
//  4. gitrepo.New(weftPath).StageAndCommit is called with weftCommitPathspec(anchorRel, relPaths),
//     and only when it returns a nil error with committed true — never on a no-op, never on an
//     error — KindCommitCreated is appended to rec at weftPath with the returned sha as the
//     detail, matching the shape the package's existing commit sites already use.
//
// The recorder is what keeps a real weft commit from being invisible in the calling verb's own
// mutation record; recording is conditional on the commit having observably landed, per the
// Mutation Record Invariant. A caller with no verb outcome of its own passes a throwaway recorder
// and discards it.
//
// opts.SkipPush is accepted and ignored because this helper never pushes; SyncOptions is kept
// whole rather than narrowed to a bool so Topology.Add can pass its own opts unchanged.
//
// Neither existing weft-commit shape fits this caller: the pair-bound commit verb (Fabric.Commit)
// resolves its routing from its own stored warp path and cannot address a pair it did not open,
// and also fires a detached push unconditionally; commitWeftAt is a stage-all, which the Fabric
// Git Invariant's cross-module-exclusions rule forbids for durable-lyx commits because that tree
// is shared by every round-loop module.
//
// The lock is taken here rather than pushed onto callers, unlike commitWeftAt's
// caller-responsible contract, because this helper has two independent callers who would
// otherwise race on the git index lock.
func CommitWeftPaths(rec *Mutations, weftPath, anchorRel string, relPaths []string, msg string, opts SyncOptions) (sha string, committed bool, err error) {
	if opts.SkipGit {
		return "", false, nil
	}
	if len(relPaths) == 0 {
		return "", false, nil
	}

	lockDir, err := ensureWeftLockDirAt(weftPath)
	if err != nil {
		return "", false, err
	}
	l, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: acquire weft write lock: %w", err)
	}
	defer func() { _ = l.Release() }()

	sha, committed, err = gitrepo.New(weftPath).StageAndCommit(msg, weftCommitPathspec(anchorRel, relPaths))
	if err != nil {
		return "", false, err
	}
	if committed {
		rec.Append(KindCommitCreated, weftPath, sha)
	}
	return sha, committed, nil
}

// CommitAnchoredPaths is CommitWeftPaths's fabric-vocabulary-neutral wrapper: it resolves the
// commit target from l itself (mirroring ReadOrigin/WriteOrigin's own l-in, no-path-out shape,
// per the weft-visibility-leak closure recorded in fabric-unified-view.md's "Slice 8"), so a
// caller outside the owner set -- internal/loomcli's session-bootstrap commit is the first one --
// can reach this narrow, no-push, explicit-paths commit shape without ever naming a weft path,
// importing internal/weftname, or otherwise learning that weft exists.
//
// relPaths are anchor-relative, the same shape OriginRecordRel already returns, so a caller
// building its commit path list never joins AnchorRel itself.
func CommitAnchoredPaths(rec *Mutations, l *lyxcwd.Location, relPaths []string, msg string, opts SyncOptions) (sha string, committed bool, err error) {
	return CommitWeftPaths(rec, WeftWorktree(l), l.AnchorRel, relPaths, msg, opts)
}
