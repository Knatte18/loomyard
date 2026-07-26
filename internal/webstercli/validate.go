// validate.go implements the `validate` webster verb: the standalone
// pre-flight half of the automatic gate websterengine.Run runs itself before
// ever spawning Master. It parses the plan and runs every plan-format v3
// machine check against it, printing exactly one JSON envelope: ok with
// {"valid": true, "cards": <n>} for a clean plan, or an error envelope
// carrying every finding for a plan with findings -- exit non-zero either
// way a finding exists, never plain text. Unlike builder, webster's own Run
// pre-flight ALSO refuses a zero-batch plan outright (nothing-to-build is a
// malformed plan, never a vacuous outcome: done, per websterengine's
// runlevel.go); validate surfaces that same emptiness through
// planparser.Validate's own findings set rather than a distinct check,
// since a zero-card plan already fails planparser's index-file-consistency
// checks.
package webstercli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/spf13/cobra"
)

// findingsEnvelope writes a single JSON error envelope carrying findings as
// a structured array (check, card, detail per entry), rather than a
// flattened error string: a Planner or human triaging validate's output
// needs each finding machine-parseable, and internal/output.Err's message
// field has no room for that. This mirrors output.Err's envelope shape
// (ok:false) and exit code (1) exactly, adding only the "findings" field --
// validate and run's automatic gate both share it, since both surface the
// same Validate findings the same way.
func findingsEnvelope(out io.Writer, findings []planparser.ValidationError) int {
	entries := make([]map[string]string, len(findings))
	for i, f := range findings {
		entries[i] = map[string]string{"check": f.Check, "card": f.Card, "detail": f.Detail}
	}
	data, _ := json.Marshal(map[string]any{
		"ok":       false,
		"error":    fmt.Sprintf("webster: plan validation found %d finding(s)", len(findings)),
		"findings": entries,
	})
	fmt.Fprintln(out, string(data))
	return 1
}

// validateCmd builds the `validate` subcommand: planparser.ParsePlan
// followed by planparser.Validate, resolving every card's typed file-op
// paths (Context/Edits/Creates/Deletes/Moves) against layout.Cwd -- the same
// worktree-base anchoring every other websterCLI dir uses.
func (c *websterCLI) validateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "lint the plan against the plan-format machine checks without running anything",
		Long: `validate parses the plan at _lyx/plan and runs every plan-format v3 machine
check against it -- the 14 checks docs/reference/plan-format-v3.md's
"Validation checks" section pins: format/approval, Card Index <-> card-file
consistency, card path well-formedness, the Moves: grammar/redundancy/
mechanic checks, the per-card structural checks, the card-numbering
heading cross-check, the existence-dependent path/commit-subject checks,
and the depends-on-order gate. A clean plan prints {"valid": true,
"cards": N}. A plan with findings prints an error envelope carrying every
finding (check, card, detail) and exits non-zero -- this is the SAME gate
"lyx webster run" runs automatically before ever forking an implementer;
validate is the lint-without-run pre-flight for a Planner or human.

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

			plan, err := planparser.ParsePlan(c.planDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			if findings := planparser.Validate(plan, c.layout.Cwd); len(findings) > 0 {
				clihelp.SetExit(cmd.Context(), findingsEnvelope(out, findings))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"valid": true,
				"cards": len(plan.Cards),
			}))
			return nil
		},
	}

	return cmd
}
