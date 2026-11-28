// fabric.go defines the Fabric handle, which coordinates a warp checkout with its paired weft checkout.
// Fabric keeps its two gitrepo.Repo instances unexported, so repo-specific operations stay inside this package.
// Cross-repo operations (Commit, Pull, Diff, Status) are Fabric methods.
// A single-sided operation gets a named Fabric method only when a caller outside this package needs it.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// DefaultCommitMessage is the commit message for a weft commit that does not need a custom one.
const DefaultCommitMessage = "weft sync"

// ErrMissingPath is the error for a warp or weft path that does not exist or is not a directory.
// It names the missing path, so a caller can tell which of the two checkouts is absent.
type ErrMissingPath struct {
	Path string
}

// Error implements the error interface, naming the missing path.
func (e *ErrMissingPath) Error() string {
	return fmt.Sprintf("fabricengine: path does not exist or is not a directory: %s", e.Path)
}

// Fabric is the cross-repo coordination handle over paired warp and weft checkouts.
// Its code and records repositories are unexported, so repo-specific operations stay inside this package.
// Cross-repo operations are Fabric methods;
// a single-sided operation gets a Fabric method only when a caller outside this package needs it.
type Fabric struct {
	code    *gitrepo.Repo
	records *gitrepo.Repo

	warpPath string
	weftPath string

	// afterMergeStart runs in a one-repository merge between the merge start and the settling of the records.
	// It is nil in production.
	// An error it returns ends the merge verb with that error and leaves the merge parked.
	afterMergeStart func() error
}

// newPaired returns a Fabric over the git checkouts at warpPath and weftPath.
// It checks that both paths exist as directories, returning an *ErrMissingPath naming the first missing path.
// warpPath is checked first.
// It is unexported: callers outside this package build a Fabric through Open.
func newPaired(warpPath, weftPath string) (*Fabric, error) {
	if err := requireDir(warpPath); err != nil {
		return nil, err
	}
	if err := requireDir(weftPath); err != nil {
		return nil, err
	}

	return &Fabric{
		code:     gitrepo.New(warpPath),
		records:  gitrepo.New(weftPath),
		warpPath: warpPath,
		weftPath: weftPath,
	}, nil
}

// requireDir returns an *ErrMissingPath naming path when path does not exist or is not a directory, and nil otherwise.
func requireDir(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return &ErrMissingPath{Path: path}
	}
	return nil
}

// SyncOptions controls git sync behavior for weft-touching operations.
type SyncOptions struct {
	// SkipGit skips git operations on the weft side if true.
	// The warp commit and the env-gated async push still run.
	SkipGit  bool
	SkipPush bool // Skip push operations if true; affects push only.
	// BackoffSleep sleeps one delay of the push backoff between a push's retries; nil means sleep.
	BackoffSleep func(time.Duration)
	// Notifier mails a failed push's record to the hub's orch; nil mails through the process's installed notifier.
	Notifier PushNotifier
	// RunSlug names the run a failed push of these options serves, so its failed-push record lists it in Runs.
	// Empty means the push serves no run.
	RunSlug string
	// AfterRecords marks a task pair's code-side push sent once the run's own records side landed.
	// Only such a push resets a code tip that no transition of the run names, when origin's code branch is ahead.
	AfterRecords bool
}

// EnvSyncOptions reads FABRIC_SKIP_GIT and FABRIC_SKIP_PUSH and returns the SyncOptions they describe.
// It is the test and CI bypass gate for every weft-touching operation.
func EnvSyncOptions() SyncOptions {
	return SyncOptions{
		SkipGit:  os.Getenv("FABRIC_SKIP_GIT") == "1",
		SkipPush: os.Getenv("FABRIC_SKIP_PUSH") == "1",
	}
}

// RecordsWorktree returns the path to the records worktree of l's worktree.
// It is the weft sibling paired with l's warp worktree, and in a one-repository hub l's worktree itself.
// Code outside this package uses it to find the records worktree, so that path stays fabric-owned.
func RecordsWorktree(l *lymcwd.Location) string {
	if oneRepositoryAt(l.WorktreePath()) {
		return l.WorktreePath()
	}
	return weftname.SiblingPath(l.HubPath, filepath.Base(l.WorktreePath()))
}

