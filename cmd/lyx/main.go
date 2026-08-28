// main.go is the entry point for the lyx CLI.
//
// It assembles a single cobra root command from every module's Command(), wires the persistent
// --json flag and JSON help renderer, and routes output to the appropriate writer.
// Cobra-level errors (unknown command, bad flag) are wrapped in the JSON envelope by
// clihelp.RunRoot so the caller always receives a machine-parseable error.
// The testable run() seam merges stdout and stderr so tests capture all output from one buffer;
// main() keeps stdout and stderr split as callers of the production binary expect.

// Package main is the cobra root for the lyx CLI.
// It assembles each module's Command() into a single root, installs --json help, and delegates
// execution to cobra via clihelp.RunRoot.
package main

import (
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/boardcli"
	"github.com/Knatte18/loomyard/internal/burlercli"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configcli"
	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/idecli"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomcli"
	"github.com/Knatte18/loomyard/internal/reedcli"
	"github.com/Knatte18/loomyard/internal/selfreportcli"
	"github.com/Knatte18/loomyard/internal/shuttlecli"
	"github.com/Knatte18/loomyard/internal/stencilcli"
	"github.com/Knatte18/loomyard/internal/webstercli"
)

func main() {
	root := newRoot()
	// Production path: split stdout and stderr.
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	code := clihelp.RunRoot(root, os.Stdout)
	// Force-open the durable sink on a non-zero exit for post-mortem inspection.
	logger.NotifyExit(code)
	os.Exit(code)
}

// run is the testable seam: it builds a fresh root, merges stdout/stderr, and returns the exit code.
func run(args []string, out io.Writer) int {
	root := newRoot()
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs(args)
	code := clihelp.RunRoot(root, out)
	// Force-open the durable sink on a non-zero exit.
	logger.NotifyExit(code)
	return code
}

// newRoot builds the lyx cobra root with all module subcommands, --json flag, and JSON help.
func newRoot() *cobra.Command {
	var jsonFlag bool
	var verbosity int

	root := &cobra.Command{
		Use:   "lyx",
		Short: "Loomyard task-tracker CLI",
		Long: `lyx is the CLI for the Loomyard task tracker.

It assembles every module's cobra command tree under a single root so that
all modules are discoverable via "lyx --help" and every subcommand carries
its own --help and --json help output.

Available modules: board, config, ide, reed, fabric, selfreport, shuttle, burler, webster, stencil, loom, run.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		// Modules' PersistentPreRunE hooks run after root's via EnableTraverseRunHooks.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			logger.SetVerbosity(verbosity)
			// Suppress trace minting/export and sink arming under testing.Testing().
			if !testing.Testing() {
				logger.MintOrAdoptAndExport()
				logger.Arm()
			}
			// Seeding must never block a command from running, regardless of its outcome.
			seedStencils(cmd)
			return nil
		},
	}

	root.PersistentFlags().BoolVar(&jsonFlag, "json", false, "emit help as structured JSON instead of plain text")
	clihelp.InstallJSONHelp(root, &jsonFlag)

	root.PersistentFlags().CountVarP(&verbosity, "verbose", "v", "increase log verbosity (-v info, -vv debug)")

	cobra.EnableTraverseRunHooks = true

	root.AddCommand(
		boardcli.Command(),
		configcli.Command(),
		idecli.Command(),
		reedcli.Command(),
		fabriccli.Command(),
		selfreportcli.Command(),
		shuttlecli.Command(),
		burlercli.Command(),
		stencilcli.Command(),
		webstercli.Command(),
		loomcli.Command(),
		// RunAliasCommand registers the same "run" verb as loomcli.Command()'s
		// subtree already carries, a second time, as a bare root child rather
		// than spliced into the argument vector, so it is discoverable in help
		// and covered by the help-tree and registration guards.
		loomcli.RunAliasCommand(),
	)

	return root
}
