// commitstore_test.go — the in-memory records store the driftengine tests share: a fake for the three seams that commit with trailers, find a trailer's commit and read a file at a commit.

package driftengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// fakeRecordsCommit is one commit of a commitStore: the whole tree as of the commit and its trailers.
type fakeRecordsCommit struct {
	sha      string
	codeHead string
	trailers []fabricengine.Trailer
	files    map[string][]byte
}

// commitStore is a fake records store that keeps each commit's tree in memory.
// A commit reads the told paths from root and keeps their bytes, or drops a path that is absent on disk, on top of the previous commit's tree.
type commitStore struct {
	root string
	// codeHead is the code HEAD the Head seam reports; a test sets it between commits.
	codeHead string

	mu      sync.Mutex
	commits []fakeRecordsCommit
}

// newCommitStore returns an empty store over a fresh anchor root.
func newCommitStore(t *testing.T) *commitStore {
	t.Helper()
	return &commitStore{root: t.TempDir(), codeHead: "code-head-0"}
}

// deps returns a Deps over p whose commit, trailer and file-at seams read and write the store; a test sets any other seam on the returned value.
func (s *commitStore) deps(p Paths) Deps {
	return Deps{
		Paths:        p,
		WorktreeRoot: s.root,
		Head:         func() (string, error) { return s.codeHead, nil },
		CommitWithTrailers: func(relPaths []string, trailers []fabricengine.Trailer) error {
			return s.commit(relPaths, trailers)
		},
		TrailerCommit: s.trailerCommit,
		FileAt:        s.fileAt,
	}
}

// commit adds a commit of relPaths with trailers.
func (s *commitStore) commit(relPaths []string, trailers []fabricengine.Trailer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	files := map[string][]byte{}
	if len(s.commits) > 0 {
		for rel, data := range s.commits[len(s.commits)-1].files {
			files[rel] = data
		}
	}
	for _, rel := range relPaths {
		data, err := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(rel)))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			delete(files, rel)
		case err != nil:
			return err
		default:
			files[rel] = data
		}
	}
	s.commits = append(s.commits, fakeRecordsCommit{
		sha:      fmt.Sprintf("commit-%d", len(s.commits)+1),
		codeHead: s.codeHead,
		trailers: slices.Clone(trailers),
		files:    files,
	})
	return nil
}

// trailerCommit finds the newest or the oldest commit carrying key, with the key's last value in that commit.
func (s *commitStore) trailerCommit(key string, newest bool) (fabricengine.TrailerCommit, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	order := slices.Clone(s.commits)
	if newest {
		slices.Reverse(order)
	}
	for _, commit := range order {
		for _, trailer := range slices.Backward(commit.trailers) {
			if trailer.Key == key {
				return fabricengine.TrailerCommit{SHA: commit.sha, Value: trailer.Value, CodeHead: commit.codeHead}, true, nil
			}
		}
	}
	return fabricengine.TrailerCommit{}, false, nil
}

// fileAt returns relPath as the commit rev holds it.
func (s *commitStore) fileAt(rev, relPath string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, commit := range s.commits {
		if commit.sha == rev {
			data, found := commit.files[relPath]
			return data, found, nil
		}
	}
	return nil, false, fmt.Errorf("commitStore: no commit %q", rev)
}
