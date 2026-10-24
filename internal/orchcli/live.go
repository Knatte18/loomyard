// live.go declares OrchLive, the liveness predicate of a worktree's orch that other modules call: the same reading `lyx orch status` makes of the strand.

package orchcli

import (
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// OrchLive reports whether the orch of worktree is live: its orch state records a strand that reed tracks and reports live.
// A worktree whose orch never started, or whose recorded strand reed no longer tracks, is not live.
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
	if err != nil {
		return false, err
	}
	strand, tracked := trackedStrand(listed, st.Strand)
	return tracked && strand.Live, nil
}
