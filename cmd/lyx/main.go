// main.go is the entry point for the lyx CLI.
//
// It assembles a single cobra root command from every module's Command(), wires
// the persistent --json flag and JSON help renderer, and routes output to the
// appropriate writer. Cobra-level errors (unknown command, bad flag) are wrapped
// in the JSON envelope by clihelp.RunRoot so the caller always receives a
// machine-parseable error. The testable run() seam merges stdout and stderr so
// tests capture all output from one buffer; main() keeps stdout and stderr split
// as callers of the production binary expect.

// Package main is the cobra root for the lyx CLI.
// It assembles each module's Command() into a single root, installs --json help,
// and delegates execution to cobra via clihelp.RunRoot.
package main

import (
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
	// correct stream. Errors are wrapped in the JSON envelope by RunRoot and
	// written to os.Stdout (matching where domain errors land) so callers can
	// parse all error output from a single stream.
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	os.Exit(clihelp.RunRoot(root, os.Stdout))
}

// run is the testable seam: it builds a fresh root via newRoot(), merges stdout
// and stderr into out (so tests capture all cobra text from one buffer), and
// returns the process exit code. RunRoot handles exit-state seeding and JSON
// error wrapping. The merged-output contract matches every module's RunCLI seam
// so tests are symmetric.
func run(args []string, out io.Writer) int {
	root := newRoot()
	// Merge stdout and stderr so tests capture cobra's error text alongside
	// handler output from a single bytes.Buffer.
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs(args)
	return clihelp.RunRoot(root, out)
}

// newRoot builds and returns the lyx cobra root command with all module
// subcommands added. It registers the persistent --json flag and installs the
// JSON help renderer. SilenceUsage and SilenceErrors are set here and reinforced
// by RunRoot so cobra never double-emits plain-text errors alongside the JSON
// envelope that RunRoot writes to stdout.
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
		SilenceErrors: true,
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
