// cancelcmd.go builds "lym batten cancel <slug> [--remove]" and wires cancelRun's seams over the hub's prime worktree.
//
// The decisions and their order are cancelRun's, in cancel.go; this file only reads and writes the hub's substrates on its behalf.

package battencli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/tendercli"
	"github.com/Knatte18/loomyard/internal/tenderengine"
	"github.com/spf13/cobra"
)

// cancelCmd builds "lym batten cancel <slug> [--remove]", the end of a run for good, run from the prime or a beam.
func (c *battenCLI) cancelCmd() *cobra.Command {
	var remove bool
	cmd := &cobra.Command{
		Use:         "cancel <slug>",
		Short:       "end a run for good, whatever its state, and optionally remove its pair",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `cancel ends the batten run of one slug for good, from the prime or a beam, whatever its state.
It ends the run's registered processes and strands, stops the tackler strand its marker names,
clears the board claim, removes the run's records and scratch and commits that removal on the
records side, lifts the slug's own pause entries and spawns the push of the records.
The pair is kept by default, and "lym batten cancel <slug> --remove" must run before the slug runs again.
--remove removes the pair, keeps its uncommitted paths under a local snapshot tag, and archives
and deletes its task branch locally and on origin, each deleted tip held by an archive tag on origin.

It refuses, before it ends or removes anything, while a hub pause covers the prime, when the run's
work already reached its parent (finish the landing with "lym batten run" instead), when another
machine owns the slug, when origin stopped the entry, when another cancel of the slug is under way,
and when nothing of the slug is left to cancel.
It skips every wait a teardown makes for a working driver.
With origin unreachable it only ends the run's processes and strands, leaves its marker, and posts a
notice; "lym batten cancel" run again finishes an interrupted cancel.

The cancel of a beam first runs the hub's origin reconcile and refuses while members remain,
naming each to land or cancel first; with origin unreachable it refuses naming the re-run.
A beam with no members ends the pawl run's processes and the beam's dobby, then stops the
beam's orch and watcher; "lym beam create <slug>" starts the run again.

Output: {"slug", "state", "row", "ended", "strands_stopped", "tackler_stopped", "pair_removed",
"snapshot_tag", "archive_tags", "records_removed", "claim_cleared", "lifted", "push_spawned",
"notice_posted", "end_only", "way_forward", "warnings"}.
push_spawned says the push was started, not that origin took it.

Example:
  lym batten cancel some-slug
  lym batten cancel some-slug --remove`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()
			fail := func(err error) error {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			slug, explicit := resolveBattenRunID(args)
			if err := refuseSelfAddress(slug, explicit); err != nil {
				return fail(err)
			}
			if err := shedrun.ValidateRunID(slug); err != nil {
				return fail(err)
			}
			cwd, err := lymcwd.CwdFrom(ctx)
			if err != nil {
				return fail(err)
			}
			location, err := lymcwd.Resolve(cwd)
			if err != nil {
				return fail(err)
			}
			prime, err := ResolveBattenPrime(location)
			if err != nil {
				return fail(err)
			}
			if remove {
				if err := refuseOwnPair(location, prime, slug, "cancel --remove"); err != nil {
					return fail(err)
				}
			}
			deps, err := hubCancelDeps(prime, slug)
			if err != nil {
				return fail(err)
			}
			report, err := cancelRun(ctx, slug, remove, deps)
			if err != nil {
				return fail(err)
			}
			clihelp.SetExit(ctx, output.Ok(out, cancelEnvelope(report)))
			return nil
		},
	}
	cmd.Flags().BoolVar(&remove, "remove", false, "also remove the pair, snapshotting its uncommitted paths, and archive and delete its task branch")
	return cmd
}

