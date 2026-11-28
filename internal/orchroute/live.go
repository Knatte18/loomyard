// live.go declares the liveness reading of one worktree's orch, the same reading `lym orch status` makes of the strand.
// A reading that is not live names its cause.

package orchroute

import (
	"errors"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
)

// The causes an orch reads as down for that lie outside its strand's own reed reading.
const (
	// DownNoStrandRecorded is an orch whose state records no strand, one that never started.
	DownNoStrandRecorded = "no-strand-recorded"
	// DownNoReedSession is an orch whose worktree has no reed session.
	DownNoReedSession = "no-reed-session"
	// DownStrandNotInReedState is an orch whose state records a strand that reed's state no longer holds.
	DownStrandNotInReedState = "strand-not-in-reed-state"
)

// downReadingLogMessage is the log line of every down reading, the one text a search of the log needs.
const downReadingLogMessage = "orch: the orch reads as down"

// Reading is one reading of a worktree's orch: whether it is live, and why not when it reads as down.
type Reading struct {
	Live bool
	// Down names why the orch reads as down, empty for a live orch.
	// It is one of this package's down causes or the reed down cause of the orch's strand.
	Down string
}

// Live reports whether the orch of worktree is live: its orch state records a strand that reed tracks and reports live.
// A worktree whose orch never started, whose reed session is gone, or whose recorded strand reed no longer tracks, is not live.
// Every down reading is logged with its cause, at Debug for an orch that never started and at Info otherwise.
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
	reading, err := readingFrom(st, func() ([]reedengine.StrandStatus, error) {
		result, err := reed.Status()
		if err != nil {
			return nil, err
		}
		return result.Strands, nil
	})
	if err != nil {
		return false, err
	}
	switch {
	case reading.Live:
	case reading.Down == DownNoStrandRecorded:
		logger.Debug(downReadingLogMessage, "worktree", worktree.WorktreeName, "cause", reading.Down)
	default:
		logger.Info(downReadingLogMessage, "worktree", worktree.WorktreeName, "strandGUID", st.Strand, "cause", reading.Down)
	}
	return reading.Live, nil
}

// ReadingOf reads the orch whose state is st against strands, the strands reed lists for its worktree.
func ReadingOf(st orchengine.State, strands []reedengine.StrandStatus) Reading {
	if st.Strand == "" {
		return Reading{Down: DownNoStrandRecorded}
	}
	for _, strand := range strands {
		if strand.GUID == st.Strand {
			return Reading{Live: strand.Live, Down: string(strand.Down)}
		}
	}
	return Reading{Down: DownStrandNotInReedState}
}

// readingFrom reads the orch whose state is st against the strands that strands lists.
// strands is read only when st records a strand, so a never-started orch costs no reed call.
func readingFrom(st orchengine.State, strands func() ([]reedengine.StrandStatus, error)) (Reading, error) {
	if st.Strand == "" {
		return ReadingOf(st, nil), nil
	}
	listed, err := strands()
	if errors.Is(err, reedengine.ErrNoSession) {
		return Reading{Down: DownNoReedSession}, nil
	}
	if err != nil {
		return Reading{}, err
	}
	return ReadingOf(st, listed), nil
}
