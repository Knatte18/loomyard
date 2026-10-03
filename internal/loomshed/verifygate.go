// verifygate.go implements NewVerifyGate, the must-pass gate that holds a Burler round's handoff until the plan's verify command passes on the committed tree.

package loomshed

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// verifyLogTailBytes bounds how much of the verify log a failing verdict quotes.
const verifyLogTailBytes = 4096

// NewVerifyGate returns a must-pass shuttleengine.Gate that, at each arrival, parses the plan under anchorPath for its `## verify:` command and runs verifytree.Verify over worktreeRoot, keeping its record, marker and log in verifyDir.
// siteLabel names the call site in the running marker, and the closure's own count of its calls is the attempt.
//
// `StatusPassed` and `StatusSkipped` pass.
// `StatusDirty` fails with the dirty paths.
// `StatusFailed` fails with the exit code, the log path and the log's tail.
// A plan with no `## verify:` section passes with a logged warning.
// A plan read error or a cancelled verify is a returned error, since neither is a defect the writer can fix.
func NewVerifyGate(anchorPath, worktreeRoot, verifyDir, siteLabel string) shuttleengine.Gate {
	attempt := 0
	paths := verifytree.NewPaths(worktreeRoot, verifyDir)
	return func() (shuttleengine.GateResult, error) {
		attempt++
		planDir := planparser.PlanDir(anchorPath)
		plan, err := planparser.ParsePlan(planDir)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("loomshed: verify gate: read the plan: %w", err)
		}
		if plan.Verify == "" {
			logger.Warn("loomshed: verify gate found no verify section in the plan", "gate", siteLabel, "planDir", planDir)
			return shuttleengine.GateResult{Passed: true}, nil
		}

		res, err := verifytree.Verify(context.Background(), paths, verifytree.Site{Label: siteLabel, Attempt: attempt}, plan.Verify)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("loomshed: verify gate: %w", err)
		}
		switch res.Status {
		case verifytree.StatusPassed, verifytree.StatusSkipped:
			return shuttleengine.GateResult{Passed: true}, nil
		case verifytree.StatusDirty:
			findings := fmt.Sprintf("The worktree has uncommitted changes, so the verify command did not run: %s. Commit or discard them.", strings.Join(res.Dirty, ", "))
			logger.Warn("loomshed: verify gate found a dirty tree", "gate", siteLabel, "attempt", attempt, "paths", strings.Join(res.Dirty, ", "))
			return shuttleengine.GateResult{Passed: false, Findings: findings}, nil
		default:
			findings := verifyFailureFindings(res, paths.Log)
			logger.Warn("loomshed: verify gate failed", "gate", siteLabel, "attempt", attempt, "exitCode", res.ExitCode, "log", paths.Log)
			return shuttleengine.GateResult{Passed: false, Findings: findings}, nil
		}
	}
}

// verifyFailureFindings renders a StatusFailed result as the exit code, the log path and the log's tail.
func verifyFailureFindings(res verifytree.Result, logPath string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The verify command failed with exit code %d.", res.ExitCode)
	if res.Detail != "" {
		fmt.Fprintf(&b, " Its shell could not start: %s.", res.Detail)
	}
	fmt.Fprintf(&b, "\n\nFull log: %s\n", logPath)
	if tail := readLogTail(logPath); tail != "" {
		fmt.Fprintf(&b, "\nLog tail:\n\n%s\n", tail)
	}
	return b.String()
}

// readLogTail returns the last verifyLogTailBytes of the log at path, or empty when it cannot be read.
func readLogTail(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(data) > verifyLogTailBytes {
		data = data[len(data)-verifyLogTailBytes:]
	}
	return string(bytes.TrimSpace(data))
}
