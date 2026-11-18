// accountpick.go picks a Claude account for a worktree that was just added to a hub.

package battencli

import (
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// PickWorktreeAccount picks the Claude account of the worktree slug, which the hub of prime holds.
// With keepEntry set, a worktree that already holds an account keeps it.
// The pick writes only machine-local state, so a failure is logged at Warn and never fails the caller:
// a launch reports a bad config itself.
func PickWorktreeAccount(prime *lymcwd.Location, slug string, keepEntry bool) {
	pair, err := lymcwd.NamedWorktree(prime.HubPath, slug)
	if err != nil {
		logger.Warn("battencli: locate the worktree to pick its Claude account failed", "slug", slug, "error", err)
		return
	}
	keeper, ok := shuttleengines.AccountKeeper(fabricengine.ConfigDirs(pair), hubgeom.EngineWorktree(pair))
	if !ok {
		return
	}
	if err := keeper.PickAccount(keepEntry); err != nil {
		logger.Warn("battencli: picking the Claude account of the worktree failed", "slug", slug, "error", err)
	}
}
