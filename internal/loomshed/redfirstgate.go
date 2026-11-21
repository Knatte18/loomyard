// redfirstgate.go implements NewRedFirstGate, the must-pass gate that holds a bug fix until its test is shown red first.
// It runs the site's red-first command through verifytree.Verify and reads the exit code it fails with.

package loomshed

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// NewRedFirstGate returns a must-pass shuttleengine.Gate that proves a bug fix's test was red before the fix.
// At each arrival it reads the entry's labels, and an entry without gateslot.BugLabel passes at once and runs nothing.
// It then reads the decision record at recordPath.
// An added entry whose title opens with gateslot.NoTestSeamTitle passes at once and runs nothing.
// The title is compared case-insensitively.
// Otherwise it runs command through verifytree.Verify over worktreeRoot, keeping its record, marker and log in verifyDir.
// command is the site's own `lym gate red-first --gate <key>` call.
// siteLabel names the call site in the running marker, and the closure's own count of its calls is the attempt.
// hold is the told pause hold of the worktree's scope; nil holds nothing.
//
// `StatusPassed` and `StatusSkipped` pass, and `StatusDirty` fails with the dirty paths.
// A `StatusFailed` run whose exit code is gateslot.RedFirstGreenExit fails with the way forward of a green test.
// One whose exit code is gateslot.RedFirstNoTestExit fails with the way forward of a diff that holds no test.
// One whose exit code is gateslot.RedFirstUnjudgedExit fails with the way forward of a check that cannot judge the diff.
// Any other failed run, a timeout or a runner crash included, judged none of the diff's tests and fails terminally:
// no change to the code mends it, and resuming the run runs the check again.
// A labels read error, a record read or parse error and a verify error are returned errors.
func NewRedFirstGate(worktreeRoot, verifyDir, siteLabel, command string, labels func() ([]string, error), recordPath string, hold *verifytree.PauseHold) shuttleengine.Gate {
	attempt := 0
	paths := verifytree.NewPaths(worktreeRoot, verifyDir)
	return func() (shuttleengine.GateResult, error) {
		attempt++
		entryLabels, err := labels()
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("loomshed: red-first gate: read the entry's labels: %w", err)
		}
		if !slices.Contains(entryLabels, gateslot.BugLabel) {
			return shuttleengine.GateResult{Passed: true}, nil
		}
		record, err := os.ReadFile(recordPath)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("loomshed: red-first gate: read the decision record: %w", err)
		}
		added, err := discussionparser.ParseAddedDecisions(record)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("loomshed: red-first gate: parse the decision record's added entries: %w", err)
		}
		for _, entry := range added {
			if strings.HasPrefix(strings.ToLower(entry.Title), strings.ToLower(gateslot.NoTestSeamTitle)) {
				logger.Info("loomshed: red-first gate ran nothing, the decision record names a bug no test can reach", "gate", siteLabel, "attempt", attempt, "title", entry.Title)
				return shuttleengine.GateResult{Passed: true}, nil
			}
		}

		site := verifytree.Site{Label: siteLabel, Attempt: attempt, BaseCommand: command, Hold: hold}
		res, err := verifytree.Verify(context.Background(), paths, site, command)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("loomshed: red-first gate: %w", err)
		}
		switch res.Status {
		case verifytree.StatusPassed, verifytree.StatusSkipped:
			return shuttleengine.GateResult{Passed: true}, nil
		case verifytree.StatusDirty:
			return dirtyResult(res, siteLabel, attempt), nil
		}
		logger.Warn("loomshed: red-first gate failed", "gate", siteLabel, "attempt", attempt, "exitCode", res.ExitCode, "timedOut", res.TimedOut, "runnerCrash", res.RunnerCrash, "log", res.Log)
		return redFirstFailure(res), nil
	}
}

// redFirstFailure renders a StatusFailed red-first result as a failing verdict.
// Only a run that exited with one of the gateslot red-first exit codes is one the writer, or its parent, can mend.
func redFirstFailure(res verifytree.Result) shuttleengine.GateResult {
	log, _ := os.ReadFile(res.Log)
	var b strings.Builder
	terminal := false
	switch {
	case res.TimedOut:
		terminal = true
		fmt.Fprintf(&b, "The red-first check timed out, so it judged none of the diff's tests: %s. %s", res.Detail, redFirstNoMend)
	case res.RunnerCrash:
		terminal = true
		fmt.Fprintf(&b, "The red-first check's test runner crashed, so it judged none of the diff's tests: %s. %s", res.Detail, redFirstNoMend)
	case res.ExitCode == gateslot.RedFirstGreenExit:
		b.WriteString("Every test the diff adds or changes passed on the merge base, so none of them shows the bug the fix fixes.\n\n" +
			"Way forward: make a test fail on the merge base and pass with the fix, " +
			"reaching the bug through code the base already has, and commit it.")
	case res.ExitCode == gateslot.RedFirstNoTestExit:
		fmt.Fprintf(&b, "The diff adds or changes no test, so nothing shows the bug the fix fixes.\n\n"+
			"Way forward: add and commit a test that fails on the merge base and passes with the fix. "+
			"When the bug has no seam a test can reach, %s", noSeamAsk())
	case res.ExitCode == gateslot.RedFirstUnjudgedExit:
		fmt.Fprintf(&b, "The red-first check cannot judge the diff's tests on the merge base: "+
			"the hub's scoper is not go. The log tail names it.\n\nWay forward: %s", noSeamAsk())
	default:
		terminal = true
		fmt.Fprintf(&b, "The red-first check failed with exit code %d, so it judged none of the diff's tests. %s", res.ExitCode, redFirstNoMend)
		if res.Detail != "" {
			fmt.Fprintf(&b, " Its shell could not start: %s.", res.Detail)
		}
	}
	fmt.Fprintf(&b, "\n\nFull log: %s\n", res.Log)
	if tail := logTail(log); tail != "" {
		fmt.Fprintf(&b, "\nLog tail:\n\n%s\n", tail)
	}
	return shuttleengine.GateResult{Passed: false, Findings: b.String(), Terminal: terminal}
}

// noSeamAsk is the way forward that only the run's parent can take.
// The parent records the bug as having no seam a test can reach.
func noSeamAsk() string {
	return fmt.Sprintf("send your parent `lym creel post --parent --expect-reply \"<text>\"` asking it to record that with "+
		"`lym loom decision add` under a title opening %q and naming why, then end your turn.", gateslot.NoTestSeamTitle)
}

// redFirstNoMend closes the findings of a red-first run that judged none of the diff's tests.
const redFirstNoMend = "No change to the code mends it, so the run stops here; resuming it runs the check again."
