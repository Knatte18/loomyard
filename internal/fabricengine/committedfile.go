// committedfile.go implements CommittedAnchoredFile and CommittedAnchoredFileHistory, the read-only twins of CommitAnchoredPaths:
// they read one anchored file as committed at HEAD, or at every commit that touched it, without the caller naming the side of the pair that holds it.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// CommittedAnchoredFile returns relPath's contents as committed at HEAD, unaffected by any uncommitted edit to the working-tree copy.
//
// relPath is anchor-relative, the shape LoomReworkDirRel and planparser.PlanDirRel return, so a caller never joins AnchorRel itself or names the side of the pair that holds the file.
// The commit target resolves from l exactly as CommitAnchoredPaths resolves it.
//
// found is false with a nil error when the path is absent at HEAD or HEAD is unborn;
// every other failure is an error.
// It is read-only, so the Fabric Git Invariant exempts it from the commit seam.
func CommittedAnchoredFile(l *lyxcwd.Location, relPath string) (data []byte, found bool, err error) {
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

// CommittedAnchoredFileHistory returns relPath's contents at every commit that touched it, newest first, unaffected by any uncommitted edit to the working-tree copy.
//
// relPath is anchor-relative and the commit target resolves from l, both exactly as CommittedAnchoredFile resolves them.
// A commit that deleted the path holds no contents and is skipped.
//
// A path with no history, or an unborn HEAD, returns an empty slice and a nil error;
// every other failure is an error naming the path.
// It is read-only, so the Fabric Git Invariant exempts it from the commit seam.
func CommittedAnchoredFileHistory(l *lyxcwd.Location, relPath string) ([][]byte, error) {
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
