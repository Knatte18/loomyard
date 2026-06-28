// main.go is the entry point for the lyx CLI.
//
// It assembles a single cobra root command from every module's Command(), wires
// the persistent --json flag and JSON help renderer, seeds the per-invocation
// exit-state holder, and routes stdout/stderr to the appropriate writer. The
// testable run() seam merges stdout and stderr so tests capture all output from
// one buffer; main() keeps stdout and stderr split as callers of the production
// binary expect.

// Package main is the cobra root for the lyx CLI.
// It assembles each module's Command() into a single root, installs --json help,
// and delegates execution to cobra.
package main

import (
	"context"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/board"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configcli"
	"github.com/Knatte18/loomyard/internal/ghissues"
	"github.com/Knatte18/loomyard/internal/ide"
	"github.com/Knatte18/loomyard/internal/initcli"
	"github.com/Knatte18/loomyard/internal/muxpoc"
	"github.com/Knatte18/loomyard/internal/update"
	"github.com/Knatte18/loomyard/internal/warp"
	"github.com/Knatte18/loomyard/internal/weft"
)

func main() {
	root := newRoot()
	// Production path: split stdout and stderr so the terminal sees each on the
	// correct stream. A fresh exit-state holder is allocated per invocation.
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	ctx, es := clihelp.NewExitContext(context.Background())
	if err := root.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
	os.Exit(es.Code())
}

// run is the testable seam: it builds a fresh root via newRoot(), merges stdout
// and stderr into out (so tests capture all cobra text from one buffer), seeds
// the exit-state holder, and returns the process exit code. The merged-output
// contract matches every module's RunCLI seam so tests are symmetric.
func run(args []string, out io.Writer) int {
	root := newRoot()
	// Merge stdout and stderr so tests capture cobra's error text alongside
	// handler output from a single bytes.Buffer.
	root.SetOut(out)
	root.SetErr(out)

	ctx, es := clihelp.NewExitContext(context.Background())
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		return 1
	}
	return es.Code()
}

// newRoot builds and returns the lyx cobra root command with all module
// subcommands added. It registers the persistent --json flag and installs the
// JSON help renderer. SilenceUsage prevents cobra from dumping the full usage
// block on error paths; SilenceErrors is left false so cobra still writes
// "unknown command" / "unknown flag" messages to the error writer.
func newRoot() *cobra.Command {
	// jsonFlag is captured by InstallJSONHelp so the help func can read it.
	var jsonFlag bool

	root := &cobra.Command{
		Use:   "lyx",
		Short: "Loomyard task-tracker CLI",
		Long: `lyx is the CLI for the Loomyard task tracker.

It assembles every module's cobra command tree under a single root so that
all modules are discoverable via "lyx --help" and every subcommand carries
its own --help and --json help output.

Available modules: init, board, config, update, ide, muxpoc, weft, warp, ghissues.`,
		SilenceUsage:  true,
		SilenceErrors: false,
	}

	// --json is a persistent flag on the root so it is inherited by all descendants.
	// InstallJSONHelp reads *jsonFlag inside the HelpFunc it installs.
	root.PersistentFlags().BoolVar(&jsonFlag, "json", false, "emit help as structured JSON instead of plain text")
	clihelp.InstallJSONHelp(root, &jsonFlag)

	// Add every module's Command() as a direct child of the root.
	root.AddCommand(
		initcli.Command(),
		board.Command(),
		configcli.Command(),
		update.Command(),
		ide.Command(),
		muxpoc.Command(),
		weft.Command(),
		warp.Command(),
		ghissues.Command(),
	)

	return root
}