// cancelEnvelope renders report as the verb's JSON envelope fields; every list is an array, empty when nothing was listed.
func cancelEnvelope(report cancelReport) map[string]any {
	ended := make([]map[string]any, 0, len(report.Ended))
	for _, process := range report.Ended {
		ended = append(ended, map[string]any{"pid": process.PID, "role": process.Role})
	}
	return map[string]any{
		"slug":            report.Slug,
		"state":           report.State,
		"row":             report.Row,
		"ended":           ended,
		"strands_stopped": slices.Concat([]string{}, report.StrandsStopped),
		"tackler_stopped": report.TacklerStopped,
		"pair_removed":    report.PairRemoved,
		"snapshot_tag":    report.SnapshotTag,
		"archive_tags":    slices.Concat([]string{}, report.ArchiveTags),
		"records_removed": report.RecordsRemoved,
		"claim_cleared":   report.ClaimCleared,
		"lifted":          slices.Concat([]string{}, report.Lifted),
		"push_spawned":    report.PushSpawned,
		"notice_posted":   report.NoticePosted,
		"end_only":        report.EndOnly,
		"way_forward":     report.WayForward,
		"warnings":        slices.Concat([]string{}, report.Warnings),
	}
}

// hubCancelDeps wires each of cancelRun's seams over the hub of the prime location for slug.
func hubCancelDeps(location *lymcwd.Location, slug string) (cancelDeps, error) {
	teardown, err := pairteardown.New(location)
	if err != nil {
		return cancelDeps{}, err
	}
	topology, err := hubTopology(location)
	if err != nil {
		return cancelDeps{}, err
	}
	pauseGeometry, err := hubgeom.PauseGeometry(location)
	if err != nil {
		return cancelDeps{}, err
	}
	boardDir := fabricengine.BoardDir(location.HubPath)
	driverLock, runLock := shedrun.DriverLock(location, slug), shedrun.RunLock(location, slug)
	runProcDir := fabricengine.RunProcDir(location.HubPath)
	board := func() (*boardengine.Board, error) { return boardengine.OpenHub(location.HubPath) }
	reader := &battenCLI{location: location, slug: slug}

	return cancelDeps{
		HubPaused: func() (bool, error) {
			record, err := pauserecord.Read(pauseGeometry.RecordPath)
			return hubPauseLive(record), err
		},
		ExemptGate:          func() (func(), error) { return pauserecord.SetGateExemptAll(), nil },
		TaskWorktreePresent: func() (bool, error) { return taskWorktreePresent(location, slug) },
		Reconcile:           func() (fabricengine.OriginReport, error) { return reconcileRun(location, slug, true) },
		PullBolt:            func() error { return pullBolt(boardDir) },
		Owner: func() (string, string, string, error) {
			machine, err := machinename.Name()
			if err != nil {
				return "", "", "", err
			}
			owners, err := boardengine.ReadOwners(boardDir)
			if err != nil {
				return "", "", "", err
			}
			parentOf := func(slug string) (string, error) { return hubgeom.ParentOf(location, slug) }
			owner, err := owners.Effective(slug, parentOf)
			if err != nil {
				return "", "", "", err
			}
			subject, err := owners.AdoptSubject(slug, parentOf)
			return machine, owner, subject, err
		},
		ReadStatus: func() (shedengine.Status, bool, error) { return readBattenStatus(location, slug) },
		BoardStatus: func() (*string, bool, error) {
			hubBoard, err := board()
			if err != nil {
				return nil, false, err
			}
			task, found, err := hubBoard.GetTask(slug)
			if err != nil || !found {
				return nil, false, err
			}
			return task.Status, true, nil
		},
		RecordsPresent: func() (bool, error) {
			_, err := os.Stat(shedrun.RunDir(location, slug))
			if err == nil {
				return true, nil
			}
			if !errors.Is(err, fs.ErrNotExist) {
				return false, err
			}
			// A run directory deleted on disk but still tracked at the records HEAD has its deletion outstanding, which the cancel's records removal commits.
			for _, rel := range []string{shedrun.StatusRel(location, slug), shedrun.SeedRel(location, slug)} {
				_, tracked, err := fabricengine.CommittedAnchoredFile(location, rel)
				if err != nil || tracked {
					return tracked, err
				}
			}
			return false, nil
		},
		ChildReachedParent: func(batten shedengine.Status, battenFound bool) (bool, error) {
			child, childFound := reader.readChildStatus()
			pastMerge := false
			if childFound {
				var err error
				pastMerge, err = battenshed.FinalizePastMerge(child, childRestepped(location, slug), childParentHolds(location, slug))
				if err != nil {
					return false, err
				}
			}
			return childReachedParent(batten, battenFound, child, childFound, pastMerge), nil
		},
		ReadMarker: func() (cancelMarkerReading, error) { return readCancelMarker(location, slug) },
		TakeMarker: func() (func(bool) error, bool, error) {
			held, err := takeCancelMarker(location, slug, pairteardown.CancelLockWait)
			if err != nil {
				return nil, false, err
			}
			return held.release, held.fresh, nil
		},
		GOOS:         runtime.GOOS,
		LiveRecords:  func() ([]runproc.Record, error) { return runproc.List(runProcDir, slug) },
		CreatedAt:    proc.CreationTime,
		PairParts:    func() (fabricengine.PairParts, error) { return topology.PairParts(location, slug) },
		SessionLive:  func() (bool, error) { return pairSessionLive(location, slug) },
		EndForCancel: teardown.EndForCancel,
		TakeBattenLocks: func() (func(), string, error) {
			for _, path := range []string{driverLock, runLock} {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					return nil, "", err
				}
			}
			return takeBattenLocks(driverLock, runLock, pairteardown.CancelLockWait)
		},
		StopTackler: func() (string, error) {
			return stopRecordedTackler(shedrun.TacklerMarker(location, slug), func(guid string) error {
				runner, err := primeShuttleRunner(location)
				if err != nil {
					return err
				}
				// A prime with no reed session holds no strand, so the tackler is already stopped.
				if err := runner.StopStrand(guid); err != nil && !errors.Is(err, reedengine.ErrNoSession) {
					return err
				}
				return nil
			})
		},
		RemovePair:         teardown.RemovePair,
		TaskBranchOnOrigin: func() (bool, error) { return topology.TaskBranchOnOrigin(location, slug) },
		DiscardTaskBranch:  func() (fabricengine.DiscardResult, error) { return topology.DiscardTaskBranch(location, slug) },
		RemoveRecords: func() (bool, error) {
			statusLockPath := shedrun.StatusLock(location, slug)
			if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
				return false, err
			}
			statusLock, err := lock.AcquireWriteLock(statusLockPath)
			if err != nil {
				return false, err
			}
			defer releaseLock(statusLock)
			return removeRunRecords(location, slug, "batten cancel: remove the run directory of "+slug, fabricCleanupDeps(location))
		},
		ClearClaim: func() (bool, bool, error) {
			return clearRunClaim(
				func() (*string, bool, error) {
					hubBoard, err := board()
					if err != nil {
						return nil, false, err
					}
					task, found, err := hubBoard.GetTask(slug)
					if err != nil || !found {
						return nil, false, err
					}
					return task.Status, true, nil
				},
				func(status *string) error {
					hubBoard, err := board()
					if err != nil {
						return err
					}
					return hubBoard.SetStatus(slug, status)
				},
			)
		},
		LiftPauses: func() ([]string, error) { return liftSlugPauses(location, slug, pauseGeometry.RecordPath) },
		SpawnPush: func() error {
			_, err := spawnStatusPush(location, "")
			return err
		},
		Notify: func(line string) error {
			_, err := notifyRunParent(location, slug, line)
			return err
		},
		BeamProject: func() (bool, error) {
			owners, err := boardengine.ReadOwners(boardDir)
			if err != nil {
				return false, err
			}
			entry := owners.Entries[slug]
			return entry.IsProject && entry.Beam, nil
		},
		ReconcileHub: func() (fabricengine.OriginReport, error) {
			return fabricengine.ReconcileOrigin(location, fabricengine.ReconcileScope{}, teardown.OriginSeams(pairteardown.HubSeamOptions(location)), fabricengine.ReconcileOptions{})
		},
		LandRefusal:  func() error { return BeamLandRefusal(location, slug) },
		LandingHeld:  func() (bool, error) { return hubgeom.LandingHeld(location, slug) },
		LandBlockers: func() (BeamLandBlockers, error) { return ReadBeamLandBlockers(location, slug) },
		StopBeam: func() error {
			present, err := taskWorktreePresent(location, slug)
			if err != nil || !present {
				return err
			}
			beamLocation, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(location, slug))
			if err != nil {
				return err
			}
			_, err = orchcli.BeamDown(beamLocation)
			return err
		},
	}, nil
}

