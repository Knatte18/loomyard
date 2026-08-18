// beginbatch.go implements the `begin-batch` webster verb: Master's own bracket call immediately
// before forking a batch's implementer.
// It runs websterengine.BeginBatch under the state-mutation lease (load, mutate, save, release),
// then performs the first of webster's four fabric-commit points (see the discussion's fabric-ownership
// decision) -- state.json (the batch's start-SHA and record) durable before Master ever forks.
// The freshly-written fork prompt is deliberately NOT part of that commit: the sync pathspec
// excludes prompts/* as machine-local re-renderable artifacts (BeginBatch rewrites a batch's own
// prompt on every begin).
// ErrPaused is an operational refusal, not a hard error: Master reads the {"paused": true} envelope
// and writes its own outcome.yaml with outcome: paused, webster's own pausedEnvelope
// pattern with an exit-0 success code, since begin-batch's own pause refusal is a steady-state
// signal Master's prompt is written to expect on every call, not a failure.
package webstercli

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// beginBatchCmd builds the `begin-batch <NN>` subcommand.
func (c *websterCLI) beginBatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "begin-batch <NN>",
		Short: "Master's bracket call immediately before forking one batch's implementer",
		Long: `begin-batch <NN> checks the webster pause flag (refusing with a
"paused": true envelope if "lyx webster pause" was called), refuses loud
when the batch's report file already exists (finished work is never
silently overwritten -- a stuck batch escalates via recover-batch), records
the batch's start-SHA in state.json, idempotently asserts Master's model for
this batch (a repeated call for the same batch never re-injects a switch
Master's pane is already running), renders and writes that batch's fork
prompt (carrying the previous batch's own persisted digest), and returns the
prompt path Master forwards to its Agent-tool fork call verbatim.

Example:
  lyx webster begin-batch 3`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			batchNumber, err := strconv.Atoi(args[0])
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: %q is not a valid batch number: %v", args[0], err)))
				return nil
			}

			plan, err := planparser.ParsePlan(c.planDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			batches := c.batcher.Batch(plan.Cards)

			// Hold the state-mutation lease across the whole load ->
			// BeginBatch (guards + mutate) -> SaveState sequence: every
			// holder's section is bounded, so the blocking acquire is
			// always short.
			mutateLock, err := websterengine.AcquireStateMutation(c.websterScratchDir)
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

			st, err := websterengine.LoadState(c.websterDir, c.websterScratchDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if st == nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, `webster: no run in progress; run "lyx webster run" first`))
				return nil
			}

			deps := websterengine.BeginDeps{
				Plan:     plan,
				Batches:  batches,
				State:    st,
				Roles:    c.roles,
				Config:   c.cfg,
				Engine:   c.engine,
				Injector: c.injector,
				Reed:     c.reed,
				Geom:     c.geom,
			}

			result, err := websterengine.BeginBatch(deps, batchNumber)
			if err != nil {
				// Nothing to persist: BeginBatch mutates deps.State only on
				// its success path, so the lease is released with no
				// SaveState and no fabric commit on every error branch below.
				_ = mutateLock.Release()
				mutateHeld = false

				if errors.Is(err, websterengine.ErrPaused) {
					clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{"paused": true}))
					return nil
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			if err := websterengine.SaveState(c.websterDir, c.websterScratchDir, st); err != nil {
				_ = mutateLock.Release()
				mutateHeld = false
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			// The state phase is over; release the lease before the fabric
			// push below so a network-slow push never serializes another
			// verb's state mutation behind it.
			_ = mutateLock.Release()
			mutateHeld = false

			if _, syncErr := fabricSync(c.layout, fmt.Sprintf("begin-batch %s", result.BatchName)); syncErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: batch %s begun but the fabric sync failed: %v", result.BatchName, syncErr)))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"batch":       result.BatchName,
				"prompt_path": result.PromptPath,
				"start_sha":   result.StartSHA,
				"model":       result.AssertedModel,
				"warnings":    ownerlessRunWarnings(c.websterScratchDir, nil),
			}))
			return nil
		},
	}

	return cmd
}
