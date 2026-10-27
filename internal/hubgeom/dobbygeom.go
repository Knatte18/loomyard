// dobbygeom.go tells dobby's engine its paths for one worktree of a hub, and fills the deps that start every dobby this machine owns.

package hubgeom

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// DobbyGeometry tells dobbyengine its paths for the worktree named worktree, the prime's name or a beam's slug.
// The lock, state and log paths and the tidy directory lie under that worktree's `.lym/beam/` directory at the worktree's own anchor, and the batten runs directory under the prime.
// The slug is empty for the prime.
// MoveRecords holds the anchor-move record paths of the worktree's own records worktree and of the prime's, the prime's once when the worktree is the prime.
// A worktree with no directory in the hub is an error naming it, and so is a lym executable that cannot be located or an anchor that cannot be read.
func DobbyGeometry(prime *lymcwd.Location, worktree string) (dobbyengine.Geometry, error) {
	root := filepath.Join(prime.HubPath, worktree)
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return dobbyengine.Geometry{}, fmt.Errorf("hubgeom: the worktree %q has no directory at %s", worktree, root)
	}
	executable, err := os.Executable()
	if err != nil {
		return dobbyengine.Geometry{}, fmt.Errorf("hubgeom: locate the lym executable: %w", err)
	}
	slug := worktree
	anchorRel := prime.AnchorRel
	moveRecords := []string{fabricengine.MoveRecordPath(fabricengine.RecordsWorktreePath(prime, prime.WorktreeName))}
	if worktree == prime.WorktreeName {
		slug = ""
	} else {
		own, err := lymcwd.NamedWorktree(prime.HubPath, worktree)
		if err != nil {
			return dobbyengine.Geometry{}, fmt.Errorf("hubgeom: read the anchor of the worktree %q: %w", worktree, err)
		}
		anchorRel = own.AnchorRel
		moveRecords = append([]string{fabricengine.MoveRecordPath(fabricengine.RecordsWorktreePath(own, worktree))}, moveRecords...)
	}
	beamDir := filepath.Join(root, anchorRel, lymdirs.DotLymDirName, lymdirs.BeamDirName)
	return dobbyengine.Geometry{
		HubRoot:       prime.HubPath,
		BoardDir:      fabricengine.BoardDir(prime.HubPath),
		PrimeAnchor:   prime.AnchorPath(),
		WorktreeRoot:  root,
		WorktreeName:  worktree,
		Slug:          slug,
		LockPath:      filepath.Join(beamDir, dobbyengine.LockFileName),
		StatePath:     filepath.Join(beamDir, dobbyengine.StateFileName),
		LogPath:       filepath.Join(beamDir, dobbyengine.LogFileName),
		TidyDir:       filepath.Join(beamDir, dobbyengine.TidyDirName),
		PrimeRunsDir:  shedrun.RunsRoot(prime),
		LymExecutable: executable,
		MoveRecords:   moveRecords,
	}, nil
}

// PawlPaths tells the pawl producers the land request, dobby-stop marker and await marker paths of the beam whose pair is at beam.
// All three lie directly under the beam worktree's `.lym/beam/` directory at the worktree's own anchor, where DobbyGeometry puts the dobby's files, so the dobby, `lym beam land` and Beam-Await read one directory.
func PawlPaths(beam *lymcwd.Location) pawlengine.Paths {
	beamDir := filepath.Join(beam.AnchorPath(), lymdirs.DotLymDirName, lymdirs.BeamDirName)
	return pawlengine.Paths{
		LandRequest:     filepath.Join(beamDir, pawlengine.LandRequestFileName),
		DobbyStopMarker: filepath.Join(beamDir, pawlengine.DobbyStopMarkerName),
		AwaitMarker:     filepath.Join(beamDir, pawlengine.AwaitMarkerName),
	}
}

// DobbyStart fills dobbyengine.StartDeps over the hub of prime.
// The owners come from the board, the prime owner claim is the one a batten resume makes, and the local beams are the pairs whose origin record carries the beam mark, each with the owner it effectively reads as through ParentOf.
// An anchor change standing on a worktree's own records worktree or on the prime's reads through fabricengine.MoveInProgress, and a landing hold reads from the local board's entry of the worktree.
func DobbyStart(prime *lymcwd.Location) dobbyengine.StartDeps {
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
		MoveInProgress: func(worktree string) (string, bool, error) {
			for _, slug := range moveGateSlugs(prime, worktree) {
				wayForward, standing, err := fabricengine.MoveInProgress(slug, fabricengine.RecordsWorktreePath(prime, slug))
				if err != nil || standing {
					return wayForward, standing, err
				}
			}
			return "", false, nil
		},
		LandingHeld: func(worktree string) (bool, error) { return LandingHeld(prime, worktree) },
		Live:        dobbyengine.Live,
		Spawn:       dobbyengine.SpawnDetached,
	}
}

// LandingHeld reports whether the board entry of the worktree named worktree is held by its run's landing hold, read from the local board.
// The prime is never held, and a worktree with no board entry reads as not held.
func LandingHeld(prime *lymcwd.Location, worktree string) (bool, error) {
	if worktree == prime.WorktreeName {
		return false, nil
	}
	tasks, err := boardengine.ReadTasks(fabricengine.BoardDir(prime.HubPath))
	if err != nil {
		return false, fmt.Errorf("hubgeom: read the board: %w", err)
	}
	for _, task := range tasks {
		if task.Slug == worktree {
			return boardengine.RunHolds(task), nil
		}
	}
	return false, nil
}

// moveGateSlugs returns the worktrees whose anchor change stops the dobby of worktree: worktree itself, then the prime, once when they are the same.
func moveGateSlugs(prime *lymcwd.Location, worktree string) []string {
	if worktree == prime.WorktreeName {
		return []string{worktree}
	}
	return []string{worktree, prime.WorktreeName}
}

// localBeams lists the pairs of the hub present on this machine whose origin record carries the beam mark, each with the owner it effectively reads as.
func localBeams(prime *lymcwd.Location, boardDir string) ([]dobbyengine.BeamPair, error) {
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