// hubTopology builds the fabric topology of the hub location sits in from its config.
func hubTopology(location *lymcwd.Location) (*fabricengine.Topology, error) {
	cfg, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(location))
	if err != nil {
		return nil, err
	}
	return fabricengine.NewTopology(cfg), nil
}

// readBattenStatus reads batten's status file for slug under its lock, creating nothing.
// found is false when the file, or the run directory around it, is absent.
func readBattenStatus(location *lymcwd.Location, slug string) (shedengine.Status, bool, error) {
	status, found, err := state.ReadJSONNoMkdir[shedengine.Status](StatusFile(location, slug), shedrun.StatusLock(location, slug))
	if errors.Is(err, fs.ErrNotExist) {
		return shedengine.Status{}, false, nil
	}
	return status, found, err
}

// childRestepped returns the reader of the task worktree's settle records for a re-step of the stop a status names.
func childRestepped(location *lymcwd.Location, slug string) func(shedengine.Status) (bool, error) {
	return func(status shedengine.Status) (bool, error) {
		task, err := taskWorktreeLocation(location, slug)
		if err != nil {
			return false, err
		}
		records, err := sheddriver.ReadSettleRecords(shedrun.SettleRecords(task, shedrun.SelfRunID))
		if err != nil {
			return false, err
		}
		return sheddriver.RestepRecorded(records, status), nil
	}
}

