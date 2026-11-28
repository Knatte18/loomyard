// codereads.go reads the code checkout's tracked files as stored in a commit, unaffected by the working tree.

package fabricengine

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// CodeDirAt returns the names of the regular files directly in dir of the code checkout's commit rev, sorted.
// dir is slash-separated and relative to the checkout's root; "" or "." names the root.
// A directory absent at rev gives an empty list and no error.
// rev is a hex commit name, such as the one HeadSHA returns.
func (f *Fabric) CodeDirAt(rev, dir string) ([]string, error) {
	names, err := f.code.FilesInDirAtRevision(rev, dir)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: list %s at %s: %w", dir, rev, err)
	}
	return names, nil
}

// CodeTreeAt returns the regular files in dir's whole subtree of the code checkout's commit rev, sorted.
// Each is a slash path relative to dir.
// dir is slash-separated and relative to the checkout's root; "" or "." names the root.
// A directory absent at rev gives an empty list and no error.
// rev is a hex commit name, such as the one HeadSHA returns.
func (f *Fabric) CodeTreeAt(rev, dir string) ([]string, error) {
	paths, err := f.code.FilesUnderDirAtRevision(rev, dir)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: list the tree of %s at %s: %w", dir, rev, err)
	}
	return paths, nil
}

// CodeFileAt returns rel as stored in the code checkout's commit rev, unaffected by any later change to the file.
// rel is slash-separated and relative to the checkout's root, and rev is a hex commit name.
// found is false with a nil error when rel is absent at rev.
func (f *Fabric) CodeFileAt(rev, rel string) (data []byte, found bool, err error) {
	data, err = f.code.FileAtRevision(rev, rel)
	if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("fabricengine: read %s at %s: %w", rel, rev, err)
	}
	return data, true, nil
}
