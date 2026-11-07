// cli.go defines the cobra command tree for the selfreport module.
// It also holds RunCLI, the seam that runs the tree in process.

// Package selfreportcli provides the cobra command tree for filing LoomYard bugs and enhancements as GitHub issues.
// The tree is reachable as `lym selfreport create`.
// The create subcommand delegates the GitHub API call to `selfreportengine`.
package selfreportcli

import (
	"io"
	"os"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/selfreportengine"
	"github.com/spf13/cobra"
)

// stdin is the source that runCreate reads body content from when --body is "-".
// Tests replace it with a strings.Reader so the stdin path runs without blocking on OS input.
var stdin io.Reader = os.Stdin

// Command builds the cobra command tree for the selfreport module.
// The parent command has no shared setup, so all flags and logic live on the create subcommand.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "selfreport",
		Short: "self-report a LoomYard bug or enhancement to lym's own repo on GitHub",
		// ArbitraryArgs lets GroupRunE handle unknown subcommands when this group is the cobra root, as in RunCLI.
		Args: cobra.ArbitraryArgs,
		RunE: clihelp.GroupRunE,
	}

	// The RunE closure passes createCmd to runCreate, which reads flags through it.
	// The variable is declared before its literal so that the closure can refer to it.
	var createCmd *cobra.Command
	createCmd = &cobra.Command{
		Use:         "create <title>",
		Short:       "file a self-report issue on the LoomYard repository via the GitHub API",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `create files a new issue on the Knatte18/loomyard GitHub repository via the GitHub REST API.
A GitHub token must be resolvable before running this command: set GH_TOKEN or
GITHUB_TOKEN, or have the gh CLI installed and authenticated as a fallback token
source -- the gh CLI is not the transport itself and is only ever shelled out to
for a token when both environment variables are unset.

Examples:

  lym selfreport create "Crash on empty board" -b -
      Create a bug issue and read the issue body from stdin; "-" means read from stdin.

  lym selfreport create "Add dark mode" --label enhancement
      Create an enhancement issue; --label replaces the default "bug" label.

  lym selfreport create "Panic on nil pointer"
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

// RunCLI runs the selfreport command tree on args in process, writing its output to out.
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}

// runCreate runs the create subcommand for title and writes the result to out.
// It resolves the body from --body, or from stdin when --body is "-".
// It applies the default "bug" label when no --label is given.
// The GitHub call is made by `selfreportengine.CreateIssue`.
func runCreate(out io.Writer, args []string, cmd *cobra.Command) int {
	title := args[0]

	// Resolve the body pointer.
	// nil means --body was not supplied, so CreateIssue omits the "body" field and GitHub applies its own default.
	// Changed reports whether the flag was set, so --body "" gives an empty body rather than none.
	var body *string
	if cmd.Flags().Changed("body") {
		bodyStr, _ := cmd.Flags().GetString("body")
		if bodyStr == "-" {
			// Read all of stdin, through the stdin variable that tests replace.
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

	// Default to the "bug" label when no --label flag is given.
	// Defect reports against LoomYard are the primary use of this command.
	labels, _ := cmd.Flags().GetStringArray("label")
	if len(labels) == 0 {
		labels = selfreportengine.DefaultLabels()
	}

	url, number, err := selfreportengine.CreateIssue(title, body, labels)
	if err != nil {
		return output.Err(out, err.Error())
	}

	// The success envelope always carries url.
	// It carries number only when CreateIssue returned a non-zero issue number.
	fields := map[string]any{"url": url}
	if number != 0 {
		fields["number"] = number
	}
	return output.Ok(out, fields)
}