// childParentHolds returns the reader of whether the parent branch already holds the task worktree's work.
func childParentHolds(location *lymcwd.Location, slug string) func() (bool, error) {
	return func() (bool, error) {
		task, err := taskWorktreeLocation(location, slug)
		if err != nil {
			return false, err
		}
		return fabricengine.ParentHoldsTaskWork(location, task, slug)
	}
}

// pairSessionLive reports whether the pair's reed session of slug is live on the hub's tmux server; a hub with no server has none.
func pairSessionLive(location *lymcwd.Location, slug string) (bool, error) {
	cfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(location), "reed")
	if err != nil {
		return false, err
	}
	live, err := reedengine.ListSessions(cfg.Tmux, reedengine.ServerName(location.HubPath))
	if err != nil {
		if reedengine.IsNoServer(err) {
			return false, nil
		}
		return false, fmt.Errorf("list the hub's live sessions: %w", err)
	}
	return slices.Contains(live, reedengine.SessionName(fabricengine.WorktreePath(location, slug))), nil
}

// primeShuttleRunner builds the shuttle runner over the prime's reed geometry, which stops a strand by guid.
func primeShuttleRunner(location *lymcwd.Location) (*shuttleengine.Runner, error) {
	dirs := fabricengine.ConfigDirs(location)
	reedConfig, err := reedengine.LoadConfig(dirs, "reed")
	if err != nil {
		return nil, err
	}
	shuttleConfig, err := shuttleengine.LoadConfig(dirs, "shuttle")
	if err != nil {
		return nil, err
	}
	reedGeometry, err := hubgeom.ReedGeometry(location)
	if err != nil {
		return nil, err
	}
	pauseGeometry, err := hubgeom.PauseGeometry(location)
	if err != nil {
		return nil, err
	}
	engines, err := shuttleengines.New(shuttleConfig, dirs)
	if err != nil {
		return nil, err
	}
	runner := shuttleengine.NewRunner(reedengine.New(reedConfig, reedGeometry, shuttleengines.EnvScrub()), engines, reedGeometry.AnchorPath, reedGeometry.WorktreeRoot, shuttleConfig)
	runner.SetPause(pauseGeometry)
	return runner, nil
}

