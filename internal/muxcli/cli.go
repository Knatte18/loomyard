// cli.go builds the cobra command tree for the mux module and the RunCLI
// seam that wires it into the standard io.Writer-based call contract. The
// parent "mux" command carries a PersistentPreRunE that resolves cwd ->
// layout -> config -> *muxengine.Engine exactly once per invocation, into a
// receiver every verb (up.go, add.go, remove.go, status.go, resume.go,
// attach.go) closes over, so no subcommand re-resolves geometry or config
// itself.

package muxcli

import (
	"io"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// muxCLI is the receiver every mux verb hangs off of, so each subcommand's
// RunE reads the same PersistentPreRunE-populated state. eng is the domain
// handle every verb calls into; cfg is kept alongside it because attach
// (card 27) needs the resolved psmux binary path to build its in-place
// attach-session invocation, and Engine exposes no such accessor (only
// Socket/SessionName, its own psmux-facing identity). The zero muxCLI is
// not valid until PersistentPreRunE has populated both fields.
type muxCLI struct {
	eng *muxengine.Engine
	cfg muxengine.Config
}

// Command returns the cobra command tree for the mux module.
//
// The parent "mux" command carries a PersistentPreRunE that resolves cwd ->
// layout -> config -> *muxengine.Engine into c, skipping that resolution
// entirely when the group command itself is invoked (bare "lyx mux" listing
// or an unknown-subcommand error via GroupRunE) so neither path requires a
// git repository. Every verb card (22-27) creates its own (c *muxCLI)
// xCmd() builder and registers it here via parent.AddCommand — this card
// registers no subcommands itself.
func Command() *cobra.Command {
	c := &muxCLI{}

	parent := &cobra.Command{
		Use:   "mux",
		Short: "manage the psmux strand overlay for this worktree",
		Long: `mux drives a per-hub psmux server that lays out one strand per pane for
this worktree's session: adding, removing, resuming, and attaching to
strands, plus rendering their layout on every mutation.`,
		// RunE is set so that bare "lyx mux" lists subcommands and "lyx mux bogus"
		// emits a JSON error envelope instead of falling through to cobra's plain-text help.
		RunE: clihelp.GroupRunE,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Guard: when the mux group command itself is invoked (bare listing or
			// unknown-subcommand error path via GroupRunE), skip cwd/layout/config
			// resolution so that neither path requires a git repository to be present.
			if cmd.Name() == "mux" {
				return nil
			}

			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			cwd, err := hubgeometry.Getwd()
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			layout, err := hubgeometry.Resolve(cwd)
			if err != nil {
				// hubgeometry.Resolve's error is already self-describing (it IS the
				// "not a git repository" sentinel); pass it through bare rather than
				// doubling that same text on top of it.
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			// The _lyx/config/ root is anchored at layout.Cwd, not WorktreeRoot or
			// any weft sibling — mux config lives with the worktree the operator is
			// actually standing in.
			cfg, err := muxengine.LoadConfig(layout.Cwd, "mux")
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			c.eng = muxengine.New(cfg, layout)
			c.cfg = cfg
			return nil
		},
	}

	return parent
}

// RunCLI is the public seam for the mux module CLI.
//
// It delegates to clihelp.Execute with the cobra command tree, passing out as
// the capture writer for all output (including cobra's error text). This
// preserves the existing call contract so that callers and tests are unchanged.
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}
