// cycle.go implements the `cycle` orch verb and the request helper it shares with `distill`:
// an operator's request that the watcher run one cycle, in a mode the verb picks, at its next idle moment.

package orchcli

import (
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// requestCycle writes the cycle request marker for mode, then checks for a live watcher.
// With no watcher live it removes the marker again, so no later watcher acts on a request made while none ran, and reports false.
func (c *orchCLI) requestCycle(mode string) (watcherLive bool, err error) {
	if err := orchengine.RequestCycle(c.paths, mode, time.Now()); err != nil {
		return false, err
	}
	watcherLive, err = orchengine.WatcherLive(c.paths)
	if err != nil {
		return false, err
	}
	if !watcherLive {
		if err := orchengine.ClearCycleRequest(c.paths); err != nil {
			return false, err
		}
	}
	return watcherLive, nil
}

// requestCmd runs requestCycle for mode and prints the shared envelope.
func (c *orchCLI) requestCmd(cmd *cobra.Command, mode string) error {
	if clihelp.ShouldAbort(cmd.Context()) {
		return nil
	}
	out := cmd.OutOrStdout()
	watcherLive, err := c.requestCycle(mode)
	if err != nil {
		clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
		return nil
	}
	clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
		"requested":    true,
		"watcher_live": watcherLive,
	}))
	return nil
}

// cycleCmd builds the `cycle` subcommand.
func (c *orchCLI) cycleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cycle",
		Short: "ask the watcher to clear the orchestrator session now",
		Long: `cycle writes a clear-cycle request the watcher picks up at the session's next
idle moment, whatever cycle_mode says. The session writes a note first; only
then does the watcher clear it and resume it from the note. The envelope's
watcher_live tells you at once when no watcher is running to act on it; the
request is then withdrawn. A request the watcher has
not finished within the handoff timeout is abandoned.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.requestCmd(cmd, orchengine.CycleClear)
		},
	}
}
