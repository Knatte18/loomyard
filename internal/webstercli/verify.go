// verify.go implements the `verify` webster verb: the self-check of the webster verify gate and of Webster-Burler's verify gate.
// It parses the plan, then calls verifytree.Verify over the worktree with the plan's `## verify:` command, the same call both gates make.
// A pass or skip is ok.
// A dirty tree or a failing command is an error envelope carrying a `findings` key, the shape the gate-parity test maps onto a stuck verdict.

package webstercli

import (
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// verifyVerbSite is the site label the verb writes to the running marker.
const verifyVerbSite = "webster verify"

// verifyCmd builds the `verify` subcommand.
func (c *websterCLI) verifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "verify",
		Short:       "run the plan's verify command over the worktree, the way the webster verify gate does",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `verify parses the plan at _lyx/plan and runs its "## verify:" command over
the worktree through the same function the webster verify gate, Webster-Burler's
verify gate and Publish and Finalize call. It is the self-check of those gates.

A passing run records the verified tree. A second call on a tree whose record
names the same command exits 0 without running. A dirty tree is refused with
its paths under "findings", and a failing command with its exit code, log path
and failing identities under "findings"; both exit non-zero. A plan with no
"## verify:" section exits 0 with a warning, as the gates pass it.

Example:
  lyx webster verify`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), c.runVerify(cmd))
			return nil
		},
	}
}

// runVerify runs the verb's check and writes its envelope, returning the exit code.
func (c *websterCLI) runVerify(cmd *cobra.Command) int {
	out := cmd.OutOrStdout()

	plan, err := planparser.ParsePlan(c.geom.PlanDir)
	if err != nil {
		return output.Err(out, fmt.Sprintf("webster: verify: read the plan: %v", err))
	}
	if plan.Verify == "" {
		logger.Warn("webstercli: verify found no verify section in the plan", "planDir", c.geom.PlanDir)
		return output.Ok(out, map[string]any{"verified": false, "warning": "the plan has no ## verify: section, so nothing ran"})
	}

	paths := verifytree.NewPaths(c.geom.WorktreeRoot, c.geom.VerifyDir)
	res, err := verifytree.Verify(cmd.Context(), paths, verifytree.Site{Label: verifyVerbSite}, plan.Verify, verifytree.Timeout, c.geom.GateSlots)
	if err != nil {
		return output.Err(out, fmt.Sprintf("webster: verify: %v", err))
	}

	switch res.Status {
	case verifytree.StatusPassed, verifytree.StatusSkipped:
		return output.Ok(out, map[string]any{"verified": true, "status": string(res.Status), "tree": res.Tree})
	case verifytree.StatusDirty:
		return output.ErrFields(out,
			"webster: verify: the worktree has uncommitted changes, so the verify command did not run; commit or discard them",
			map[string]any{"findings": map[string]any{"dirty": res.Dirty}})
	}

	findings := map[string]any{"exit_code": res.ExitCode, "log": res.Log}
	if res.Detail != "" {
		findings["detail"] = res.Detail
	}
	if log, readErr := os.ReadFile(res.Log); readErr == nil {
		failures := make([]map[string]string, 0)
		for _, f := range websterengine.ParseVerifyFailures(string(log)) {
			failures = append(failures, map[string]string{"id": f.ID, "kind": f.Kind, "package": f.Package})
		}
		findings["failures"] = failures
	}
	return output.ErrFields(out,
		fmt.Sprintf("webster: verify: the verify command failed with exit code %d; way forward: fix the failures named under findings, commit, and re-run `lyx webster verify`", res.ExitCode),
		map[string]any{"findings": findings})
}
