// verifygate.go declares the clean-tree and post-merge landing and publish gate checks both landing producers run around their parent merge-in:
// a thin adapter over internal/verifytree, which owns the clean-tree check, the verified-tree record and the run.
// A dirty tree or a failing verify is reported as a Stuck reason, so nothing leaves the worktree on a tree that is not committed and verified.

package landingshed

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// dirtyPathsShown caps how many dirty paths a Stuck reason names.
const dirtyPathsShown = 10

// verifyGate holds the row's gate key, the told command and base command closures, the told gate tier lookup, the told wait-mark callback, the verify paths and the two verifytree seams.
// The dirty, verify and now fields are in-package seams tests replace with fakes,
// so unit tests never spawn git or a shell and the mark's start time is fixed.
// A zero dirty seam skips the clean-tree check, a nil command skips the landing verify, a nil baseCommand names no base command and a nil waitMark marks nothing.
type verifyGate struct {
	// key is the gate key of the row the gate belongs to: the key command renders and the kind a failure record carries.
	key string
	// command renders the call of a gate key, and baseCommand the call whose record entry every pass keeps.
	command     func(gateKey string) (string, error)
	baseCommand func() (string, error)
	// tiers lists the tier numbers of a named gate key; recordFailure reads it for a log that names no failing tier.
	tiers    func(gate string) ([]int, error)
	waitMark func(label string, start time.Time) error
	// hold is the told pause hold both verifies run under; nil holds nothing.
	hold   *verifytree.PauseHold
	paths  verifytree.Paths
	dirty  func(worktree string) ([]string, error)
	verify func(ctx context.Context, p verifytree.Paths, site verifytree.Site, command string) (verifytree.Result, error)
	now    func() time.Time
	// recorder is set by Parent-Align and Finalize: nil leaves a failed verify unrecorded.
	recorder *failureRecorder
}

// failureRecorder holds what the gate needs to write the landing failure record.
// mergeCommit is set by the producer after its merge-in and read when a verify fails.
type failureRecorder struct {
	failingTests func(log string) []verifytree.FailedTest
	// traceFailure traces the log to the plan; nil traces nothing.
	traceFailure func(log, head string) (verifytree.FailureTrace, error)
	head         func() (string, error)
	mergeCommit  string
}

// recordFailure writes the landing failure record for a verify of the row's gate key that ended in result.
// The record's kind is that key, and its tier is the first failing tier the log's summary lines name;
// a log without one, such as a refusal before the first tier, takes the highest tier of the key.
// A result that is not a failed run, a timeout and a shell that could not start write none, since none of them names a failing test.
// It returns whether it wrote the record: a write failure is logged, reported as false and changes nothing else.
func (g verifyGate) recordFailure(result verifytree.Result) bool {
	kind := verifytree.FailureKind(g.key)
	if g.recorder == nil || result.Status != verifytree.StatusFailed || result.TimedOut || result.ExitCode < 0 {
		return false
	}
	failure := verifytree.LandingFailure{
		Kind:        kind,
		LogPath:     result.Log,
		MergeCommit: g.recorder.mergeCommit,
		WrittenAt:   g.now().UTC().Format(time.RFC3339),
	}
	if g.recorder.head != nil {
		head, err := g.recorder.head()
		if err != nil {
			logger.Warn("landingshed: could not read HEAD for the landing failure record", "cause", err)
		}
		failure.Head = head
	}
	var log string
	content, readErr := os.ReadFile(result.Log)
	if readErr == nil {
		log = string(content)
	}
	failedTier, build, tierFound := gateslot.FailedTier(log)
	failure.Build = tierFound && build
	if readErr == nil && !failure.Build && g.recorder.failingTests != nil {
		failure.Tests = g.recorder.failingTests(log)
	}
	if g.recorder.traceFailure != nil && log != "" && failure.Head != "" {
		trace, err := g.recorder.traceFailure(log, failure.Head)
		if err != nil {
			logger.Warn("landingshed: could not trace the failed verify to the plan", "kind", kind, "cause", err)
		} else {
			failure.Cards = trace.Cards
			failure.LaterRange = trace.LaterRange
			failure.Untraced = trace.Untraced
		}
	}
	if tierFound {
		failure.Tier = failedTier
	} else {
		list, err := g.tiers(g.key)
		if err != nil || len(list) == 0 {
			logger.Warn("landingshed: could not read the gate's tiers for the landing failure record", "gate", g.key, "tiers", len(list), "cause", err)
			return false
		}
		failure.Tier = slices.Max(list)
	}
	if err := verifytree.WriteLandingFailure(g.paths, failure); err != nil {
		logger.Warn("landingshed: could not write the landing failure record", "kind", kind, "cause", err)
		return false
	}
	return true
}