// hubPauseLive reports whether record holds a live hub-scope entry, the one pause that refuses a cancel.
// A live entry of any slug's own scope never does.
func hubPauseLive(record pauserecord.Record) bool {
	return slices.ContainsFunc(record.Live, func(entry pauserecord.Entry) bool { return entry.Scope == pauserecord.ScopeHub })
}

// slugPauseRefs returns the ref of each live entry of record whose scope is exactly slug, never a hub entry or another slug's.
func slugPauseRefs(record pauserecord.Record, slug string) []tenderengine.EntryRef {
	var refs []tenderengine.EntryRef
	for _, entry := range record.Live {
		if entry.Scope == slug {
			refs = append(refs, tenderengine.EntryRef{Scope: entry.Scope, Caller: entry.Caller, PausedAt: entry.PausedAt})
		}
	}
	return refs
}

// liftSlugPauses lifts the live pause entries of slug's own scope in the record at recordPath and returns their scopes.
// The first failed lift ends the loop with its error, keeping the scopes lifted before it.
func liftSlugPauses(location *lymcwd.Location, slug, recordPath string) ([]string, error) {
	record, err := pauserecord.Read(recordPath)
	if err != nil {
		return nil, err
	}
	refs := slugPauseRefs(record, slug)
	if len(refs) == 0 {
		return nil, nil
	}
	deps, _, err := tendercli.HubDeps(location, os.Getenv(agentname.StrandNameEnv))
	if err != nil {
		return nil, err
	}
	var lifted []string
	for _, ref := range refs {
		if _, err := tenderengine.Lift(deps, ref); err != nil {
			return lifted, fmt.Errorf("lift the pause of %s: %w", ref.Scope, err)
		}
		lifted = append(lifted, ref.Scope)
	}
	return lifted, nil
}

// clearRunClaim releases the run's claim on the board entry by writing the draft status through write when read shows a run-form status.
// A done status, any other non-run status and an absent entry write nothing; found reports whether the entry exists.
func clearRunClaim(read func() (status *string, found bool, err error), write func(status *string) error) (cleared, found bool, err error) {
	status, found, err := read()
	if err != nil || !found {
		return false, found, err
	}
	if !boardengine.IsRunStatus(status) {
		return false, true, nil
	}
	draft := boardengine.StatusDraft
	if err := write(&draft); err != nil {
		return false, true, err
	}
	return true, true, nil
}

// stopRecordedTackler stops the strand the tackler marker at markerPath names and returns its guid, or "" when no marker is there.
// The stop's error is returned with no guid.
func stopRecordedTackler(markerPath string, stop func(guid string) error) (string, error) {
	marker, found, err := sheddriver.ReadTacklerMarker(markerPath)
	if err != nil || !found {
		return "", err
	}
	if err := stop(marker.StrandGUID); err != nil {
		return "", err
	}
	return marker.StrandGUID, nil
}

// takeBattenLocks takes batten's driver lock and then its run lock, each waited for at most wait.
// A lock still held at the bound returns its path as busy with nothing held, the driver lock already taken included, so a retry takes neither against this process's own handle.
// The returned release drops both.
func takeBattenLocks(driverLockPath, runLockPath string, wait time.Duration) (release func(), busy string, err error) {
	driver, acquired, err := lock.AcquireWriteLockWithin(driverLockPath, wait)
	if err != nil {
		return nil, "", err
	}
	if !acquired {
		return nil, driverLockPath, nil
	}
	run, acquired, err := lock.AcquireWriteLockWithin(runLockPath, wait)
	if err != nil {
		return nil, "", errors.Join(err, driver.Release())
	}
	if !acquired {
		return nil, runLockPath, driver.Release()
	}
	return func() {
		if err := errors.Join(run.Release(), driver.Release()); err != nil {
			logger.Warn("battencli: could not release batten's locks after the cancel", "cause", err)
		}
	}, "", nil
}
