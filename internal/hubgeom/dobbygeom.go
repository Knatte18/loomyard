// dobbygeom.go tells dobby's engine its paths for one worktree of a hub, and fills the deps that start every dobby this machine owns.

package hubgeom

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// DobbyGeometry tells dobbyengine its paths for the worktree named worktree, the prime's name or a beam's slug.
// The lock, state and marker paths lie under that worktree's `.lyx/beam/` directory at the hub's anchor, and the batten runs directory under the prime.
// The slug is empty for the prime.
// A worktree with no directory in the hub is an error naming it, and so is a lyx executable that cannot be located.
func DobbyGeometry(prime *lyxcwd.Location, worktree string) (dobbyengine.Geometry, error) {
	root := filepath.Join(prime.HubPath, worktree)
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return dobbyengine.Geometry{}, fmt.Errorf("hubgeom: the worktree %q has no directory at %s", worktree, root)
	}
	executable, err := os.Executable()
	if err != nil {
		return dobbyengine.Geometry{}, fmt.Errorf("hubgeom: locate the lyx executable: %w", err)
	}
	beamDir := filepath.Join(root, prime.AnchorRel, lyxdirs.DotLyxDirName, lyxdirs.BeamDirName)
	slug := worktree
	if worktree == prime.WorktreeName {
		slug = ""
	}
	return dobbyengine.Geometry{
		HubRoot:       prime.HubPath,
		BoardDir:      fabricengine.BoardDir(prime.HubPath),
		PrimeAnchor:   prime.AnchorPath(),
		WorktreeRoot:  root,
		WorktreeName:  worktree,
		Slug:          slug,
		LockPath:      filepath.Join(beamDir, dobbyengine.LockFileName),
		StatePath:     filepath.Join(beamDir, dobbyengine.StateFileName),
		MarkerDir:     filepath.Join(beamDir, dobbyengine.MarkerDirName),
		PrimeRunsDir:  shedrun.RunsRoot(prime),
		LyxExecutable: executable,
	}, nil
}

// DobbyStart fills dobbyengine.StartDeps over the hub of prime.
// The owners come from the board, the prime owner claim is the one a batten resume makes, and the local beams are the pairs whose origin record carries the beam mark, each with the owner it effectively reads as through ParentOf.
func DobbyStart(prime *lyxcwd.Location) dobbyengine.StartDeps {
	boardDir := fabricengine.BoardDir(prime.HubPath)
	return dobbyengine.StartDeps{
		Machine: machinename.Name,
		PrimeOwner: func() (string, error) {
			owners, err := boardengine.ReadOwners(boardDir)
			return owners.Prime, err
		},
		ClaimPrime: func(machine string) (string, error) {
			write, err := boardengine.SetOwners(boardDir, boardengine.OwnerRequest{Machine: machine, Prime: true})
			if err != nil {
				return "", err
			}
			return write.Lost[boardengine.OwnerWritePrime], nil
		},
		PrimeName: prime.WorktreeName,
		Beams:     func() ([]dobbyengine.BeamPair, error) { return localBeams(prime, boardDir) },
		Geometry:  func(worktree string) (dobbyengine.Geometry, error) { return DobbyGeometry(prime, worktree) },
		Live:      dobbyengine.Live,
		Spawn:     dobbyengine.SpawnDetached,
	}
}

// localBeams lists the pairs of the hub present on this machine whose origin record carries the beam mark, each with the owner it effectively reads as.
func localBeams(prime *lyxcwd.Location, boardDir string) ([]dobbyengine.BeamPair, error) {
	entries, err := fabricengine.List(prime.AnchorPath())
	if err != nil {
		return nil, err
	}
	owners, err := boardengine.ReadOwners(boardDir)
	if err != nil {
		return nil, err
	}
	parentOf := func(slug string) (string, error) { return ParentOf(prime, slug) }
	var beams []dobbyengine.BeamPair
	for _, entry := range entries {
		if entry.Main || entry.Prunable {
			continue
		}
		slug := filepath.Base(filepath.Clean(filepath.FromSlash(entry.Path)))
		origin, found, err := fabricengine.ReadOriginFor(prime, slug)
		if err != nil {
			return nil, fmt.Errorf("hubgeom: read origin record for %q: %w", slug, err)
		}
		if !found || !origin.Beam {
			continue
		}
		owner, err := owners.Effective(slug, parentOf)
		if err != nil {
			return nil, fmt.Errorf("hubgeom: resolve the owner of %q: %w", slug, err)
		}
		beams = append(beams, dobbyengine.BeamPair{Slug: slug, Owner: owner})
	}
	return beams, nil
}
