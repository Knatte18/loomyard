// live.go declares what other modules read of the hub's orchs: HubOrchs, the worktrees an orch may run in, and OrchLive, the liveness predicate of one worktree's orch, the same reading `lyx orch status` makes of the strand.

package orchcli

import (
	"errors"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// HubOrchs lists the worktrees of the hub that location belongs to in which an orch may run: the prime's location first, then each local pair whose origin record carries the beam mark.
// A pair whose origin record cannot be read is skipped with a warning, since a reader that must keep working under a pause cannot stop on one half-present pair.
func HubOrchs(location *lyxcwd.Location) ([]*lyxcwd.Location, error) {
	prime, err := resolvePrime(location)
	if err != nil {
		return nil, err
	}
	worktrees, err := fabricengine.List(prime.WorktreePath())
	if err != nil {
		return nil, err
	}
	orchs := []*lyxcwd.Location{prime}
	for _, worktree := range worktrees {
		if worktree.Main || worktree.Prunable {
			continue
		}
		pair, err := lyxcwd.ResolveWorktree(worktree.Path)
		if err != nil {
			logger.Warn("orch: skipping a worktree that does not resolve while listing the hub's orchs", "worktree", worktree.Path, "error", err)
			continue
		}
		origin, found, err := fabricengine.ReadOrigin(pair)
		if err != nil {
			logger.Warn("orch: skipping a pair whose origin record cannot be read while listing the hub's orchs", "worktree", pair.WorktreeName, "error", err)
			continue
		}
		if found && origin.Beam {
			orchs = append(orchs, pair)
		}
	}
	return orchs, nil
}

// OrchLive reports whether the orch of worktree is live: its orch state records a strand that reed tracks and reports live.
// A worktree whose orch never started, whose reed session is gone, or whose recorded strand reed no longer tracks, is not live.
func OrchLive(worktree *lyxcwd.Location) (bool, error) {
	st, err := orchengine.LoadState(orchPaths(worktree))
	if err != nil {
		return false, err
	}
	reedCfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(worktree), "reed")
	if err != nil {
		return false, err
	}
	reedGeom, err := hubgeom.ReedGeometry(worktree)
	if err != nil {
		return false, err
	}
	reed := reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub())
	return orchLiveFrom(st, newReedStrandOps(reed).Strands)
}

// orchLiveFrom reports whether st records a strand that strands lists as live.
// strands is read only when st records a strand, so a never-started orch costs no reed call.
func orchLiveFrom(st orchengine.State, strands func() ([]reedengine.StrandStatus, error)) (bool, error) {
	if st.Strand == "" {
		return false, nil
	}
	listed, err := strands()
	if errors.Is(err, reedengine.ErrNoSession) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	strand, tracked := trackedStrand(listed, st.Strand)
	return tracked && strand.Live, nil
}
