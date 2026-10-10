// gogit.go implements the go-git handle infrastructure every go-git read builds on: goGit, the lazily-opened and cached *git.Repository accessor, and readGoGit, the whole-read helper that retries once after a pack-fingerprint-gated reindex.
// Every go-git read in this package goes through readGoGit, the only caller of goGit.

package gitrepo

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// goGit returns this Repo's cached go-git handle, opening it on first use via
// git.PlainOpenWithOptions(r.path, &git.PlainOpenOptions{
// EnableDotGitCommonDir: true}). Failed opens are not cached; new calls may
// still succeed later. It takes r.goGitMu itself, so readGoGit is its only caller.
func (r *Repo) goGit() (*git.Repository, error) {
	r.goGitMu.Lock()
	defer r.goGitMu.Unlock()

	if r.goGitOK {
		return r.goGitRepo, nil
	}

	repo, err := git.PlainOpenWithOptions(r.path, &git.PlainOpenOptions{
		EnableDotGitCommonDir: true,
	})
	if err != nil {
		return nil, fmt.Errorf("gitrepo: open go-git handle at %s: %w", r.path, err)
	}

	r.goGitRepo = repo
	r.goGitOK = true
	return repo, nil
}

// readGoGit runs read against this Repo's go-git handle under the shared lock and returns its result.
// A cached handle can read a stale pack index after a repack, so when read fails with plumbing.ErrObjectNotFound and the pack set has changed since the read began, readGoGit reindexes the handle and runs read once more.
// Every other error, and a not-found over an unchanged pack set, passes through unchanged.
// read runs whole on each attempt, so it must resolve everything it needs from the handle it is given and must not call another Repo method.
func readGoGit[T any](r *Repo, read func(repo *git.Repository) (T, error)) (T, error) {
	var zero T

	repo, err := r.goGit()
	if err != nil {
		return zero, err
	}

	r.goGitMu.RLock()
	snapshot := r.lastPackFingerprint
	result, readErr := read(repo)
	r.goGitMu.RUnlock()

	if readErr == nil || !errors.Is(readErr, plumbing.ErrObjectNotFound) {
		return result, readErr
	}
	if !r.reindexIfPacksChanged(repo, snapshot) {
		return result, readErr
	}

	r.goGitMu.RLock()
	defer r.goGitMu.RUnlock()
	return read(repo)
}

// reindexIfPacksChanged reports whether the pack set differs from snapshot, the fingerprint a failed read began under, reindexing repo's storer first unless another caller already did for this pack set.
// It returns false, leaving the failed read's error to stand, for a storer that is not filesystem-backed and for a fingerprint it cannot read.
func (r *Repo) reindexIfPacksChanged(repo *git.Repository, snapshot string) bool {
	storer, ok := repo.Storer.(*filesystem.Storage)
	if !ok {
		return false
	}

	r.goGitMu.Lock()
	defer r.goGitMu.Unlock()

	current, err := packFingerprint(storer)
	if err != nil || current == snapshot {
		return false
	}
	if current != r.lastPackFingerprint {
		storer.Reindex()
		r.lastPackFingerprint = current
		r.reindexCount++
	}
	return true
}

// packFingerprint computes the sorted (name, size) list of every *.idx file
// in objects/pack, joined into one comparable string. Missing pack directory
// is not an error, returning empty string.
func packFingerprint(storer *filesystem.Storage) (string, error) {
	entries, err := storer.Filesystem().ReadDir("objects/pack")
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	type idxEntry struct {
		name string
		size int64
	}
	var idxFiles []idxEntry
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".idx") {
			continue
		}
		idxFiles = append(idxFiles, idxEntry{name: entry.Name(), size: entry.Size()})
	}
	sort.Slice(idxFiles, func(i, j int) bool { return idxFiles[i].name < idxFiles[j].name })

	var b strings.Builder
	for _, f := range idxFiles {
		fmt.Fprintf(&b, "%s:%d;", f.name, f.size)
	}
	return b.String(), nil
}
