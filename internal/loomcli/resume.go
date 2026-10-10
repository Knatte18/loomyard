// resume.go implements the `resume` loom verb: wake a halted run's live, parked driver with the resume line, and refuse everything else with a way forward.
// A run awaiting at a review segment's Bouncer row with a pending circling decision takes the halted path, since its driver's step re-calls the Bouncer, which acts on the decision.
// It spawns no driver, adds no strand, never brings reed up and never switches a tmux client.

package loomcli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// resumeRefusal is one refusal the verb reports: its message, and the envelope kind when a caller tells the refusal apart by it.
type resumeRefusal struct {
	message string
	kind    string
}

// resumeFromRunState answers `lyx loom resume` for the run's persisted state.
// It must be called holding bootstrapLock, and releases it exactly once on every path out.
func (c *loomCLI) resumeFromRunState(ctx context.Context, out io.Writer, bootstrapLock *lock.FileLock) {
	refuse := func(r resumeRefusal) {
		_ = bootstrapLock.Release()
		if r.kind == "" {
			clihelp.SetExit(ctx, output.Err(out, r.message))
			return
		}
		clihelp.SetExit(ctx, output.ErrFields(out, r.message, map[string]any{"kind": r.kind}))
	}
	succeed := func(fields map[string]any) {
		_ = bootstrapLock.Release()
		output.Ok(out, fields)
	}

	// The status lock lives in the ephemeral scratch directory, which a pair re-created from its branch lacks while the status file is there.
	if err := os.MkdirAll(filepath.Dir(c.shedPaths.StatusLockPath), 0o755); err != nil {
		refuse(resumeRefusal{message: err.Error()})
		return
	}
	status, found, err := state.ReadJSONStrict[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
	if err != nil {
		refuse(resumeRefusal{message: err.Error()})
		return
	}
	if !found {
		refuse(resumeRefusal{message: "loom resume: no status file at " + c.shedPaths.StatusPath + `; run "` + retryStart + `" in the task worktree to begin the run`})
		return
	}

	runID := shedrun.ResolveRunID(c.location, c.runID)
	switch status.State {
	case shedengine.StateDone:
		succeed(map[string]any{"run_id": runID, "state": string(status.State), "message": "the run is done; there is nothing to resume"})
		return
	case shedengine.StateAwaiting:
		if refusal, resumable := c.awaitingResumability(status.CurrentProducer); !resumable {
			refuse(refusal)
			return
		}
	}

	runLockHeld, err := c.runLockHeld()
	if err != nil {
		refuse(resumeRefusal{message: err.Error()})
		return
	}
	driver, driverFound, err := c.driverDirectory.DriverRow()
	if err != nil {
		refuse(resumeRefusal{message: err.Error()})
		return
	}
	driverLive := driverFound && driver.Live && !driver.Retiring

	if status.State == shedengine.StateRunning {
		if driverLive || runLockHeld {
			succeed(map[string]any{"run_id": runID, "state": string(status.State), "message": "the run is already running"})
			return
		}
	} else if runLockHeld {
		refuse(resumeRefusal{
			message: `loom resume: a driver still holds the run lock and is finishing its post-run work; run "` + retryResume + `" again in a few seconds`,
			kind:    shedrun.StartNotParkedKind,
		})
		return
	}

	if !driverLive {
		if !c.refuseOverUnfinishedMerge(ctx, out, bootstrapLock, retryResume, status.CurrentProducer) {
			return
		}
		refuse(resumeRefusal{message: noLiveDriverMessage(driver, driverFound)})
		return
	}

	markerPath := shedrun.ParkMarker(c.location, runID)
	if _, err := os.Stat(markerPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			refuse(resumeRefusal{message: err.Error()})
			return
		}
		refuse(resumeRefusal{message: driverNotParkedMessage(status.State, retryResume), kind: shedrun.StartNotParkedKind})
		return
	}
	if !c.refuseOverUnfinishedMerge(ctx, out, bootstrapLock, retryResume, status.CurrentProducer) {
		return
	}
	reportPath, err := c.resumeParkedDriver(driver.GUID, retryResume)
	if err != nil {
		refuse(resumeRefusal{message: err.Error()})
		return
	}
	succeed(map[string]any{"run_id": runID, "state": string(status.State), "message": "the parked driver was woken", "stop_report": reportPath})
}

