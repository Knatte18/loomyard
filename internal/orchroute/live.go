// live.go declares the liveness predicate of one worktree's orch, the same reading `lym orch status` makes of the strand.

package orchroute

import (
	"errors"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// Live reports whether the orch of worktree is live: its orch state records a strand that reed tracks and reports live.
// A worktree whose orch never started, whose reed session is gone, or whose recorded strand reed no longer tracks, is not live.
func Live(worktree *lymcwd.Location) (bool, error) {
	st, err := orchengine.LoadState(Paths(worktree))
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
	return liveFrom(st, func() ([]reedengine.StrandStatus, error) {
		result, err := reed.Status()
		if err != nil {
			return nil, err
		}
		return result.Strands, nil
	})
}

// liveFrom reports whether st records a strand that strands lists as live.
// strands is read only when st records a strand, so a never-started orch costs no reed call.
func liveFrom(st orchengine.State, strands func() ([]reedengine.StrandStatus, error)) (bool, error) {
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
	for _, strand := range listed {
		if strand.GUID == st.Strand {
			return strand.Live, nil
		}
	}
	return false, nil
}