// clearFailure removes the landing failure record of an earlier failed verify.
// It does nothing for a gate with no recorder, and a removal failure is logged.
func (g verifyGate) clearFailure() {
	if g.recorder == nil {
		return
	}
	if err := verifytree.RemoveLandingFailure(g.paths); err != nil {
		logger.Warn("landingshed: could not remove the landing failure record", "cause", err)
	}
}

// newVerifyGate copies the gate's told values from deps and wires the real verifytree functions.
func newVerifyGate(deps Deps) verifyGate {
	return verifyGate{
		key:         deps.GateKey,
		command:     deps.VerifyCommand,
		baseCommand: deps.BaseCommand,
		tiers:       deps.GateTiers,
		waitMark:    deps.VerifyWaitMark,
		hold:        deps.VerifyHold,
		paths:       verifytree.NewPaths(deps.WorktreeRoot, deps.VerifyDir),
		dirty:       verifytree.DirtyPaths,
		now:         time.Now,
		verify: func(ctx context.Context, p verifytree.Paths, site verifytree.Site, command string) (verifytree.Result, error) {
			return verifytree.Verify(ctx, p, site, command)
		},
	}
}

// markWait sets the driver strand's pane mark to label, started now, and clears it when label is empty.
// A failure is logged and changes nothing else: the mark is display only.
func (g verifyGate) markWait(label string) {
	if g.waitMark == nil {
		return
	}
	var start time.Time
	if label != "" {
		start = g.now()
	}
	if err := g.waitMark(label, start); err != nil {
		logger.Warn("landingshed: could not set the verify wait mark", "label", label, "cause", err)
	}
}

// clean checks that the task worktree has no uncommitted changes at point, a phrase such as "before the merge-in".
//
// It returns a non-empty Stuck reason naming the dirty paths, a non-nil error when git cannot report the status,
// and ("", nil) when the tree is clean.
func (g verifyGate) clean(producer, point string) (string, error) {
	if g.dirty == nil {
		return "", nil
	}
	paths, err := g.dirty(g.paths.Worktree)
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: check the worktree is clean %s: %w", producer, point, err)
	}
	if len(paths) == 0 {
		return "", nil
	}
	return dirtyReason(point, paths), nil
}

// dirtyWayForward ends every dirty-tree reason with what the operator does about it.
const dirtyWayForward = "; commit or remove them on the task branch, then resume"

// dirtyReason is the Stuck reason for a dirty tree found at point.
func dirtyReason(point string, paths []string) string {
	return fmt.Sprintf("the task worktree has uncommitted changes %s: %s%s", point, listShown(paths), dirtyWayForward)
}

// listShown names the first dirtyPathsShown of items, comma-separated, and counts the rest.
func listShown(items []string) string {
	shown := items
	if len(shown) > dirtyPathsShown {
		shown = shown[:dirtyPathsShown]
	}
	list := strings.Join(shown, ", ")
	if more := len(items) - len(shown); more > 0 {
		list = fmt.Sprintf("%s and %d more", list, more)
	}
	return list
}

// readBase returns the base command, the one whose record entry every pass of this gate keeps.
// A gate told no base command names none.
func (g verifyGate) readBase(producer string) (string, error) {
	if g.baseCommand == nil {
		return "", nil
	}
	base, err := g.baseCommand()
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: read base command: %w", producer, err)
	}
	return base, nil
}

