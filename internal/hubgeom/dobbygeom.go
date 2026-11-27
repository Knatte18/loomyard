// dobbygeom.go tells dobby's engine its paths for one worktree of a hub.
// It also fills the deps that start every dobby this machine owns.

package hubgeom

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// DobbyGeometry tells dobbyengine its paths for the worktree named worktree, the prime's name or a beam's slug.
// The lock, state, log, output and tidy paths lie in the worktree's `.lym/beam/` directory.
// That directory is at the worktree's own anchor, and the batten runs directory lies under the prime.
// The slug is empty for the prime.
// MoveRecords holds the anchor-move record paths of the worktree's own records worktree and of the prime's.
// The prime's path appears once when the worktree is the prime.
// A worktree with no directory in the hub is an error naming it.
// So is a lym executable that cannot be located or an anchor that cannot be read.
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
	moveRecords := []string{fabriclayout.MoveRecordPath(fabricengine.RecordsWorktreePath(prime, prime.WorktreeName))}
	own := prime
	if worktree == prime.WorktreeName {
		slug = ""
	} else {
		var err error
		own, err = lymcwd.NamedWorktree(prime.HubPath, worktree)
		if err != nil {
			return dobbyengine.Geometry{}, fmt.Errorf("hubgeom: read the anchor of the worktree %q: %w", worktree, err)
		}
		moveRecords = append([]string{fabriclayout.MoveRecordPath(fabricengine.RecordsWorktreePath(own, worktree))}, moveRecords...)
	}
	beamDir := DobbyBeamDir(own)
	return dobbyengine.Geometry{
		HubRoot:       prime.HubPath,
		BoardDir:      fabriclayout.BoardDir(prime.HubPath),
		PrimeAnchor:   prime.AnchorPath(),
		WorktreeRoot:  root,
		WorktreeName:  worktree,
		Slug:          slug,
		LockPath:      filepath.Join(beamDir, dobbyengine.LockFileName),
		StatePath:     filepath.Join(beamDir, dobbyengine.StateFileName),
		LogPath:       filepath.Join(beamDir, dobbyengine.LogFileName),
		OutPath:       filepath.Join(beamDir, dobbyengine.OutFileName),
		PrimeLogPath:  filepath.Join(DobbyBeamDir(prime), dobbyengine.LogFileName),
		TidyDir:       filepath.Join(beamDir, dobbyengine.TidyDirName),
		DescribeDir:   filepath.Join(beamDir, dobbyengine.DescribeDirName),
		PrimeRunsDir:  shedrun.RunsRoot(prime),
		LymExecutable: executable,
		MoveRecords:   moveRecords,
	}, nil
}

// DobbyBeamDir returns the `.lym/beam/` directory of worktree, at the worktree's own anchor.
// The dobby keeps its lock, state, log and output files there.
func DobbyBeamDir(worktree *lymcwd.Location) string {
	return filepath.Join(worktree.HubPath, worktree.WorktreeName, worktree.AnchorRel, lymdirs.DotLymDirName, lymdirs.BeamDirName)
}

// PawlPaths returns the paths the pawl producers use for the beam whose pair is at beam.
// They are the land request, dobby-stop, await, hand-off and drain markers.
// They all lie directly in the `.lym/beam/` directory of the beam worktree, at its own anchor.
// That is the directory the dobby's files use, so the dobby, `lym beam land` and Beam-Await read one directory.
func PawlPaths(beam *lymcwd.Location) pawlengine.Paths {
	beamDir := filepath.Join(beam.AnchorPath(), lymdirs.DotLymDirName, lymdirs.BeamDirName)
	return pawlengine.Paths{
		LandRequest:     filepath.Join(beamDir, pawlengine.LandRequestFileName),
		DobbyStopMarker: filepath.Join(beamDir, pawlengine.DobbyStopMarkerName),
		AwaitMarker:     filepath.Join(beamDir, pawlengine.AwaitMarkerName),
		HandOffMarker:   filepath.Join(beamDir, pawlengine.HandOffMarkerName),
		DrainMarker:     filepath.Join(beamDir, pawlengine.DrainMarkerName),
	}
}

// DobbyStart fills dobbyengine.StartDeps over the hub of prime.
// The owners come from the board.
// Each is judged through the board's machine registry against this machine's identity.
// The Machine seam returns the name of the identity `boardengine.EnsureIdentity` returns, which writes the id once;
// its error is the refusal of a machine whose config directory resolves from no variable.
// The prime owner claim is the one a batten resume makes.
// The local beams are the pairs whose origin record carries the beam mark.
// Each carries the owner it effectively reads as through its parent, and whether that owner reads as another machine.
// The move check reads an anchor change standing on a worktree's own records worktree or on the prime's.
// A landing hold is read from the local board's entry of the worktree.
func DobbyStart(prime *lymcwd.Location) dobbyengine.StartDeps {
	boardDir := fabriclayout.BoardDir(prime.HubPath)
	return dobbyengine.StartDeps{
		Machine: func() (string, error) {
			identity, err := boardengine.EnsureIdentity()
			return identity.Name, err
		},
		PrimeOwner: func() (string, bool, error) {
			here, err := machinename.ReadIdentity()
			if err != nil {
				return "", false, err
			}
			owners, err := boardengine.ReadOwners(boardDir)
			if err != nil {
				return "", false, err
			}
			return owners.Prime, boardengine.OwnedElsewhere(owners.Prime, owners.Registry, here), nil
		},
		ClaimPrime: func(string) (string, error) {
			identity, err := boardengine.EnsureIdentity()
			if err != nil {
				return "", err
			}
			write, err := boardengine.SetOwners(boardDir, boardengine.OwnerRequest{Machine: identity.Name, Identity: identity, Prime: true})
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
	tasks, err := boardengine.ReadTasks(fabriclayout.BoardDir(prime.HubPath))
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

// moveGateSlugs returns the worktrees whose anchor change stops the dobby of worktree:
// worktree itself, then the prime, once when they are the same.
func moveGateSlugs(prime *lymcwd.Location, worktree string) []string {
	if worktree == prime.WorktreeName {
		return []string{worktree}
	}
	return []string{worktree, prime.WorktreeName}
}

// localBeams lists the pairs of the hub present on this machine whose origin record carries the beam mark.
// Each pair carries the owner it effectively reads as and whether that owner reads as another machine.
func localBeams(prime *lymcwd.Location, boardDir string) ([]dobbyengine.BeamPair, error) {
	entries, err := fabricengine.List(prime.AnchorPath())
	if err != nil {
		return nil, err
	}
	owners, err := boardengine.ReadOwners(boardDir)
	if err != nil {
		return nil, err
	}
	here, err := machinename.ReadIdentity()
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
		pair, err := beamPair(slug, owners, here, parentOf)
		if err != nil {
			return nil, err
		}
		beams = append(beams, pair)
	}
	return beams, nil
}

// beamPair returns the pair of the beam slug with the owner it effectively reads as through parentOf.
// Elsewhere is set when the board's registry reads that owner as a machine other than here,
// so an owner recorded under a former name of here's registry entry reads as here.
func beamPair(slug string, owners boardengine.Owners, here machinename.Identity, parentOf func(slug string) (string, error)) (dobbyengine.BeamPair, error) {
	owner, err := owners.Effective(slug, parentOf)
	if err != nil {
		return dobbyengine.BeamPair{}, fmt.Errorf("hubgeom: resolve the owner of %q: %w", slug, err)
	}
	return dobbyengine.BeamPair{Slug: slug, Owner: owner, Elsewhere: boardengine.OwnedElsewhere(owner, owners.Registry, here)}, nil
}
