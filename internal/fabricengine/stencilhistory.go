// stencilhistory.go holds StencilBaseByStamp, a read-only accessor for the shipped default of a stencil.
// It walks the board repo's history for the stencil's path and matches each revision's body hash against a stamp.
// `lym stencil diff` builds on it.
// The Fabric Git Invariant routes every git operation on either repo through this package.
// Therefore internal/stencilcli must never call internal/gitrepo directly.

package fabricengine

import (
	"errors"
	"fmt"
	"path"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// StencilBaseByStamp returns the shipped default that the stencil named name was forked from.
// It walks the board repo's history for the stencil, newest revision first.
// It returns the contents of the first revision whose body hash equals stampHash.
// The body hash ignores the leading banner and normalises line endings to LF.
// found is false with a nil error when no revision matches.
// That happens when history was pruned, the stamp was written by hand, or the file predates its history.
// The caller must report that case explicitly, since an empty diff would read as "no upstream changes".
// rev is the matching revision's SHA, valid only when found is true.
// It mutates nothing, so it takes no *Mutations and returns a plain tuple.
func StencilBaseByStamp(hub, name, stampHash string) (base []byte, rev string, found bool, err error) {
	relPath := path.Join(lymdirs.LymDirName, stencilsDirName, stencilstore.RelPath(name))
	repo := gitrepo.New(BoltDir(hub))

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
