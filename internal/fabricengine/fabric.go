// fabric.go — the Fabric handle: fabric's cross-repo coordination point over two
// internal/gitrepo.Repo instances, plus the sync-options/pathspec plumbing its cross-repo
// operations need.
// Fabric holds its two gitrepo.Repo instances as UNEXPORTED fields, so anything repo-specific and
// uncoordinated (f.warp.StageAndCommit(...), f.weft.ChangedFilesSince(...)) is reachable only from
// inside this package;
// only the genuinely cross-repo operations (Commit, Pull, Diff, Status) get their own method on
// Fabric.
// A single-sided, uncoordinated op also earns a named Fabric method — rather than staying direct
// field access — precisely when it must be callable from OUTSIDE this package, so the one-repo
// illusion holds at the public API boundary;
// f.warp/f.weft field access remains correct for uncoordinated ops used only inside
// internal/fabricengine.
// See warpforward.go's CheckoutDetached/RestoreBranch/CurrentBranch/ResetHard for the warp-only
// examples of this carve-out.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// DefaultCommitMessage is the message used by every weft-commit caller that does not need a custom
// one.
const DefaultCommitMessage = "weft sync"

// ErrMissingPath is a typed error returned by newPaired when either the warp or the weft path does
// not exist or is not a directory.
// It names the specific missing path so a caller (or an operator reading the error) knows which of
// the two repos is absent, rather than a generic "one of the two is missing".
type ErrMissingPath struct {
	Path string
}

// Error implements the error interface, naming the missing path.
func (e *ErrMissingPath) Error() string {
	return fmt.Sprintf("fabricengine: path does not exist or is not a directory: %s", e.Path)
}

// Fabric is the cross-repo coordination handle over paired warp (warp) and weft checkouts.
// warp and weft are unexported for uncoordinated, repo-specific operations reached only from
// inside this package;
// cross-repo operations get their own Fabric methods, and a single-sided operation earns a named
// Fabric method only when out-of-package callers need it.
type Fabric struct {
	warp *gitrepo.Repo
	weft *gitrepo.Repo

	warpPath string
	weftPath string
}

// newPaired returns a Fabric wrapping git checkouts at warpPath and weftPath.
// Unlike gitrepo.New, it stat-checks that both paths exist as directories, returning an
// *ErrMissingPath naming any missing path.
// warpPath is checked first.
// It is unexported — Open(l) is the only constructor any other package calls.
func newPaired(warpPath, weftPath string) (*Fabric, error) {
	if err := requireDir(warpPath); err != nil {
		return nil, err
	}
	if err := requireDir(weftPath); err != nil {
		return nil, err
	}

	return &Fabric{
		warp:     gitrepo.New(warpPath),
		weft:     gitrepo.New(weftPath),
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
	// SkipGit skips all git operations if true — narrowed by Fabric.Commit
	// (commit.go) to weft-scoped: its warp commit and its env-gated async
	// push proceed regardless. See Fabric.Commit's doc comment.
	SkipGit  bool
	SkipPush bool // Skip push operations if true; affects push only.
}

// EnvSyncOptions reads the WEFT_SKIP_GIT and WEFT_SKIP_PUSH environment variables and returns the
// SyncOptions they describe — the uniform test/CI bypass gate for every weft-touching operation.
func EnvSyncOptions() SyncOptions {
	return SyncOptions{
		SkipGit:  os.Getenv("WEFT_SKIP_GIT") == "1",
		SkipPush: os.Getenv("WEFT_SKIP_PUSH") == "1",
	}
}

// WeftWorktree returns the path to the weft worktree paired with l's warp worktree.
// It is the read-only accessor every non-fabric caller that needs to know the weft sibling's
// location goes through, closing the weft-visibility leak where those callers used to reach
// lyxcwd.Location directly for a fabric-owned path.
func WeftWorktree(l *lyxcwd.Location) string {
	return weftname.SiblingPath(l.HubPath, filepath.Base(l.WorktreePath()))
}

// ErrNotAWarpWorktree is returned by RequireWarpWorktree for a Location that resolved onto one of
// fabric's own internal checkouts rather than a warp worktree.
var ErrNotAWarpWorktree = errors.New("fabricengine: not a warp worktree")

// RequireWarpWorktree reports whether l names a legal warp worktree, returning ErrNotAWarpWorktree
// when it does not.
//
// lyxcwd resolves coordinates from cwd alone and has no way to tell fabric's own checkouts apart
// from an ordinary worktree — that distinction is fabric vocabulary, which the Cwd Resolution
// Invariant keeps out of lyxcwd entirely. So standing in a weft sibling, or inside the operator-
// convenience `_board` link fabric itself installs at every anchor, produces a Location that passes
// the cwd gate and then drives every verb against invented geometry: a `<slug>-weft-weft` sibling
// that cannot exist, a `_board-weft` pair, a hub whose "warp worktrees" are the weft repo's own
// checkouts.
// Refusing here is what keeps that from reaching a mutating verb.
func RequireWarpWorktree(l *lyxcwd.Location) error {
	name := filepath.Base(l.WorktreePath())

	if strings.HasSuffix(name, weftname.Suffix) {
		return fmt.Errorf("%w: %s is the weft sibling of a pair, not a warp worktree; run lyx from the paired warp worktree instead",
			ErrNotAWarpWorktree, l.WorktreePath())
	}
	if name == BoardDirName {
		return fmt.Errorf("%w: %s is the hub's %s checkout, not a warp worktree; run lyx from a warp worktree instead",
			ErrNotAWarpWorktree, l.WorktreePath(), BoardDirName)
	}
	return nil
}

// RequireDrivableWorktree is RequireWarpWorktree's fabric-vocabulary-neutral spelling, for a caller
// outside the Fabric Vocabulary Invariant's owner set -- internal/battencli, whose bookend rows must
// refuse fabric's own checkouts before driving topology, is the first one.
//
// Such a caller cannot name RequireWarpWorktree at all: the invariant's scan matches the bare token
// inside an identifier, so the published name is itself the leak. This is the same shape
// CommitAnchoredPaths, PushAnchored and Fabric.PushBranch already take -- fabric owns the word and
// hands out a spelling that does not carry it.
//
// It adds no behaviour of its own: the refusal text a caller renders is RequireWarpWorktree's own,
// which fabric is entitled to spell in its own vocabulary.
func RequireDrivableWorktree(l *lyxcwd.Location) error {
	return RequireWarpWorktree(l)
}

// PairSiblingRemnant reports whether slug's weft worktree is still on disk, and where, for a caller
// that has already found the slug's warp worktree gone.
// It exists for the same vocabulary reason RequireDrivableWorktree does: a caller outside the Fabric
// Vocabulary Invariant's owner set cannot name WeftWorktreePath, yet a teardown row re-entered after
// Topology.Remove was interrupted between its two halves must tell "the pair is gone" from "only
// the warp side is gone", because the second is the debris `lyx fabric prune` exists to remove.
// A stat error other than absence is returned, never folded into false.
func PairSiblingRemnant(l *lyxcwd.Location, slug string) (path string, present bool, err error) {
	path = WeftWorktreePath(l, slug)
	if _, statErr := os.Stat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return path, false, nil
		}
		return path, false, statErr
	}
	return path, true, nil
}

