// stencilpins.go declares a run's stencil pins record:
// the body hash of each stencil the run may render, as installed when the run was first seeded.

package shedrun

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

// stencilPinsFileName is the filename of the run's stencil pins record, whose path StencilPinsFile returns.
const stencilPinsFileName = "stencil-pins.json"

// stencilPinsLockSuffix ends the name of the lock guarding the stencil pins record.
// lock.TrackedFileLockPath places that lock outside the tracked run directory.
const stencilPinsLockSuffix = ".lock"

// StencilPins records the stencils a run may render, written once when the run is first seeded.
// Stencils maps each stencil's name to the body hash its installed board copy had then.
type StencilPins struct {
	Stencils map[string]string `json:"stencils"`
}

// StencilPinsFile returns the path to the run's durable stencil pins record, under RunDir(l, runID).
func StencilPinsFile(l *lymcwd.Location, runID string) string {
	return filepath.Join(RunDir(l, runID), stencilPinsFileName)
}

// StencilPinsRel returns the worktree-anchor-relative form of StencilPinsFile's path.
// It is the pathspec a commit of the record takes.
func StencilPinsRel(l *lymcwd.Location, runID string) string {
	return filepath.Join(lymdirs.LymDirName, shedDirName, runSegment(l, runID), stencilPinsFileName)
}

// WriteStencilPins writes pins as the run's stencil pins record through internal/state.
func WriteStencilPins(l *lymcwd.Location, runID string, pins StencilPins) error {
	path := StencilPinsFile(l, runID)
	if err := state.WriteJSON(path, lock.TrackedFileLockPath(path, stencilPinsLockSuffix), pins); err != nil {
		return fmt.Errorf("shedrun: write stencil pins %s: %w", path, err)
	}
	return nil
}

// ReadStencilPins reads the run's stencil pins record, found false when the run has none.
// A key the record does not declare is accepted.
func ReadStencilPins(l *lymcwd.Location, runID string) (StencilPins, bool, error) {
	path := StencilPinsFile(l, runID)
	return state.ReadJSON[StencilPins](path, lock.TrackedFileLockPath(path, stencilPinsLockSuffix))
}
