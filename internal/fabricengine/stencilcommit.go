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
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/buildinfo"
	"github.com/Knatte18/loomyard/internal/buildvcs"
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

const (
	seedSubjectPrefix = "lyx: seed "
	syncSubjectPrefix = "lyx: sync "
)

// BinaryLabel renders the running binary for a commit subject:
// `(<revision label> <channel> <executable path>)`, from the binary's VCS stamp, its build channel and its resolved path.
// Every lyx commit that names its writer shares this one label.
func BinaryLabel() string {
	executable, err := os.Executable()
	if err != nil {
		executable = "unknown"
	}
	return binaryLabel(buildvcs.Running(), buildinfo.Channel, executable)
}

// binaryLabel renders the parenthesised label for an identity, a channel string (`unstamped` when empty) and an executable path.
func binaryLabel(id buildvcs.Identity, channel, executable string) string {
	if channel == "" {
		channel = "unstamped"
	}
	return fmt.Sprintf("(%s %s %s)", id.Label(), channel, executable)
}

// SeedCommitMessage returns the subject of a seeding commit for a subtree (`stencils` or `specs`).
// label is the already-parenthesised BinaryLabel.
func SeedCommitMessage(subtree, label string) string {
	return seedSubjectPrefix + subtree + " " + label
}

// SyncCommitMessage returns the subject of an operator-requested `lyx stencil sync` commit for a subtree.
// label is the already-parenthesised BinaryLabel.
func SyncCommitMessage(subtree, label string) string {
	return syncSubjectPrefix + subtree + " " + label
}

// IsSeedCommit reports whether a commit is a droppable seeding commit:
// its first line is `lyx: seed <subtree>` followed by one space and a non-empty parenthesised label, and every path lies under the stencils or specs subtree.
// A bare `lyx: seed <subtree>` subject, a sync commit, a hub-wide config commit and a commit touching any other path all answer false.
func IsSeedCommit(message string, paths []string) bool {
	subject, _, _ := strings.Cut(message, "\n")
	rest, found := strings.CutPrefix(subject, seedSubjectPrefix)
	if !found {
		return false
	}
	subtree, label, found := strings.Cut(rest, " ")
	if !found || subtree == "" || len(label) <= len("()") || !strings.HasPrefix(label, "(") || !strings.HasSuffix(label, ")") {
		return false
	}
	if len(paths) == 0 {
		return false
	}
	for _, p := range paths {
		if !underSubtree(p, StencilsSubtreeRel()) && !underSubtree(p, SpecsSubtreeRel()) {
			return false
		}
	}
	return true
}

func underSubtree(p, subtreeRel string) bool {
	return strings.HasPrefix(p, subtreeRel+"/")
}
