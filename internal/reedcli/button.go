// button.go implements the `button` reed verb: it runs one clicked status-bar button of a told session of a told tmux server.
// The status bar's click binding and a button's ask and confirm prompts call it through run-shell; it prints nothing and exits 0 once its flags are valid, since a failed button is shown on the clicking client.
// It carries clihelp.SkipStencilSeedAnnotation, since a button click must never run the root pre-run's stencil-seed pass.

package reedcli

import (
	"errors"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/spf13/cobra"
)

// validateButtonFlags refuses a button call that lacks the socket, tmux path, session, range or client it is told, or a confirmed or answered call that lacks its hash, naming the flag.
func validateButtonFlags(req reedengine.ButtonRequest) error {
	switch {
	case req.SocketPath == "":
		return errors.New("--socket must be a non-empty tmux socket path")
	case req.TmuxPath == "":
		return errors.New("--tmux must be a non-empty path to the tmux binary")
	case req.Session == "":
		return errors.New("--session must be a non-empty tmux session name")
	case req.Range == "":
		return errors.New("--range must be the clicked status range")
	case req.Client == "":
		return errors.New("--client must be the clicking tmux client")
	case req.Confirmed && req.Hash == "":
		return errors.New("--confirmed needs --hash, the hash of the run text the prompt was raised for")
	case req.Answered && req.Hash == "":
		return errors.New("--answered needs --hash, the hash of the run text the prompt was raised for")
	}
	return nil
}

// buttonRunDirectory returns the anchored directory, as recorded now, of the worktree holding dir, which a click runs its entry in.
// It applies no cwd gate and spawns no git.
func buttonRunDirectory(dir string) (string, error) {
	location, err := lymcwd.ResolveWorktree(dir)
	if err != nil {
		return "", err
	}
	return location.AnchorPath(), nil
}

// buttonCmd builds the `button` subcommand.
func (c *reedCLI) buttonCmd() *cobra.Command {
	var req reedengine.ButtonRequest

	cmd := &cobra.Command{
		Use:   "button",
		Short: "run a clicked status-bar button of a tmux session",
		Long: `button runs the status-bar button that the told range names in the told
session: it reads the session's rendered button spec, asks the entry's
ask prompt and then its confirm prompt when it has them, runs the entry's
command in the current anchor of the worktree holding the session's run
directory, logs it and shows a failure on the clicking client.
It reads no working directory, no config and no reed state, and prints
nothing: the status bar's click binding and a button's prompts call it
through tmux's run-shell.

Example:
  lym reed button --socket /tmp/tmux-1000/default --tmux /usr/bin/tmux --session my-worktree --range lymbtn0 --client /dev/pts/3`,
		Annotations: map[string]string{
			clihelp.AudienceAnnotation:        clihelp.AudienceRole,
			clihelp.SkipStencilSeedAnnotation: clihelp.AnnotationEnabled,
		},
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			if err := validateButtonFlags(req); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(cmd.OutOrStdout(), err.Error()))
				return nil
			}
			reedengine.RunButton(req, buttonRunDirectory)
			return nil
		},
	}

	cmd.Flags().StringVar(&req.SocketPath, "socket", "", "path of the tmux server's socket (required)")
	cmd.Flags().StringVar(&req.TmuxPath, "tmux", "", "path to the tmux binary (required)")
	cmd.Flags().StringVar(&req.Session, "session", "", "name of the tmux session the button is in (required)")
	cmd.Flags().StringVar(&req.Range, "range", "", "status range that was clicked (required)")
	cmd.Flags().StringVar(&req.Client, "client", "", "name of the clicking tmux client (required)")
	cmd.Flags().BoolVar(&req.Confirmed, "confirmed", false, "the confirm prompt was answered yes; needs --hash")
	cmd.Flags().BoolVar(&req.Answered, "answered", false, "the ask prompt was answered into the session's answer option; needs --hash")
	cmd.Flags().StringVar(&req.Hash, "hash", "", "hash of the run text the prompt was raised for")
	cmd.Flags().StringVar(&req.Strand, "strand", "", "strand name the prompt was raised for")
	cmd.Flags().StringVar(&req.Answer, "answer", "", "answer to the entry's ask prompt, carried by the confirm prompt")

	return cmd
}
