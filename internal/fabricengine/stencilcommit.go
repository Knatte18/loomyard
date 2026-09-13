// stencilcommit.go declares CommitSeededStencils, the locked, pathspec-scoped commit verb that
// lands a seeding pass' written files as one commit under the board write lock, whatever seeded
// subtree it is told about — the stencils subtree, or the deployed-specs subtree.
// It deliberately does not build on Bolt: Bolt.Sync takes board.push.lock while board's own file
// writes take board.lock, so seeding under Bolt would not exclude a concurrent
// boardCriticalSection, and Bolt.Commit stages everything in the board repo via
// StageAllAndCommit, which could capture a half-written board.

package fabricengine

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// StencilsSubtreeRel returns the board-repo-relative, slash-separated prefix the stencils subtree's
// written paths hang off — the subtreeRel argument a caller pairs with StencilsDir(hub) when calling
// CommitSeededStencils for a stencil-seeding pass.
func StencilsSubtreeRel() string {
	return path.Join(lyxdirs.LyxDirName, stencilsDirName)
}

// SpecsSubtreeRel returns the board-repo-relative, slash-separated prefix the deployed-specs
// subtree's written paths hang off — the subtreeRel argument a caller pairs with SpecsDir(hub) when
// calling CommitSeededStencils for a specs-seeding pass.
func SpecsSubtreeRel() string {
	return path.Join(lyxdirs.LyxDirName, specsDirName)
}

// StencilSeedResult reports what CommitSeededStencils did: the landed commit SHA and whether a
// commit was made.
// It embeds MutationRecord, which the Mutation Record Invariant requires of every mutating verb's
// result type.
type StencilSeedResult struct {
	MutationRecord
	SHA       string
	Committed bool
}

// CommitSeededStencils commits the files a seeding pass just wrote to a named subtree of the
// board — the stencils subtree, or the deployed-specs subtree — under the board write lock, with an
// explicit positive pathspec confined to that subtree.
// It is called once per seeded subtree, never once per process: a caller with two subtrees to seed
// (stencils and specs) calls this twice, each with its own subtreeRel/subtreeDir pair.
//
// subtreeRel is the board-repo-relative, slash-separated prefix writtenRelPaths hang off (e.g.
// StencilsSubtreeRel() or SpecsSubtreeRel()); subtreeDir is the absolute on-disk directory that same
// subtree resolves to (e.g. StencilsDir(hub) or SpecsDir(hub)).
// subtreeDir must be the absolute directory subtreeRel names relative to the board repository — a
// caller passing a mismatched pair produces a correct commit with a false mutation record, since
// subtreeDir alone determines where the mutation record's file-written entries are filed.
// There is deliberately no helper deriving one from the other: every caller already has both values
// in hand.
//
// writtenRelPaths arrive relative to subtreeDir (e.g. "loom/loom-template-discussion.md" and
// ".gitattributes"); message is the commit message; rec accumulates the mutation record.
//
// When writtenRelPaths is empty, this returns the zero result with Committed: false and no error,
// taking no lock and running no git at all — the common case on an ordinary run, and what keeps the
// seeding pass free.
//
// This never pushes: the commit rides board's next push through the existing coalescing path, since
// pushing per run would fire a push on nearly every lyx invocation.
func CommitSeededStencils(hub, subtreeRel, subtreeDir string, writtenRelPaths []string, message string, rec *Mutations) (res StencilSeedResult, err error) {
	if len(writtenRelPaths) == 0 {
		return StencilSeedResult{}, nil
	}

	l, err := lock.AcquireWriteLock(BoardWriteLockPath(hub))
	if err != nil {
		return StencilSeedResult{}, fmt.Errorf("fabricengine: acquire board write lock: %w", err)
	}
	defer func() { _ = l.Release() }()

	defer func() { res.Mutations = rec.Snapshot() }()

	pathspec := ScopedPathspec(subtreeRel, writtenRelPaths)

	sha, committed, err := gitrepo.New(BoardDir(hub)).StageAndCommit(message, pathspec)
	if err != nil {
		return StencilSeedResult{}, fmt.Errorf("fabricengine: commit seeded stencils: %w", err)
	}

	for _, relPath := range writtenRelPaths {
		rec.Append(KindFileWritten, filepath.Join(subtreeDir, relPath), "")
	}
	if committed {
		rec.Append(KindCommitCreated, BoardDir(hub), sha)
	}

	return StencilSeedResult{SHA: sha, Committed: committed}, nil
}
