// ownerpause.go holds what the bring-up does about machine ownership: the process's owner pause seam, the claim of the hub's prime for this machine and the read of the facts its refusals judge.

package orchcli

import (
	"fmt"
	"slices"
	"sync"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
)

var (
	ownerPauseMu sync.Mutex
	ownerPause   func(location *lymcwd.Location, slug string) error
)

// InstallOwnerPause stores the process's pause of a run owned by another machine, which a bring-up calls for each such entry of its reconcile.
// The root pre-run installs it, since the pause lives in a package that imports this one.
func InstallOwnerPause(pause func(location *lymcwd.Location, slug string) error) {
	ownerPauseMu.Lock()
	defer ownerPauseMu.Unlock()
	ownerPause = pause
}

// installedOwnerPause returns the pause InstallOwnerPause stored, nil when none was.
func installedOwnerPause() func(location *lymcwd.Location, slug string) error {
	ownerPauseMu.Lock()
	defer ownerPauseMu.Unlock()
	return ownerPause
}

// readBeamFacts reads what the bring-up's refusals judge for the worktree at location, from the local Bolt and the move records.
// The owner of a beam is the one hubgeom.ParentOf's chain gives its entry, read from the prime the beam belongs to, and the prime's owner is the board's prime owner.
// A standing anchor change reads as the dobby start reads it, on the worktree or on the prime, and only a beam's entry is held by a run.
func readBeamFacts(location *lymcwd.Location, beam bool) (beamFacts, error) {
	prime, err := resolvePrime(location)
	if err != nil {
		return beamFacts{}, err
	}
	boardDir := fabricengine.BoardDir(location.HubPath)
	owners, err := boardengine.ReadOwners(boardDir)
	if err != nil {
		return beamFacts{}, err
	}
	here, err := machinename.Name()
	if err != nil {
		return beamFacts{}, err
	}
	facts := beamFacts{here: here, owner: owners.Prime}
	if beam {
		slug := location.WorktreeName
		facts.slug = slug
		facts.owner, err = owners.Effective(slug, func(entry string) (string, error) { return hubgeom.ParentOf(prime, entry) })
		if err != nil {
			return beamFacts{}, err
		}
		tasks, err := boardengine.ReadTasks(boardDir)
		if err != nil {
			return beamFacts{}, err
		}
		for _, task := range tasks {
			if task.Slug != slug {
				continue
			}
			facts.held = boardengine.RunHolds(task)
			if task.Status != nil {
				facts.status = *task.Status
			}
			break
		}
	}
	wayForward, standing, err := hubgeom.DobbyStart(prime).MoveInProgress(location.WorktreeName)
	if err != nil {
		return beamFacts{}, fmt.Errorf("read whether an anchor change stands: %w", err)
	}
	if standing {
		facts.moveWayForward = wayForward
	}
	return facts, nil
}

// claimPrimeFor records this machine as the owner of the hub's prime, writing only when the hub's board records no prime owner.
// claimed is this machine's name after a claim that stands, lostTo the other machine's name when a competing claim won, and both are empty when the board already records a prime owner.
func claimPrimeFor(location *lymcwd.Location) (claimed, lostTo string, err error) {
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
