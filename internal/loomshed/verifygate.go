// verifygate.go implements NewVerifyGate, the must-pass gate that holds a handoff until the headroom lint, the comment lint and the told verify command pass on the committed tree, the told command being the row's own gate key call, or in the darn form the diff's tree-scoped command when the diff is wholly under the configured trees.

package loomshed

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/commentlint"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/patternlint"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// verifyLogTailBytes bounds how much of the verify log a failing verdict quotes.
const verifyLogTailBytes = 4096

// verifyFailuresBytes bounds the failures section a failing verdict quotes in place of the log tail; one failure longer than the bound is cut to it.
const verifyFailuresBytes = 8192

// verifyFailureTruncated ends a failure cut to verifyFailuresBytes.
const verifyFailureTruncated = "\n[truncated]"

// NewVerifyGate returns a must-pass shuttleengine.Gate that, at each arrival, reads the told verify command, lints the comments the work added, and runs the command through verifytree.Verify over worktreeRoot, keeping its record, marker and log in verifyDir.
// siteLabel names the call site in the running marker, and the closure's own count of its calls is the attempt.
// command is the told verify-command source, the site's own `lym gate test --wait --gate <key>` call, read at each arrival; it runs as it is, since the gate derives its own diff base, and a pass keeps its record entry so a later arrival on the same tree skips.
// hold is the told pause hold of the worktree's scope: a non-nil hold holds the verify under a pause covering the worktree, and nil holds nothing.
//
// With treeScope nil the gate takes the round form.
// The comment lint runs before any test from its own base: the commit of the told command's latest recorded pass when that commit is an ancestor of HEAD, else the merge base mergeBase reads, so a first round lints rather than skips.
// With neither base, no comment lint runs.
// An empty told command passes with a logged warning.
// When mergeBase is non-nil, the headroom lint also runs from the merge base it reads to HEAD, before the comment lint and any test; nil skips it.
//
// With treeScope non-nil the gate takes the darn form, and mergeBase must be non-nil.
// The headroom lint and then the comment lint run from the merge base mergeBase reads to HEAD, so every attempt diffs from the merge base.
// Then treeScope is read: a tree-scoped diff runs the tree-scoped command it answers, and passes with a logged note, running nothing, when that command is empty;
// any other diff runs the told command.
// The form has no skip path: an empty told command is an error naming the source, never a pass, and so is a nil mergeBase.
//
// `StatusPassed` and `StatusSkipped` pass.
// `StatusDirty` fails with the dirty paths.
// `StatusFailed` fails with the exit code, the log path and the log's tail, and, after a CPU watchdog quit, the window and the `busy_quit_sec` way forward; a test file the command's guard scan rejects, such as a misplaced `//lym:guard` marker, reaches the findings through that tail with its file and line.
// A headroom finding or a comment lint finding fails with its file and line or its bytes and the way forward.
// A command, merge base or tree scope read error, a failure to read git, a lint run error or a cancelled verify is a returned error, since none is a defect the writer can fix.
func NewVerifyGate(worktreeRoot, verifyDir, siteLabel string, command, mergeBase func() (string, error), treeScope func() (string, bool, error), hold *verifytree.PauseHold) shuttleengine.Gate {
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

		var lintBase, headroomBase string
		runCommand := told
		if mergeBase != nil {
			headroomBase, err = mergeBase()
			if err != nil {
				return shuttleengine.GateResult{}, fmt.Errorf("loomshed: verify gate: read the merge base: %w", err)
			}
		}
		if darnForm {
			lintBase = headroomBase
		} else if pass, ok := verifytree.LatestPass(paths, told); ok && isAncestorOfHead(worktreeRoot, pass.Commit) {
			lintBase = pass.Commit
		} else {
			lintBase = headroomBase
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
			}
		}

		site := verifytree.Site{Label: siteLabel, Attempt: attempt, BaseCommand: told, Hold: hold}
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

