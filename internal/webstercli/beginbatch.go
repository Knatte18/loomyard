// beginbatch.go implements the `begin-batch` webster verb: Master's own bracket call immediately
// before forking a batch's implementer.
// It runs websterengine.BeginBatch under the state-mutation lease (load, mutate, save, release),
// then performs the first of webster's four fabric-commit points (see the discussion's fabric-ownership
// decision) -- state.json (the batch's start-SHA and record) durable before Master ever forks.
// The freshly-written fork prompt is deliberately NOT part of that commit: the sync pathspec
// excludes prompts/* as machine-local re-renderable artifacts (BeginBatch rewrites a batch's own
// prompt on every begin).
package webstercli

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// fabricSyncWayForward is the trailing clause of every refusal raised because the fabric sync failed.
// The state is already saved locally, and fabricSync only commits the scoped _lyx pathspec and never pushes.
const fabricSyncWayForward = "way forward: the state is saved locally; `lyx fabric commit` commits it, or the next bracket verb's own sync carries it; a session lyx refuses the verb from reports status: FAILED and the orch runs it"

// rebaselineWayForward is the trailing clause of every refusal raised because persisting the plan-fingerprint re-baseline failed.
const rebaselineWayForward = "way forward: re-run the same verb; the re-baseline is recomputed from the plan on disk"

// beginDeps builds the websterengine.BeginDeps for this CLI's current wiring over the loaded plan, execution batches and run state.
func (c *websterCLI) beginDeps(plan *planparser.Plan, batches []batcher.Batch, st *websterengine.State) websterengine.BeginDeps {
	return websterengine.BeginDeps{
		Plan:        plan,
		Batches:     batches,
		State:       st,
		Config:      c.cfg,
		Stopper:     c.runner,
		Geom:        c.geom,
		FrictionDir: c.frictionDir,
		VerifyTiers: c.verifyTiers,
		RenameTools: c.renameTools,
	}
}

// beginBatchCmd builds the `begin-batch <nn>` subcommand.
func (c *websterCLI) beginBatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "begin-batch <nn>",
		Short:       "open one batch's bracket before forking its implementer: record its start and render its fork prompt",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `begin-batch <nn> refuses loud
when the batch's report file already exists for a batch state.json records
(finished work is never silently overwritten -- a stuck batch escalates via
recover-batch; a refusal over a batch whose recoveries are exhausted carries
"recovery_exhausted": true), archives a report that has no begin-batch record and goes on
(the envelope's archived_report names it), records
the batch's start-SHA in state.json, renders and writes that batch's fork
prompt (carrying the previous batch's own persisted digest), and returns the
prompt path Master forwards to its Agent-tool fork call verbatim.

A refusal because the plan's re-resolution against the tree found a blocking
defect carries "plan_conflict" with its cause: "plan_defect" when no batch is
begun, "tree_moved" when one is.

Example:
  lyx webster begin-batch 3`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			// A record exists only when the latest begin-batch refused with a plan conflict, so every other refusal leaves none.
			if err := websterengine.RemoveConflictRecord(c.geom.ScratchDir); err != nil {
				logger.Warn("webstercli: stale plan-conflict record not removed", "err", err)
			}

			batchNumber, err := strconv.Atoi(args[0])
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: %q is not a valid batch number: %v", args[0], err)))
				return nil
			}

			plan, err := planparser.ParsePlan(c.geom.PlanDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			// Hold the state-mutation lease across the whole load ->
			// BeginBatch (guards + mutate) -> SaveState sequence: every
			// holder's section is bounded, so the blocking acquire is
			// always short.
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

			batches, err := c.executionBatches(plan, st)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			deps := c.beginDeps(plan, batches, st)

			fingerprintBefore := st.PlanFingerprint

			result, err := websterengine.BeginBatch(deps, batchNumber)
			if err != nil {
				// BeginBatch re-baselines st's plan fingerprint the moment its own
				// ValidateDispatch canonicalizes handles and rewrites the plan on disk, which
				// happens BEFORE that call reports a blocking finding and before the eight
				// further fallible steps that follow it. Persist that re-baseline before
				// releasing the lease; every other mutation on this path is still deliberately
				// dropped, and an unchanged fingerprint writes nothing at all.
				saveErr := persistPlanFingerprintRebaseline(c.geom, st, fingerprintBefore)
				var conflict *websterengine.PlanConflictError
				isConflict := errors.As(err, &conflict)
				// A failed persist prints the combined refusal with no class, so it leaves no record.
				if saveErr == nil && isConflict {
					if recordErr := websterengine.WriteConflictRecord(c.geom.ScratchDir, conflict); recordErr != nil {
						logger.Warn("webstercli: plan-conflict record not written", "err", recordErr)
					}
				}
				_ = mutateLock.Release()
				mutateHeld = false

				if saveErr != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("%s; additionally, persisting the plan-fingerprint re-baseline this call had already earned failed: %v; %s", err.Error(), saveErr, rebaselineWayForward)))
					return nil
				}
				if isConflict {
					clihelp.SetExit(cmd.Context(), output.ErrFields(out, err.Error(), map[string]any{"plan_conflict": conflict.Cause}))
					return nil
				}
				if errors.Is(err, websterengine.ErrRecoveryExhausted) {
					clihelp.SetExit(cmd.Context(), output.ErrFields(out, err.Error(), map[string]any{"recovery_exhausted": true}))
					return nil
				}
				if errors.Is(err, websterengine.ErrRenameToolMissing) {
					clihelp.SetExit(cmd.Context(), output.ErrFields(out, err.Error(), map[string]any{"rename_tool_missing": true}))
					return nil
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			if err := websterengine.SaveState(c.geom.WebsterDir, c.geom.ScratchDir, st); err != nil {
				_ = mutateLock.Release()
				mutateHeld = false
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			// The batch begun, any conflict last routed to plan review is history.
			if err := websterengine.RemoveRoutedConflict(c.geom.ScratchDir); err != nil {
				logger.Warn("webstercli: routed plan-conflict file not removed", "err", err)
			}
			// The state phase is over; release the lease before the fabric
			// push below so a network-slow push never serializes another
			// verb's state mutation behind it.
			_ = mutateLock.Release()
			mutateHeld = false

			// The sync commits the scoped _lyx pathspec, which holds the reports directory,
			// so an archived report's rename is committed with the state.
			if _, syncErr := fabricSync(c.openFabric, c.anchorRel, fmt.Sprintf("begin-batch %s", result.BatchName)); syncErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("webster: batch %s begun but the fabric sync failed: %v; %s", result.BatchName, syncErr, fabricSyncWayForward)))
				return nil
			}

			envelope := map[string]any{
				"batch":       result.BatchName,
				"prompt_path": result.PromptPath,
				"start_sha":   result.StartSHA,
				"warnings":    ownerlessRunWarnings(c.geom.ScratchDir, nil),
				"advisories":  result.Advisories,
			}
			if result.ArchivedReport != "" {
				envelope["archived_report"] = result.ArchivedReport
			}
			clihelp.SetExit(cmd.Context(), output.Ok(out, envelope))
			return nil
		},
	}

	return cmd
}
