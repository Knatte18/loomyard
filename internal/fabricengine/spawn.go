// spawn.go — the engine-level detached push spawn and its synchronous warp-push counterparts.
// A PushSpawn is one detached push request: the side paths to push, the two slug flags the child is told, and the directory it starts in.
// PauseSlugFlag names the pause entry the child's network calls are held under;
// TeardownSlugFlag names the pair whose teardown the child must not outlive.
// SpawnStatusPush is the one entry point for a status push: from a location alone it applies the prime rule, the slug roles and the local-branch check.
// SpawnDetachedPush starts the child a request describes, which re-enters the fabric CLI's bypass mode and pushes whichever side(s) the request carries.
// PushWarpAt is the synchronous, no-Fabric-instance push of the warp side through gitrepo.PushCoalesced.
// It has no production caller: the detached child's bypass handler (internal/fabriccli's
// `push` RunE) drives CoalescePushRequest instead, which pushes both sides under one absorbing lock.
// PushWarpRebaseFreeAt is the warp-side push a production caller reaches, classifying and recording a failure.

package fabricengine

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
)

const (
	// PauseSlugFlag is the hidden fabric flag that tells a detached push child the slug its network calls are held under, empty for the hub's prime.
	PauseSlugFlag = "pause-slug"
	// TeardownSlugFlag is the hidden fabric flag that tells a detached push child the pair whose teardown it yields to.
	TeardownSlugFlag = "teardown-slug"
)

// ErrSpawnUnderTest is returned by SpawnDetachedPush in a test binary, where forking the running binary would re-run the test suite.
var ErrSpawnUnderTest = errors.New("fabricengine: detached push not spawned under test")

// PushSpawn is one detached push request.
// WarpPath and WeftPath name the worktrees to push, either empty to leave that side out.
// PauseSlug is the slug the child's network calls are held under, empty for the hub's prime.
// TeardownSlug is the pair whose teardown the child yields to, empty when no teardown can remove what the child pushes.
// WorkDir is the directory the child starts in.
type PushSpawn struct {
	WarpPath     string
	WeftPath     string
	PauseSlug    string
	TeardownSlug string
	WorkDir      string
}

// SpawnResult reports what SpawnStatusPush did.
// Spawned reports whether a child was requested, and Request is what it was asked to push.
// LocalSkipped names each branch left out of the request because it is local-only.
type SpawnResult struct {
	Spawned      bool
	Request      PushSpawn
	LocalSkipped []string
}

// SpawnDetachedPush launches a detached, windowless child that pushes the sides req carries.
// Returns nil immediately, forking no child, when FABRIC_SKIP_GIT or FABRIC_SKIP_PUSH is set or when both paths are empty — there is nothing to push.
// Returns ErrSpawnUnderTest, forking no child, in a test binary.
// The child is started but never Waited,
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
	pid := cmd.Process.Pid
	logger.Info("fabricengine: spawned detached push", "exe", exe, "args", args, "dir", dir, "pid", pid)
	if err := runproc.Register(RunProcDir(filepath.Dir(dir)), owner, runproc.ChildRecord(pid, runproc.RolePush, args)); err != nil {
		logger.Warn("fabricengine: detached push not registered", "owner", owner, "pid", pid, "error", err)
	}
	return nil // intentionally not Wait()ed
}

// pushSpawnCommand returns the arguments, working directory and owner of the child req describes.
// The path flags and the teardown flag are present only when set, and the pause flag always, written `--pause-slug=` when empty so the prime's empty slug reaches the child.
// The owner is the teardown slug when set, so a pair's teardown ends its own pushes, and the hub owner otherwise, so it never ends a prime records push.
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
	args = append(args, "--"+PauseSlugFlag+"="+req.PauseSlug, "push")
	owner = runproc.HubOwner
	if req.TeardownSlug != "" {
		owner = req.TeardownSlug
	}
	return args, req.WorkDir, owner
}

