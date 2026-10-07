// verifygate.go implements NewVerifyGate, the must-pass gate that holds a Burler round's handoff until the comment lint and the round's impacted-set command pass on the committed tree.

package loomshed

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/commentlint"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// verifyLogTailBytes bounds how much of the verify log a failing verdict quotes.
const verifyLogTailBytes = 4096

// NewVerifyGate returns a must-pass shuttleengine.Gate that, at each arrival, parses the plan under anchorPath for its `## verify:` command, lints the comments the round added, and runs the round's command through verifytree.Verify over worktreeRoot, keeping its record, marker and log in verifyDir.
// siteLabel names the call site in the running marker, and the closure's own count of its calls is the attempt.
//
// The base of the round is the commit of the plan verify command's latest recorded pass.
// With a usable base the comment lint runs from it to HEAD before any test, and the round command is the impacted-set command impactset derives.
// With no usable base the lint passes and the round command is the plan's own verify command, as it is wherever impactset names a fallback.
// The pass of the round command keeps the plan verify command's record entry, so the next round still diffs from it.
//
// `StatusPassed` and `StatusSkipped` pass.
// `StatusDirty` fails with the dirty paths.
// `StatusFailed` fails with the exit code, the log path and the log's tail.
// A comment lint finding or a misplaced `//lyx:guard` marker fails with its file and line.
// A plan with no `## verify:` section passes with a logged warning.
// A plan read error, a lint run error or a cancelled verify is a returned error, since none is a defect the writer can fix.
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

		base := ""
		if pass, ok := verifytree.LatestPass(paths, plan.Verify); ok {
			base = pass.Commit
		}
		derivation, err := impactset.Derive(worktreeRoot, base)
		if err != nil {
			logger.Warn("loomshed: verify gate could not derive the round command", "gate", siteLabel, "attempt", attempt, "err", err)
			return shuttleengine.GateResult{Passed: false, Findings: fmt.Sprintf("The round command could not be derived: %v.", err)}, nil
		}
		if derivation.Base != "" {
			commentFindings, err := commentlint.Lint(worktreeRoot, derivation.Base)
			if err != nil {
				return shuttleengine.GateResult{}, fmt.Errorf("loomshed: verify gate: lint the comments: %w", err)
			}
			if len(commentFindings) > 0 {
				logger.Warn("loomshed: verify gate found fixed-column comment wraps", "gate", siteLabel, "attempt", attempt, "count", len(commentFindings))
				return shuttleengine.GateResult{Passed: false, Findings: commentLintFindings(commentFindings)}, nil
			}
		}
		command := derivation.Command
		if command == "" {
			command = plan.Verify
			logger.Info("loomshed: verify gate runs the plan's verify command", "gate", siteLabel, "attempt", attempt, "reason", derivation.Fallback)
		}

		site := verifytree.Site{Label: siteLabel, Attempt: attempt, BaseCommand: plan.Verify}
		res, err := verifytree.Verify(context.Background(), paths, site, command)
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

// commentLintFindings renders the comment lint's findings as one line each, file and line first.
func commentLintFindings(findings []commentlint.Finding) string {
	var b strings.Builder
	b.WriteString("The round added fixed-column-wrapped comment lines, so no test ran. Break each comment at sentence boundaries, one sentence per line:\n\n")
	for _, f := range findings {
		fmt.Fprintf(&b, "%s:%d: %s\n", f.File, f.Line, f.Text)
	}
	return b.String()
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
