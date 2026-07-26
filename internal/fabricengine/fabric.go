// fabric.go — the Fabric handle: fabric's cross-repo coordination point over
// two internal/gitrepo.Repo instances, plus the sync-options/pathspec plumbing
// SyncWeft (a later batch) needs. Fabric exposes Warp and Weft directly as
// exported fields rather than a forwarding method per gitrepo operation —
// consumers call f.Warp.StageAndCommit(...) / f.Weft.ChangedFilesSince(...) for
// anything repo-specific and uncoordinated; only the genuinely cross-repo
// operations (SyncWeft, RevertWithWeft, added in later batches) get their own
// method on Fabric.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// DefaultCommitMessage is the message used by every weft-commit caller that
// does not need a custom one.
const DefaultCommitMessage = "weft sync"

// ErrMissingPath is a typed error returned by New when either the warp or the
// weft path does not exist or is not a directory. It names the specific
// missing path so a caller (or an operator reading the error) knows which of
// the two repos is absent, rather than a generic "one of the two is missing".
type ErrMissingPath struct {
	Path string
}

// Error implements the error interface, naming the missing path.
func (e *ErrMissingPath) Error() string {
	return fmt.Sprintf("fabricengine: path does not exist or is not a directory: %s", e.Path)
}

// Fabric is the cross-repo coordination handle over a paired warp (host) and
// weft checkout, each wrapped as an internal/gitrepo.Repo. Fabric is the only
// type in this module that knows both repos exist; Warp and Weft are exported
// so uncoordinated, repo-specific operations go straight through gitrepo with
// no forwarding-method boilerplate on Fabric itself.
type Fabric struct {
	Warp *gitrepo.Repo
	Weft *gitrepo.Repo

	warpPath string
	weftPath string
}

// New returns a Fabric wrapping the git checkouts at warpPath and weftPath.
// Unlike gitrepo.New (which performs no I/O), New stat-checks that both paths
// already exist as directories — repo topology (clone, worktree add) is
// fabric's own job, so by the time a Fabric handle is constructed, both
// checkouts are expected to be real — and returns an *ErrMissingPath naming
// whichever path is absent or not a directory. warpPath is checked before
// weftPath, so a caller with both missing sees the warp path named first.
func New(warpPath, weftPath string) (*Fabric, error) {
	if err := requireDir(warpPath); err != nil {
		return nil, err
	}
	if err := requireDir(weftPath); err != nil {
		return nil, err
	}

	return &Fabric{
		Warp:     gitrepo.New(warpPath),
		Weft:     gitrepo.New(weftPath),
		warpPath: warpPath,
		weftPath: weftPath,
	}, nil
}

// requireDir returns an *ErrMissingPath naming path when path does not exist
// or is not a directory, and nil otherwise.
func requireDir(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return &ErrMissingPath{Path: path}
	}
	return nil
}

// SyncOptions controls git sync behavior for weft-touching operations.
type SyncOptions struct {
	SkipGit  bool // Skip all git operations if true.
	SkipPush bool // Skip push operations if true; affects push only.
}

// EnvSyncOptions reads the WEFT_SKIP_GIT and WEFT_SKIP_PUSH environment
// variables and returns the SyncOptions they describe — the uniform test/CI
// bypass gate for every weft-touching operation.
func EnvSyncOptions() SyncOptions {
	return SyncOptions{
		SkipGit:  os.Getenv("WEFT_SKIP_GIT") == "1",
		SkipPush: os.Getenv("WEFT_SKIP_PUSH") == "1",
	}
}

// ScopedPathspec returns a slice of pathspec entries, each being the join of
// relPath with each directory in dirs. At relPath == ".", this returns dirs
// unchanged; at relPath == "sub", ["_lyx"] becomes ["sub/_lyx"].
func ScopedPathspec(relPath string, dirs []string) []string {
	result := make([]string, len(dirs))
	for i, dir := range dirs {
		result[i] = filepath.Join(relPath, dir)
	}
	return result
}
