// joinparts.go implements the `join-parts` loom verb: the chair's one call that joins the parts' plans into the plan directory.
// The cobra shell is thin over a verb function that takes its side effects as joinPartsDeps, so the refusal table is testable without a run or a worktree.

package loomcli

import (
	"fmt"
	"io"
	"os"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/spf13/cobra"
)

// joinPartsDeps is every side effect and run fact the join-parts verb reads, injected so tests supply fakes.
type joinPartsDeps struct {
	// row is the name of the recipe's Parts-Plan-Write row.
	row string
	// planDir is the plan directory the join writes.
	planDir string
	// readStatus reads the run's status.
	readStatus reviewStatusReader
	// caller is the calling session's strand name, empty for a session that is no lym strand.
	caller string
	// chair is the strand name the seat engine gives the Parts-Plan-Write row's chair.
	chair string
	// join joins the parts' plans into the plan directory.
	join func() (*planparser.JoinResult, error)
}

// joinPartsRefusal words one refusal of the join-parts verb as `loom: join-parts: <cause>; way forward: <step>`.
func joinPartsRefusal(cause, step string) string {
	return "loom: join-parts: " + cause + "; way forward: " + step
}

// joinPartsVerb joins the parts' plans into the plan directory when the run is at its running Parts-Plan-Write row and the caller is the row's chair.
// It returns the process exit code and emits exactly one envelope on out.
func joinPartsVerb(out io.Writer, d joinPartsDeps) int {
	status, found, err := d.readStatus()
	if err != nil {
		return output.Err(out, joinPartsRefusal(fmt.Sprintf("read the run's status: %v", err), "re-run the verb; nothing is mutated"))
	}
	switch {
	case found && status.CurrentProducer == d.row && status.State != shedengine.StateRunning:
		return output.Err(out, joinPartsRefusal(
			fmt.Sprintf("the join runs only while %s is the running row", d.row),
			"\"lym loom resume\", after which the row's chair runs the join: a live chair is resumed and joins itself, and otherwise the fresh spawn archives every part, rotates the plan directory and has the new chair join"))
	case !found || status.CurrentProducer != d.row:
		return output.Err(out, joinPartsRefusal(
			fmt.Sprintf("the join is the chair's alone, and a running %s row's chair runs it", d.row),
			fmt.Sprintf("past %s the joined plan is edited in place, for example with \"lym loom insert-card\"", d.row)))
	}

	if d.caller != d.chair {
		caller := d.caller
		if caller == "" {
			caller = "a session that is no lym strand"
		}
		return output.Err(out, joinPartsRefusal(
			fmt.Sprintf("the join is the chair's alone, and %s is not the chair of the running %s row", caller, d.row),
			"the chair runs the join once every part passes its check; a part writer reports its finished part to the chair instead"))
	}

	result, err := d.join()
	if err != nil {
		return output.Err(out, "loom: join-parts: "+err.Error())
	}

	cards := make([]map[string]any, len(result.Cards))
	for i, card := range result.Cards {
		cards[i] = map[string]any{"part": card.Part, "from": card.From, "to": card.To, "file": card.File}
	}
	return output.Ok(out, map[string]any{"plan_dir": d.planDir, "cards": cards})
}

// newJoinPartsDeps builds the real facts and side effects of the join-parts verb over the run the loom CLI wired.
// The row, the chair's strand name and the plan context are resolved here and on the join's first call, so the verb's own refusals are decided before any config is read.
func (c *loomCLI) newJoinPartsDeps() (joinPartsDeps, error) {
	row, found, err := loomrecipe.EngineRow(c.recipe, partsPlanWriteEngine)
	if err != nil {
		return joinPartsDeps{}, err
	}
	if !found {
		return joinPartsDeps{}, fmt.Errorf("loom: join-parts does not apply to this run, whose %q recipe has no Parts-Plan-Write row and so no parts to join", c.recipe)
	}
	geometry, err := hubgeom.ReedGeometry(c.location)
	if err != nil {
		return joinPartsDeps{}, err
	}
	chair, err := agentname.Format(geometry.NameShortname, geometry.NameSlug, seatengine.SeatRole(loomengine.PartsPlanRole, ""))
	if err != nil {
		return joinPartsDeps{}, err
	}

	return joinPartsDeps{
		row:        row,
		planDir:    planparser.PlanDir(c.env.AnchorPath),
		readStatus: circlingStatusReader(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath),
		caller:     os.Getenv(agentname.StrandNameEnv),
		chair:      chair,
		join: func() (*planparser.JoinResult, error) {
			pc, err := c.planContext()
			if err != nil {
				return nil, err
			}
			return loomshed.JoinParts(c.env.AnchorPath, c.env.WorktreeRoot, c.env.PlanIndex, pc)
		},
	}, nil
}

// joinPartsCmd builds the `join-parts` subcommand.
func (c *loomCLI) joinPartsCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "join-parts",
		Short:       "join the parts' plans into the plan directory, from the chair of the running Parts-Plan-Write row",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `join-parts re-checks every part's plan with the part check, then numbers the
cards of the parts into one plan in the parts file's order and writes it to the
plan directory, overview last. It takes no arguments and no flags, and reports
the joined cards, one {part, from, to, file} each, under cards.

It runs only from the chair of a running Parts-Plan-Write row. A halted or
later run, a part writer, a Plan-Burler-Fix fixer and Webster cannot reach it.
It overwrites the plan directory, so a rerun within the row discards whatever
the chair added after the last join; run it again only after fixing a part, and
add nothing to the plan directory before the join succeeds.

Example:
  lym loom join-parts`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			deps, err := c.newJoinPartsDeps()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(cmd.OutOrStdout(), "loom: join-parts: "+err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), joinPartsVerb(cmd.OutOrStdout(), deps))
			return nil
		},
	}
}
