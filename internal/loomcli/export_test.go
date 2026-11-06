// export_test.go re-exports the run owner verdict for the package loomcli_test files that drive it directly over a forged hub.

package loomcli

import (
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// DriftDepsAt builds the Drift seams over location with no agent tables.
func DriftDepsAt(location *lymcwd.Location, statusPath, statusLockPath string) driftengine.Deps {
	return driftDeps(location, statusPath, statusLockPath, driftTables{})
}

// LoomRecordsDirs re-exports the records paths the judge snapshot leaves out.
var LoomRecordsDirs = loomRecordsDirs

// CaptureSpawnRequest re-exports the request that starts the detached capture child.
var CaptureSpawnRequest = captureSpawnRequest

// StrandRoleStopper re-exports the seam that ends the chairs of told roles.
var StrandRoleStopper = strandRoleStopper

// OwnerVerdict is the verdict RunOwnerVerdict returns.
type OwnerVerdict = ownerVerdict

// RunOwnerVerdict judges whether another machine owns the run of a slug.
var RunOwnerVerdict = runOwnerVerdict
