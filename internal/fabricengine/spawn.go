// spawn.go — the engine-level detached both-sides push spawn helper and its synchronous warp-push
// counterpart.
// SpawnDetachedPush mirrors boardengine.spawnSync and the pre-consolidation
// internal/fabriccli.spawnPush: it launches a detached `lyx fabric --warp-path <abs> --weft-path
// <abs> push` child (either flag omitted when its path is empty) that re-enters the fabric CLI's
// bypass mode and pushes whichever side(s) were supplied.
// PushWarpAt is the warp-side sibling of weftgit.go's PushWeftAt — the synchronous, no-Fabric-
// instance push primitive the detached child's bypass handler calls for the warp side.

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
	return nil // intentionally not Wait()ed
}

// PushWarpAt pushes unpushed commits at warpPath directly, with no Fabric instance and no weft path
// involved — the warp-side analog of weftgit.go's PushWeftAt, and the detached push child's
// bypass-push entry point for the warp side (see fabriccli's --warp-path bypass flag).
// Gating matches PushWeftAt exactly.
func PushWarpAt(warpPath string, opts SyncOptions) error {
	if opts.SkipGit || opts.SkipPush {
		return nil
	}
	return gitrepo.New(warpPath).PushCoalesced()
}
