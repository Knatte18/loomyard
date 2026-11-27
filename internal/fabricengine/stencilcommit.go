// stencilcommit.go declares the board's seeded-subtree vocabulary:
// the subtree prefixes, the commit subjects that name the writing binary, and the test for a droppable seed commit.
// It also declares CommitSeededStencils, which lands the files a seeding pass or `lym stencil sync` writes.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/buildinfo"
	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
)

// StencilSeedResult reports the landed commit's SHA and whether CommitSeededStencils made a commit.
// It embeds MutationRecord, which every mutating verb's result type carries.
type StencilSeedResult struct {
	fabriclayout.MutationRecord
	SHA       string
	Committed bool
}

// CommitSeededStencils commits, under the board write lock, the files a seeding pass just wrote to one subtree.
// The subtree is the stencils subtree or the deployed-specs subtree.
// An explicit positive pathspec confines the commit to that subtree.
// Callers seeding both subtrees call it once for each, with that subtree's pair.
//
// subtreeRel is the board-repo-relative, slash-separated prefix that writtenRelPaths hang off.
// subtreeDir is the absolute directory that subtreeRel names on disk.
// The two must agree. A mismatched pair still commits correctly, but its mutation record is false.
// subtreeDir alone decides where the record's file-written entries go.
//
// writtenRelPaths are relative to subtreeDir.
// message is the commit message, and rec accumulates the mutation record.
//
// When writtenRelPaths is empty, it returns the zero result with Committed false and no error,
// and it takes no lock and runs no git.
//
// This never pushes: the commit rides the board's next push through the coalescing path.
func CommitSeededStencils(hub, subtreeRel, subtreeDir string, writtenRelPaths []string, message string, rec *Mutations) (res StencilSeedResult, err error) {
	if len(writtenRelPaths) == 0 {
		return StencilSeedResult{}, nil
	}

	lockPath, err := BoardWriteLockPath(hub)
	if err != nil {
		return StencilSeedResult{}, err
	}
	l, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		return StencilSeedResult{}, fmt.Errorf("fabricengine: acquire board write lock: %w", err)
	}
	defer func() { _ = l.Release() }()

	defer func() { res.Mutations = rec.Snapshot() }()

	pathspec := fabriclayout.ScopedPathspec(subtreeRel, writtenRelPaths)

	sha, committed, err := gitrepo.New(fabriclayout.BoltDir(hub)).StageAndCommit(message, pathspec)
	if err != nil {
		return StencilSeedResult{}, fmt.Errorf("fabricengine: commit seeded stencils: %w", err)
	}

	for _, relPath := range writtenRelPaths {
		rec.Append(fabriclayout.KindFileWritten, filepath.Join(subtreeDir, relPath), "")
	}
	if committed {
		rec.Append(fabriclayout.KindCommitCreated, fabriclayout.BoltDir(hub), sha)
	}

	return StencilSeedResult{SHA: sha, Committed: committed}, nil
}

// BinaryLabel renders the running binary for a commit subject.
// The label has the form `(<revision label> <channel> <executable path>)`.
// The fields are the VCS stamp's revision label, the build channel and the resolved executable path.
// Every lym commit that names its writer uses this label.
func BinaryLabel() string {
	executable, err := os.Executable()
	if err != nil {
		executable = "unknown"
	}
	return binaryLabel(buildvcs.Running(), buildinfo.Channel, executable)
}

// binaryLabel renders the parenthesised label for an identity, a channel string and an executable path.
// An empty channel renders as `unstamped`.
func binaryLabel(id buildvcs.Identity, channel, executable string) string {
	if channel == "" {
		channel = "unstamped"
	}
	return fmt.Sprintf("(%s %s %s)", id.Label(), channel, executable)
}

// IsSeedCommit reports whether a commit is a droppable seeding commit:
// its first line is `lym: seed <subtree>` followed by one space and a non-empty parenthesised label,
// and every path lies under the stencils or specs subtree.
// A bare `lym: seed <subtree>` subject, a sync commit and a hub-wide config commit report false.
// A commit that touches any other path also reports false.
func IsSeedCommit(message string, paths []string) bool {
	subject, _, _ := strings.Cut(message, "\n")
	rest, found := strings.CutPrefix(subject, fabriclayout.SeedSubjectPrefix)
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
		if !underSubtree(p, fabriclayout.StencilsSubtreeRel()) && !underSubtree(p, fabriclayout.SpecsSubtreeRel()) {
			return false
		}
	}
	return true
}

// underSubtree reports whether the board-relative, slash-separated path p lies beneath the subtree prefix subtreeRel.
func underSubtree(p, subtreeRel string) bool {
	return strings.HasPrefix(p, subtreeRel+"/")
}