// absoluteOrAsGiven returns path made absolute, or path itself when the working directory it would be joined to cannot be read.
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
// A side whose current branch is local-only is left out of the request and named in LocalSkipped, and with no side left no child is spawned.
// A local-only records branch spawns nothing either, since the child's push cannot run without the records side.
// A failed or never-run push leaves the commits local; the next detached push carries them, and creates a never-pushed branch's upstream.
func SpawnStatusPush(l *lyxcwd.Location, gateSlug string) (SpawnResult, error) {
	req, localSkipped, err := statusPushRequest(l, gateSlug)
	if err != nil {
		return SpawnResult{}, err
	}
	res := SpawnResult{Request: req, LocalSkipped: localSkipped}
	if req.WeftPath == "" {
		return res, nil
	}
	res.Spawned = true
	return res, spawnDetachedPushFn(req)
}

// statusPushRequest builds the detached push request for l's status records and names the local-only branches it left out.
func statusPushRequest(l *lyxcwd.Location, gateSlug string) (req PushSpawn, localSkipped []string, err error) {
	isPrime, err := IsPrimeWorktree(l)
	if err != nil {
		return PushSpawn{}, nil, err
	}
	req = PushSpawn{WeftPath: RecordsWorktree(l), PauseSlug: gateSlug, WorkDir: BoardDir(l.HubPath)}
	if !isPrime {
		req.WarpPath = l.WorktreePath()
		req.PauseSlug = l.WorktreeName
		req.TeardownSlug = l.WorktreeName
	}

	for _, side := range []*string{&req.WarpPath, &req.WeftPath} {
		if *side == "" {
			continue
		}
		branch, marked, err := localMarkOfCurrentBranch(*side)
		if err != nil {
			return PushSpawn{}, nil, err
		}
		if marked {
			localSkipped = append(localSkipped, branch)
			*side = ""
		}
	}
	return req, localSkipped, nil
}

// localMarkOfCurrentBranch returns the branch checked out at worktree and whether it carries the local-only mark.
// A detached HEAD has no branch to mark, so it reads unmarked.
func localMarkOfCurrentBranch(worktree string) (branch string, marked bool, err error) {
	repo := gitrepo.New(worktree)
	branch, err = repo.CurrentBranch()
	if err != nil {
		return "", false, nil
	}
	marked, err = repo.BranchMarkedLocal(branch)
	return branch, marked, err
}

// SpawnPairPush launches the detached push of the task pair at l: its code worktree and its paired records worktree.
// A failed or never-run push leaves the pair's commits local; the pair's next detached push carries them, and creates a never-pushed branch's upstream.
func SpawnPairPush(l *lyxcwd.Location) error {
	_, err := SpawnStatusPush(l, "")
	return err
}

// PushWarpAt pushes unpushed commits at warpPath directly through gitrepo.PushCoalesced, with no Fabric instance and no weft path involved.
// It honors SkipGit and SkipPush.
//
// It has NO production caller.
// The detached push child's bypass handler drives CoalescePushRequest instead.
// Nothing in shipped fabric ever runs gitrepo.PushCoalesced against a warp worktree.
// That is worth stating rather than leaving to a reader to rediscover: PushCoalesced writes its
// single-pusher lock file at the repo root, and while the weft repo excludes that artifact through
// seedWeftArtifactExcludes, the warp repo has no such entry — so wiring this function into a live
// path would start leaving untracked residue in the user's own repo, which fabric's whole junction
// design exists to avoid. Any future caller must seed the warp-side exclude first.
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

// PushWarpRebaseFreeAt pushes unpushed commits at warpPath directly, rebase-free, with
// no Fabric instance and no weft path involved.
//
// The push never runs `git pull --rebase`, so it never rewrites this side's SHAs while the
// paired side is not rebased and never invalidates the correspondence index; and it never
// takes the push lock, so it leaves no untracked .gitrepo-push.lock residue in the operator's own
// warp repo, which has no exclude entry for that file.
//
// A failed push is classified, recorded as a failed push and returned as a *PushFailedError.
// The error wraps gitrepo.ErrPushRejected for a rejection and gitrepo.ErrPushAuthRefused for a refused credential.
// A rejection is a human-decidable condition, never retried here.
// A task branch origin no longer has, or rewound past commits this machine holds, is not pushed.
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
