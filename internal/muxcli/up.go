// up.go implements the `up` and `down` mux verbs: up boots the substrate
// (server + session) this worktree's strands render into, and down tears
// this worktree's session and persisted state back down (the shared per-hub
// server dies only when this was its last session). Neither verb touches a
// strand's command — up never launches one.

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

Before booting, up verifies the configured multiplexer binary meets the
pinned minimum version and exposes the required command surface, failing
loud with a JSON error ({"ok":false,"error":...}) if it does not.

up is substrate-only: it never launches or relaunches a strand command.
Bringing strand content back after a server restart is "lyx mux resume"'s
job, not up's.

Setting debug_log in mux.yaml (or LYX_MUX_DEBUG=1) enables server verbose
logging to <hub>/.lyx/logs/, as forensics for unexplained server deaths; it
applies only when this up actually boots the shared per-hub server, and
existing hubs need "lyx config reconcile" after upgrading to adopt the key.

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

// downCmd builds the `down` subcommand: kills this worktree's psmux session
// and clears its persisted strand state.
func (c *muxCLI) downCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "kill this worktree's mux session and clear its strand state",
		Long: `down kills this worktree's psmux session and deletes its persisted
strand state. Sibling worktrees sharing the hub's server are untouched;
when this was the server's last session, the now-empty server is shut down
too (down waits until the server process has actually exited). down is
idempotent: calling it again with no session up still succeeds.

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
