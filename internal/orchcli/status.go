// status.go implements the `status` orch verb: one envelope describing the recorded strand, the watcher and the persisted cycle state.

package orchcli

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/spf13/cobra"
)

// trackedStrand returns the strand reed tracks under guid, and whether one does.
func trackedStrand(strands []reedengine.StrandStatus, guid string) (reedengine.StrandStatus, bool) {
	if guid == "" {
		return reedengine.StrandStatus{}, false
	}
	for _, s := range strands {
		if s.GUID == guid {
			return s, true
		}
	}
	return reedengine.StrandStatus{}, false
}

// statusFields builds the status envelope's fields; context_tokens is nil while unknown, and strand_elsewhere is empty for an untracked strand.
func statusFields(st orchengine.State, cfg orchengine.Config, strand reedengine.StrandStatus, tracked, watcherLive, watcherRestarted bool) map[string]any {
	elsewhere := ""
	if tracked {
		elsewhere = strand.Elsewhere
	}
	var tokens any
	if st.LastContextKnown {
		tokens = st.LastContextTokens
	}
	var lastDeferral any
	if !st.LastDeferral.IsZero() {
		lastDeferral = st.LastDeferral.UTC().Format(time.RFC3339)
	}
	return map[string]any{
		"strand":                st.Strand,
		"session_id":            st.SessionID,
		"strand_live":           tracked && strand.Live,
		"watcher_live":          watcherLive,
		"watcher_restarted":     watcherRestarted,
		"strand_elsewhere":      elsewhere,
		"context_tokens":        tokens,
		"soft_threshold_tokens": cfg.SoftThreshold(),
		"threshold_tokens":      cfg.Threshold(),
		"phase":                 string(st.Phase),
		"cycle_count":           st.CycleCount,
		"cycle_trigger":         st.CycleTrigger,
		"last_handoff":          st.LastHandoff,
		"last_abort_reason":     st.LastAbortReason,
		"last_deferral":         lastDeferral,
		"stuck":                 st.Stuck,
		"watcher_exit":          st.WatcherExit,
	}
}

// errStartLockHeld reports that restartWatcher found start.lock held, so a `start` or a `stop` runs and the restart was left to it.
var errStartLockHeld = errors.New("orch: start.lock is held")

// startLockHeldHint is the envelope hint for a live strand with no watcher whose restart was skipped for a held start.lock.
const startLockHeldHint = "a `lyx beam start` or `lyx orch stop` is running; rerun `lyx orch status` once it ends, or run `lyx beam start`"

// restartWatcher spawns a watcher for the recorded orch strand under start.lock and answers whether it did.
// It restarts nothing and answers errStartLockHeld when another process holds the lock.
// Holding the lock it reads the state, the strands and the watch lock again, since a `stop` or `start` may have run since the caller's reads, and spawns only for a strand still tracked and live with no watcher holding the watch lock.
// st is the state the caller read; the re-read replaces it.
func (c *orchCLI) restartWatcher(st orchengine.State) (bool, error) {
	if err := os.MkdirAll(c.paths.Dir, 0o755); err != nil {
		return false, fmt.Errorf("orch: create %s: %w", c.paths.Dir, err)
	}
	startLock, acquired, err := lock.TryAcquireWriteLock(c.paths.StartLockPath)
	if err != nil {
		return false, err
	}
	if !acquired {
		return false, errStartLockHeld
	}
	defer func() { _ = startLock.Release() }()

	if st, err = orchengine.LoadState(c.paths); err != nil {
		return false, err
	}
	strands, err := c.strands.Strands()
	if err != nil {
		return false, err
	}
	watcherLive, err := orchengine.WatcherLive(c.paths)
	if err != nil {
		return false, err
	}
	strand, tracked := trackedStrand(strands, st.Strand)
	if !tracked || !strand.Live || watcherLive {
		return false, nil
	}
	if err := c.spawnWatcher(); err != nil {
		return false, err
	}
	logger.Info("orch: restarted a dead watcher", "strandGUID", st.Strand, "previousExit", st.WatcherExit)
	return true, nil
}

// hubPauseHolds reports whether the pause record holds a live hub entry, the entry that holds the watcher's cycle machine.
// A record that cannot be read answers false, as the watcher reads it uncovered.
func (c *orchCLI) hubPauseHolds() bool {
	record, err := pauserecord.Read(c.pauseRecordPath)
	if err != nil {
		return false
	}
	_, live := record.Covering("")
	return live
}

// cycleRequestField builds the status envelope's cycle_request value: nil when no request is pending, else the request's mode, its time and whether a hub pause holds it.
// A marker that does not parse reports an empty mode and a null time.
func cycleRequestField(req orchengine.CycleRequest, pending, held bool) any {
	if !pending {
		return nil
	}
	var requestedAt any
	if !req.RequestedAt.IsZero() {
		requestedAt = req.RequestedAt.UTC().Format(time.RFC3339)
	}
	return map[string]any{"mode": req.Mode, "requested_at": requestedAt, "held_by_pause": held}
}

// statusCmd builds the `status` subcommand.
func (c *orchCLI) statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "status",
		Short:       "report the orchestrator strand, watcher and cycle state",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `status prints one JSON envelope: the recorded strand and whether it is live,
whether a watcher holds its lock, the latest context reading against the soft
threshold (soft_threshold_tokens) and the hard cap (threshold_tokens), and the
persisted cycle phase (idle, handoff-requested, clearing, resuming or compacting), count, trigger (cycle_trigger), last handoff, abort reason
and last deferral (last_deferral, RFC 3339 UTC, null when none).
stuck names the stall reason creel last reported for the watcher's pending inject,
or the creel failure holding the cycle.
session_id is the orch session's recorded provider id, which "lyx beam start"
resumes; it is empty when none is recorded.
cycle_request is null while no cycle request is pending, else an object with the
request's mode, requested_at (RFC 3339 UTC, null for a marker that does not parse)
and held_by_pause, true while a hub pause holds the watcher's cycle machine, so
the request waits for the resume. "lyx orch withdraw" removes a pending request.
A live strand whose watcher is dead gets a new watcher from status, spawned under
start.lock so it never races a start or a stop; watcher_restarted is true only
when this call spawned one, and watcher_live keeps what the probe found. While a
start or stop holds the lock nothing is restarted and a hint says to rerun
status. A dead strand gets no watcher: "lyx beam start" relaunches strand and
watcher together. strand_elsewhere names the session:window of the strand's pane
when it sits in another session of reed's server, empty otherwise.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			st, err := orchengine.LoadState(c.paths)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			strands, err := c.strands.Strands()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			watcherLive, err := orchengine.WatcherLive(c.paths)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			req, pending, err := orchengine.CycleRequested(c.paths)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			strand, tracked := trackedStrand(strands, st.Strand)
			restarted, hint := false, ""
			if tracked && strand.Live && !watcherLive {
				restarted, err = c.restartWatcher(st)
				if errors.Is(err, errStartLockHeld) {
					restarted, err, hint = false, nil, startLockHeldHint
				}
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("orch: the watcher is dead and could not be restarted: %v; way forward: run `lyx beam start`, which spawns a watcher for a live strand", err)))
					return nil
				}
			}
			fields := statusFields(st, c.cfg, strand, tracked, watcherLive, restarted)
			if hint != "" {
				fields["hint"] = hint
			}
			fields["cycle_request"] = cycleRequestField(req, pending, pending && c.hubPauseHolds())
			clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
			return nil
		},
	}
}
