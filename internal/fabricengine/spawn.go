// spawn.go — the detached push spawn, which starts a child to push a PushSpawn request.
// SpawnStatusPush builds that request from a location alone.
// PushWarpAt and PushWarpRebaseFreeAt are the synchronous warp-side pushes, with no Fabric instance.

package fabricengine

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
)

const (
	// PauseSlugFlag is the hidden flag naming the slug a push child's network calls are held under; empty for the prime.
	PauseSlugFlag = "pause-slug"
	// TeardownSlugFlag is the hidden fabric flag that tells a detached push child the pair whose teardown it yields to.
	TeardownSlugFlag = "teardown-slug"
	// ParentLockedFlag is the hidden flag marking a child spawned under a held parent lock; it never exits for coverage.
	ParentLockedFlag = "parent-locked"
)

// ErrSpawnUnderTest is returned by SpawnDetachedPush in a test binary, where forking the binary would re-run the suite.
var ErrSpawnUnderTest = errors.New("fabricengine: detached push not spawned under test")

// PushSpawn is one detached push request.
// WarpPath and WeftPath name the worktrees to push, either empty to leave that side out.
// PauseSlug is the slug the child's network calls are held under, empty for the hub's prime.
// TeardownSlug is the pair whose teardown the child yields to, empty when no teardown can remove what the child pushes.
// ParentLocked reports that the pair's parent lock was held when the child was spawned;
// the child never exits for a pause covering it.
// WorkDir is the directory the child starts in.
type PushSpawn struct {
	WarpPath     string
	WeftPath     string
	PauseSlug    string
	TeardownSlug string
	ParentLocked bool
	WorkDir      string
}

// SpawnResult reports what SpawnStatusPush did.
// Spawned reports whether a child was requested, and Request is what it was asked to push.
type SpawnResult struct {
	Spawned bool
	Request PushSpawn
}

