// committedfile.go reads one anchored file as committed, at HEAD or at every commit that touched it.
// A caller never names the side of the pair that holds the file.
// Uncommitted edits to the working-tree copy never show.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// CommittedAnchoredFile returns relPath's contents as committed at HEAD.
// It is unaffected by any uncommitted edit to the working-tree copy.
//
// relPath is anchor-relative, so a caller never joins the anchor path itself.
// The commit target is the records worktree that l resolves to.
//
// found is false with a nil error when the path is absent at HEAD or HEAD is unborn;
// every other failure is an error.
// It is read-only, so the Fabric Git Invariant exempts it from the commit seam.
func CommittedAnchoredFile(l *lymcwd.Location, relPath string) (data []byte, found bool, err error) {
	repo := gitrepo.New(RecordsWorktree(l))
	sha, err := repo.CurrentSHA()
	if err != nil {
		if errors.Is(err, gitrepo.ErrNoCommits) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("fabricengine: resolve HEAD for %s: %w", relPath, err)
	}

	scoped := ScopedPathspec(l.AnchorRel, []string{relPath})[0]
	data, err = repo.FileAtRevision(sha, filepath.ToSlash(scoped))
	if err != nil {
		if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("fabricengine: read %s at %s: %w", relPath, sha, err)
	}
	return data, true, nil
}

// AnchoredFileVersion is one committed version of an anchored file on the records branch's first-parent line.
type AnchoredFileVersion struct {
	// SHA is the full object name of the commit that changed the file.
	SHA string
	// Position is that commit's first-parent position on the records branch:
	// the number of first-parent steps from the branch's root commit, so the root is 0 and each later commit is one more.
	Position int
	// Data is the file's contents at that commit.
	Data []byte
}

// AnchoredFileFirstParentHistory returns relPath's contents at each records-branch first-parent commit that changed it.
// The versions come newest first, in first-parent order and never in committer time.
// Commits made within the same second therefore keep their order.
// A commit counts as changing the path when the path differs from the commit's first parent;
// a root commit counts every path as changed.
// A commit that deleted the path holds no contents and is skipped.
//
// relPath is anchor-relative, and the commit target is the records worktree that l resolves to.
// A path with no history, or an unborn HEAD, returns an empty slice and a nil error;
// every other failure is an error naming the path.
// It is read-only, so the Fabric Git Invariant exempts it from the commit seam.
func AnchoredFileFirstParentHistory(l *lymcwd.Location, relPath string) ([]AnchoredFileVersion, error) {
	all, err := recordsFirstParentCommits(l)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: list history for %s: %w", relPath, err)
	}
	versions := []AnchoredFileVersion{}
	if len(all) == 0 {
		return versions, nil
	}
	positions := make(map[string]int, len(all))
	for i, commit := range all {
		positions[commit.SHA] = len(all) - 1 - i
	}

	repo := gitrepo.New(RecordsWorktree(l))
	scoped := filepath.ToSlash(ScopedPathspec(l.AnchorRel, []string{relPath})[0])
	changed, err := repo.Log(all[0].SHA, gitrepo.LogQuery{FirstParent: true, PathPrefix: scoped})
	if err != nil {
		return nil, fmt.Errorf("fabricengine: list commits that changed %s: %w", relPath, err)
	}
	for _, commit := range changed {
		data, err := repo.FileAtRevision(commit.SHA, scoped)
		if err != nil {
			if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
				continue
			}
			return nil, fmt.Errorf("fabricengine: read %s at %s: %w", relPath, commit.SHA, err)
		}
		versions = append(versions, AnchoredFileVersion{SHA: commit.SHA, Position: positions[commit.SHA], Data: data})
	}
	return versions, nil
}

// CommittedAnchoredFileHistory returns relPath's contents at every commit that touched it, newest first.
// It is unaffected by any uncommitted edit to the working-tree copy.
//
// relPath is anchor-relative, and the commit target is the records worktree that l resolves to.
// A commit that deleted the path holds no contents and is skipped.
//
// A path with no history, or an unborn HEAD, returns an empty slice and a nil error;
// every other failure is an error naming the path.
// It is read-only, so the Fabric Git Invariant exempts it from the commit seam.
func CommittedAnchoredFileHistory(l *lymcwd.Location, relPath string) ([][]byte, error) {
	repo := gitrepo.New(RecordsWorktree(l))
	scoped := filepath.ToSlash(ScopedPathspec(l.AnchorRel, []string{relPath})[0])

	revisions, err := repo.PathRevisions(scoped, 0)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: list revisions for %s: %w", relPath, err)
	}

	versions := make([][]byte, 0, len(revisions))
	for _, sha := range revisions {
		data, err := repo.FileAtRevision(sha, scoped)
		if err != nil {
			if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
				continue
			}
			return nil, fmt.Errorf("fabricengine: read %s at %s: %w", relPath, sha, err)
		}
		versions = append(versions, data)
	}
	return versions, nil
}
