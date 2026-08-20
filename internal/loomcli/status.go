// status.go implements the `status` loom verb: a one-shot JSON envelope of the current phase, and a
// --watch mode that tails the same status file as a one-line-per-poll terminal keepalive.

package loomcli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/spf13/cobra"
)

// renderStatusLine composes st into exactly one line: the literal prefix "loom", then the state,
// then " | now " and the activity's now field, then " | last " and the last field only when it is
// non-empty, then " | wait " and the wait field only when it is non-empty.
//
// The format is pinned to this exact string rather than left to judgment because a test asserts it,
// mirroring how shedengine itself pins composeActivity's own Last field format.
func renderStatusLine(st shedengine.Status) string {
	line := fmt.Sprintf("loom %s | now %s", st.State, st.Activity.Now)
	if st.Activity.Last != "" {
		line += " | last " + st.Activity.Last
	}
	if st.Activity.Wait != "" {
		line += " | wait " + st.Activity.Wait
	}
	return line
}

// statusCmd builds the `status` subcommand.
func (c *loomCLI) statusCmd() *cobra.Command {
	var watch bool
	var interval time.Duration

	cmd := &cobra.Command{
		Use:   "status",
		Short: "report loom's current phase, once or as a live-tailed watch",
		Long: `status reports the current phase-machine state.

Without --watch, it reads the status file once and emits a single JSON
envelope carrying the current producer, state, error text, pause flag,
composed activity, history length, and the task's slug/parent.

With --watch, it performs the same read once as a pre-flight, then prints
one line per poll to the terminal and never exits -- this is the one
documented interactive-handoff exception on this verb, taken narrowly on
the tail only, after every fallible step has already run pre-flight.

Example:
  lyx loom status
  lyx loom status --watch
  lyx loom status --watch --interval 200ms`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			st, found, err := state.ReadJSONStrict[shedengine.Status](c.deps.StatusPath, c.deps.StatusLockPath)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: decode status file "+c.deps.StatusPath+": "+err.Error()))
				return nil
			}
			if !found {
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: no status file at "+c.deps.StatusPath+"; run \"lyx loom run\" first to bootstrap this task"))
				return nil
			}

			if watch {
				// Watch tail: this is the narrow, explicitly-taken interactive-handoff exception --
				// everything fallible already ran above, on the envelope. A read failure inside the
				// loop below must not terminate it and must not write an envelope: the pane is
				// expected to survive the driver rewriting the file underneath it.
				for {
					polled, polledFound, pollErr := state.ReadJSONStrict[shedengine.Status](c.deps.StatusPath, c.deps.StatusLockPath)
					switch {
					case pollErr != nil || !polledFound:
						fmt.Fprintln(out, "loom status unavailable (status file transiently unreadable)")
					default:
						fmt.Fprintln(out, renderStatusLine(polled))
					}
					time.Sleep(interval)
				}
			}

			var product loomengine.Status
			if len(st.Product) > 0 {
				if err := json.Unmarshal(st.Product, &product); err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, "loom: decode status file "+c.deps.StatusPath+"'s product payload: "+err.Error()))
					return nil
				}
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"current_producer": st.CurrentProducer,
				"state":            string(st.State),
				"error":            st.Error,
				"pause_requested":  st.PauseRequested,
				"activity":         st.Activity,
				"history_length":   len(st.History),
				"slug":             product.Slug,
				"parent":           product.Parent,
			}))
			return nil
		},
	}

	cmd.Flags().BoolVar(&watch, "watch", false, "tail the status file one line per poll instead of emitting a single JSON envelope")
	cmd.Flags().DurationVar(&interval, "interval", time.Second, "poll interval for --watch; exists so a test can drive the poll fast without a real wall-clock wait")

	return cmd
}
