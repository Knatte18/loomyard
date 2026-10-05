// watch.go implements the hidden `watch` orch verb: the detached daemon `start` launches, which polls the orchestrator session and runs the handoff cycle until the strand is gone.

package orchcli

import (
	"errors"
	"os/signal"
	"syscall"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// realClock is the wall clock.
type realClock struct{}

// Now returns the current time.
func (realClock) Now() time.Time { return time.Now() }

// watchCmd builds the `watch` subcommand, hidden because start launches it and an operator never needs to.
func (c *orchCLI) watchCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "watch",
		Short:  "run the orchestrator watcher daemon (launched by start)",
		Hidden: true,
		Long: `watch polls the orchestrator session and runs the handoff cycle until the strand
is gone or it is signalled. start launches it detached with its output in watch.log.
A second watcher finding the first alive reports already_running and exits.`,
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
			if st.Strand == "" {
				clihelp.SetExit(cmd.Context(), output.Err(out, "orch: no orchestrator strand is recorded; nothing to watch -- run `lyx orch start` first"))
				return nil
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			watcher := orchengine.NewWatcher(runnerSession{runner: c.runner, strands: c.strands}, c.cfg, c.paths, c.stencilsDir, orchSkills, realClock{})
			runErr := watcher.Run(ctx, time.Sleep)

			if errors.Is(runErr, orchengine.ErrWatcherRunning) {
				clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{"already_running": true}))
				return nil
			}
			exit := ""
			if final, loadErr := orchengine.LoadState(c.paths); loadErr == nil {
				exit = final.WatcherExit
			}
			if runErr != nil {
				clihelp.SetExit(cmd.Context(), output.ErrFields(out, runErr.Error(), map[string]any{"watcher_exit": exit}))
				return nil
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"already_running": false,
				"watcher_exit":    exit,
			}))
			return nil
		},
	}
}
