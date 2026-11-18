// ownerpause.go holds what the bring-up does about machine ownership:
// the claim of the hub's prime for this machine, and the read of the facts its refusals judge.

package orchcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// readBeamFacts reads the facts the bring-up's refusals judge for the worktree at location.
// It reads them from the local Bolt and the move records.
// The owner of a beam is the effective owner of its slug, resolved up the parent chain of its prime.
// The prime's owner is the board's prime owner.
// A standing anchor change is read from the prime's move in progress for the worktree's name.
// Only a beam's entry is held by a run.
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
	here, err := machinename.ReadIdentity()
	if err != nil {
		return beamFacts{}, err
	}
	facts := beamFacts{here: here.Name, owner: owners.Prime}
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
			if facts.held {
				run, found, err := readOwnRunStatus(location)
				if err != nil {
					return beamFacts{}, fmt.Errorf("read the beam's own run status: %w", err)
				}
				facts.ownLanding = ownLandingHold(task.Status, run, found)
			}
			break
		}
	}
	facts.elsewhere = boardengine.OwnedElsewhere(facts.owner, owners.Registry, here)
	wayForward, standing, err := hubgeom.DobbyStart(prime).MoveInProgress(location.WorktreeName)
	if err != nil {
		return beamFacts{}, fmt.Errorf("read whether an anchor change stands: %w", err)
	}
	if standing {
		facts.moveWayForward = wayForward
	}
	return facts, nil
}

// readOwnRunStatus reads the status of the pair's own run, the one its worktree's slug names.
// found is false when the pair has no such run.
func readOwnRunStatus(location *lymcwd.Location) (shedengine.Status, bool, error) {
	statusPath := shedrun.StatusFile(location, shedrun.SelfRunID)
	if _, err := os.Stat(statusPath); errors.Is(err, fs.ErrNotExist) {
		return shedengine.Status{}, false, nil
	}
	lockPath := shedrun.StatusLock(location, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return shedengine.Status{}, false, err
	}
	return state.ReadJSON[shedengine.Status](statusPath, lockPath)
}

// claimPrimeFor records this machine as the owner of the hub's prime.
// It writes only when the hub's board records no prime owner.
// claimed is this machine's name after a claim that stands;
// lostTo is the other machine's name when a competing claim won.
// Both are empty when the board already records a prime owner.
func claimPrimeFor(location *lymcwd.Location) (claimed, lostTo string, err error) {
	boardDir := fabricengine.BoardDir(location.HubPath)
	owners, err := boardengine.ReadOwners(boardDir)
	if err != nil {
		return "", "", err
	}
	if owners.Prime != "" {
		return "", "", nil
	}
	identity, err := boardengine.EnsureIdentity()
	if err != nil {
		return "", "", err
	}
	machine := identity.Name
	write, err := boardengine.SetOwners(boardDir, boardengine.OwnerRequest{Machine: machine, Identity: identity, Prime: true})
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