// verifyFailureFindings renders a StatusFailed result as the exit code or the timeout, the log path, and the failing tests and packages the log names, each with its own output, and the gate's closing line of failed runs;
// a log that names no failing test or package is quoted as its tail instead, since in the output of parallel packages the last lines need not hold the failures.
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
	if failures := failuresSection(log); failures != "" {
		fmt.Fprintf(&b, "\nFailures:\n\n%s\n", failures)
		if line := failedRunsLine(log); line != "" {
			fmt.Fprintf(&b, "\n%s\n", line)
		}
	} else if tail := logTail(log); tail != "" {
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

// failuresSection returns the failing tests and packages websterengine.ParseVerifyFailures names in log, each as its identity and its own output, within verifyFailuresBytes, or "" when it names none.
// A test whose output the parser found nested under its header gets that output, and otherwise the output above its header, where `lym gate test` renders a test's output.
func failuresSection(log []byte) string {
	lines := strings.Split(strings.ReplaceAll(string(log), "\r\n", "\n"), "\n")
	var b strings.Builder
	for _, failure := range websterengine.ParseVerifyFailures(string(log)) {
		if failure.Kind == websterengine.FailureKindOpaque {
			continue
		}
		output := strings.TrimSpace(failure.Tail)
		if output == "" && failure.Kind == websterengine.FailureKindTest {
			output = outputAboveHeader(lines, failure.Package, strings.TrimPrefix(failure.ID, failure.Package+"."))
		}
		entry := fmt.Sprintf("%s:\n%s\n\n", failure.ID, output)
		if len(entry) > verifyFailuresBytes {
			cut := verifyFailuresBytes - len(verifyFailureTruncated) - len("\n\n")
			for cut > 0 && !utf8.RuneStart(entry[cut]) {
				cut--
			}
			entry = entry[:cut] + verifyFailureTruncated + "\n\n"
		}
		if b.Len() > 0 && b.Len()+len(entry) > verifyFailuresBytes {
			b.WriteString("More failures are in the full log.\n")
			break
		}
		b.WriteString(entry)
	}
	return strings.TrimSpace(b.String())
}

// failedRunsLine returns the last line of log that `lym gate test` closes a failed call with, naming each failed run, or "" when log holds none.
func failedRunsLine(log []byte) string {
	lines := strings.Split(strings.TrimSpace(string(log)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); strings.HasPrefix(line, gateslot.FailedRunsPrefix) {
			return line
		}
	}
	return ""
}

// verifyOutputAboveLines caps the output outputAboveHeader quotes for one test.
const verifyOutputAboveLines = 40

// outputAboveHeader returns the output lines directly above the first `--- FAIL: <test> ` header in the block of package pkg, back to the nearest `=== ` or `--- ` marker line, at most verifyOutputAboveLines of them, or "" when no block of pkg holds such a header.
// A block runs from the line after the previous package's closing line to pkg's own `FAIL\t<pkg>` line, so a test name another package shares never borrows that package's output.
func outputAboveHeader(lines []string, pkg, test string) string {
	header := "--- FAIL: " + test + " "
	blockStart := 0
	for closing, line := range lines {
		isFail := strings.HasPrefix(line, "FAIL\t")
		if !isFail && !strings.HasPrefix(line, "ok  \t") && !strings.HasPrefix(line, "?   \t") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "FAIL\t"))
		if !isFail || len(fields) == 0 || fields[0] != pkg {
			blockStart = closing + 1
			continue
		}
		for i := blockStart; i < closing; i++ {
			if !strings.HasPrefix(strings.TrimSpace(lines[i]), header) {
				continue
			}
			start := i
			for start > blockStart && i-start < verifyOutputAboveLines {
				above := strings.TrimSpace(lines[start-1])
				if strings.HasPrefix(above, "=== ") || strings.HasPrefix(above, "--- ") {
					break
				}
				start--
			}
			return strings.TrimSpace(strings.Join(lines[start:i], "\n"))
		}
		blockStart = closing + 1
	}
	return ""
}
