// requiredseams.go installs the seams fabric requires.
// Each one adapts the module that owns run records, run locks or status records.

package fabriccli

import (
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// InstallRequiredSeams installs fabric's run-records paths, run locks and status codec for the whole process.
// Every process that runs fabric on a hub needs them before its first verb.
func InstallRequiredSeams() {
	fabricengine.SetRunRecordPaths(runRecordPaths())
	fabricengine.SetRunLocks(&fabricengine.RunLocks{PrimeRunLock: shedrun.PrimeRunLock})
	fabricengine.SetStatusCodec(&fabricengine.StatusCodec{
		LastMachine: shedengine.StatusLastMachine,
		Recorded:    shedengine.StatusRecorded,
	})
}

// runRecordPaths adapts shed's run directories to fabric's run-records paths.
func runRecordPaths() *fabricengine.RunRecordPaths {
	return &fabricengine.RunRecordPaths{
		RunsRoot:   shedrun.RunsRoot,
		Reserved:   shedrun.IsReserved,
		StatusFile: shedrun.StatusFile,
		TreeStatusRel: func(runID string) (string, error) {
			_, statusRel, err := shedrun.TreeRunFileRels(runID)
			return statusRel, err
		},
	}
}
