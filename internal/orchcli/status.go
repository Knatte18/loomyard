// status.go implements the `status` orch verb: one envelope describing the recorded strand, the watcher and the persisted cycle state.

package orchcli

import (
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
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

// statusFields builds the status envelope's fields; context_tokens is nil while unknown.
func statusFields(st orchengine.State, cfg orchengine.Config, strand reedengine.StrandStatus, tracked, watcherLive bool) map[string]any {
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
session_id is the orch session's recorded provider id, which "lyx orch start
--continue" resumes; it is empty when none is recorded.
cycle_request is null while no cycle request is pending, else an object with the
request's mode, requested_at (RFC 3339 UTC, null for a marker that does not parse)
and held_by_pause, true while a hub pause holds the watcher's cycle machine, so
the request waits for the resume. "lyx orch withdraw" removes a pending request.`,
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
			fields := statusFields(st, c.cfg, strand, tracked, watcherLive)
			fields["cycle_request"] = cycleRequestField(req, pending, pending && c.hubPauseHolds())
			clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
			return nil
		},
	}
}
