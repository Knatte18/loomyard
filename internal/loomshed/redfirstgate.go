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

// RedFirstGate holds the two closures of a red-first gate entry, which share one attempt count.
type RedFirstGate struct {
	// Gate is the closure each arrival runs.
	// It answers pending on a refusal the writer's next turn may not lift, so its entry must be MayHold.
	Gate shuttleengine.Gate
	// Final is the closure a final evaluation runs in place of Gate: it fails on that refusal with its findings.
	Final shuttleengine.Gate
}

// NewRedFirstGate returns the closures of a must-pass gate entry that proves a bug fix's test was red before the fix.
// Each call reads the entry's labels, and an entry without gateslot.BugLabel passes at once and runs nothing.
// It then reads the decision record at recordPath.
// An added entry there whose title opens with gateslot.NoTestSeamTitle or gateslot.NoBugFixedTitle passes the same way.
// The title is compared case-insensitively.
// Otherwise it runs command through verifytree.Verify over worktreeRoot, keeping its record, marker and log in verifyDir.
// command is the site's own `lym gate red-first --gate <key>` call.
// siteLabel names the call site in the running marker, and the closures' shared count of their calls is the attempt.
// hold is the told pause hold of the worktree's scope; nil holds nothing.
//
// `StatusPassed` and `StatusSkipped` pass, and `StatusDirty` fails with the dirty paths.
// A `StatusFailed` run whose exit code is gateslot.RedFirstGreenExit fails with the way forward of a green test.
// An exit of gateslot.RedFirstUnjudgedExit passes:
// the check cannot judge the tests, as on a hub whose scoper is neither go nor dotnet.
// Holding would halt a bug fix that no change of the writer's can let through.
// An exit of gateslot.RedFirstNoTestExit waits on a new commit or the parent's record.
// Gate answers it pending, so the run holds without spending an attempt.
// A hold's pending answers carry one line telling the writer its way forward until the loop reports it delivered;
// later ones send nothing.
// While HEAD's tree stays the one the refusal judged, Gate answers pending again without running command.
// Final answers that refusal as a failure whose findings name the same way forward.
// On that same tree it runs no command again.
// Any other failed run, a timeout or an unjudged run included, judged none of the diff's tests and fails terminally:
// no change to the code mends it, and resuming the run runs the check again.
// A labels read error, a record read or parse error, a HEAD read error and a verify error are returned errors.
func NewRedFirstGate(worktreeRoot, verifyDir, siteLabel, command string, labels func() ([]string, error), recordPath string, hold *verifytree.PauseHold) RedFirstGate {
	g := &redFirstGate{
		paths:        verifytree.NewPaths(worktreeRoot, verifyDir),
		worktreeRoot: worktreeRoot,
		siteLabel:    siteLabel,
		command:      command,
		labels:       labels,
		recordPath:   recordPath,
		hold:         hold,
	}
	return RedFirstGate{Gate: g.arrival, Final: g.final}
}

// redFirstGate is the state a red-first entry's closures share.
type redFirstGate struct {
	paths        verifytree.Paths
	worktreeRoot string
	siteLabel    string
	command      string
	labels       func() ([]string, error)
	recordPath   string
	hold         *verifytree.PauseHold
	// attempt counts the closures' calls.
	attempt int
	// heldTree is the HEAD tree a held refusal judged, empty while the gate holds nothing.
	heldTree string
	// heldResult is the refusal the hold judged on heldTree.
	heldResult verifytree.Result
	// holdLine is the line the hold offers the writer until told.
	holdLine string
	// told reports that the loop delivered holdLine to the writer.
	told bool
}

