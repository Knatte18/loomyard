// state.go implements the read-only `state` shuttle verb: it lists the session state of every running run in the current worktree, read from files.
// It writes no file and changes no run or session.

package shuttlecli

import (
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/spf13/cobra"
)

// stateCmd builds the `state` subcommand, listing the session state of each running run.
func (c *shuttleCLI) stateCmd() *cobra.Command {
	var strand string
	cmd := &cobra.Command{
		Use:   "state",
		Short: "show what each running shuttle run's agent session is doing",
		Long: `state lists, for every run of this worktree whose record reads running, the
state of its agent session, read from the run's files alone: the strand name
and guid, the state, its cause, the detail (an API error's text), the
outstanding background tasks of a busy session, since when (RFC 3339), and the
history of states the reading passed through.

The states are busy, idle-done, idle-stalled, asking, dead and unknown.
The state model is in shadow mode: it is read here and logged beside the wait
loop's own judgment, and no stop, notice or return depends on it yet.

--strand narrows the list to the run whose strand name or guid equals the
value; a value that matches no running run prints an empty list, since a
finished run is not an error.

Example:
  lyx shuttle state --strand hub:task:driver`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			readings, err := shuttleengine.ReadSessionStates(c.cfg, c.anchorPath, c.engine, time.Now())
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			runs := []map[string]any{}
			for _, reading := range readings {
				if strand != "" && reading.StrandName != strand && reading.StrandGUID != strand {
					continue
				}
				history := []map[string]any{}
				for _, state := range reading.History {
					history = append(history, stateFields(state))
				}
				run := stateFields(reading.State)
				run["strand"] = reading.StrandName
				run["guid"] = reading.StrandGUID
				run["history"] = history
				runs = append(runs, run)
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{"runs": runs}))
			return nil
		},
	}
	cmd.Flags().StringVar(&strand, "strand", "", "narrow the list to the run with this strand name or guid")
	return cmd
}

// stateFields renders one session state as the envelope fields the verb prints for the current state and for each history entry.
func stateFields(state shuttleengine.SessionState) map[string]any {
	since := ""
	if !state.Since.IsZero() {
		since = state.Since.UTC().Format(time.RFC3339)
	}
	outstanding := []map[string]any{}
	for _, task := range state.Outstanding {
		outstanding = append(outstanding, map[string]any{"kind": string(task.Kind), "id": task.ID, "label": task.Label})
	}
	return map[string]any{
		"state":       string(state.Name),
		"cause":       state.Cause,
		"detail":      state.Detail,
		"outstanding": outstanding,
		"since":       since,
	}
}