// ErrNotAWarpWorktree is returned when a Location resolves to a fabric checkout, not a warp worktree.
var ErrNotAWarpWorktree = errors.New("fabricengine: not a warp worktree")

// RequireWarpWorktree reports whether l names a legal warp worktree, returning ErrNotAWarpWorktree when it does not.
//
// lymcwd resolves coordinates from the cwd alone and cannot tell fabric's own checkouts from ordinary worktrees.
// Standing in a weft sibling, or in the Bolt checkout fabric installs at every anchor, passes the cwd gate.
// The resulting Location then drives every verb against invented geometry:
// a `<slug>-weft-weft` sibling, which cannot exist;
// a weft sibling of the Bolt checkout;
// or a hub whose warp worktrees are the weft repo's own checkouts.
// Refusing here keeps such a Location away from mutating verbs.
func RequireWarpWorktree(l *lymcwd.Location) error {
	name := filepath.Base(l.WorktreePath())

	if strings.HasSuffix(name, weftname.Suffix) {
		return fmt.Errorf("%w: %s is the records sibling of a pair, not a warp worktree; run lym from the paired warp worktree instead",
			ErrNotAWarpWorktree, l.WorktreePath())
	}
	if name == BoltDirName {
		return fmt.Errorf("%w: %s is the hub's %s checkout, not a warp worktree; run lym from a warp worktree instead",
			ErrNotAWarpWorktree, l.WorktreePath(), BoltDirName)
	}
	return nil
}

// RequireDrivableWorktree reports whether l names a legal warp worktree, returning ErrNotAWarpWorktree when it does not.
// A caller outside fabric uses it to refuse fabric's own checkouts without naming the fabric-owned word.
// It adds no behaviour of its own.
func RequireDrivableWorktree(l *lymcwd.Location) error {
	return RequireWarpWorktree(l)
}

// PairComplete reports whether the pair of l is complete.
// l must be the warp worktree's own resolved Location, not the hub's prime.
// A complete pair has its paired sibling worktree present and its warp junctions resolving into it.
// Its origin record must also name a parent branch, because the pair's creation writes it last, before pushing.
// A pair without it makes every later reader of the parent branch refuse.
// A SIGKILL can interrupt the creation partway through, and no rollback runs in a killed process.
// So a bare check that the warp worktree directory resolves cannot tell a finished pair from an interrupted one.
// ok is true only when every check passes; reason names the first one that did not, in fabric's own vocabulary.
// A caller outside fabric should not repeat reason verbatim in its operator-facing text.
func PairComplete(l *lymcwd.Location) (ok bool, reason string, err error) {
	siblingPath := RecordsWorktree(l)
	if _, statErr := os.Stat(siblingPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return false, "the pair's other-side worktree is missing", nil
		}
		return false, "", statErr
	}
	healthy, unhealthyReason := checkJunctionHealth(l)
	if !healthy {
		return false, unhealthyReason, nil
	}
	origin, found, err := ReadOrigin(l)
	if err != nil {
		return false, "", err
	}
	if !found || origin.ParentBranch == "" {
		return false, "the pair's origin record is missing", nil
	}
	return true, "", nil
}

// OriginURL returns the URL of the warp side's configured "origin" remote.
// It is a single-sided operation that a caller outside this package needs, so it is a named method.
func (f *Fabric) OriginURL() (string, error) {
	return f.code.RemoteURL("origin")
}

// PushBranch pushes f's warp side without a rebase and returns the PushResult as it came back.
// It discards nothing from the PushResult; the caller decides what to keep.
func (f *Fabric) PushBranch(opts SyncOptions) (PushResult, error) {
	return PushWarpRebaseFreeAt(f.warpPath, opts)
}

// ScopedPathspec returns the entries of dirs joined under relPath.
// At relPath == ".", this returns dirs unchanged;
// at relPath == "sub", ["_lym"] becomes ["sub/_lym"].
func ScopedPathspec(relPath string, dirs []string) []string {
	result := make([]string, len(dirs))
	for i, dir := range dirs {
		result[i] = filepath.Join(relPath, dir)
	}
	return result
}