// awaitingResumability decides whether a run awaiting at row may take the halted path: it may only when row is a review segment's Bouncer row whose latest round carries a pending circling decision.
// Otherwise it returns the refusal to report.
func (c *loomCLI) awaitingResumability(row string) (resumeRefusal, bool) {
	subdir, isBouncer, err := c.bouncerSubdir(row)
	if err != nil {
		return resumeRefusal{message: "loom resume: " + err.Error() + `; the embedded loom recipe failed to parse, so this lyx build is broken; rebuild or reinstall lyx, then re-run "` + retryResume + `"`}, false
	}
	if !isBouncer {
		return resumeRefusal{message: `loom resume: the run is awaiting a decision, which resume does not give; run "lyx loom approve" or "lyx loom reject" in the task worktree, after which a batten run watching the child resumes it on the recorded decision, or run "lyx batten run <slug>" from the prime when no batten run watches it`}, false
	}
	pending, err := shedadapters.PendingCirclingDecision(filepath.Join(loomengine.LoomReviewsDir(c.location), subdir))
	if err != nil {
		return resumeRefusal{message: "loom resume: " + err.Error() + `; fix the file or directory the message names, and for a malformed decision file delete it and record the decision again with "lyx loom circling accept <slug>" or "lyx loom circling continue <slug>", then re-run "` + retryResume + `"`}, false
	}
	if !pending {
		return resumeRefusal{message: "loom resume: the run is awaiting at " + row + ", a review segment's Bouncer row, with no circling decision recorded; " + `run "lyx loom circling accept <slug>" or "lyx loom circling continue <slug>", then "` + retryResume + `"`}, false
	}
	return resumeRefusal{}, true
}

// runLockHeld reports whether a driver holds the run's lock, probing it without keeping it.
// The lock's directory is created first, since the lock opens with O_CREATE but never creates a parent.
func (c *loomCLI) runLockHeld() (bool, error) {
	if err := os.MkdirAll(filepath.Dir(c.shedPaths.LockPath), 0o755); err != nil {
		return false, err
	}
	probe, free, err := lock.TryAcquireWriteLock(c.shedPaths.LockPath)
	if err != nil {
		return false, err
	}
	if free {
		_ = probe.Release()
	}
	return !free, nil
}

// noLiveDriverMessage is the refusal for a run with no driver to wake, its way forward told apart by what reed's directory holds for the driver strand.
func noLiveDriverMessage(driver reedengine.DirectoryRow, driverFound bool) string {
	const head = "loom resume: the run has no live driver to wake, and resume starts none; "
	switch {
	case driverFound && driver.Retiring:
		return head + `its driver strand is retiring, so run "` + retryStart + `" in the task worktree, which removes the retiring strand and spawns a fresh driver`
	case driverFound:
		return head + `its driver strand is dead, so run "lyx batten run <slug>" from the prime when no batten run is watching the child, which brings the driver back, then run "` + retryResume + `" again; ` +
			`run "` + retryStart + `" in the task worktree when a batten run is already watching or its revive failed`
	default:
		return head + `reed holds no driver strand for it, so run "` + retryStart + `" in the task worktree`
	}
}

// resumeCmd builds the `resume` subcommand.
func (c *loomCLI) resumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume",
		Short: "wake this task's parked loom driver after a halt, without starting anything",
		Long: `resume wakes a halted run's live, parked driver by typing the resume line into
its pane and removing the park marker, the same step "lyx loom start" takes for a
parked driver. It never spawns a driver, never adds or attaches a strand, never
brings reed up and never switches a tmux client.

It takes the bootstrap lock "lyx loom start" takes, so the two never run at once,
and decides by the run's state first:

  no status file    refused; "lyx loom start" in the task worktree begins the run
  done              a no-op success saying the run is done
  awaiting, at a review segment's Bouncer row, a circling decision recorded
                    taken as a halted run is below: a live, parked driver is
                    woken and its step acts on the decision
  awaiting, at a Bouncer row, no decision recorded
                    refused; "lyx loom circling accept <slug>" or "lyx loom
                    circling continue <slug>", then "lyx loom resume"
  awaiting, at a Bouncer row, the decision unreadable
                    refused naming the file; fix it, or delete it and record the
                    decision again, then "lyx loom resume"
  awaiting, at any other row
                    refused; "lyx loom approve" or "lyx loom reject" in the task
                    worktree, after which a batten run watching the child resumes
                    it, or "lyx batten run <slug>" from the prime when none does
  running           a no-op success when the driver is live or holds the run
                    lock; refused as below when neither
  halted, run lock held
                    refused with the kind "driver_not_parked": the driver is
                    still finishing its post-run work; run resume again shortly
  halted, live driver, no park marker
                    refused with the kind "driver_not_parked"; run resume again
                    shortly
  halted, live driver, park marker
                    resumed; the envelope names the stop-report path; refused
                    with the kind "merge_in_progress" over an unfinished merge,
                    except a parked merge-in of the run's own parent branch at
                    Publish or Finalize, which that row aborts and redoes
  halted, no live driver
                    refused over an unfinished merge with "merge_in_progress"
                    (same exception), otherwise with the way forward its driver
                    strand calls for:
                    a dead strand takes "lyx batten run <slug>" from the prime
                    and then resume again, a retiring strand or none takes
                    "lyx loom start"

The driver strand is read from reed's directory, which a session that died with
the machine still lists, never from the run's seed. A resume line that cannot be
delivered is refused, naming "lyx loom status" and a re-run of "lyx loom resume".

Example:
  lyx loom resume`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			bootstrapLockPath := loomengine.LoomBootstrapLock(c.location)
			if err := os.MkdirAll(filepath.Dir(bootstrapLockPath), 0o755); err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			bootstrapLock, err := lock.AcquireWriteLock(bootstrapLockPath)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			c.resumeFromRunState(ctx, out, bootstrapLock)
			return nil
		},
	}
}
