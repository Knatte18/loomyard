// switch.go implements the `switch` reed verb: it moves one told client to the next or previous session of a told tmux server in session-id order.
// The key bindings call it through run-shell; it prints nothing on success, so run-shell opens no output view.
// It carries clihelp.SkipStencilSeedAnnotation, since a key press must never run the root pre-run's stencil-seed pass.

package reedcli

import (
	"errors"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/spf13/cobra"
)

// validateSwitchFlags refuses a switch call that does not name exactly one direction, or lacks the socket, client or tmux path it is told.
func validateSwitchFlags(next, prev bool, socketPath, client, tmuxPath string) error {
	if next == prev {
		return errors.New("exactly one of --next and --prev is required")
	}
	if socketPath == "" {
		return errors.New("--socket must be a non-empty tmux socket path")
	}
	if client == "" {
		return errors.New("--client must be a non-empty tmux client name")
	}
	if tmuxPath == "" {
		return errors.New("--tmux must be a non-empty path to the tmux binary")
	}
	return nil
}

// switchCmd builds the `switch` subcommand.
func (c *reedCLI) switchCmd() *cobra.Command {
	var next, prev bool
	var socketPath, client, tmuxPath string

	cmd := &cobra.Command{
		Use:   "switch",
		Short: "switch a tmux client to the next or previous session of the server",
		Long: `switch moves the told client to the next or previous session of the
server listening on the told socket, in numeric session-id order and
wrapping at either end. It reads no working directory, no config and no
state, and prints nothing on success: the Alt-key bindings call it through
tmux's run-shell.

Example:
  lyx reed switch --next --socket /tmp/tmux-1000/default --client /dev/pts/3 --tmux /usr/bin/tmux`,
		Annotations: map[string]string{
			clihelp.SkipStencilSeedAnnotation: clihelp.AnnotationEnabled,
		},
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			if err := validateSwitchFlags(next, prev, socketPath, client, tmuxPath); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if err := reedengine.SwitchClient(tmuxPath, socketPath, client, next); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&next, "next", false, "switch to the next session in id order")
	cmd.Flags().BoolVar(&prev, "prev", false, "switch to the previous session in id order")
	cmd.Flags().StringVar(&socketPath, "socket", "", "path of the tmux server's socket (required)")
	cmd.Flags().StringVar(&client, "client", "", "name of the tmux client to switch (required)")
	cmd.Flags().StringVar(&tmuxPath, "tmux", "", "path to the tmux binary (required)")

	return cmd
}
