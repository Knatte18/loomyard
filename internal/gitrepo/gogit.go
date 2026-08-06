// gogit.go implements the go-git handle infrastructure every migrated read in
// later batches builds on: goGit, the lazily-opened and cached *git.Repository
// accessor, and lookupObjectRetrying, the pack-fingerprint-gated
// reindex-and-retry helper every migrated object lookup (commit, tree, or
// blob resolution) must route through. Nothing in this file changes any
// existing method's backend — see gitrepo.go's Repo struct doc and this
// file's own godoc for the locking discipline both pieces establish.

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
// still succeed later. Callers must hold r.goGitMu for the entire duration of
// their use of the returned handle, not just across the call to goGit.
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

// lookupObjectRetrying calls lookup; on object-not-found, it checks if the
// pack fingerprint has changed and reindexes if so, then retries. Held read
// lock via r.goGitMu throughout. The fingerprint gate keeps genuinely-absent
// objects from paying a reindex cost on every call.
func lookupObjectRetrying[T any](r *Repo, repo *git.Repository, lookup func() (T, error)) (T, error) {
	r.goGitMu.Lock()
	defer r.goGitMu.Unlock()

	result, err := lookup()
	if err == nil || !errors.Is(err, plumbing.ErrObjectNotFound) {
		return result, err
	}

	storer, ok := repo.Storer.(*filesystem.Storage)
	if !ok {
		return result, err
	}

	fingerprint, fpErr := packFingerprint(storer)
	if fpErr != nil {
		return result, err
	}
	if fingerprint == r.lastPackFingerprint {
		return result, err
	}

	storer.Reindex()
	r.lastPackFingerprint = fingerprint
	return lookup()
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
