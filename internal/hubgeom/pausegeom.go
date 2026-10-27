// pausegeom.go converts a resolved *lymcwd.Location into the geometry every reader of the pause record is told.

package hubgeom

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// PauseGeometry builds the pauserecord.Geometry for l: the hub's pause record and l's pair slug, empty for the prime.
// Failing to tell the prime from a task worktree is the one error, as for ReedGeometry.
func PauseGeometry(l *lymcwd.Location) (pauserecord.Geometry, error) {
	prime, err := isPrimeWorktree(l.WorktreePath())
	if err != nil {
		return pauserecord.Geometry{}, err
	}
	return pauseGeometry(l, prime), nil
}

// pauseGeometry is PauseGeometry once the prime is told from a task worktree, split out so the unit test can tell either without a .git entry.
// The record lives under the board-level .lym every worktree of the hub reaches by path.
func pauseGeometry(l *lymcwd.Location, prime bool) pauserecord.Geometry {
	var slug string
	if !prime {
		slug = l.WorktreeName
	}
	return pauserecord.Geometry{
		RecordPath: filepath.Join(fabricengine.HubScratchDir(l.HubPath), lymdirs.PauseDirName, pauserecord.RecordFileName),
		Slug:       slug,
	}
}