// arrival is the entry's Gate closure.
func (g *redFirstGate) arrival() (shuttleengine.GateResult, error) {
	g.attempt++
	exempt, err := g.exempt()
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	if exempt {
		g.heldTree = ""
		return shuttleengine.GateResult{Passed: true}, nil
	}
	if g.heldTree != "" {
		tree, err := verifytree.HeadTree(g.worktreeRoot)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("loomshed: red-first gate: %w", err)
		}
		if tree == g.heldTree {
			return g.holdResult(), nil
		}
	}

	res, verdict, err := g.verify()
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	if verdict != nil {
		g.heldTree = ""
		return *verdict, nil
	}
	if !holdsForCommitOrRecord(res) {
		g.heldTree = ""
		return redFirstFailure(res), nil
	}
	if g.heldTree == "" {
		logger.Info("loomshed: red-first gate holds until a new commit or the parent's record", "gate", g.siteLabel, "attempt", g.attempt, "exitCode", res.ExitCode, "log", res.Log)
		g.told = false
	}
	if !g.told {
		g.holdLine = redFirstHoldLine(res)
	}
	g.heldTree = res.Tree
	g.heldResult = res
	return g.holdResult(), nil
}

// holdResult is the pending answer of a hold.
// It offers the hold line until the loop reports it delivered.
func (g *redFirstGate) holdResult() shuttleengine.GateResult {
	result := shuttleengine.GateResult{Pending: true, SendFailedWayForward: redFirstHoldSendFailed, Delivered: &g.told}
	if !g.told {
		result.Send = g.holdLine
	}
	return result
}

// final is the entry's Final closure.
func (g *redFirstGate) final() (shuttleengine.GateResult, error) {
	g.attempt++
	exempt, err := g.exempt()
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	if exempt {
		return shuttleengine.GateResult{Passed: true}, nil
	}
	if g.heldTree != "" {
		tree, err := verifytree.HeadTree(g.worktreeRoot)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("loomshed: red-first gate: %w", err)
		}
		if tree == g.heldTree {
			return redFirstFailure(g.heldResult), nil
		}
	}
	res, verdict, err := g.verify()
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	if verdict != nil {
		return *verdict, nil
	}
	return redFirstFailure(res), nil
}

// exempt reports whether the entry needs no red-first check:
// it is not labelled a bug, or the parent recorded that the bug has no test seam or that the change fixes no bug.
func (g *redFirstGate) exempt() (bool, error) {
	entryLabels, err := g.labels()
	if err != nil {
		return false, fmt.Errorf("loomshed: red-first gate: read the entry's labels: %w", err)
	}
	if !slices.Contains(entryLabels, gateslot.BugLabel) {
		return true, nil
	}
	record, err := os.ReadFile(g.recordPath)
	if err != nil {
		return false, fmt.Errorf("loomshed: red-first gate: read the decision record: %w", err)
	}
	added, err := discussionparser.ParseAddedDecisions(record)
	if err != nil {
		return false, fmt.Errorf("loomshed: red-first gate: parse the decision record's added entries: %w", err)
	}
	for _, entry := range added {
		for _, title := range []string{gateslot.NoTestSeamTitle, gateslot.NoBugFixedTitle} {
			if strings.HasPrefix(strings.ToLower(entry.Title), strings.ToLower(title)) {
				logger.Info("loomshed: red-first gate ran nothing, the decision record lifts the check", "gate", g.siteLabel, "attempt", g.attempt, "title", entry.Title)
				return true, nil
			}
		}
	}
	return false, nil
}

