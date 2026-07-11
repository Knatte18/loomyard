// status.go implements the `status` builder verb: an instant, side-effect-
// free snapshot of _lyx/builder/state.json plus the reports dir -- the
// discussion's navigation refresher, human- and loom-facing. It never
// spawns an agent, never weft-commits, and never mutates state.json; a
// batch whose report has already landed on disk is reported terminal even
// if state.json has not yet been updated by poll's own next tick, so a
// crash between the report landing and the next poll call never shows a
// stale "running" snapshot here.

package buildercli

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// batchReportFileName returns the batch-report filename plan-format.md
// pins for a batch numbered number with slug slug: "NN-<slug>.yaml" --
// matching builderengine's own batchReportFileName/RestartChain naming,
// reimplemented here since that helper is package-private to builderengine.
func batchReportFileName(number int, slug string) string {
	return fmt.Sprintf("%02d-%s.yaml", number, slug)
}

// statusCmd builds the `status` subcommand: LoadState, then for each
// persisted BatchState (sorted by number) an on-disk report scan that can
// promote a not-yet-Terminal batch to its real on-disk status without
// mutating state.json itself -- status is a pure read, so classifying a
// batch here never SaveStates the promotion back; poll's own terminal tick
// is what durably persists it.
func (c *builderCLI) statusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "print an instant snapshot of state.json and the reports dir",
		Long: `status reads _lyx/builder/state.json and reports the run's identity, the
in-flight batch cursor, the plan fingerprint, every batch's own persisted
record (number, slug, status, role, start_sha, terminal), and whether a
pause has been requested. It is a plain read -- no weft commit, no engine
spawn, no state.json write -- so it is safe to run at any time, including
mid-batch, as a navigation refresher for a human or an orchestrator
resuming after a crash.

A run that has never started (no state.json yet) prints
{"initialized": false} instead.

Example:
  lyx builder status`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			st, err := builderengine.LoadState(c.builderDir)
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
				status := bs.Status
				terminal := bs.Terminal

				if !terminal {
					reportPath := filepath.Join(c.reportsDir, batchReportFileName(n, bs.Slug))
					if report, rerr := builderengine.ParseReport(reportPath); rerr == nil {
						status = report.Status
						terminal = true
					}
				}

				batches = append(batches, map[string]any{
					"number":    n,
					"slug":      bs.Slug,
					"status":    status,
					"role":      bs.Role,
					"start_sha": bs.StartSHA,
					"terminal":  terminal,
				})
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"run_guid":         st.RunGUID,
				"current_batch":    st.CurrentBatch,
				"plan_fingerprint": st.PlanFingerprint,
				"batches":          batches,
				"paused":           builderengine.PauseRequested(c.builderDir),
			}))
			return nil
		},
	}

	return cmd
}
