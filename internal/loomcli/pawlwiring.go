// pawlwiring.go holds the wiring only a pawl run needs beyond the BeamDescribe row: the seams of Beam-Up, Beam-Await and Beam-Down over the hub, the landing re-check Beam-Await runs, and the blocker read Finalize's land check backstops it with.

package loomcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fswatch"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// pawlProgram is the program name the pawl rows sign their notices to the beam's orch with.
const pawlProgram = "PAWL"

// pawlDeps assembles the seams of the pawl producers for the beam pair at location, whose slug names its project entry.
// Every closure reads or writes on demand and opens nothing at build time, since wire also runs for status and pause.
// The run id is the seed's start time, left empty when the run has no seed yet, which the producer's constructor refuses naming the field.
func pawlDeps(location *lyxcwd.Location, slug string) (pawlengine.Deps, error) {
	seed, found, err := shedrun.ReadSeed(location, shedrun.SelfRunID)
	if err != nil {
		return pawlengine.Deps{}, err
	}
	runID := ""
	if found {
		runID = seed.StartedAt
	}

	prime := func() (*lyxcwd.Location, error) { return primeOfHub(location) }
	return pawlengine.Deps{
		Slug:  slug,
		Paths: hubgeom.PawlPaths(location),
		RunID: runID,
		SetStatus: func(producer string) error {
			board, err := boardengine.OpenHub(location.HubPath)
			if err != nil {
				return err
			}
			return board.SetRunStatus(slug, boardengine.RunStatus("running", producer))
		},
		EntryProducer: func() (string, error) {
			board, err := boardengine.OpenHub(location.HubPath)
			if err != nil {
				return "", err
			}
			task, found, err := board.GetTask(slug)
			if err != nil {
				return "", err
			}
			if !found {
				return "", fmt.Errorf("the board holds no entry %q", slug)
			}
			producer, _ := boardengine.RunStatusProducer(task.Status)
			return producer, nil
		},
		OwnedElsewhere: func() (bool, error) {
			here, err := machinename.Name()
			if err != nil {
				return false, err
			}
			owner, _, err := ownerOfSubject(location, slug)
			if err != nil {
				return false, err
			}
			return boardengine.OwnedElsewhere(owner, here), nil
		},
		MarkBeam: func() error {
			hub, err := prime()
			if err != nil {
				return err
			}
			_, err = fabricengine.MarkBeam(fabricengine.NewMutations(""), hub, slug)
			return err
		},
		SpawnPush: func() error { return fabricengine.SpawnPairPush(location) },
		RegenerateWorkspace: func() error {
			hub, err := prime()
			if err != nil {
				return err
			}
			_, err = fabricengine.RegenerateHubWorkspace(fabricengine.NewMutations(""), hub)
			return err
		},
		Steps: []pawlengine.Step{{
			Name: "bring up the beam's dobby and orch",
			Run: func() error {
				_, err := orchcli.BeamUp(location, orchcli.BeamUpOptions{Prepare: false})
				return err
			},
		}},
		StopDobby: func() error {
			hub, err := prime()
			if err != nil {
				return err
			}
			return stopBeamDobby(hub, slug)
		},
		StartDobby: func() error {
			hub, err := prime()
			if err != nil {
				return err
			}
			return startBeamDobbyAlone(hub, slug)
		},
		DobbyLive: func() (bool, error) {
			hub, err := prime()
			if err != nil {
				return false, err
			}
			geometry, err := hubgeom.DobbyGeometry(hub, slug)
			if err != nil {
				return false, err
			}
			return dobbyengine.Live(geometry)
		},
		Recheck: func() ([]string, error) { return pawlRecheck(location, slug) },
		WriteSnapshot: func() error {
			if err := writeBoardEntrySnapshot(location); err != nil {
				return err
			}
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{loomengine.DiscussionDirRel()}, fmt.Sprintf("loom: board entry snapshot for %s", slug), fabricengine.EnvSyncOptions())
			return err
		},
		Notify: func(line string) error {
			_, err := orchcli.NotifyOrchAt(location, pawlProgram, line)
			return err
		},
		Down: func() error {
			_, err := orchcli.BeamDown(location)
			return err
		},
		Now:   time.Now,
		Sleep: sleepContext,
	}, nil
}

// openLandRequestWatch returns a channel that delivers a value whenever the land request's directory changes, or nil when no watch can be opened, which leaves Beam-Await on its timer.
// The directory is created when absent, since Beam-Await may start before the beam's dobby has made it.
// The watcher lives until the process ends.
func openLandRequestWatch(paths pawlengine.Paths) <-chan struct{} {
	dir := filepath.Dir(paths.LandRequest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.Warn("loom: the land request directory could not be created, so Beam-Await polls", "dir", dir, "error", err)
		return nil
	}
	watcher, err := fswatch.Watch(dir, filepath.Base(paths.LandRequest))
	if err != nil {
		logger.Warn("loom: the land request directory could not be watched, so Beam-Await polls", "dir", dir, "error", err)
		return nil
	}
	changed := make(chan struct{}, 1)
	go func() {
		for range watcher.Events() {
			select {
			case changed <- struct{}{}:
			default:
			}
		}
	}()
	return changed
}

// ownerOfSubject returns the owner machine the board entry slug reads as, from the local board, and the subject `lyx adopt` takes to move it.
// An empty slug is the root: its owner is the prime owner and its subject the root's.
func ownerOfSubject(location *lyxcwd.Location, slug string) (owner, subject string, err error) {
	prime, err := primeOfHub(location)
	if err != nil {
		return "", "", err
	}
	owners, err := boardengine.ReadOwners(fabricengine.BoardDir(location.HubPath))
	if err != nil {
		return "", "", err
	}
	if slug == "" {
		return owners.Prime, boardengine.RootSubject, nil
	}
	parentOf := func(entry string) (string, error) { return hubgeom.ParentOf(prime, entry) }
	if owner, err = owners.Effective(slug, parentOf); err != nil {
		return "", "", err
	}
	subject, err = owners.AdoptSubject(slug, parentOf)
	return owner, subject, err
}

