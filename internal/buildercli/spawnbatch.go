// spawnbatch.go implements the `spawn-batch` builder verb: the same
// automatic plan-validation gate "validate" lints for, the ErrPaused
// "paused" signal, builderengine.SpawnBatch's role-override/restart-chain
// wiring, and the spawn-boundary weft commit of state.json -- the first of
// the loop's three weft-commit points (see builderengine's package doc).

package buildercli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// pausedEnvelope writes a single JSON error envelope carrying "paused": true
// -- the orchestrator's own pause signal -- alongside the usual ok:false and
// error fields, and returns exit code 1. Like findingsEnvelope, this exists
// because output.Err's message field has no room for a structured extra
// field, and the orchestrator branches on "paused" being present, not on
// the error text.
func pausedEnvelope(out io.Writer, err error) int {
	data, _ := json.Marshal(map[string]any{
		"ok":     false,
		"error":  err.Error(),
		"paused": true,
	})
	fmt.Fprintln(out, string(data))
	return 1
}

// spawnBatchCmd builds the `spawn-batch <NN>` subcommand.
func (c *builderCLI) spawnBatchCmd() *cobra.Command {
	var (
		roleOverride string
		restartChain bool
	)

	cmd := &cobra.Command{
		Use:   "spawn-batch <NN>",
		Short: "validate the plan, then spawn one batch's implementer via shuttle",
		Long: `spawn-batch <NN> runs the same automatic plan-validation gate "validate"
lints for, checks the builder pause flag (refusing with "paused": true if
"lyx builder pause" was called), records the batch's start-SHA in
state.json, and spawns one implementer session via shuttle. Go selects the
implementer role from the batch's own "oversized:" frontmatter; the
orchestrator overrides only for the recovery escalation path with
--role recovery. --restart-chain resets the host repo to the batch's
deferred-verify chain's recorded start SHA and clears every chain member's
stale report before spawning.

Example:
  lyx builder spawn-batch 3
  lyx builder spawn-batch 3 --role recovery
  lyx builder spawn-batch 3 --restart-chain`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			batchNumber, err := strconv.Atoi(args[0])
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("builder: %q is not a valid batch number: %v", args[0], err)))
				return nil
			}

			// Go picks the role from the batch's own oversized: frontmatter;
			// the orchestrator overrides only for the recovery escalation
			// path. Reject any other override before ever touching the
			// engine, mirroring builderengine.SpawnBatch's own selectRole
			// guard (the CLI-level check exists so a typo'd --role value
			// surfaces its own clear flag error rather than SpawnBatch's
			// more generic one).
			var role builderengine.Role
			switch roleOverride {
			case "":
			case string(builderengine.RoleRecovery):
				role = builderengine.RoleRecovery
			default:
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("builder: --role %q is invalid; only %q is a valid override", roleOverride, builderengine.RoleRecovery)))
				return nil
			}

			plan, err := builderengine.ParsePlan(c.planDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			caps := builderengine.ValidateCaps{
				ContextCapTokens: c.cfg.BatchContextCapTokens,
				CardCap:          c.cfg.BatchCardCap,
			}
			if findings := builderengine.Validate(plan, c.layout.Cwd, caps); len(findings) > 0 {
				clihelp.SetExit(cmd.Context(), findingsEnvelope(out, findings))
				return nil
			}

			st, err := builderengine.LoadState(c.builderDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if st == nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, `builder: no run in progress; run "lyx builder run" first`))
				return nil
			}

			deps := builderengine.SpawnDeps{
				Starter:      c.starter,
				Plan:         plan,
				State:        st,
				Roles:        c.roles,
				Config:       c.cfg,
				WorktreeRoot: c.layout.Cwd,
				BuilderDir:   c.builderDir,
				ReportsDir:   c.reportsDir,
				ShuttleCfg:   c.shuttleCfg,
				Layout:       c.layout,
			}

			result, err := builderengine.SpawnBatch(deps, builderengine.SpawnBatchOptions{
				BatchNumber:  batchNumber,
				RoleOverride: role,
				RestartChain: restartChain,
			})
			if err != nil {
				// ErrPaused is the orchestrator's own operational signal,
				// never a hard error: it writes its own outcome.yaml with
				// outcome: paused and exits cleanly on seeing this refusal.
				if errors.Is(err, builderengine.ErrPaused) {
					clihelp.SetExit(cmd.Context(), pausedEnvelope(out, err))
					return nil
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			if _, weftErr := weftCommit(c.layout, fmt.Sprintf("spawn-batch %02d", batchNumber)); weftErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("builder: batch %s spawned but the weft sync failed: %v", result.BatchName, weftErr)))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"batch_name":  result.BatchName,
				"role":        string(result.Role),
				"start_sha":   result.StartSHA,
				"strand_guid": result.StrandGUID,
				"run_dir":     result.RunDir,
				"report_path": result.ReportPath,
			}))
			return nil
		},
	}

	cmd.Flags().StringVar(&roleOverride, "role", "", `role override; only "recovery" is accepted`)
	cmd.Flags().BoolVar(&restartChain, "restart-chain", false, "reset the host repo to this batch's deferred-verify chain start SHA before spawning")

	return cmd
}
