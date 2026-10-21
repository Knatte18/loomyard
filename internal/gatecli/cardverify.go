// cardverify.go implements `lyx gate card-verify <card id>`, which runs one plan card's `**Verify:**` line through the shared card-verify runner.

package gatecli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/cardverify"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// cardVerifyCmd builds `lyx gate card-verify <card id>`.
func cardVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "card-verify <card-id>",
		Short:       "run one plan card's **Verify:** line",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `card-verify reads the plan in this worktree, finds the card named by its number or its N-slug id, and runs the card's **Verify:** line through the same runner webster re-runs at record-batch.
A line that is one "lyx gate test --wait" call runs as written, taking its own slots.
Any other line takes one gate slot of the hub's pool, waiting for it without a cap, and runs under the runner's timeout with the slot's environment.
Outside every hub the line runs unslotted.
The line's output passes through, and the exit code is the line's own; a line that timed out or could not start exits 1.
A card with no **Verify:** line passes with a message saying so.
Run it from the task worktree whose _lyx/plan holds the card.

Examples:

  lyx gate card-verify 4
  lyx gate card-verify 4-gate-run-log`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), runCardVerify(cmd.Context(), cmd.OutOrStdout(), args[0]))
			return nil
		},
	}
}

// runCardVerify runs the card named id and returns the verb's exit code: the line's own, 1 for a refusal or a line that did not exit, and 0 for a pass.
func runCardVerify(ctx context.Context, out io.Writer, id string) int {
	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate card-verify: cannot read the working directory: %v; way forward: re-run the same command", err))
	}
	location, inHub, err := resolveHub(cwd)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate card-verify: %v; way forward: re-run the same command", err))
	}
	if !inHub {
		location, err = lyxcwd.ResolveWorktree(cwd)
		if errors.Is(err, lyxcwd.ErrNotAGitRepo) {
			return output.Err(out, fmt.Sprintf("gate card-verify: %s is in no git worktree, so it holds no plan; way forward: run it from the task worktree whose _lyx/plan holds the card", cwd))
		}
		if err != nil {
			return output.Err(out, fmt.Sprintf("gate card-verify: resolve the worktree of %s: %v; way forward: re-run the same command", cwd, err))
		}
	}

	planDir := planparser.PlanDir(location.AnchorPath())
	if _, err := os.Stat(planparser.PlanOverview(location.AnchorPath())); errors.Is(err, fs.ErrNotExist) {
		return output.Err(out, fmt.Sprintf("gate card-verify: this worktree holds no plan at %s; way forward: run it from the task worktree whose _lyx/plan holds the card", planDir))
	}
	plan, err := planparser.ParsePlan(planDir)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate card-verify: cannot read the plan at %s: %v; way forward: fix the plan, then re-run the same command", planDir, err))
	}
	index := slices.IndexFunc(plan.Cards, func(card planparser.Card) bool { return cardMatches(card, id) })
	if index < 0 {
		known := make([]string, len(plan.Cards))
		for i, card := range plan.Cards {
			known[i] = card.ID()
		}
		return output.Err(out, fmt.Sprintf("gate card-verify: %q names no card of the plan; way forward: pass one of %s, then re-run the same command", id, strings.Join(known, ", ")))
	}
	card := plan.Cards[index]
	if !card.HasVerify || strings.TrimSpace(card.Verify) == "" {
		return output.Ok(out, map[string]any{"message": fmt.Sprintf("card %s has no **Verify:** line; nothing to run", card.ID())})
	}

	runner := cardverify.Runner{Worktree: location.WorktreePath()}
	if inHub {
		runner.Slots = hubgeom.GateSlots(location)
		runner.WaitDir = gateslot.WaitDir(location.AnchorPath())
	}
	how := runner.RunLine(card.Verify, out)
	if how == "" {
		return 0
	}
	fmt.Fprintf(out, "gate card-verify: card %s verify %s %s\n", card.ID(), strings.TrimSpace(card.Verify), how)
	if code, ok := strings.CutPrefix(how, "exited "); ok {
		if parsed, err := strconv.Atoi(code); err == nil && parsed != 0 {
			return parsed
		}
	}
	return 1
}
