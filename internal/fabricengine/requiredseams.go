// requiredseams.go holds fabric's process-wide required seams: where run records sit, the locks a run is held by,
// and the reading of a status record.
// Each is a set of function values in fabric's own vocabulary, supplied from outside fabric.

package fabricengine

import (
	"sync/atomic"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// RunRecordPaths is where the records of runs sit.
type RunRecordPaths struct {
	// RunsRoot returns the directory every run's records sit under in the prime worktree.
	RunsRoot func(prime *lymcwd.Location) string
	// Reserved reports whether runID is a name no run may take.
	Reserved func(runID string) bool
	// StatusFile returns the path of one run's status record in the prime worktree.
	StatusFile func(prime *lymcwd.Location, runID string) string
	// TreeStatusRel returns the slash-separated path of one run's status record in a records tree.
	TreeStatusRel func(runID string) (string, error)
}

// RunLocks is the locks a run is held by.
type RunLocks struct {
	// PrimeRunLock returns the path of the hub's prime run lock.
	PrimeRunLock func(l *lymcwd.Location) string
}

// StatusCodec reads a run's status record from its bytes.
type StatusCodec struct {
	// LastMachine returns the machine the record's last history entry names, or "" when it has none.
	LastMachine func(data []byte) (string, error)
	// Recorded reports whether data is a run status record.
	Recorded func(data []byte) (bool, error)
}

var (
	installedRunRecordPaths atomic.Pointer[RunRecordPaths]
	installedRunLocks       atomic.Pointer[RunLocks]
	installedStatusCodec    atomic.Pointer[StatusCodec]
)

// SetRunRecordPaths stores paths as the process's run-records paths.
// It returns a function that puts back the value before.
// A nil argument unsets the seam.
func SetRunRecordPaths(paths *RunRecordPaths) (restore func()) {
	previous := installedRunRecordPaths.Swap(paths)
	return func() { installedRunRecordPaths.Store(previous) }
}

// SetRunLocks stores locks as the process's run locks and returns a function that puts back the value before.
// A nil argument unsets the seam.
func SetRunLocks(locks *RunLocks) (restore func()) {
	previous := installedRunLocks.Swap(locks)
	return func() { installedRunLocks.Store(previous) }
}

// SetStatusCodec stores codec as the process's status codec and returns a function that puts back the value before.
// A nil argument unsets the seam.
func SetStatusCodec(codec *StatusCodec) (restore func()) {
	previous := installedStatusCodec.Swap(codec)
	return func() { installedStatusCodec.Store(previous) }
}

// refuseUnsuppliedSeams returns the refusal of a run that lacks one of the needed seams, or nil.
// A verb that takes no snapshot of its own runs it before it reads or acts.
func refuseUnsuppliedSeams(needed ...fabriccore.RequiredSeam) error {
	var snapshot fabriccore.Snapshot
	if err := (snapshotReader{}).read(fabriccore.PhasePreLock, []fabriccore.Field{fabriccore.FieldSeams}, &snapshot); err != nil {
		return err
	}
	if refusal := fabriccore.RefuseUnsupplied(snapshot, needed...); refusal != nil {
		return refusal
	}
	return nil
}

// suppliedSeams reports, for each required seam, whether it is installed.
func suppliedSeams() map[fabriccore.RequiredSeam]bool {
	return map[fabriccore.RequiredSeam]bool{
		fabriccore.SeamRunRecordPaths: installedRunRecordPaths.Load() != nil,
		fabriccore.SeamRunLocks:       installedRunLocks.Load() != nil,
		fabriccore.SeamStatusCodec:    installedStatusCodec.Load() != nil,
	}
}
