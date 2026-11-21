// gogit.go holds the go-git handle accessor, goGit, and readGoGit, which runs one read against that handle.
// readGoGit holds the shared lock for the read and retries it once after a reindex gated on the pack fingerprint.
// The handle's storer reads the repository config without go-git's branch validation.

package gitrepo

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-billy/v5/util"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	formatconfig "github.com/go-git/go-git/v5/plumbing/format/config"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-git/v5/storage/filesystem/dotgit"
)

// gitConfigFile is the repository config's file name inside the git directory.
const gitConfigFile = "config"

// gitCommonDirFile is the file in a linked worktree's git directory that names the shared common directory.
const gitCommonDirFile = "commondir"

// gitDirFilePrefix opens the line of a linked worktree's .git file that names its git directory.
const gitDirFilePrefix = "gitdir: "

// goGit returns this Repo's cached go-git handle, opening it on first use.
// A failed open is not cached, so a later call may still succeed.
// goGit takes r.goGitMu itself, so the caller must not hold it.
func (r *Repo) goGit() (*git.Repository, error) {
	r.goGitMu.Lock()
	defer r.goGitMu.Unlock()

	if r.goGitOK {
		return r.goGitRepo, nil
	}

	repo, err := openGoGitWithUnvalidatedBranchConfig(r.path)
	if err != nil {
		return nil, fmt.Errorf("gitrepo: open go-git handle at %s: %w", r.path, err)
	}

	r.goGitRepo = repo
	r.goGitOK = true
	return repo, nil
}

// openGoGitWithUnvalidatedBranchConfig opens the repository whose worktree, or bare directory, is path.
// It finds the git directory as go-git's PlainOpenWithOptions does with EnableDotGitCommonDir set and DetectDotGit unset:
// a linked worktree reads the shared common directory, and a path that is not itself a repository fails.
// Its storer reads the repository config without go-git's branch validation.
// go-git's own read refuses the whole config, and with it the open, when a branch merges a ref outside refs/heads/;
// a branch tracking a namespaced remote ref does.
func openGoGitWithUnvalidatedBranchConfig(path string) (*git.Repository, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	root := osfs.New(absolute)

	var worktree billy.Filesystem
	gitDir := root
	info, err := root.Stat(git.GitDirName)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return nil, err
	case info.IsDir():
		worktree = root
		if gitDir, err = root.Chroot(git.GitDirName); err != nil {
			return nil, err
		}
	default:
		worktree = root
		if gitDir, err = linkedWorktreeGitDir(absolute, root); err != nil {
			return nil, err
		}
	}
	if _, err := gitDir.Stat(""); err != nil {
		if os.IsNotExist(err) {
			return nil, git.ErrRepositoryNotExists
		}
		return nil, err
	}

	commonDir, err := commonGitDir(gitDir)
	if err != nil {
		return nil, err
	}
	storage := filesystem.NewStorage(dotgit.NewRepositoryFilesystem(gitDir, commonDir), cache.NewObjectLRUDefault())
	return git.Open(unvalidatedBranchConfigStorage{storage}, worktree)
}

// linkedWorktreeGitDir returns the git directory a linked worktree's .git file names.
// A relative name resolves against the worktree at path.
func linkedWorktreeGitDir(path string, root billy.Filesystem) (billy.Filesystem, error) {
	content, err := util.ReadFile(root, git.GitDirName)
	if err != nil {
		return nil, err
	}
	line, _, _ := strings.Cut(string(content), "\n")
	if !strings.HasPrefix(line, gitDirFilePrefix) {
		return nil, fmt.Errorf(".git file has no %s prefix", gitDirFilePrefix)
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, gitDirFilePrefix))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(path, gitDir)
	}
	return osfs.New(gitDir), nil
}

// commonGitDir returns the shared common directory a linked worktree's git directory names.
// It returns nil for a git directory that names none.
// A named directory that does not exist is git.ErrRepositoryIncomplete.
func commonGitDir(gitDir billy.Filesystem) (billy.Filesystem, error) {
	content, err := util.ReadFile(gitDir, gitCommonDirFile)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	named := strings.TrimSpace(string(content))
	if named == "" {
		return nil, nil
	}
	if !filepath.IsAbs(named) {
		named = filepath.Join(gitDir.Root(), named)
	}
	commonDir := osfs.New(named)
	if _, err := commonDir.Stat(""); err != nil {
		if os.IsNotExist(err) {
			return nil, git.ErrRepositoryIncomplete
		}
		return nil, err
	}
	return commonDir, nil
}