// pawlRecheck reads what the landing re-check judges for the beam slug at location: the members and pairs working under it, the beam's owner and its parent's owner.
// It returns the lines of pawlRecheckLines, empty when the beam may land.
func pawlRecheck(location *lyxcwd.Location, slug string) ([]string, error) {
	here, err := machinename.Name()
	if err != nil {
		return nil, err
	}
	prime, err := primeOfHub(location)
	if err != nil {
		return nil, err
	}
	blockers, err := battencli.ReadBeamLandBlockers(prime, slug)
	if err != nil {
		return nil, err
	}
	refusal := battencli.BeamLandRefusal(prime, slug)
	beamOwner, _, err := ownerOfSubject(location, slug)
	if err != nil {
		return nil, err
	}
	parent, err := hubgeom.ParentOf(prime, slug)
	if err != nil {
		return nil, err
	}
	parentOwner, parentSubject, err := ownerOfSubject(location, parent)
	if err != nil {
		return nil, err
	}
	return pawlRecheckLines(slug, blockers, refusal, here, beamOwner, parentOwner, parentSubject), nil
}

// pawlRecheckLines words the failed landing re-check of the beam slug, one line per blocker, empty when every check passes.
// Each way forward of refusal, the text of the beam's blockers, is a line; a beam this machine does not own names `lyx adopt <slug>`, and a parent another machine owns names `lyx adopt <parentSubject>`, which is `root` for the prime.
func pawlRecheckLines(slug string, blockers battencli.BeamLandBlockers, refusal error, here, beamOwner, parentOwner, parentSubject string) []string {
	var lines []string
	switch {
	case refusal != nil:
		lines = append(lines, refusalWays(refusal)...)
	case !blockers.Empty():
		lines = append(lines, fmt.Sprintf("work remains under the beam %s; run `lyx beam land %s` again once it is gone", slug, slug))
	}
	if boardengine.OwnedElsewhere(beamOwner, here) {
		lines = append(lines, fmt.Sprintf("the beam %s is owned by %s, not this machine (%s): run `lyx adopt %s` to take it", slug, beamOwner, here, slug))
	}
	if boardengine.OwnedElsewhere(parentOwner, here) {
		lines = append(lines, fmt.Sprintf("the beam's parent is owned by %s, not this machine (%s): run `lyx adopt %s` to take it", parentOwner, here, parentSubject))
	}
	return lines
}

// refusalWays splits a BeamLandRefusal into the ways forward it names, one per blocker; a text with no way forward is one line.
func refusalWays(refusal error) []string {
	text := refusal.Error()
	if _, ways, found := strings.Cut(text, "way forward: "); found {
		return strings.Split(ways, "; ")
	}
	return []string{text}
}

// pawlLandBlockers returns the seam Finalize's land check reads: the open members and the local pairs forked from the beam slug, from the hub of prime.
func pawlLandBlockers(prime *lyxcwd.Location, slug string) func() ([]landingshed.LandBlocker, error) {
	return func() ([]landingshed.LandBlocker, error) {
		found, err := battencli.ReadBeamLandBlockers(prime, slug)
		if err != nil {
			return nil, err
		}
		var blockers []landingshed.LandBlocker
		for _, member := range found.Members {
			blockers = append(blockers, landingshed.LandBlocker{Slug: member.Slug, HasPair: member.HasPair, Running: member.Running})
		}
		for _, pair := range found.ChildPairs {
			blockers = append(blockers, landingshed.LandBlocker{Slug: pair, Pair: true})
		}
		return blockers, nil
	}
}

// stopBeamDobby ends the dobby of the beam slug when its lock reads held, through its `RoleDobby` record alone, so no other process of the beam is signalled.
func stopBeamDobby(prime *lyxcwd.Location, slug string) error {
	geometry, err := hubgeom.DobbyGeometry(prime, slug)
	if err != nil {
		return err
	}
	live, err := dobbyengine.Live(geometry)
	if err != nil || !live {
		return err
	}
	_, err = runproc.EndRole(fabricengine.RunProcDir(prime.HubPath), slug, runproc.RoleDobby, os.Getpid())
	return err
}

// startBeamDobbyAlone starts the dobby of the beam slug unless it is live, naming no other worktree.
// A refusal, such as a beam another machine owns, is returned as an error carrying its detail.
func startBeamDobbyAlone(prime *lyxcwd.Location, slug string) error {
	report, err := dobbyengine.EnsureLive(hubgeom.DobbyStart(prime), []string{slug})
	if err != nil {
		return err
	}
	var refusals []error
	for _, entry := range report.Entries {
		if entry.Outcome == dobbyengine.OutcomeRefused {
			refusals = append(refusals, fmt.Errorf("the dobby of %s was not started: %s", entry.Worktree, entry.Detail))
		}
	}
	return errors.Join(refusals...)
}

// beamParentWorktree returns the name of the parent worktree the beam slug records in its origin record, the prime's name when the record names none.
func beamParentWorktree(prime *lyxcwd.Location, slug string) (string, error) {
	origin, found, err := fabricengine.ReadOriginFor(prime, slug)
	if err != nil {
		return "", err
	}
	if found && origin.ParentWorktree != "" {
		return origin.ParentWorktree, nil
	}
	return fabricengine.PrimeName(prime)
}
