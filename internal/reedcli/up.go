// up.go implements the `up` and `down` reed verbs: up boots the substrate (server + session) this
// worktree's strands render into,
// and down tears this worktree's session and persisted state back down (the shared per-hub server
// dies only when this was its last session).
// Neither verb touches a strand's command — up never launches one.

package reedcli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// upCmd builds the `up` subcommand.
func (c *reedCLI) upCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "boot the reed substrate (server + session) for this worktree",
		Long: `up ensures this hub's named tmux server and this worktree's session
exist (booting them if absent, a no-op if already up), then reconciles dead
panes and re-applies the current strand layout.

Before booting, up verifies the configured multiplexer binary meets the
pinned minimum version and exposes the required command surface, failing
loud with a JSON error ({"ok":false,"error":...}) if it does not.

up is substrate-only: it never launches or relaunches a strand command.
Bringing strand content back after a server restart is "lyx reed resume"'s
job, not up's.

Setting debug_log in reed.yaml (or LYX_REED_DEBUG=1) enables server verbose
logging to <hub>/_board/.lyx/logs/, as forensics for unexplained server deaths; it
applies only when this up actually boots the shared per-hub server, and
existing hubs need "lyx config reconcile" after upgrading to adopt the key.

Example:
  lyx reed up`,
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

// downCmd builds the `down` subcommand.
func (c *reedCLI) downCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "kill this worktree's reed session and clear its strand state",
		Long: `down kills this worktree's tmux session and deletes its persisted
strand state. Sibling worktrees sharing the hub's server are untouched;
when this was the server's last session, the now-empty server is shut down
too (down waits until the server process has actually exited). down is
idempotent: calling it again with no session up still succeeds.

When this worktree's state was recorded against a DIFFERENT session that is
still running on the hub's server — after the worktree directory was renamed,
or a .lyx directory was copied here from another worktree — down does not kill
that session (it may be a sibling worktree's live work) but reports it as
"abandonedSession", because deleting the state file removes the only record
naming it. Tear it down by hand if it is not one you need.

Example:
  lyx reed down`,
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

			payload := map[string]any{"session": result.Session}
			// Emitted only when there IS an abandoned session, so the ordinary teardown envelope is
			// unchanged and the key's mere presence is the signal.
			if result.AbandonedSession != "" {
				payload["abandonedSession"] = result.AbandonedSession
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, payload))
			return nil
		},
	}
}
