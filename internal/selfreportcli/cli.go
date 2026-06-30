// cli.go assembles the Cobra command tree for the selfreport module and wires the
// RunCLI seam. The "create" subcommand's RunE closure reads flag state via the
// named createCmd variable to distinguish "flag not set" from "flag set to empty
// string" using Changed("body"), following the warp module pattern for local-flag
// access inside a cobra RunE.

// Package selfreportcli provides the cobra command tree for filing LoomYard bugs
// and enhancements as GitHub issues directly from lyx.exe. The module is
// reachable as `lyx selfreport create` and delegates the actual gh invocation to
// the selfreportengine domain package.
package selfreportcli

import (
	"io"
	"os"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/selfreportengine"
	"github.com/spf13/cobra"
)

// stdin is the seam that runCreate reads for body content when --body is "-".
// Tests replace this with a strings.Reader to exercise the stdin path without
// blocking on real OS input.
var stdin io.Reader = os.Stdin

// Command builds the cobra command tree for the selfreport module.
//
// The parent command has no PersistentPreRunE and no persistent flags because
// selfreport requires no shared setup — the only verb is "create", which is fully
// self-contained and talks to a hardcoded external service (gh CLI +
// Knatte18/loomyard). Per-verb flags are declared as local flags on the "create"
// subcommand; body and label resolution happens inside that subcommand's RunE.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "selfreport",
		Short: "self-report a LoomYard bug or enhancement to lyx's own repo via gh",
	}

	// createCmd is declared as a named variable so that its RunE closure can
	// call createCmd.Flags().Changed("body") to detect whether --body was
	// explicitly provided. This follows the warp module pattern for reading
	// local flags from within a cobra RunE.
	var createCmd *cobra.Command
	createCmd = &cobra.Command{
		Use:   "create <title>",
		Short: "file a self-report issue on the LoomYard repository via gh",
		Long: `create files a new issue on the Knatte18/loomyard GitHub repository via the gh CLI.
The gh CLI must be installed and authenticated before running this command.

Examples:

  lyx selfreport create "Crash on empty board" -b -
      Create a bug issue and read the issue body from stdin; "-" means read from stdin.

  lyx selfreport create "Add dark mode" --label enhancement
      Create an enhancement issue; --label replaces the default "bug" label.

  lyx selfreport create "Panic on nil pointer"
      Create a bug issue with no body; the default "bug" label is applied automatically.`,
		Args: cobra.ExactArgs(1),
		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int {
			return runCreate(out, args, createCmd)
		}),
	}
	createCmd.Flags().StringP("body", "b", "", `issue body text; use "-" to read from stdin`)
	createCmd.Flags().StringArray("label", []string{}, `label to apply (repeatable); defaults to "bug" when omitted`)
	cmd.AddCommand(createCmd)

	return cmd
}

// RunCLI is the public seam for the selfreport module.
//
// It delegates to clihelp.Execute(Command(), out, args) so in-process tests can
// capture all output via a single io.Writer. Returns the exit code (0 on success,
// 1 on error).
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}

// runCreate executes the logic for "selfreport create <title>".
//
// It reads the title from args[0], resolves the body from the --body flag (nil
// when the flag was not provided, full stdin content when set to "-", or the flag
// string directly for any other value), applies the default "bug" label when no
// --label flags are given, and delegates to selfreportengine.CreateIssue. The cmd parameter is the
// cobra command for the "create" subcommand; the closure passes it so that
// Changed("body") can distinguish "flag not set" from "flag set to empty string".
func runCreate(out io.Writer, args []string, cmd *cobra.Command) int {
	title := args[0]

	// Resolve the body pointer. nil means --body was not supplied, which causes
	// buildCreateArgs to omit the --body flag entirely so gh uses its default
	// (no body). Using Changed() rather than comparing to "" correctly handles
	// --body "" as an explicit empty body vs. the flag not being set at all.
	var body *string
	if cmd.Flags().Changed("body") {
		bodyStr, _ := cmd.Flags().GetString("body")
		if bodyStr == "-" {
			// Read the entire stdin seam; white-box tests replace the stdin
			// package variable with a strings.Reader to avoid blocking on
			// real OS input.
			content, err := io.ReadAll(stdin)
			if err != nil {
				return output.Err(out, "failed to read stdin: "+err.Error())
			}
			s := string(content)
			body = &s
		} else {
			body = &bodyStr
		}
	}

	// Default to "bug" when the caller omits all --label flags, as defect
	// reports against LoomYard are the primary use case for this command.
	labels, _ := cmd.Flags().GetStringArray("label")
	if len(labels) == 0 {
		labels = []string{"bug"}
	}

	url, number, err := selfreportengine.CreateIssue(title, body, labels)
	if err != nil {
		return output.Err(out, err.Error())
	}

	// Build the success envelope: url is always included; number is included
	// only when the gh output URL's trailing path segment parsed as an integer.
	fields := map[string]any{"url": url}
	if number != 0 {
		fields["number"] = number
	}
	return output.Ok(out, fields)
}
