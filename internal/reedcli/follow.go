// follow.go implements the `follow` reed verb: it turns follow on for a told session and selects its relevant window.
// The follow button calls it through run-shell; it prints nothing on success, so run-shell opens no output view.
// It carries clihelp.SkipStencilSeedAnnotation so that a button click never runs the root pre-run's stencil-seed pass.

package reedcli

import (
	"errors"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/spf13/cobra"
)

// validateFollowFlags refuses a follow call whose socket, session or tmux value is empty, naming the flag.
func validateFollowFlags(socketPath, session, tmuxPath string) error {
	if socketPath == "" {
		return errors.New("--socket must be a non-empty tmux socket path")
	}
	if session == "" {
		return errors.New("--session must be a non-empty tmux session name")
	}
	if tmuxPath == "" {
		return errors.New("--tmux must be a non-empty path to the tmux binary")
	}
	return nil
}

func (c *reedCLI) followCmd() *cobra.Command {
	var socketPath, session, tmuxPath string

	cmd := &cobra.Command{
		Use:   "follow",
		Short: "turn follow on for a tmux session and select its relevant window",
		Long: `follow turns the follow state of the told session on and selects the
session's relevant window: the newest live agent window, else the
lowest-index overview window.
It reads no working directory, no config and no state, and prints nothing
on success: the status bar's follow button calls it through tmux's
run-shell.

Example:
  lym reed follow --socket /tmp/tmux-1000/default --session my-worktree --tmux /usr/bin/tmux`,
		Annotations: map[string]string{
			clihelp.AudienceAnnotation:        clihelp.AudienceRole,
			clihelp.SkipStencilSeedAnnotation: clihelp.AnnotationEnabled,
		},
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			if err := validateFollowFlags(socketPath, session, tmuxPath); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if err := reedengine.FollowSession(tmuxPath, socketPath, session); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&socketPath, "socket", "", "path of the tmux server's socket (required)")
	cmd.Flags().StringVar(&session, "session", "", "name of the tmux session to follow (required)")
	cmd.Flags().StringVar(&tmuxPath, "tmux", "", "path to the tmux binary (required)")

	return cmd
}
