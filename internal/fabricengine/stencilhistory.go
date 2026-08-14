// stencilhistory.go declares StencilBaseByStamp, the read-only accessor that recovers the shipped
// default a stencil was forked from by walking the board repo's own history for that stencil's path
// and matching each historical revision's body hash against the caller-supplied stamp. It is the
// board-scoped wrapper `lyx stencil diff` builds on: the Fabric Git Invariant routes every git
// operation on either repo through this package, so internal/stencilcli must never call
// internal/gitrepo directly.

package fabricengine

import (
	"errors"
	"fmt"
	"path"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// StencilBaseByStamp walks the board repo's history for the stencil named name, newest revision
// first, and returns the contents of the first revision whose stripped-and-LF-normalised body
// hashes to stampHash -- that revision is by definition the default the on-disk file was forked
// from.
// found is false with a nil error when no revision matches: history pruned, the stamp hand-written,
// or the file predating its own history. The caller must report that explicitly rather than
// rendering an empty diff, which would read as "no upstream changes" when the truth is "base not
// found".
// rev is the matching revision's SHA, valid only when found is true.
// This accessor is read-only and mutates nothing, so it takes no *Mutations and returns a plain
// tuple rather than embedding MutationRecord.
func StencilBaseByStamp(hub, name, stampHash string) (base []byte, rev string, found bool, err error) {
	relPath := path.Join(lyxdirs.LyxDirName, stencilsDirName, stencilstore.RelPath(name))
	repo := gitrepo.New(BoardDir(hub))

	revisions, err := repo.PathRevisions(relPath, 0)
	if err != nil {
		return nil, "", false, fmt.Errorf("fabricengine: list revisions for %s: %w", relPath, err)
	}

	for _, sha := range revisions {
		content, err := repo.FileAtRevision(sha, relPath)
		if err != nil {
			if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
				continue
			}
			return nil, "", false, fmt.Errorf("fabricengine: read %s at %s: %w", relPath, sha, err)
		}
		if stencilstore.BodyHash(content) == stampHash {
			return content, sha, true, nil
		}
	}

	return nil, "", false, nil
}
