// cli.go exposes the cobra command tree for the help module: the help command that replaces cobra's built-in one, and its index child.

package helpcli

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
)

// Command returns the help command: `help [<command>...]` prints the help of the named command, and `help index` prints the command index.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "help [<command>...]",
		Short: "print a command's help, or the command index",
		Long: `Prints the help of the named command; bare "lyx help" prints the root's help.
A word that names no subcommand of the command reached so far is refused.
"lyx help index" prints the command index of this binary.`,
		Annotations: map[string]string{
			clihelp.AudienceAnnotation:        clihelp.AudienceOperator,
			clihelp.SkipStencilSeedAnnotation: clihelp.AnnotationEnabled,
		},
		ValidArgsFunction: completeCommandNames,
		RunE:              runHelp,
	}
	cmd.AddCommand(indexCmd())
	return cmd
}

// completeCommandNames completes the names of the children of the command the words so far reach from the root.
func completeCommandNames(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	reached, _, err := cmd.Root().Find(args)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var completions []cobra.Completion
	for _, child := range reached.Commands() {
		if (child.IsAvailableCommand() || child == cmd) && strings.HasPrefix(child.Name(), toComplete) {
			completions = append(completions, cobra.CompletionWithDesc(child.Name(), child.Short))
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

// runHelp resolves the words of args word by word from the root and prints the help of the command they reach.
func runHelp(cmd *cobra.Command, args []string) error {
	if clihelp.ShouldAbort(cmd.Context()) {
		return nil
	}
	target := cmd.Root()
	for _, word := range args {
		next := childNamed(target, word)
		if next == nil {
			clihelp.SetExit(cmd.Context(), output.Err(cmd.OutOrStdout(), fmt.Sprintf("unknown subcommand %q for %q", word, target.CommandPath())))
			return nil
		}
		target = next
	}
	return target.Help()
}

// childNamed returns the child of parent whose name or alias is word, or nil.
func childNamed(parent *cobra.Command, word string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == word || child.HasAlias(word) {
			return child
		}
	}
	return nil
}

// indexCmd returns the `index` child, which prints the command index of the running binary for one audience.
func indexCmd() *cobra.Command {
	var audience string
	cmd := &cobra.Command{
		Use:   "index",
		Short: "print the one-line-per-command index of this binary",
		Long: `Prints a nested markdown list with one line per command, filtered by the audience annotation each command carries.
The default audience is operator.`,
		Annotations: map[string]string{
			clihelp.AudienceAnnotation:        clihelp.AudienceOperator,
			clihelp.SkipStencilSeedAnnotation: clihelp.AnnotationEnabled,
		},
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			if !slices.Contains(clihelp.Audiences, audience) {
				clihelp.SetExit(cmd.Context(), output.Err(cmd.OutOrStdout(), fmt.Sprintf("unknown audience %q; want one of %s", audience, strings.Join(clihelp.Audiences, ", "))))
				return nil
			}
			_, err := io.WriteString(cmd.OutOrStdout(), clihelp.RenderIndex(cmd.Root(), audience))
			return err
		},
	}
	cmd.Flags().StringVar(&audience, "audience", clihelp.AudienceOperator, "audience to list: operator, role or internal")
	return cmd
}

// RunCLI is the public seam for the help module CLI.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn is RunCLI's seam-cwd-carrying sibling: an empty cwd means "read the process cwd", any other value seeds cwd into the execution context.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
