// up.go implements the `up` and `down` mux verbs: up boots the substrate
// (server + session) this worktree's strands render into, and down tears it
// back down. Neither verb touches a strand's command — up never launches
// one, and down deletes the whole persisted state along with the server.

package muxcli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// upCmd builds the `up` subcommand: ensures this hub's named psmux server
// and this worktree's session exist, then reconciles and re-applies the
// current strand layout. It follows Idiom B — ShouldAbort guard, engine
// call, error/success envelope, always return nil.
func (c *muxCLI) upCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "boot the mux substrate (server + session) for this worktree",
		Long: `up ensures this hub's named psmux server and this worktree's session
exist (booting them if absent, a no-op if already up), then reconciles dead
panes and re-applies the current strand layout.

up is substrate-only: it never launches or relaunches a strand command.
Bringing strand content back after a server restart is "lyx mux resume"'s
job, not up's.

Example:
  lyx mux up`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			result, err := c.eng.Up()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"session": result.Session,
				"socket":  result.Socket,
				"strands": result.Strands,
			}))
			return nil
		},
	}
}

// downCmd builds the `down` subcommand: kills this hub's named psmux server
// and clears this worktree's persisted strand state.
func (c *muxCLI) downCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "kill the mux server and clear this worktree's strand state",
		Long: `down kills this hub's named psmux server and deletes the persisted
strand state for this worktree. down is idempotent: calling it again with
no server up still succeeds.

Example:
  lyx mux down`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			result, err := c.eng.Down()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"session": result.Session,
			}))
			return nil
		},
	}
}
