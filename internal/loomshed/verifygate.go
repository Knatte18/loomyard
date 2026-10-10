// verifygate.go implements NewVerifyGate, the must-pass gate that holds a handoff until the comment lint and the told verify command pass on the committed tree, narrowed to the round's impacted set or run over the whole diff from a told merge base.

package loomshed

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/commentlint"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// verifyLogTailBytes bounds how much of the verify log a failing verdict quotes.
const verifyLogTailBytes = 4096

// NewVerifyGate returns a must-pass shuttleengine.Gate that, at each arrival, reads the told verify command, lints the comments the work added, and runs the gate's command through verifytree.Verify over worktreeRoot, keeping its record, marker and log in verifyDir.
// siteLabel names the call site in the running marker, and the closure's own count of its calls is the attempt.
// command is the told verify-command source, read at each arrival.
// slots is the hub pool the command waits on before it runs; nil runs it unslotted.
//
// With mergeBase nil the gate takes the round form.
// The base of the round is the commit of the told command's latest recorded pass.
// With a usable base the comment lint runs from it to HEAD before any test, and the round command is the impacted-set command impactset derives.
// With no usable base the lint passes and the round command is the told command itself, as it is wherever impactset names a fallback.
// The pass of the round command keeps the told command's record entry, so the next round still diffs from it.
// An empty told command passes with a logged warning.
//
// With mergeBase set the gate takes the whole-diff form.
// The comment lint runs from the merge base mergeBase reads to HEAD, and the told command runs in full with the command as the site's base command, so Publish skips on the same tree and command.
// The form has no skip path: an empty told command is an error naming the source, never a pass.
//
// While a Publish failure record is present, the command also runs the `tmux` tier to confirm the fix:
// each failing test the checked record names, by name in its package, and for a `publish_verify` failure the impacted set under `tmux`, or `./...` where the round fell back to the told command or the form is whole-diff.
// The record's fields are shape-checked first, and a field that fails is dropped and logged, so no record text reaches the command as shell.
//
// `StatusPassed` and `StatusSkipped` pass.
// `StatusDirty` fails with the dirty paths.
// `StatusFailed` fails with the exit code, the log path and the log's tail.
// A comment lint finding, or in the round form a test file the guard scan rejects such as a misplaced `//lyx:guard` marker, fails with its file and line and the way forward.
// A command or merge base read error, a failure to read git or a package directory, a lint run error or a cancelled verify is a returned error, since none is a defect the writer can fix.
func NewVerifyGate(worktreeRoot, verifyDir, siteLabel string, command, mergeBase func() (string, error), slots *gateslot.Pool) shuttleengine.Gate {
	attempt := 0
	paths := verifytree.NewPaths(worktreeRoot, verifyDir)
	return func() (shuttleengine.GateResult, error) {
		attempt++
		told, err := command()
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("loomshed: verify gate: read the verify command: %w", err)
		}
		if told == "" && mergeBase != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("loomshed: verify gate: the told verify command source returned an empty command")
		}
		if told == "" {
			logger.Warn("loomshed: verify gate was told no verify command", "gate", siteLabel)
			return shuttleengine.GateResult{Passed: true}, nil
		}

		var lintBase string
		runCommand := told
		tmuxPackages := []string{"./..."}
		if mergeBase != nil {
			lintBase, err = mergeBase()
			if err != nil {
				return shuttleengine.GateResult{}, fmt.Errorf("loomshed: verify gate: read the merge base: %w", err)
			}
		} else {
			base := ""
			if pass, ok := verifytree.LatestPass(paths, told); ok {
				base = pass.Commit
			}
			derivation, err := impactset.Derive(worktreeRoot, base)
			if impactset.IsGuardScanError(err) {
				logger.Warn("loomshed: verify gate found a test file the guard scan rejects", "gate", siteLabel, "attempt", attempt, "err", err)
				return shuttleengine.GateResult{Passed: false, Findings: guardScanFindings(err)}, nil
			}
			if err != nil {
				return shuttleengine.GateResult{}, fmt.Errorf("loomshed: verify gate: derive the round command: %w", err)
			}
			lintBase = derivation.Base
			if derivation.Command != "" {
				runCommand = derivation.Command
			} else {
				logger.Info("loomshed: verify gate runs the told verify command", "gate", siteLabel, "attempt", attempt, "reason", derivation.Fallback)
			}
			if derivation.Fallback == "" {
				tmuxPackages = derivation.Packages
			}
		}
		if lintBase != "" {
			commentFindings, err := commentlint.Lint(worktreeRoot, lintBase)
			if err != nil {
				return shuttleengine.GateResult{}, fmt.Errorf("loomshed: verify gate: lint the comments: %w", err)
			}
			if len(commentFindings) > 0 {
				logger.Warn("loomshed: verify gate found fixed-column comment wraps", "gate", siteLabel, "attempt", attempt, "count", len(commentFindings))
				return shuttleengine.GateResult{Passed: false, Findings: commentLintFindings(commentFindings)}, nil
			}
		}
		if failure, ok := checkedPublishFailure(paths, worktreeRoot); ok {
			if extra := publishFailureCommand(failure, tmuxPackages); extra != "" {
				runCommand += " && " + extra
				logger.Info("loomshed: verify gate confirms a publish failure", "gate", siteLabel, "attempt", attempt, "kind", failure.Kind)
			}
		}

		site := verifytree.Site{Label: siteLabel, Attempt: attempt, BaseCommand: told}
		res, err := verifytree.Verify(context.Background(), paths, site, runCommand, verifytree.Timeout, slots)
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
			findings := verifyFailureFindings(res, res.Log)
			logger.Warn("loomshed: verify gate failed", "gate", siteLabel, "attempt", attempt, "exitCode", res.ExitCode, "log", res.Log)
			return shuttleengine.GateResult{Passed: false, Findings: findings}, nil
		}
	}
}

// guardScanFindings renders a guard scan rejection with the file and line it names and the way to fix it.
func guardScanFindings(err error) string {
	return fmt.Sprintf("A test file blocks the guard scan, so no test ran: %v.\n\nPut `//lyx:guard` on the line directly above its top-level `func Test…` line (only `//testtiming:keep` lines may sit between), keep it out of `tmux` and `llm` test files, and make every test file parse.\n", err)
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

// verifyFailureFindings renders a StatusFailed result as the exit code or the timeout, the log path and the log's tail.
func verifyFailureFindings(res verifytree.Result, logPath string) string {
	var b strings.Builder
	if res.TimedOut {
		fmt.Fprintf(&b, "The verify command did not finish within %s and was killed.", verifytree.Timeout)
	} else {
		fmt.Fprintf(&b, "The verify command failed with exit code %d.", res.ExitCode)
		if res.Detail != "" {
			fmt.Fprintf(&b, " Its shell could not start: %s.", res.Detail)
		}
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
