// pawlwiring.go holds the wiring only a pawl run needs: the seams of Beam-Up, Beam-Await and Beam-Down over the hub, the notice of Beam-Up's warnings, the landing re-check Beam-Await runs, the blocker read Finalize's land check backstops it with, and the side effects of a move off a retired row.

package loomcli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// pawlProgram is the program name the pawl rows sign their notices to the beam's orch with.
const pawlProgram = "PAWL"

// retiredMoveGitHubTimeout bounds the GitHub read of a move off a retired row, so a stuck network holds the move for no longer than that.
const retiredMoveGitHubTimeout = 30 * time.Second

// pawlDeps assembles the seams of the pawl producers for the beam pair at location, whose slug names its project entry.
// Every closure reads or writes on demand and opens nothing at build time, since wire also runs for status and pause.
// The run id is the seed's start time, left empty when the run has no seed yet, which the producer's constructor refuses naming the field.
func pawlDeps(location *lymcwd.Location, slug string) (pawlengine.Deps, error) {
	seed, found, err := shedrun.ReadSeed(location, shedrun.SelfRunID)
	if err != nil {
		return pawlengine.Deps{}, err
	}
	runID := ""
	if found {
		runID = seed.StartedAt
	}
	retiredRows, err := loomrecipe.RetiredRowNames(shedrun.RecipePawl)
	if err != nil {
		return pawlengine.Deps{}, err
	}

	prime := func() (*lymcwd.Location, error) { return primeOfHub(location) }
	notify := func(line string) error {
		_, err := orchcli.NotifyOrchAt(location, pawlProgram, line)
		return err
	}
	return pawlengine.Deps{
		Slug:  slug,
		Paths: hubgeom.PawlPaths(location),
		RunID: runID,
		// RetiredRows are the rows the recipe's retired map names, so a hold an older binary wrote under one of them is still found.
		RetiredRows: retiredRows,
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
				return runBeamUpStep(func() (orchcli.BeamUpReport, error) {
					return orchcli.BeamUp(location, orchcli.BeamUpOptions{Prepare: false})
				}, notify, slug)
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
			return orchcli.BeamUpWorktree(hub, slug)
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
		ReadyStands: func() (bool, error) {
			return pawlengine.ReadyFor(loomengine.BeamReadyPath(location), loomengine.BeamReadyLockPath(location), slug)
		},
		StampWake: func(at time.Time) error {
			return shedrun.WriteWakeStamp(shedrun.WakeRecord(location, shedrun.SelfRunID), pawlengine.NameBeamAwait, at)
		},
		Notify: notify,
		Down: func() error {
			_, err := orchcli.BeamDown(location)
			return err
		},
		Now:   time.Now,
		Sleep: sleepContext,
	}, nil
}

// pawlRetiredMove returns the seam that gives the pawl run of the beam slug at location the side effects of a move off a retired row, over the hub.
// The PR-Review session ends through the run's `RoleSession` records, and the pull request's state reads through the pair's origin; pawl's own status and notice seams are reused.
// Every closure reads or writes on demand and opens nothing at build time, since wire also runs for status and pause.
func pawlRetiredMove(location *lymcwd.Location, slug string, pawl pawlengine.Deps) func(from, to string, resumed bool) error {
	recordPath := shedrun.PullRequestRecord(location, shedrun.SelfRunID)
	recordLock := shedrun.PullRequestRecordLock(location, shedrun.SelfRunID)
	deps := pawlengine.RetiredMoveDeps{
		Slug:            slug,
		Paths:           hubgeom.PawlPaths(location),
		LandingPath:     loomengine.BeamLandingPath(location),
		LandingLockPath: loomengine.BeamLandingLockPath(location),
		ReadyPath:       loomengine.BeamReadyPath(location),
		ReadyLockPath:   loomengine.BeamReadyLockPath(location),
		EndSession: func() error {
			record, found, err := pullrequest.ReadRecord(recordPath, recordLock)
			if err != nil || !found || record.StrandGUID == "" {
				return err
			}
			_, err = runproc.EndRole(fabricengine.RunProcDir(location.HubPath), slug, runproc.RoleSession, os.Getpid())
			return err
		},
		ScratchPullRequest: func() (int, string, bool, error) {
			record, found, err := pullrequest.ReadRecord(recordPath, recordLock)
			if err != nil || !found {
				return 0, "", false, err
			}
			return record.Number, record.URL, true, nil
		},
		PullRequestOpen: func(number int) (bool, error) { return pullRequestOpen(location, number) },
		BeamHead: func() (string, error) {
			fabric, err := fabricengine.Open(location)
			if err != nil {
				return "", err
			}
			return fabric.HeadSHA()
		},
		RemoveScratch: func() error { return pullrequest.RemoveRecord(recordPath, recordLock) },
		Commit:        func() error { return commitLandingDirectory(location, slug) },
		SetStatus:     pawl.SetStatus,
		Notify:        pawl.Notify,
	}
	return func(from, to string, resumed bool) error {
		return pawlengine.MoveOffRetired(deps, from, to, resumed)
	}
}

// pullRequestOpen reports whether GitHub reports the pull request number open, read through the origin of the pair at location.
func pullRequestOpen(location *lymcwd.Location, number int) (bool, error) {
	fabric, err := fabricengine.Open(location)
	if err != nil {
		return false, err
	}
	originURL, err := fabric.OriginURL()
	if err != nil {
		return false, err
	}
	client, err := pullrequest.Open(originURL)
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), retiredMoveGitHubTimeout)
	defer cancel()
	status, err := client.Status(ctx, number)
	if err != nil {
		return false, err
	}
	return status.State == pullrequest.StateOpen, nil
}

