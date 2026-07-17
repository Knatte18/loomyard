// status.go implements the `status` webster verb: an instant, side-effect-
// free snapshot of _lyx/webster/state.json plus the reports dir -- the same
// navigation refresher buildercli's own status.go documents (human- and
// loom-facing). It never spawns an agent, never weft-commits, and never
// mutates state.json; unlike builder (which only learns a batch's terminal
// status from poll's own next tick or a live report-file scan), webster's
// own record-batch/recover-batch persist BatchState.Status/Terminal/Digest
// directly onto state.json the moment the batch reaches a terminal
// classification, so status here is a plain read of the persisted record --
// builder-parity in verb shape (a pure snapshot, never a mutation), not in
// mechanism.
package webstercli

import (
	"sort"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// statusCmd builds the `status` subcommand: LoadState, then for each
// persisted BatchState (sorted by number) a summary row naming the batch's
// kind (fork/recovery), its current status, whether it has reached a
// terminal classification, and whether a distilled digest has been
// persisted for it -- status is a plain read, never a state.json write.
func (c *websterCLI) statusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "print an instant snapshot of state.json and the reports dir",
		Long: `status reads _lyx/webster/state.json and reports the run's identity, the
in-flight batch cursor, the plan fingerprint, every batch's own persisted
record (number, slug, kind, status, terminal, whether a digest is
persisted), and whether a pause has been requested. It is a plain read --
no weft commit, no engine spawn, no state.json write -- so it is safe to
run at any time, including mid-batch, as a navigation refresher for a human
or Master resuming after a crash.

A run that has never started (no state.json yet) prints
{"initialized": false} instead.

Example:
  lyx webster status`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			st, err := websterengine.LoadState(c.websterDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if st == nil {
				clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
					"initialized": false,
				}))
				return nil
			}

			// Batches is keyed by number for O(1) SaveState mutation
			// elsewhere; status renders it as a stable, number-ordered list
			// instead, since that map iteration order is not deterministic.
			numbers := make([]int, 0, len(st.Batches))
			for n := range st.Batches {
				numbers = append(numbers, n)
			}
			sort.Ints(numbers)

			batches := make([]map[string]any, 0, len(numbers))
			for _, n := range numbers {
				bs := st.Batches[n]
				batches = append(batches, map[string]any{
					"number":     n,
					"slug":       bs.Slug,
					"kind":       bs.Kind,
					"status":     bs.Status,
					"terminal":   bs.Terminal,
					"has_digest": bs.Digest != nil,
				})
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"run_guid":         st.RunGUID,
				"current_batch":    st.CurrentBatch,
				"plan_fingerprint": st.PlanFingerprint,
				"batches":          batches,
				"paused":           builderengine.PauseRequested(c.websterDir),
			}))
			return nil
		},
	}

	return cmd
}
