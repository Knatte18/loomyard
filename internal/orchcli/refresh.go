// refresh.go implements the `refresh` orch verb and the request helper it shares with `distill`:
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
// It removes only the request it wrote, so a request another caller wrote meanwhile stays.
func (c *orchCLI) requestCycle(mode string) (watcherLive bool, err error) {
	request := orchengine.CycleRequest{Mode: mode, RequestedAt: time.Now()}
	if err := orchengine.RequestCycle(c.paths, request.Mode, request.RequestedAt); err != nil {
		return false, err
	}
	watcherLive, err = orchengine.WatcherLive(c.paths)
	if err != nil {
		return false, err
	}
	if !watcherLive {
		if _, err := orchengine.RemoveCycleRequestIf(c.paths, request); err != nil {
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
	fields := map[string]any{
		"requested":    true,
		"watcher_live": watcherLive,
	}
	if watcherLive && c.hubPauseHolds() {
		fields["held_by_pause"] = true
		fields["note"] = "the request waits for the hub pause to be resumed"
	}
	clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
	return nil
}

// refreshCmd builds the `refresh` subcommand.
func (c *orchCLI) refreshCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "refresh",
		Short:       "ask the watcher to clear the orchestrator session now",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `refresh writes a clear-cycle request the watcher picks up at the session's next
idle moment, whatever cycle_mode says. The session writes a note first; only
then does the watcher clear it and resume it from the note. The envelope's
watcher_live tells you at once when no watcher is running to act on it; the
request is then withdrawn. While a hub pause holds the watcher the request
waits for the resume, and the envelope adds held_by_pause true with a note.
A request the watcher has not started within the handoff timeout is
abandoned; once it starts, the session gets the full handoff timeout for the
note from when it reads the note request. Neither timeout counts time a hub
pause held the request.
"lym orch withdraw" removes a pending request, and "lym orch status" shows it
as cycle_request.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.requestCmd(cmd, orchengine.CycleClear)
		},
	}
}