// check runs the call of the row's gate key for producer after a merge of parentBranch.
// verifytree skips the run when its record already names the tree and the command,
// so the verify runs exactly when the tree differs from the last verified one:
// after a resolved conflict, after new parent commits and after a crash between a merge and its verify.
// The base command is the site's BaseCommand, so a pass keeps the entry the next round diffs from.
//
// It returns a non-empty Stuck reason naming the gate when the call fails or the tree is dirty,
// a non-nil error for an infrastructure fault or a cancellation, and ("", nil) when the producer may proceed.
// A pass, and a skip over a tree already recorded as passed, remove a landing failure record an earlier failed call left.
func (g verifyGate) check(ctx context.Context, producer, parentBranch string) (string, error) {
	if g.command == nil {
		return "", nil
	}

	command, err := g.command(g.key)
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: read gate command: %w", producer, err)
	}
	if command == "" {
		logger.Warn("landingshed: post-merge verify skipped because no verify command is configured", "producer", producer, "parentBranch", parentBranch)
		return "", nil
	}
	base, err := g.readBase(producer)
	if err != nil {
		return "", err
	}

	g.markWait("")
	g.markWait("verify " + producer)
	result, err := g.verify(ctx, g.paths, verifytree.Site{Label: producer, BaseCommand: base, Hold: g.hold}, command)
	g.markWait("")
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: %w", producer, err)
	}
	g.recordFailure(result)
	reason, err := g.verdict(result, "landing gate", producer, parentBranch)
	if reason == "" && err == nil {
		g.clearFailure()
	}
	return reason, err
}

// checkHead runs the call of the row's gate key over the task head for producer, the PR-Review gate over an agent's commits.
// It reads the key's command and base command and marks the wait as check does, skipping with a warning when no command is configured, and writes no landing failure record.
//
// It returns the reason the gate refuses the head, written for the agent to act on in a new commit, an error for an infrastructure fault or a cancellation, and ("", nil) on a pass or a skip.
// A dirty result returns the dirty reason.
func (g verifyGate) checkHead(ctx context.Context, producer string) (string, error) {
	if g.command == nil {
		return "", nil
	}
	command, err := g.command(g.key)
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: read gate command: %w", producer, err)
	}
	if command == "" {
		logger.Warn("landingshed: the pr-review gate skipped because no verify command is configured", "producer", producer)
		return "", nil
	}
	base, err := g.readBase(producer)
	if err != nil {
		return "", err
	}

	g.markWait("")
	g.markWait("verify " + producer)
	result, err := g.verify(ctx, g.paths, verifytree.Site{Label: producer, BaseCommand: base, Hold: g.hold}, command)
	g.markWait("")
	if err != nil {
		return "", fmt.Errorf("landingshed: %s: %w", producer, err)
	}
	switch result.Status {
	case verifytree.StatusPassed:
		logger.Info("landingshed: the pr-review gate passed", "producer", producer)
	case verifytree.StatusSkipped:
		logger.Info("landingshed: the pr-review gate skipped because the tree is already verified", "producer", producer, "tree", result.Tree)
	case verifytree.StatusDirty:
		return dirtyReason("when the pr-review gate was about to run", result.Dirty), nil
	case verifytree.StatusFailed:
		return g.headFailureReason(result), nil
	default:
		return "", fmt.Errorf("landingshed: %s: unknown pr-review gate status %q", producer, result.Status)
	}
	return "", nil
}

// headFailureReason is the reason a failed run of the pr-review gate gives the agent: the failing tier or the build, the log and the instruction to fix it in a new commit.
// A timeout and a shell that could not start name no tier, since neither names a failing test.
func (g verifyGate) headFailureReason(result verifytree.Result) string {
	switch {
	case result.TimedOut:
		return fmt.Sprintf("the pr-review gate timed out: %s; output: %s; fix the hanging test or build in a new commit", result.Detail, result.Log)
	case result.ExitCode < 0:
		return fmt.Sprintf("the pr-review gate could not start: %s; output: %s", result.Detail, result.Log)
	}
	var log string
	if content, err := os.ReadFile(result.Log); err == nil {
		log = string(content)
	}
	if tier, build, found := gateslot.FailedTier(log); found {
		if build {
			return fmt.Sprintf("the pr-review gate build failed; output: %s; fix it in a new commit", result.Log)
		}
		return fmt.Sprintf("the pr-review gate failed at tier %d; output: %s; fix it in a new commit", tier, result.Log)
	}
	if g.tiers != nil {
		if list, err := g.tiers(g.key); err == nil && len(list) > 0 {
			return fmt.Sprintf("the pr-review gate failed at tier %d; output: %s; fix it in a new commit", slices.Max(list), result.Log)
		}
	}
	return fmt.Sprintf("the pr-review gate failed; output: %s; fix it in a new commit", result.Log)
}