// unvalidatedBranchConfigStorage is a filesystem storer whose config read keeps a branch merge setting.
// That includes a setting naming a ref outside refs/heads/.
// Every other read and write is the filesystem storer's own.
type unvalidatedBranchConfigStorage struct {
	*filesystem.Storage
}

// Config reads the repository config, keeping a branch merge setting go-git's own read refuses.
// A missing config file reads as an empty config.
func (s unvalidatedBranchConfigStorage) Config() (*config.Config, error) {
	content, err := util.ReadFile(s.Filesystem(), gitConfigFile)
	if os.IsNotExist(err) {
		return config.NewConfig(), nil
	}
	if err != nil {
		return nil, err
	}
	return decodeConfigWithoutBranchValidation(content)
}

// decodeConfigWithoutBranchValidation decodes a repository config as go-git's config read does.
// A branch whose merge setting names a ref outside refs/heads/ keeps it instead of failing the read.
// Git accepts such a setting: a branch tracking a namespaced remote ref has one.
// The returned config's Raw holds every option as decoded.
func decodeConfigWithoutBranchValidation(content []byte) (*config.Config, error) {
	raw := formatconfig.New()
	if err := formatconfig.NewDecoder(bytes.NewReader(content)).Decode(raw); err != nil {
		return nil, err
	}

	validated := formatconfig.New()
	if err := formatconfig.NewDecoder(bytes.NewReader(content)).Decode(validated); err != nil {
		return nil, err
	}
	refusedMerges := map[string]plumbing.ReferenceName{}
	for _, branch := range validated.Section("branch").Subsections {
		merge := plumbing.ReferenceName(branch.Option("merge"))
		if merge != "" && !merge.IsBranch() {
			refusedMerges[branch.Name] = merge
			branch.RemoveOption("merge")
		}
	}
	var encoded bytes.Buffer
	if err := formatconfig.NewEncoder(&encoded).Encode(validated); err != nil {
		return nil, err
	}

	cfg := config.NewConfig()
	if err := cfg.Unmarshal(encoded.Bytes()); err != nil {
		return nil, err
	}
	for name, merge := range refusedMerges {
		cfg.Branches[name].Merge = merge
	}
	cfg.Raw = raw
	return cfg, nil
}

// readGoGit runs read against this Repo's go-git handle under the shared lock and returns its result.
// A repack can leave the cached handle with a stale pack index, so readGoGit retries a failed read once.
// The retry needs read to fail with plumbing.ErrObjectNotFound and the pack set to have changed since it began.
// Every other error, and a not-found over an unchanged pack set, passes through unchanged.
// read runs whole on each attempt,
// so it must resolve everything it needs from the handle it is given and must not call another Repo method.
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

// reindexIfPacksChanged reports whether the pack set differs from snapshot, the fingerprint a failed read began under.
// When it differs, it reindexes repo's storer unless an earlier call already did so for the same pack set.
// It returns false, leaving the failed read's error to stand, for a storer that is not filesystem-backed.
// It also returns false when the fingerprint cannot be read.
// The index rebuild is forced under the exclusive lock because go-git rebuilds it unsynchronized on the next read.
// Readers holding only the shared lock would otherwise race into a half-built index.
func (r *Repo) reindexIfPacksChanged(repo *git.Repository, snapshot string) bool {
	storer, ok := repo.Storer.(unvalidatedBranchConfigStorage)
	if !ok {
		return false
	}

	r.goGitMu.Lock()
	defer r.goGitMu.Unlock()

	current, err := packFingerprint(storer.Storage)
	if err != nil || current == snapshot {
		return false
	}
	if current != r.lastPackFingerprint {
		storer.Reindex()
		_ = storer.HasEncodedObject(plumbing.ZeroHash)
		r.lastPackFingerprint = current
		r.reindexCount++
	}
	return true
}

// packFingerprint joins the sorted (name, size) of every *.idx file in objects/pack into one comparable string.
// A missing pack directory is not an error and yields the empty string.
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
