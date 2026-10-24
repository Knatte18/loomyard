// ownerpause.go holds what `start` does about machine ownership: the process's owner pause seam, the claim of the hub's prime for this machine and the owner check of a beam.

package orchcli

import (
	"fmt"
	"slices"
	"sync"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
)

var (
	ownerPauseMu sync.Mutex
	ownerPause   func(location *lyxcwd.Location, slug string) error
)

// InstallOwnerPause stores the process's pause of a run owned by another machine, which `start` calls for each such entry of its reconcile.
// The root pre-run installs it, since the pause lives in a package that imports this one.
func InstallOwnerPause(pause func(location *lyxcwd.Location, slug string) error) {
	ownerPauseMu.Lock()
	defer ownerPauseMu.Unlock()
	ownerPause = pause
}

// installedOwnerPause returns the pause InstallOwnerPause stored, nil when none was.
func installedOwnerPause() func(location *lyxcwd.Location, slug string) error {
	ownerPauseMu.Lock()
	defer ownerPauseMu.Unlock()
	return ownerPause
}

// checkBeamStart judges whether an orch may start in the beam at location: the beam's owner by the board's owners and its entry's status.
// The owner is the one hubgeom.ParentOf's chain gives the beam's entry, read from the prime the beam belongs to.
func checkBeamStart(location *lyxcwd.Location) error {
	prime, err := resolvePrime(location)
	if err != nil {
		return err
	}
	boardDir := fabricengine.BoardDir(location.HubPath)
	owners, err := boardengine.ReadOwners(boardDir)
	if err != nil {
		return err
	}
	tasks, err := boardengine.ReadTasks(boardDir)
	if err != nil {
		return err
	}
	here, err := machinename.Name()
	if err != nil {
		return err
	}
	slug := location.WorktreeName
	owner, err := owners.Effective(slug, func(entry string) (string, error) { return hubgeom.ParentOf(prime, entry) })
	if err != nil {
		return err
	}
	var status *string
	for _, task := range tasks {
		if task.Slug == slug {
			status = task.Status
			break
		}
	}
	return refuseBeamStart(slug, here, owner, status)
}

// claimPrimeFor records this machine as the owner of the hub's prime, writing only when the hub's board records no prime owner.
// claimed is this machine's name after a claim that stands, lostTo the other machine's name when a competing claim won, and both are empty when the board already records a prime owner.
func claimPrimeFor(location *lyxcwd.Location) (claimed, lostTo string, err error) {
	boardDir := fabricengine.BoardDir(location.HubPath)
	owners, err := boardengine.ReadOwners(boardDir)
	if err != nil {
		return "", "", err
	}
	if owners.Prime != "" {
		return "", "", nil
	}
	machine, err := machinename.Name()
	if err != nil {
		return "", "", err
	}
	write, err := boardengine.SetOwners(boardDir, boardengine.OwnerRequest{Machine: machine, Prime: true})
	if err != nil {
		return "", "", fmt.Errorf("claim the prime for machine %s: %w", machine, err)
	}
	if other := write.Lost[boardengine.OwnerWritePrime]; other != "" {
		return "", other, nil
	}
	if slices.Contains(write.Recorded, boardengine.OwnerWritePrime) {
		return machine, "", nil
	}
	return "", "", nil
}
