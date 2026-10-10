// rebaseline.go implements the `rebaseline` webster verb: the operator's way to accept a plan edit made mid-run without discarding any batch record.
// It runs websterengine.Rebaseline under the state-mutation lease (load, restamp, save, release), then fabric-syncs the restamped state.json.
package webstercli

import (
	"fmt"
	"strconv"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// rebaselineCmd builds the `rebaseline` subcommand.
func (c *websterCLI) rebaselineCmd() *cobra.Command {
	var cardFlags []string
	cmd := &cobra.Command{
		Use:         "rebaseline",
		Short:       "accept an on-disk plan edit as the run's plan without dropping batch records",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `rebaseline accepts the plan on disk as the run's plan after a mid-run edit,
keeping every batch record.
Batches up to the last begun one stay as recorded; the cards after them are
re-batched by the active batcher.yaml profile.
It refuses, leaving state.json untouched, when the edit changes the cards of a
batch the run already begun (a begun card's content counts, not only its id),
or removes such a batch; the way forward then is
to restore those cards, or to run "lyx webster reset --to start" and then
"lyx webster run".
A named card of a batch that is failed, dead or stuck is the exception: its
edit is accepted, and "lyx webster recover-batch NN" then runs on the edited
card. A named card of an in-flight batch is accepted too, and the envelope's
cards_amended lists it: the running fork or recovery strand keeps working on
the old text, and recovery re-runs the batch on the edited card. An edited
card of a done batch is refused; the way forward is to restore the card, or
to add a follow-up card after the last begun batch that carries the decision,
with its Card Index line, and run rebaseline naming it.
The operator names every card the edit changed with --card (repeatable);
an edited card that is not named is refused.
A change to 00-overview.md is accepted only inside its "## Card Index"
section, so a follow-up card and its index line land together; the index
change is held to the same rule as the cards, so only cards after the last
begun batch can be added, removed or reordered. The rest of the overview,
which carries the plan's integration verify, is never accepted.
On success the envelope carries previous_fingerprint, plan_fingerprint,
batches_kept and cards_accepted.

Example:
  lyx webster rebaseline --card 05 --card 07`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			cards := make([]int, 0, len(cardFlags))
			for _, v := range cardFlags {
				n, convErr := strconv.Atoi(v)
				if convErr != nil || n <= 0 {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: --card %q is not a card number; way forward: re-run \"lyx webster rebaseline\" naming each changed card by its number, such as --card 5 or --card 05", v)))
					return nil
				}
				cards = append(cards, n)
			}

			plan, err := planparser.ParsePlan(c.geom.PlanDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			mutateLock, err := websterengine.AcquireStateMutation(c.geom.ScratchDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			mutateHeld := true
			defer func() {
				if mutateHeld {
					_ = mutateLock.Release()
				}
			}()

			st, err := websterengine.LoadState(c.geom.WebsterDir, c.geom.ScratchDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if st == nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, `webster: no run in progress; run "lyx webster run" first`))
				return nil
			}

			base, err := websterengine.MerriamBase(c.geom)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			result, err := websterengine.Rebaseline(websterengine.RebaselineDeps{Plan: plan, Active: c.batcher, Sizes: batcher.DiskSizes(c.geom.WorktreeRoot), Base: base, State: st, Cards: cards, Geom: c.geom})
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if err := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, st); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			_ = mutateLock.Release()
			mutateHeld = false

			if _, syncErr := fabricSync(c.openFabric, c.anchorRel, "rebaseline"); syncErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: plan rebaselined but the fabric sync failed: %v; %s", syncErr, fabricSyncWayForward)))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"previous_fingerprint": result.PreviousFingerprint,
				"plan_fingerprint":     result.Fingerprint,
				"batches_kept":         result.BatchesKept,
				"cards_accepted":       result.CardsAccepted,
				"cards_amended":        result.CardsAmended,
			}))
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&cardFlags, "card", nil, "card number the edit changed (repeatable)")
	return cmd
}