// SpawnDetachedPush launches a detached, windowless child that pushes the sides req carries.
// Returns nil immediately, forking no child, when FABRIC_SKIP_GIT or FABRIC_SKIP_PUSH is set.
// It also returns nil when both paths are empty, since there is nothing to push.
// Returns ErrSpawnUnderTest, forking no child, in a test binary.
// The child is reaped in the background through proc.Reap rather than waited for,
// and its stdin/stdout/stderr are left nil so no handle is inherited from the parent.
// The started child is registered in the hub's owner record.
func SpawnDetachedPush(req PushSpawn) error {
	if os.Getenv("FABRIC_SKIP_GIT") == "1" || os.Getenv("FABRIC_SKIP_PUSH") == "1" {
		return nil
	}
	if req.WarpPath == "" && req.WeftPath == "" {
		return nil
	}
	if testing.Testing() {
		return ErrSpawnUnderTest
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	return startDetachedPush(exe, req)
}

// startDetachedPush starts the detached push child of req from exe and registers it in the owner record.
// A registration failure is logged and never fails the spawn, since the push is already running.
func startDetachedPush(exe string, req PushSpawn) error {
	args, dir, owner := pushSpawnCommand(req)
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	proc.Detach(cmd)
	// Leave stdin/stdout/stderr nil so no handles are inherited from the parent.
	if err := cmd.Start(); err != nil {
		logger.Warn("fabricengine: spawn detached push failed", "exe", exe, "args", args, "dir", dir, "error", err)
		return err
	}
	proc.Reap(cmd)
	pid := cmd.Process.Pid
	logger.Info("fabricengine: spawned detached push", "exe", exe, "args", args, "dir", dir, "pid", pid)
	if err := runproc.Register(RunProcDir(filepath.Dir(dir)), owner, runproc.ChildRecord(pid, runproc.RolePush, args)); err != nil {
		logger.Warn("fabricengine: detached push not registered", "owner", owner, "pid", pid, "error", err)
	}
	return nil
}

// pushSpawnCommand returns the arguments, working directory and owner of the child req describes.
// The path flags, the teardown flag and the parent-locked flag are present only when set.
// The pause flag is always present, written `--pause-slug=` when empty so the prime's empty slug reaches the child.
// The owner is the teardown slug when set, so a pair's teardown ends its own pushes;
// otherwise it is the hub owner, so a teardown never ends a prime's records push.
func pushSpawnCommand(req PushSpawn) (args []string, dir, owner string) {
	args = []string{"fabric"}
	if req.WarpPath != "" {
		args = append(args, "--warp-path", absoluteOrAsGiven(req.WarpPath))
	}
	if req.WeftPath != "" {
		args = append(args, "--weft-path", absoluteOrAsGiven(req.WeftPath))
	}
	if req.TeardownSlug != "" {
		args = append(args, "--"+TeardownSlugFlag, req.TeardownSlug)
	}
	if req.ParentLocked {
		args = append(args, "--"+ParentLockedFlag)
	}
	args = append(args, "--"+PauseSlugFlag+"="+req.PauseSlug, "push")
	owner = runproc.HubOwner
	if req.TeardownSlug != "" {
		owner = req.TeardownSlug
	}
	return args, req.WorkDir, owner
}

// absoluteOrAsGiven returns path made absolute, or path itself when the working directory cannot be read.
func absoluteOrAsGiven(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

// SpawnStatusPush launches the detached push of the status records at l and reports what it requested.
// It resolves every path from l alone, so no caller names a records path or pushes the prime's code branch.
// In the hub's prime only the records side is pushed, held under gateSlug and with no teardown slug;
// in a task pair both sides are pushed, held and teardown-guarded under the pair's own slug.
// The child starts in the hub's board worktree, outside both of the pair's worktrees.
// A failed or never-run push leaves the commits local; the next detached push carries them.
// That push also creates the upstream of a never-pushed branch.
func SpawnStatusPush(l *lymcwd.Location, gateSlug string) (SpawnResult, error) {
	req, err := statusPushRequest(l, gateSlug)
	if err != nil {
		return SpawnResult{}, err
	}
	return SpawnResult{Request: req, Spawned: true}, spawnDetachedPushFn(req)
}

// statusPushRequest builds the detached push request for l's status records.
func statusPushRequest(l *lymcwd.Location, gateSlug string) (PushSpawn, error) {
	isPrime, err := IsPrimeWorktree(l)
	if err != nil {
		return PushSpawn{}, err
	}
	req := PushSpawn{WeftPath: RecordsWorktree(l), PauseSlug: gateSlug, WorkDir: BoltDir(l.HubPath)}
	if !isPrime {
		req.WarpPath = l.WorktreePath()
		req.PauseSlug = l.WorktreeName
		req.TeardownSlug = l.WorktreeName
		req.ParentLocked = parentLockHeldBy(l)
	}
	return req, nil
}

// parentLockHeldBy reports whether a live parent lock record names l's pair slug.
// It is set only for the pushes a Finalize spawns past its parent-side merge.
// A failed read is logged and reads as not held.
func parentLockHeldBy(l *lymcwd.Location) bool {
	store := parentlock.Store{Dir: parentlock.Dir(BoltDir(l.HubPath))}
	held, err := store.HeldBy(l.WorktreeName)
	if err != nil {
		logger.Warn("fabricengine: parent lock records unreadable; the push is spawned without the parent-locked mark", "slug", l.WorktreeName, "error", err)
		return false
	}
	return held
}

// SpawnPairPush launches the detached push of the task pair at l: its code worktree and its paired records worktree.
// A failed or never-run push leaves the pair's commits local; the pair's next detached push carries them.
// That push also creates the upstream of a never-pushed branch.
func SpawnPairPush(l *lymcwd.Location) error {
	_, err := SpawnStatusPush(l, "")
	return err
}

// PushWarpAt pushes unpushed commits at warpPath directly through gitrepo.PushCoalesced.
// It needs no Fabric instance and no weft path, and it honors SkipGit and SkipPush.
//
// PushCoalesced writes its single-pusher lock file at the repo root.
// The weft repo excludes that file through seedWeftArtifactExcludes, and the warp repo has no such entry,
// so a caller must first seed the warp-side exclude, or the call leaves untracked residue in the user's repo.
func PushWarpAt(warpPath string, opts SyncOptions) (res PushResult, err error) {
	rec := NewMutations(filepath.Dir(warpPath))
	defer func() { res.Mutations = rec.Snapshot() }()

	if opts.SkipGit || opts.SkipPush {
		return PushResult{}, nil
	}

	repo := gitrepo.New(warpPath)
	hadUnpushed, hadUnpushedErr := repo.HasUnpushed()
	if err := repo.PushCoalesced(); err != nil {
		return PushResult{}, err
	}
	recordPushIfAdvanced(rec, repo, "code", warpPath, hadUnpushed, hadUnpushedErr)

	return PushResult{}, nil
}

// PushWarpRebaseFreeAt pushes unpushed commits at warpPath directly, rebase-free.
// It takes no Fabric instance and no weft path.
//
// The push never runs `git pull --rebase`, so it never rewrites this side's SHAs while the paired side is not rebased.
// It also never invalidates the correspondence index, and it takes no push lock.
//
// A failed push is classified and recorded as a failed push, and it is returned as a *PushFailedError.
// The error wraps gitrepo.ErrPushRejected for a rejection and gitrepo.ErrPushAuthRefused for a refused credential.
// A rejection is retried over the push backoff only while origin holds no commit the branch lacks.
// A rejection that remains is a condition a human must decide.
// A task branch that origin does not have, or that origin rewound past commits this machine holds, is not pushed.
func PushWarpRebaseFreeAt(warpPath string, opts SyncOptions) (res PushResult, err error) {
	rec := NewMutations(filepath.Dir(warpPath))
	defer func() { res.Mutations = rec.Snapshot() }()

	if opts.SkipGit || opts.SkipPush {
		return PushResult{}, nil
	}

	if err := pushClassified(rec, filepath.Dir(warpPath), failedPushSideCode, warpPath, opts, nil); err != nil {
		return PushResult{}, err
	}
	return PushResult{}, nil
}
