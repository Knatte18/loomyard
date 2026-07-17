// validate.go implements the `validate` webster verb: the standalone
// pre-flight half of the automatic gate websterengine.Run runs itself before
// ever spawning Master -- copied from buildercli's own validate.go
// (internal/buildercli/validate.go). It parses the plan and runs every
// plan-format v2 machine check against it, printing exactly one JSON
// envelope: ok with {"valid": true, "batches": <n>} for a clean plan, or an
// error envelope carrying every finding for a plan with findings -- exit
// non-zero either way a finding exists, never plain text. Unlike builder,
// webster's own Run pre-flight ALSO refuses a zero-batch plan outright
// (nothing-to-build is a malformed plan, never a vacuous outcome: done, per
// websterengine's runlevel.go); validate surfaces that same zero-batch
// finding through builderengine.Validate's own findings set rather than a
// distinct check, since a zero-batch plan already fails builderengine's
// batch-index-consistency checks.
package webstercli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// findingsEnvelope writes a single JSON error envelope carrying findings as
// a structured array (check, batch, detail per entry), rather than a
// flattened error string: a Planner or human triaging validate's output
// needs each finding machine-parseable, and internal/output.Err's message
// field has no room for that. This mirrors output.Err's envelope shape
// (ok:false) and exit code (1) exactly, adding only the "findings" field --
// validate and run's automatic gate both share it, since both surface the
// same Validate findings the same way. Mirrors buildercli's own
// findingsEnvelope exactly.
func findingsEnvelope(out io.Writer, findings []builderengine.ValidationError) int {
	entries := make([]map[string]string, len(findings))
	for i, f := range findings {
		entries[i] = map[string]string{"check": f.Check, "batch": f.Batch, "detail": f.Detail}
	}
	data, _ := json.Marshal(map[string]any{
		"ok":       false,
		"error":    fmt.Sprintf("webster: plan validation found %d finding(s)", len(findings)),
		"findings": entries,
	})
	fmt.Fprintln(out, string(data))
	return 1
}

// validateCmd builds the `validate` subcommand: ParsePlan followed by
// Validate against the resolved caps (webster.yaml's
// batch_context_cap_tokens/batch_card_cap), resolving Scope and every
// card's typed file-op paths (Context/Edits/Creates/Deletes/Moves) against
// layout.Cwd -- the same worktree-base anchoring every other websterCLI dir
// uses.
func (c *websterCLI) validateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "lint the plan against the plan-format machine checks without running anything",
		Long: `validate parses the plan at _lyx/plan and runs every plan-format v2 machine
check against it (e.g. format/approval, Batch Index <-> file consistency,
verify: presence, chain-end soundness, the oversized-batch context/card-
count cap, scope well-formedness, and the move-*/card-*/path-missing
file-op structural checks -- the full set evolves with plan-format, so
this list is illustrative, not exhaustive). A clean plan prints
{"valid": true, "batches": N}. A plan with findings prints an error
envelope carrying every finding (check, batch, detail) and exits non-zero
-- this is the SAME gate "lyx webster run" runs automatically before ever
forking an implementer; validate is the lint-without-run pre-flight for a
Planner or human.

Example:
  lyx webster validate`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			// A failing PersistentPreRunE has already written an error
			// response and recorded the exit code; short-circuit rather
			// than touch c's fields, which are unpopulated on that path.
			if clihelp.ShouldAbort(cmd.Context()) {
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

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"valid":   true,
				"batches": len(plan.Batches),
			}))
			return nil
		},
	}

	return cmd
}