// verify runs the red-first command.
// It returns the verdict of a run that passed, was skipped, found the tree dirty or could not judge the tests,
// and a nil verdict with the result of a run that failed.
func (g *redFirstGate) verify() (verifytree.Result, *shuttleengine.GateResult, error) {
	site := verifytree.Site{Label: g.siteLabel, Attempt: g.attempt, BaseCommand: g.command, Hold: g.hold}
	res, err := verifytree.Verify(context.Background(), g.paths, site, g.command)
	if err != nil {
		return verifytree.Result{}, nil, fmt.Errorf("loomshed: red-first gate: %w", err)
	}
	switch res.Status {
	case verifytree.StatusPassed, verifytree.StatusSkipped:
		return res, &shuttleengine.GateResult{Passed: true}, nil
	case verifytree.StatusDirty:
		dirty := dirtyResult(res, g.siteLabel, g.attempt)
		return res, &dirty, nil
	}
	if res.ExitCode == gateslot.RedFirstUnjudgedExit && !res.TimedOut && !res.Unjudged() {
		logger.Info("loomshed: red-first gate passed unjudged, the check cannot judge the tests", "gate", g.siteLabel, "attempt", g.attempt, "log", res.Log)
		return res, &shuttleengine.GateResult{Passed: true}, nil
	}
	logger.Warn("loomshed: red-first gate failed", "gate", g.siteLabel, "attempt", g.attempt, "exitCode", res.ExitCode, "timedOut", res.TimedOut, "runnerCrash", res.RunnerCrash, "memoryPressure", res.MemoryPressure, "log", res.Log)
	return res, nil, nil
}

// holdsForCommitOrRecord reports whether a failed red-first run is a refusal the writer's next turn may not lift.
// Only a commit that adds a failing test, or the parent's record that lifts the gate, lifts a diff with no test.
func holdsForCommitOrRecord(res verifytree.Result) bool {
	if res.TimedOut || res.Unjudged() {
		return false
	}
	return res.ExitCode == gateslot.RedFirstNoTestExit
}

// redFirstHoldLine is the one line a hold's first arrival sends the writer, naming the way forward of res's refusal.
func redFirstHoldLine(res verifytree.Result) string {
	return fmt.Sprintf("The red-first gate holds your handoff, spending no attempt, since the diff adds or changes no test (log: %s): "+
		"add and commit a test that fails on the merge base and passes with the fix, then end your turn; "+
		"when the bug has no seam a test can reach, %s", res.Log, parentRecordAsk(gateslot.NoTestSeamTitle))
}

// redFirstHoldSendFailed is the way forward the wait loop logs when the hold's line cannot reach the writer.
// The hold offers the line again until it is delivered.
const redFirstHoldSendFailed = "the writer was not told why the red-first gate holds; " +
	"the hold lifts on a commit that adds a failing test or on the parent's `lym loom decision add` record under a title opening \"" +
	gateslot.NoTestSeamTitle + "\""

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
	case res.Unjudged():
		terminal = true
		fmt.Fprintf(&b, "The red-first check's %s, so it judged none of the diff's tests: %s. No change to the code mends it, so the run stops here; resuming it runs the check again, and %s.", res.UnjudgedStop(), res.Detail, res.UnjudgedAdvice())
	case res.ExitCode == gateslot.RedFirstGreenExit:
		fmt.Fprintf(&b, "Every test the diff adds or changes passed on the merge base, so none of them fails before the change.\n\n"+
			"Way forward: when the change fixes a bug, make a test fail on the merge base and pass with the fix, "+
			"reaching the bug through code the base already has, and commit it. "+
			"When the change fixes no bug, as one that only adds tests pinning existing behaviour, invent no bug and keep the tests: %s",
			parentRecordAsk(gateslot.NoBugFixedTitle))
	case res.ExitCode == gateslot.RedFirstNoTestExit:
		fmt.Fprintf(&b, "The diff adds or changes no test, so nothing shows the bug the fix fixes.\n\n"+
			"Way forward: add and commit a test that fails on the merge base and passes with the fix. "+
			"When the bug has no seam a test can reach, %s", parentRecordAsk(gateslot.NoTestSeamTitle))
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

// parentRecordAsk is a way forward that only the run's parent can take:
// it records, under an added decision whose title opens title, why the red-first check does not apply.
func parentRecordAsk(title string) string {
	return fmt.Sprintf("send your parent `lym creel post --parent --expect-reply \"<text>\"` asking it to record that with "+
		"`lym loom decision add` under a title opening %q and naming why, then end your turn.", title)
}

// redFirstNoMend closes the findings of a red-first run that judged none of the diff's tests.
const redFirstNoMend = "No change to the code mends it, so the run stops here; resuming it runs the check again."
