// ownerpause.go holds what `start` does about machine ownership: the process's owner pause seam and the claim of the hub's prime for this machine.

package orchcli

import (
	"fmt"
	"slices"
	"sync"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
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
