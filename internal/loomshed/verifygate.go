// verifygate.go implements NewVerifyGate, the must-pass gate that holds a handoff until the headroom lint, the comment lint and the told verify command pass on the committed tree, narrowed to the hub's `round` gate over the diff, or in the darn form to the diff's tree-scoped command or the `round` gate from a told merge base.

package loomshed

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/commentlint"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/patternlint"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// verifyLogTailBytes bounds how much of the verify log a failing verdict quotes.
const verifyLogTailBytes = 4096

// RoundGate is the hub's `round` gate as the verify gate reads it at each arrival.
type RoundGate struct {
	// Tiers are the tier numbers the `round` gate lists, in order.
	Tiers []int
	// Render spells the one `lyx gate test --wait --tier <round list>` call over the given scope words.
	Render func(scope ...string) string
	// GateTiers returns the tier numbers a named gate lists, or the config refusal with its way forward.
	GateTiers func(gate string) ([]int, error)
}

// NewVerifyGate returns a must-pass shuttleengine.Gate that, at each arrival, reads the told verify command, lints the comments the work added, and runs the gate's command through verifytree.Verify over worktreeRoot, keeping its record, marker and log in verifyDir.
// siteLabel names the call site in the running marker, and the closure's own count of its calls is the attempt.
// command is the told verify-command source, read at each arrival; it is the site's base command, so a pass keeps its record entry and Publish skips on the same tree and command.
// round is the told `round` gate source, read at each arrival after the told command; its error is the gate's returned error, carrying the config refusal's own way forward.
//
// With treeScope nil the gate takes the round form.
// The base of the round is the commit of the told command's latest recorded pass, when that commit is an ancestor of HEAD.
// With a usable base the comment lint runs from it to HEAD before any test, and the command is the round call over `--base` that commit, whose scope narrows, falls back to whole and runs the guard step.
// With no usable base the lint passes and the command is the told command itself.
// An empty told command passes with a logged warning.
// When mergeBase is non-nil, the headroom lint also runs from the merge base it reads to HEAD, before the comment lint and any test; nil skips it.
//
// With treeScope non-nil the gate takes the darn form, and mergeBase must be non-nil.
// The headroom lint and then the comment lint run from the merge base mergeBase reads to HEAD, so every attempt diffs from the merge base.
// Then treeScope is read: a tree-scoped diff runs the tree-scoped command it answers, and passes with a logged note, running nothing, when that command is empty;
// any other diff runs the round call over `--base` the merge base.
// The form has no skip path: an empty told command is an error naming the source, never a pass, and so is a nil mergeBase.
//
// While a Publish failure record is present, the command also confirms the fix.
// It first runs each failing test the checked record names, by name in its package under the record's failing tier, so a failure that still holds ends the run in seconds, and last one package pass of that tier over the failing tests' packages.
// The record's fields are shape-checked first, and a field that fails is dropped and logged, so no record text reaches the command as shell.
//
// `StatusPassed` and `StatusSkipped` pass.
// `StatusDirty` fails with the dirty paths.
// `StatusFailed` fails with the exit code, the log path and the log's tail, and, after a CPU watchdog quit, the window and the `busy_quit_sec` way forward; a test file the round call's guard scan rejects, such as a misplaced `//lyx:guard` marker, reaches the findings through that tail with its file and line.
// A headroom finding or a comment lint finding fails with its file and line or its bytes and the way forward.
// A command, round, merge base or tree scope read error, a failure to read git, a lint run error or a cancelled verify is a returned error, since none is a defect the writer can fix.
func NewVerifyGate(worktreeRoot, verifyDir, siteLabel string, command, mergeBase func() (string, error), treeScope func() (string, bool, error), round func() (RoundGate, error)) shuttleengine.Gate {
	attempt := 0
	darnForm := treeScope != nil
	paths := verifytree.NewPaths(worktreeRoot, verifyDir)
	return func() (shuttleengine.GateResult, error) {
		attempt++
		told, err := command()
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("loomshed: verify gate: read the verify command: %w", err)
		}
		if told == "" && darnForm {
			return shuttleengine.GateResult{}, fmt.Errorf("loomshed: verify gate: the told verify command source returned an empty command")
		}
		if told == "" {
			logger.Warn("loomshed: verify gate was told no verify command", "gate", siteLabel)
			return shuttleengine.GateResult{Passed: true}, nil
		}
		if darnForm && mergeBase == nil {
			return shuttleengine.GateResult{}, fmt.Errorf("loomshed: verify gate: the darn form was told no merge base reader")
		}
		roundGate, err := round()
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("loomshed: verify gate: read the round gate: %w", err)
		}

		var lintBase, headroomBase string
		runCommand := told
		if darnForm {
			lintBase, err = mergeBase()
			if err != nil {
				return shuttleengine.GateResult{}, fmt.Errorf("loomshed: verify gate: read the merge base: %w", err)
			}
			headroomBase = lintBase
		} else {
			if pass, ok := verifytree.LatestPass(paths, told); ok && isAncestorOfHead(worktreeRoot, pass.Commit) {
				lintBase = pass.Commit
				runCommand = roundGate.Render("--base", lintBase)
			} else {
				logger.Info("loomshed: verify gate runs the told verify command", "gate", siteLabel, "attempt", attempt, "reason", "no recorded pass of the told command to diff from")
			}
			if mergeBase != nil {
				headroomBase, err = mergeBase()
				if err != nil {
					return shuttleengine.GateResult{}, fmt.Errorf("loomshed: verify gate: read the merge base: %w", err)
				}
			}
		}
		if mergeBase != nil {
			finding, found, err := patternlint.Lint(worktreeRoot, headroomBase)
			if err != nil {
				return shuttleengine.GateResult{}, fmt.Errorf("loomshed: verify gate: lint the overview headroom: %w", err)
			}
			if found {
				logger.Warn("loomshed: verify gate found the overview past its headroom line", "gate", siteLabel, "attempt", attempt)
				return shuttleengine.GateResult{Passed: false, Findings: headroomFindings(finding)}, nil
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
		if darnForm {
			scoped, isScoped, err := treeScope()
			if err != nil {
				return shuttleengine.GateResult{}, fmt.Errorf("loomshed: verify gate: read the tree scope: %w", err)
			}
			switch {
			case isScoped && scoped == "":
				logger.Info("loomshed: verify gate ran nothing, the tree-scoped command is empty", "gate", siteLabel, "attempt", attempt)
				return shuttleengine.GateResult{Passed: true}, nil
			case isScoped:
				runCommand = scoped
			default:
				runCommand = roundGate.Render("--base", lintBase)
			}
		}
		if failure, ok := checkedPublishFailure(paths, worktreeRoot, roundGate.GateTiers); ok {
			failedTests, packagePass, err := publishFailureSteps(failure, roundGate.GateTiers)
			if err != nil {
				return shuttleengine.GateResult{}, fmt.Errorf("loomshed: verify gate: confirm the publish failure: %w", err)
			}
			if failedTests != "" {
				runCommand = failedTests + " && " + runCommand
			}
			if packagePass != "" {
				runCommand += " && " + packagePass
			}
			if failedTests != "" || packagePass != "" {
				logger.Info("loomshed: verify gate confirms a publish failure", "gate", siteLabel, "attempt", attempt, "kind", failure.Kind)
			}
		}

		site := verifytree.Site{Label: siteLabel, Attempt: attempt, BaseCommand: told}
		res, err := verifytree.Verify(context.Background(), paths, site, runCommand)
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

// isAncestorOfHead reports whether commit is an ancestor of HEAD in worktreeRoot; any git refusal reads as not an ancestor.
func isAncestorOfHead(worktreeRoot, commit string) bool {
	ancestor, err := gitrepo.New(worktreeRoot).IsAncestor(commit, "HEAD")
	return err == nil && ancestor
}

// headroomFindings renders a headroom finding with its message, the way forward and the instruction to re-step the row.
func headroomFindings(f pattern.Finding) string {
	return fmt.Sprintf("The branch grew PATTERN.md past its headroom line, so no test ran: %s.\n\nWay forward: %s, then re-step the row.\n", f.Message, patternlint.WayForward)
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
// When the whole log holds the CPU watchdog's line, a paragraph after the first sentence names the window and the way forward for a healthy package that is CPU-bound.
func verifyFailureFindings(res verifytree.Result, logPath string) string {
	log, _ := os.ReadFile(logPath)
	var b strings.Builder
	if res.TimedOut {
		fmt.Fprintf(&b, "The verify command timed out: %s.", res.Detail)
	} else {
		fmt.Fprintf(&b, "The verify command failed with exit code %d.", res.ExitCode)
		if res.Detail != "" {
			fmt.Fprintf(&b, " Its shell could not start: %s.", res.Detail)
		}
	}
	if window, quit := gateslot.FindWatchdogQuit(string(log)); quit {
		fmt.Fprintf(&b, "\n\nThe gate test's CPU watchdog quit a test binary that kept a CPU core busy for %s, so its goroutine stacks are in the log: look there for a livelock or a spin, and if the package is CPU-bound but healthy, %s, then re-step the row.", window, gateslot.BusyQuitWayForward)
	}
	fmt.Fprintf(&b, "\n\nFull log: %s\n", logPath)
	if tail := logTail(log); tail != "" {
		fmt.Fprintf(&b, "\nLog tail:\n\n%s\n", tail)
	}
	return b.String()
}

// logTail returns the last verifyLogTailBytes of log, trimmed.
func logTail(log []byte) string {
	if len(log) > verifyLogTailBytes {
		log = log[len(log)-verifyLogTailBytes:]
	}
	return string(bytes.TrimSpace(log))
}