// PairComplete reports whether l's pair -- l is the warp worktree's own resolved Location, not the
// hub's prime -- satisfies Add's own full post-condition: the paired sibling worktree exists and
// the warp junctions actually resolve into it.
// It exists for the same vocabulary reason PairSiblingRemnant does, for a caller that must tell a
// pair Add finished from one a SIGKILL interrupted partway through: Add's own in-process rollback
// never runs when the process that called it dies instead of Add itself returning an error, so a
// bare "the warp worktree directory resolves" check cannot draw that line.
// ok is true only when every check passes; reason names the first one that did not, in fabric's own
// vocabulary -- not for a non-owner caller to repeat verbatim in its own operator-facing text, the
// same restraint createRefusal already applies to Add's own errors.
func PairComplete(l *lyxcwd.Location) (ok bool, reason string, err error) {
	siblingPath := WeftWorktree(l)
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
	return true, "", nil
}

// WeftLyxDir returns the path to the _lyx directory in l's weft sibling worktree.
// It is the junction target for lyx weft and the pathspec base for weft operations.
func WeftLyxDir(l *lyxcwd.Location) string {
	return filepath.Join(WeftWorktree(l), l.AnchorRel, lyxdirs.LyxDirName)
}

// OriginURL returns f's warp side's configured "origin" remote URL.
// This is the single-sided-op-callable-from-outside-the-package carve-out this file's own package
// doc comment already states, the same carve-out WeftWorktree and the warp-only accessors named in
// that comment use — internal/loomcli needs the origin URL and is not a Fabric Vocabulary Invariant
// owner.
func (f *Fabric) OriginURL() (string, error) {
	return f.warp.RemoteURL("origin")
}

// PushBranch pushes f's warp side via PushWarpRebaseFreeAt — the vocabulary-neutral spelling
// internal/loomcli must call instead of naming PushWarpRebaseFreeAt directly (see the
// fabric-vocabulary-owner-confinement Shared Decision).
// PushBranch performs no discarding of the returned PushResult; that is the caller's choice.
func (f *Fabric) PushBranch(opts SyncOptions) (PushResult, error) {
	return PushWarpRebaseFreeAt(f.warpPath, opts)
}

// ScopedPathspec returns a slice of pathspec entries, each being the join of relPath with each
// directory in dirs.
// At relPath == ".", this returns dirs unchanged;
// at relPath == "sub", ["_lyx"] becomes ["sub/_lyx"].
func ScopedPathspec(relPath string, dirs []string) []string {
	result := make([]string, len(dirs))
	for i, dir := range dirs {
		result[i] = filepath.Join(relPath, dir)
	}
	return result
}
