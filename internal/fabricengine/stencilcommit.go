// stencilcommit.go declares CommitSeededStencils, the locked, pathspec-scoped commit verb that
// lands a stencil-seeding pass' written files as one commit under the board write lock.
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

// StencilSeedResult reports what CommitSeededStencils did: the landed commit SHA and whether a
// commit was made.
// It embeds MutationRecord, which the Mutation Record Invariant requires of every mutating verb's
// result type.
type StencilSeedResult struct {
	MutationRecord
	SHA       string
	Committed bool
}

// CommitSeededStencils commits the stencils this process just wrote to the board's stencils
// subtree, under the board write lock, with an explicit positive pathspec confined to that
// subtree.
// writtenRelPaths arrive relative to the stencils directory (e.g.
// "loom/loom-template-discussion.md" and ".gitattributes"); message is the commit message; rec
// accumulates the mutation record.
//
// When writtenRelPaths is empty, this returns the zero result with Committed: false and no error,
// taking no lock and running no git at all — the common case on an ordinary run, and what keeps the
// seeding pass free.
//
// This never pushes: the commit rides board's next push through the existing coalescing path, since
// pushing per run would fire a push on nearly every lyx invocation.
func CommitSeededStencils(hub string, writtenRelPaths []string, message string, rec *Mutations) (res StencilSeedResult, err error) {
	if len(writtenRelPaths) == 0 {
		return StencilSeedResult{}, nil
	}

	l, err := lock.AcquireWriteLock(BoardWriteLockPath(hub))
	if err != nil {
		return StencilSeedResult{}, fmt.Errorf("fabricengine: acquire board write lock: %w", err)
	}
	defer func() { _ = l.Release() }()

	defer func() { res.Mutations = rec.Snapshot() }()

	stencilsRel := path.Join(lyxdirs.LyxDirName, stencilsDirName)
	pathspec := ScopedPathspec(stencilsRel, writtenRelPaths)

	sha, committed, err := gitrepo.New(BoardDir(hub)).StageAndCommit(message, pathspec)
	if err != nil {
		return StencilSeedResult{}, fmt.Errorf("fabricengine: commit seeded stencils: %w", err)
	}

	for _, relPath := range writtenRelPaths {
		rec.Append(KindFileWritten, filepath.Join(StencilsDir(hub), relPath), "")
	}
	if committed {
		rec.Append(KindCommitCreated, BoardDir(hub), sha)
	}

	return StencilSeedResult{SHA: sha, Committed: committed}, nil
}
