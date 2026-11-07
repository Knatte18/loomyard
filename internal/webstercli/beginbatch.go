// beginbatch.go implements the `begin-batch` webster verb.
// Master calls it immediately before forking a batch's implementer.
// It runs websterengine.BeginBatch under the state-mutation lease (load, mutate, save, release).
// It then makes one of webster's fabric-commit points:
// state.json, holding the batch's start-SHA and record, is durable before Master forks.
// The fork prompt is not part of that commit:
// the sync pathspec excludes prompts/*, which are machine-local artifacts that BeginBatch rewrites on every begin.
package webstercli

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// fabricSyncWayForward is the trailing clause of every refusal raised because the fabric sync failed.
// The state is already saved locally, and fabricSync only commits the scoped _lym pathspec and never pushes.
const fabricSyncWayForward = "way forward: the state is saved locally; `lym fabric commit` commits it, or the next bracket verb's own sync carries it; a session lym refuses the verb from reports status: FAILED and the orch runs it"

// rebaselineWayForward is the trailing clause of every refusal raised because persisting the plan-fingerprint re-baseline failed.
const rebaselineWayForward = "way forward: re-run the same verb; the re-baseline is recomputed from the plan on disk"

// beginDeps builds the websterengine.BeginDeps from this CLI's wiring and the loaded plan, batches and run state.
// It loads the gate config through c.geom.
// It refuses, before the caller mutates anything, a gate config that cannot list its `card` or `batch` gate.
func (c *websterCLI) beginDeps(plan *planparser.Plan, batches []batcher.Batch, st *websterengine.State) (websterengine.BeginDeps, error) {
	gate, err := c.loadGateConfig()
	if err != nil {
		return websterengine.BeginDeps{}, err
	}
	return websterengine.BeginDeps{
		Plan:        plan,
		Batches:     batches,
		State:       st,
		Config:      c.cfg,
		Stopper:     c.runner,
		Geom:        c.geom,
		FrictionDir: c.frictionDir,
		GateTiers:   c.gateTiers,
		RenameTools: c.renameTools,
		Gate:        gate,
		CardFixer:   cardFixerOn(gate, c.pairSlug()),
	}, nil
}

// cardFixerOn reports whether a failed card gate call goes through a cold fixer first:
// the pair has a slug and the gate config's `card` key carries a non-empty `fixer`.
func cardFixerOn(gate gateslot.Config, slug string) bool {
	key, found := gate.Gates["card"]
	return slug != "" && found && key.Fixer != ""
}

// loadGateConfig reads the gate config through c.geom.
// It refuses one whose `card` or `batch` list the prompts cannot render.
// The refusal carries the config's way forward and role clause.
func (c *websterCLI) loadGateConfig() (gateslot.Config, error) {
	gate, err := c.geom.GateConfig()
	if err != nil {
		return gateslot.Config{}, err
	}
	for _, name := range []string{"card", "batch"} {
		if _, err := gate.TierList(name); err != nil {
			return gateslot.Config{}, err
		}
	}
	return gate, nil
}

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
defect carries "plan_conflict" with its cause: "plan_order" when every finding
is a delete a later card's Edit code still references, otherwise "plan_defect"
when no batch is begun and "tree_moved" when one is.

Under a pause that covers the pair it refuses first, claiming Master into it.

Example:
  lym webster begin-batch 3`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			if refusal := c.refuseCoveredBatch("begin-batch", args[0]); refusal != "" {
				clihelp.SetExit(cmd.Context(), output.Err(out, refusal))
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
			// Hold the state-mutation lease across the whole load -> BeginBatch (guards + mutate) -> SaveState sequence:
			// every holder's section is bounded, so the blocking acquire is always short.
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
				clihelp.SetExit(cmd.Context(), output.Err(out, `webster: no run in progress; run "lym webster run" first`))
				return nil
			}

			batches, err := c.executionBatches(plan, st)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			deps, err := c.beginDeps(plan, batches, st)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			fingerprintBefore := st.PlanFingerprint

			result, err := websterengine.BeginBatch(deps, batchNumber)
			if err != nil {
				// BeginBatch re-baselines st's plan fingerprint as soon as ValidateDispatch rewrites the plan on disk.
				// That happens before the call reports a blocking finding and before the later fallible steps that follow it.
				// Persist that re-baseline before releasing the lease.
				// Every other mutation on this path is dropped, and an unchanged fingerprint writes nothing at all.
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
			// Once the batch has begun, a conflict routed to plan review is no longer current.
			if err := websterengine.RemoveRoutedConflict(c.geom.ScratchDir); err != nil {
				logger.Warn("webstercli: routed plan-conflict file not removed", "err", err)
			}
			// The state phase is over; release the lease before the fabric push below,
			// so a network-slow push never serializes another verb's state mutation behind it.
			_ = mutateLock.Release()
			mutateHeld = false

			// The sync commits the scoped _lym pathspec, which holds the reports directory,
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