// commitLandingDirectory commits the landing directory of the beam pair at location, and commits nothing when the directory holds nothing git could take.
func commitLandingDirectory(location *lymcwd.Location, slug string) error {
	hasContent, err := fabricengine.AnchoredPathHasContent(location, loomengine.LandingDirRel())
	if err != nil || !hasContent {
		return err
	}
	_, _, err = fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{loomengine.LandingDirRel()}, fmt.Sprintf("loom: move the landing of %s off a retired row", slug), fabricengine.EnvSyncOptions())
	return err
}

// runBeamUpStep brings the beam slug up through beamUp and surfaces the warnings of its report through notify.
// An error from beamUp is returned and halts the row; the warnings never do.
func runBeamUpStep(beamUp func() (orchcli.BeamUpReport, error), notify func(line string) error, slug string) error {
	report, err := beamUp()
	if err != nil {
		return err
	}
	surfaceBeamUpWarnings(notify, slug, report.Warnings)
	return nil
}

// surfaceBeamUpWarnings logs each warning the bring-up of the beam slug reported and posts them through notify as one notice to the beam's orch,
// so a dobby that did not start or a missing overview reaches the operator without halting the row.
// A notice that cannot be posted is logged, since the warnings are already in the log.
func surfaceBeamUpWarnings(notify func(line string) error, slug string, warnings []string) {
	line := beamUpWarningLine(slug, warnings)
	if line == "" {
		return
	}
	for _, warning := range warnings {
		logger.Warn("loom: Beam-Up brought the beam up with a warning", "slug", slug, "warning", warning)
	}
	if err := notify(line); err != nil {
		logger.Warn("loom: Beam-Up could not post its warnings to the beam's orch", "slug", slug, "cause", err)
	}
}

// beamUpWarningLine words the notice of the warnings the bring-up of the beam slug reported, empty when it reported none.
func beamUpWarningLine(slug string, warnings []string) string {
	if len(warnings) == 0 {
		return ""
	}
	return fmt.Sprintf("Beam-Up brought the beam %s up with warnings: %s", slug, strings.Join(warnings, "; "))
}

// openLandRequestWatch returns a channel that delivers a value whenever the land request changes, or nil when no watch can be opened, which leaves Beam-Await on its timer.
// The directory is created when absent, since Beam-Await may start before the beam's dobby has made it.
// The watcher lives until the process ends.
func openLandRequestWatch(paths pawlengine.Paths) <-chan struct{} {
	dir := filepath.Dir(paths.LandRequest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.Warn("loom: the land request directory could not be created, so Beam-Await polls", "dir", dir, "error", err)
		return nil
	}
	changed, _ := watchFileChanges(paths.LandRequest, "Beam-Await")
	return changed
}

// ownerOfSubject returns the owner machine the board entry slug reads as, from the local board, and the subject `lym adopt` takes to move it.
// An empty slug is the root: its owner is the prime owner and its subject the root's.
func ownerOfSubject(location *lymcwd.Location, slug string) (owner, subject string, err error) {
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

// pawlRecheck reads what the landing re-check judges for the beam slug at location: the members and pairs working under it, the beam's owner, its parent's owner and its ready record.
// It returns the lines of pawlRecheckLines, empty when the beam may land.
func pawlRecheck(location *lymcwd.Location, slug string) ([]string, error) {
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
	ready, err := pawlengine.ReadyFor(loomengine.BeamReadyPath(location), loomengine.BeamReadyLockPath(location), slug)
	if err != nil {
		return nil, err
	}
	return pawlRecheckLines(slug, ready, blockers, refusal, here, beamOwner, parentOwner, parentSubject), nil
}

// pawlRecheckLines words the failed landing re-check of the beam slug, one line per blocker, empty when every check passes.
// Each way forward of refusal, the text of the beam's blockers, is a line; a beam this machine does not own names `lym adopt <slug>`, a parent another machine owns names `lym adopt <parentSubject>`, which is `root` for the prime, and a beam not flagged ready names `lym beam ready <slug>`.
func pawlRecheckLines(slug string, ready bool, blockers battencli.BeamLandBlockers, refusal error, here, beamOwner, parentOwner, parentSubject string) []string {
	var lines []string
	switch {
	case refusal != nil:
		lines = append(lines, refusalWays(refusal)...)
	case !blockers.Empty():
		lines = append(lines, fmt.Sprintf("work remains under the beam %s; run `lym beam land %s` again once it is gone", slug, slug))
	}
	if boardengine.OwnedElsewhere(beamOwner, here) {
		lines = append(lines, fmt.Sprintf("the beam %s is owned by %s, not this machine (%s): run `lym adopt %s` to take it", slug, beamOwner, here, slug))
	}
	if boardengine.OwnedElsewhere(parentOwner, here) {
		lines = append(lines, fmt.Sprintf("the beam's parent is owned by %s, not this machine (%s): run `lym adopt %s` to take it", parentOwner, here, parentSubject))
	}
	if !ready {
		lines = append(lines, fmt.Sprintf("the beam %s is not flagged ready to merge: run `lym beam ready %s` to flag it", slug, slug))
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
func pawlLandBlockers(prime *lymcwd.Location, slug string) func() ([]landingshed.LandBlocker, error) {
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
func stopBeamDobby(prime *lymcwd.Location, slug string) error {
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

// beamParentWorktree returns the name of the parent worktree the beam slug records in its origin record, the prime's name when the record names none.
func beamParentWorktree(prime *lymcwd.Location, slug string) (string, error) {
	origin, found, err := fabricengine.ReadOriginFor(prime, slug)
	if err != nil {
		return "", err
	}
	if found && origin.ParentWorktree != "" {
		return origin.ParentWorktree, nil
	}
	return fabricengine.PrimeName(prime)
}
