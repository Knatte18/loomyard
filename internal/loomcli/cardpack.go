// cardpack.go implements the `card-pack` loom verb, which prints one plan card's kick-start pack as plain markdown for a per-card webster fork.
// It is a thin caller of websterengine.CardPackMarkdown.

package loomcli

import (
	"fmt"
	"strconv"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// cardPackCmd builds the `card-pack` subcommand.
// It reads the plan and the tree and writes nothing to webster, plan or tracked state.
func (c *loomCLI) cardPackCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "card-pack <card>",
		Short:       "print one plan card's kick-start pack against the current tree",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `card-pack prints card N's kick-start pack as plain markdown: each member
glyph the card names, with the file, line span and signature of its
declarations as the worktree stands now, committed or not, then the refs that
did not resolve. It reads the plan afresh and no webster state, mutates
nothing and holds no lock.

A per-card webster fork calls it before the card's first edit. The card
argument is the card's number. A card number the plan lacks, a plan that does
not parse and a missing plan directory are refused, each naming the way
forward: locate the card's glyphs with "lyx quarry resolve" or a grep.

Example:
  lyx loom card-pack 3`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			number, err := strconv.Atoi(args[0])
			if err != nil || number == 0 {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("loom: card-pack: the card argument %q is not a card number; %s", args[0], websterengine.CardPackWayForward)))
				return nil
			}

			markdown, err := websterengine.CardPackMarkdown(c.env.PlanIndex, planparser.PlanDir(c.env.AnchorPath), c.env.AnchorPath, c.env.StencilsDir, number)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			_, _ = out.Write(markdown)
			clihelp.SetExit(cmd.Context(), 0)
			return nil
		},
	}
}
