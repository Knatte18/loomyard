// spawn.go — the engine-level detached both-sides push spawn helper and its synchronous warp-push
// counterparts.
// SpawnDetachedPush mirrors boardengine.spawnSync and the pre-consolidation
// internal/fabriccli.spawnPush: it launches a detached `lyx fabric --warp-path <abs> --weft-path
// <abs> push` child (either flag omitted when its path is empty) that re-enters the fabric CLI's
// bypass mode and pushes whichever side(s) were supplied.
// PushWarpAt is the warp-side sibling of weftgit.go's pushWeftAt — the synchronous, no-Fabric-
// instance push primitive for the warp side.
// It has no production caller today: the detached child's bypass handler (internal/fabriccli's
// `push` RunE) drives CoalescePushBothAt instead, which pushes both sides through PushRebaseFree
// under one absorbing lock rather than calling either per-side primitive.
// It is retained as pushWeftAt's symmetric counterpart, and the distinction matters — see PushWarpAt's
// own doc comment for the operational consequence.
// PushWarpRebaseFreeAt is a second warp-side push primitive, routing to gitrepo.PushRebaseFree
// instead of PushCoalesced — see its own doc comment for why it exists beside PushWarpAt rather than
// replacing it.

package fabricengine

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
)

// SpawnDetachedPush launches a detached, windowless `lyx fabric --warp-path <abs> --weft-path <abs>
// push` child that pushes both repos supplied to it — the async-push-both-sides-via-detached-child
// Shared Decision's spawn point.
// Either flag is omitted from the child's args when its corresponding path is empty;
// the caller may pass one or both paths.
// Returns nil immediately, forking no child, when WEFT_SKIP_GIT or WEFT_SKIP_PUSH is set (skip-env
// gating is helper-internal, matching the pre-consolidation fabriccli.spawnPush) or when both paths
// are empty — there is nothing to push.
// The child is started but never Waited,
// and its stdin/stdout/stderr are left nil so no handle is inherited from the parent.
func SpawnDetachedPush(warpPath, weftPath string) error {
	if os.Getenv("WEFT_SKIP_GIT") == "1" || os.Getenv("WEFT_SKIP_PUSH") == "1" {
		return nil
	}
	if warpPath == "" && weftPath == "" {
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	args := []string{"fabric"}
	if warpPath != "" {
		abs, err := filepath.Abs(warpPath)
		if err != nil {
			return err
		}
		args = append(args, "--warp-path", abs)
	}
	if weftPath != "" {
		abs, err := filepath.Abs(weftPath)
		if err != nil {
			return err
		}
		args = append(args, "--weft-path", abs)
	}
	args = append(args, "push")

	cmd := exec.Command(exe, args...)
	proc.Detach(cmd)
	// Leave stdin/stdout/stderr nil so no handles are inherited from the parent.
	if err := cmd.Start(); err != nil {
		logger.Warn("fabricengine: spawn detached push failed", "exe", exe, "args", args, "error", err)
		return err
	}
	logger.Info("fabricengine: spawned detached push", "exe", exe, "args", args, "pid", cmd.Process.Pid)
	return nil // intentionally not Wait()ed
}

// PushWarpAt pushes unpushed commits at warpPath directly, with no Fabric instance and no weft path
// involved — the warp-side analog of weftgit.go's pushWeftAt.
// Gating matches pushWeftAt exactly.
//
// It has NO production caller. The detached push child's bypass handler drives CoalescePushBothAt,
// not this function, so nothing in shipped fabric ever runs gitrepo.PushCoalesced against a warp
// worktree.
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
	recordPushIfAdvanced(rec, repo, hadUnpushed, hadUnpushedErr)

	return PushResult{}, nil
}

// PushWarpRebaseFreeAt pushes unpushed commits at warpPath directly via gitrepo.PushRebaseFree, with
// no Fabric instance and no weft path involved — a second warp-side push primitive beside PushWarpAt,
// not a replacement for it.
//
// It exists to discharge two hazards PushWarpAt's own doc comment names rather than merely mitigate:
// PushRebaseFree never runs `git pull --rebase`, so it never rewrites this side's SHAs while the
// paired side is not rebased and never invalidates the correspondence index the way PushWarpAt's
// route through gitrepo.PushCoalesced → pushWithRebaseRetry can on a rejected push; and it never
// takes the push lock, so it leaves no untracked .gitrepo-push.lock residue in the operator's own
// warp repo — the undischarged precondition PushWarpAt's own doc comment names (the warp repo has no
// exclude entry for that file, unlike weft).
//
// A rejected push surfaces as gitrepo.ErrPushRejected, which the caller is expected to treat as a
// human-decidable condition rather than retrying.
func PushWarpRebaseFreeAt(warpPath string, opts SyncOptions) (res PushResult, err error) {
	rec := NewMutations(filepath.Dir(warpPath))
	defer func() { res.Mutations = rec.Snapshot() }()

	if opts.SkipGit || opts.SkipPush {
		return PushResult{}, nil
	}

	repo := gitrepo.New(warpPath)
	hadUnpushed, hadUnpushedErr := repo.HasUnpushed()
	if err := repo.PushRebaseFree(); err != nil {
		return PushResult{}, err
	}
	recordPushIfAdvanced(rec, repo, hadUnpushed, hadUnpushedErr)

	return PushResult{}, nil
}
