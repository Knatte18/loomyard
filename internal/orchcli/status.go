// status.go implements the `status` orch verb: one envelope describing the recorded strand, the watcher and the persisted cycle state.

package orchcli

import (
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/output"
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

// statusCmd builds the `status` subcommand.
func (c *orchCLI) statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "report the orchestrator strand, watcher and cycle state",
		Long: `status prints one JSON envelope: the recorded strand and whether it is live,
whether a watcher holds its lock, the latest context reading against the soft
threshold (soft_threshold_tokens) and the hard cap (threshold_tokens), and the
persisted cycle phase (idle, handoff-requested, clearing, resuming or compacting), count, trigger (cycle_trigger), last handoff, abort reason
and last DEFER deferral (last_deferral, RFC 3339 UTC, null when none).
stuck names why an overdue phase is still waiting for the session to go idle.`,
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

			strand, tracked := trackedStrand(strands, st.Strand)
			clihelp.SetExit(cmd.Context(), output.Ok(out, statusFields(st, c.cfg, strand, tracked, watcherLive)))
			return nil
		},
	}
}