// watchdogClause returns the clause a failed verify's reason carries when the log at logPath holds the CPU watchdog's line, and empty for an unreadable log or one without it.
// The clause names the window, says the goroutine stacks are in the log, and gives the way forward for a package that is CPU-bound but healthy.
func watchdogClause(logPath string) string {
	log, err := os.ReadFile(logPath)
	if err != nil {
		return ""
	}
	window, quit := gateslot.FindWatchdogQuit(string(log))
	if !quit {
		return ""
	}
	return fmt.Sprintf("; the gate test's CPU watchdog quit a test binary that kept a CPU core busy for %s, with its goroutine stacks in the log; if the package is CPU-bound but healthy, %s, then resume", window, gateslot.BusyQuitWayForward)
}

// verdict maps a verify result to the producer's decision, naming the gate whose call ran as what.
// It returns a non-empty Stuck reason for a dirty tree or a failed command, an error for an unknown status, and ("", nil) when the producer may proceed.
// A failed command with an exit code ends its reason with the clause asking to fix forward on the task branch.
// After a CPU watchdog quit the reason names the window and the busy_quit_sec way forward before that clause.
func (g verifyGate) verdict(result verifytree.Result, what, producer, parentBranch string) (string, error) {
	switch result.Status {
	case verifytree.StatusPassed:
		logger.Info("landingshed: post-merge "+what+" passed", "producer", producer, "parentBranch", parentBranch)
	case verifytree.StatusSkipped:
		logger.Info("landingshed: post-merge "+what+" skipped because the tree is already verified", "producer", producer, "parentBranch", parentBranch, "tree", result.Tree)
	case verifytree.StatusDirty:
		return dirtyReason("when the "+what+" was about to run", result.Dirty), nil
	case verifytree.StatusFailed:
		if result.TimedOut && strings.HasPrefix(result.Detail, "gate build ") {
			return fmt.Sprintf("%s timed out after merging parent branch %q: %s; output: %s; fix the hanging build, such as a stalled restore or compile, on the task branch, or, for a build that is slow but healthy, rewrite the build whole from the prime with `lym config gate --set 'build={...}'`, carrying every key of the build and the raised timeout_min, since a dotted key under the build writes an entry the loader refuses; a session that is not the prime reports status: FAILED and the orch runs it; then resume", what, parentBranch, result.Detail, result.Log), nil
		}
		if result.TimedOut {
			return fmt.Sprintf("%s timed out after merging parent branch %q: %s; output: %s; fix the hanging test on the task branch, or, for a tier that is slow but healthy, rewrite the tier whole from the prime with `lym config gate --set 'tiers.tier<N>={...}'`, carrying every key of the tier and the raised timeout_min, since a dotted key under the tier writes an entry the loader refuses; a session that is not the prime reports status: FAILED and the orch runs it; then resume", what, parentBranch, result.Detail, result.Log), nil
		}
		if result.ExitCode < 0 {
			return fmt.Sprintf("%s could not start after merging parent branch %q: %s; output: %s", what, parentBranch, result.Detail, result.Log), nil
		}
		watchdog := watchdogClause(result.Log)
		return fmt.Sprintf("%s failed after merging parent branch %q (exit code %d); output: %s%s; fix forward on the task branch, then resume", what, parentBranch, result.ExitCode, result.Log, watchdog), nil
	default:
		return "", fmt.Errorf("landingshed: %s: unknown %s status %q", producer, what, result.Status)
	}
	return "", nil
}
