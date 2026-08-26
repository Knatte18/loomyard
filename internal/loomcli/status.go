// status.go implements the `status` loom verb: a one-shot JSON envelope of the current phase, and a
// --watch mode that tails the same status file, printing a line only when the composed activity
// actually changes rather than once per poll.

package loomcli

import (
	"encoding/json"
	"fmt"
	"io"
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

// statusUnavailableLine is the line the watch tail prints when a poll cannot read the status file.
// It is a constant rather than an inline literal because printStatusLinesOnChange dedupes on the
// printed text, so a transient fault must render byte-identically on every poll or it becomes its
// own flood.
const statusUnavailableLine = "loom status unavailable (status file transiently unreadable)"

// printStatusLinesOnChange runs the watch tail: it polls, prints the polled line into out only when
// that line differs from the one it last printed, and sleeps between polls.
//
// Suppressing an unchanged line is the whole point. A producer call lasts minutes while the tail
// polls every second, so printing unconditionally emits hundreds of byte-identical lines per
// producer: it turns the one-line strand pane manifest/designs/loom.md specifies into a scrolling
// ticker, it fills tmux's scrollback (measured at 434 lines in fifteen minutes, against a 2000-line
// default history limit) so nothing else that pane printed survives, and it buries the one line an
// operator actually needs -- the moment the activity changes -- among the identical ones around it.
//
// polls bounds the loop so a test can drive a finite sequence with no wall-clock wait, exactly as
// awaitRunLock's own attempts argument does; a non-positive polls means poll forever, which is what
// the production call passes.
func printStatusLinesOnChange(out io.Writer, poll func() string, sleep func(), polls int) {
	lastPrinted := ""
	printedAny := false
	for i := 0; polls <= 0 || i < polls; i++ {
		line := poll()
		if !printedAny || line != lastPrinted {
			fmt.Fprintln(out, line)
			lastPrinted = line
			printedAny = true
		}
		sleep()
	}
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

			st, found, err := state.ReadJSONStrict[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: decode status file "+c.shedPaths.StatusPath+": "+err.Error()))
				return nil
			}
			if !found {
				clihelp.SetExit(cmd.Context(), output.Err(out, "loom: no status file at "+c.shedPaths.StatusPath+"; run \"lyx loom run\" first to bootstrap this task"))
				return nil
			}

			if watch {
				// Watch tail: this is the narrow, explicitly-taken interactive-handoff exception --
				// everything fallible already ran above, on the envelope. A read failure inside the
				// poll below must not terminate the tail and must not write an envelope: the pane is
				// expected to survive the driver rewriting the file underneath it.
				poll := func() string {
					polled, polledFound, pollErr := state.ReadJSONStrict[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
					if pollErr != nil || !polledFound {
						return statusUnavailableLine
					}
					return renderStatusLine(polled)
				}
				printStatusLinesOnChange(out, poll, func() { time.Sleep(interval) }, 0)
			}

			var product loomengine.Status
			if len(st.Product) > 0 {
				if err := json.Unmarshal(st.Product, &product); err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, "loom: decode status file "+c.shedPaths.StatusPath+"'s product payload: "+err.Error()))
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
