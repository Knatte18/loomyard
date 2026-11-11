// verifygate.go holds the clean-tree and verify gate checks the landing producers run around their parent merge-in.
// It adapts internal/verifytree, which owns the clean-tree check, the verified-tree record and the run.
// A dirty tree or a failing verify is a Stuck reason, so nothing leaves the worktree uncommitted or unverified.

package landingshed

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// dirtyPathsShown caps how many dirty paths a Stuck reason names.
const dirtyPathsShown = 10

// verifyGate holds the told values of one gate.
// The dirty, verify and now fields are in-package seams tests replace with fakes,
// so unit tests never spawn git or a shell and the mark's start time is fixed.
// A nil dirty seam skips the clean-tree check, and a nil command skips the landing verify.
// A nil baseCommand names no base command, and a nil waitMark marks nothing.
type verifyGate struct {
	// key is the gate key of the row the gate belongs to: the key command renders and the kind a failure record carries.
	key string
	// command renders the call of a gate key, and baseCommand the call whose record entry every pass keeps.
	command     func(gateKey string) (string, error)
	baseCommand func() (string, error)
	// tiers lists the tier numbers of a named gate key;
	// a failure record takes the highest of them for a log that names no failing tier.
	tiers    func(gate string) ([]int, error)
	waitMark func(label string, start time.Time) error
	// hold is the told pause hold the verifies run under; nil holds nothing.
	hold   *verifytree.PauseHold
	paths  verifytree.Paths
	dirty  func(worktree string) ([]string, error)
	verify func(ctx context.Context, p verifytree.Paths, site verifytree.Site, command string) (verifytree.Result, error)
	now    func() time.Time
	// recorder is set by the landing producers: nil leaves a failed verify unrecorded.
	recorder *failureRecorder
	// onFlakyPass, when set, makes check run a failure of test runs once more on the same head before judging it.
	// It gets each failure that rerun did not repeat; nil runs no rerun.
	onFlakyPass func(flakyPass)
}

// flakyPass is a failed verify that passed when run once more on the same head.
type flakyPass struct {
	// failures name the first run's failures, each opened by its run's label, such as `tier0 example.com/m/a TestA`.
	failures []string
	// log is the first run's verify log.
	log string
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
// A result that is not a failed run, nor a timeout, a runner crash or a shell that could not start, writes none.
// Those results name no failing test.
// It reports whether it wrote the record: a write failure is logged, reported as false and changes nothing else.
func (g verifyGate) recordFailure(result verifytree.Result) bool {
	kind := verifytree.FailureKind(g.key)
	if g.recorder == nil || result.Status != verifytree.StatusFailed || result.TimedOut || result.RunnerCrash || result.ExitCode < 0 {
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
// It returns a non-empty Stuck reason naming the dirty paths, or a non-nil error when git cannot report the status.
// It returns ("", nil) when the tree is clean.
func (g verifyGate) clean(producer, point string) (string, error) {
	paths, err := g.dirtyPaths(producer, point)
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return dirtyReason(point, paths), nil
}

// dirtyPaths returns the task worktree's uncommitted or untracked paths at point.
// It returns none for a gate told no dirty check, and a non-nil error when git cannot report the status.
func (g verifyGate) dirtyPaths(producer, point string) ([]string, error) {
	if g.dirty == nil {
		return nil, nil
	}
	paths, err := g.dirty(g.paths.Worktree)
	if err != nil {
		return nil, fmt.Errorf("landingshed: %s: check the worktree is clean %s: %w", producer, point, err)
	}
	return paths, nil
}

// dirtyWayForward ends every dirty-tree reason with what the operator does about it.
const dirtyWayForward = "; commit or remove them on the task branch, then resume"

// dirtyReason is the Stuck reason for a dirty tree found at point.
func dirtyReason(point string, paths []string) string {
	return fmt.Sprintf("the task worktree has uncommitted changes %s: %s%s", point, listShown(paths), dirtyWayForward)
}

// listShown names the first `dirtyPathsShown` of items, comma-separated, and counts the rest.
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
// It returns a non-empty Stuck reason naming the gate when the call fails or the tree is dirty.
// It returns a non-nil error for an infrastructure fault or a cancellation.
// It returns ("", false, nil) when the producer may proceed.
// crashed is set when every failed run of the call crashed its test runner with no failing test,
// so no test judged the code: the reason is then a halt reason opening with `LandingStopPrefix`.
// The producer halts with it instead of spawning a fixer.
// A pass, and a skip over a tree already recorded as passed, remove a landing failure record an earlier failed call left.
// A gate told onFlakyPass runs a failure once more on the same head when its report lists tier and guard runs alone.
// It then judges the rerun's result in its place.
func (g verifyGate) check(ctx context.Context, producer, parentBranch string) (reason string, crashed bool, err error) {
	if g.command == nil {
		return "", false, nil
	}

	command, err := g.command(g.key)
	if err != nil {
		return "", false, fmt.Errorf("landingshed: %s: read gate command: %w", producer, err)
	}
	if command == "" {
		logger.Warn("landingshed: post-merge verify skipped because no verify command is configured", "producer", producer, "parentBranch", parentBranch)
		return "", false, nil
	}
	base, err := g.readBase(producer)
	if err != nil {
		return "", false, err
	}

	site := verifytree.Site{Label: producer, BaseCommand: base, Hold: g.hold}
	g.markWait("")
	g.markWait("verify " + producer)
	result, err := g.verify(ctx, g.paths, site, command)
	g.markWait("")
	if err != nil {
		return "", false, fmt.Errorf("landingshed: %s: %w", producer, err)
	}
	if g.onFlakyPass != nil {
		if result, err = g.rerunTestFailure(ctx, producer, site, command, result); err != nil {
			return "", false, fmt.Errorf("landingshed: %s: %w", producer, err)
		}
	}
	g.recordFailure(result)
	reason, err = g.verdict(result, "landing gate", producer, parentBranch)
	if reason == "" && err == nil {
		g.clearFailure()
	}
	return reason, result.Status == verifytree.StatusFailed && result.RunnerCrash, err
}

// rerunTestFailure runs the failed verify first once more on the same head and returns the result the gate judges.
// A test can fail on a race with another test, so only a failure whose report lists tier and guard runs alone is rerun.
// A timeout, a runner crash and a shell that could not start come back as they are.
// So do a build, vet or lint failure, a package that failed outside any test and a log with no report.
// The rerun runs the report's failed tests alone when it names Go tests only, and command otherwise.
// A rerun that passes hands first's failures to onFlakyPass and returns the pass.
// Any other rerun result comes back, so a failure the rerun repeats is judged on the rerun's log.
func (g verifyGate) rerunTestFailure(ctx context.Context, producer string, site verifytree.Site, command string, first verifytree.Result) (verifytree.Result, error) {
	if first.Status != verifytree.StatusFailed || first.TimedOut || first.RunnerCrash || first.ExitCode < 0 || first.Log == "" {
		return first, nil
	}
	content, err := os.ReadFile(first.Log)
	if err != nil {
		logger.Warn("landingshed: could not read the failed verify's log, so the verify does not run again", "producer", producer, "log", first.Log, "cause", err)
		return first, nil
	}
	failures, testRuns := gateslot.TierRunFailures(string(content))
	if !testRuns {
		return first, nil
	}
	rerun := command
	if tests, named := gateslot.FailedTests(string(content)); named {
		call, err := gateslot.FailedTestsCall(runtime.GOOS, tests)
		if err != nil {
			logger.Warn("landingshed: cannot render the run of the failed tests alone; the whole verify runs again", "producer", producer, "cause", err)
		} else {
			rerun = call
		}
	}
	logger.Warn("landingshed: the verify failed in its test runs; it runs once more on the same head before any fixer", "producer", producer, "failures", strings.Join(failures, "; "), "log", first.Log, "rerun", rerun)
	g.markWait("verify " + producer)
	second, err := g.verify(ctx, g.paths, site, rerun)
	g.markWait("")
	if err != nil {
		return verifytree.Result{}, err
	}
	if second.Status == verifytree.StatusPassed || second.Status == verifytree.StatusSkipped {
		logger.Warn("landingshed: the failed verify passed when run once more on the same head, so its failures are flaky", "producer", producer, "failures", strings.Join(failures, "; "), "log", first.Log)
		g.onFlakyPass(flakyPass{failures: failures, log: first.Log})
	}
	return second, nil
}

// checkHead runs the pr-review gate over an agent's commits:
// the call of the row's gate key over the task head for producer.
// It reads the key's command and base command, and it marks the wait as verifying the producer while the call runs.
// It skips with a warning when no command is configured, and it writes no landing failure record.
//
// It returns the reason the gate refuses the head, written for the agent to act on in a new commit.
// It returns an error for an infrastructure fault or a cancellation, and ("", false, nil) on a pass or a skip.
// A dirty result returns the dirty reason.
// crashed is set when every failed run of the call crashed its test runner with no failing test:
// no test judged the head, so the reason is a halt reason opening with `LandingStopPrefix`.
// The caller halts the row with that reason instead of telling the agent.
func (g verifyGate) checkHead(ctx context.Context, producer string) (reason string, crashed bool, err error) {
	if g.command == nil {
		return "", false, nil
	}
	command, err := g.command(g.key)
	if err != nil {
		return "", false, fmt.Errorf("landingshed: %s: read gate command: %w", producer, err)
	}
	if command == "" {
		logger.Warn("landingshed: the pr-review gate skipped because no verify command is configured", "producer", producer)
		return "", false, nil
	}
	base, err := g.readBase(producer)
	if err != nil {
		return "", false, err
	}

	g.markWait("")
	g.markWait("verify " + producer)
	result, err := g.verify(ctx, g.paths, verifytree.Site{Label: producer, BaseCommand: base, Hold: g.hold}, command)
	g.markWait("")
	if err != nil {
		return "", false, fmt.Errorf("landingshed: %s: %w", producer, err)
	}
	switch result.Status {
	case verifytree.StatusPassed:
		logger.Info("landingshed: the pr-review gate passed", "producer", producer)
	case verifytree.StatusSkipped:
		logger.Info("landingshed: the pr-review gate skipped because the tree is already verified", "producer", producer, "tree", result.Tree)
	case verifytree.StatusDirty:
		return dirtyReason("when the pr-review gate was about to run", result.Dirty), false, nil
	case verifytree.StatusFailed:
		return g.headFailureReason(result), result.RunnerCrash, nil
	default:
		return "", false, fmt.Errorf("landingshed: %s: unknown pr-review gate status %q", producer, result.Status)
	}
	return "", false, nil
}

// headFailureReason is the reason a failed run of the pr-review gate gives the agent:
// the failing tier or the build, the log and the instruction to fix it in a new commit.
// A timeout, a runner crash and a shell that could not start name no tier, since none names a failing test.
// A runner crash is a halt reason opening with `LandingStopPrefix`:
// no change to the code mends it, and the agent has no commit to make.
func (g verifyGate) headFailureReason(result verifytree.Result) string {
	switch {
	case result.TimedOut:
		return fmt.Sprintf("the pr-review gate timed out: %s; output: %s; fix the hanging test or build in a new commit", result.Detail, result.Log)
	case result.RunnerCrash:
		return fmt.Sprintf("%sthe pr-review gate's test runner crashed in its run and its one rerun (%s), so no test judged the head and no change to the code mends it; output: %s; way forward: `lym loom resume`, which runs the gate again, and when the runner crashes again, find the runtime's own fault in the output", LandingStopPrefix, result.Detail, result.Log)
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

// watchdogClause returns the clause a failed verify's reason carries when its log holds the CPU watchdog's line.
// It returns empty for an unreadable log or one without the line.
// The clause names the window and says the goroutine stacks are in the log.
// The clause also gives the way forward for a package that is CPU-bound but healthy.
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

// verdict maps a verify result to the producer's decision, and `what` names the gate its messages report.
// It returns a non-empty Stuck reason for a dirty tree or a failed command.
// It returns an error for an unknown status, and ("", nil) when the producer may proceed.
// A failed command with an exit code ends its reason with the clause asking to fix forward on the task branch.
// After a CPU watchdog quit the reason names the window and the `busy_quit_sec` way forward before that clause.
// A runner crash is a halt reason opening with `LandingStopPrefix`.
// The reason names the crashed runs and the way forward of resuming, which runs the gate again.
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
		if result.RunnerCrash {
			return fmt.Sprintf("%s%s's test runner crashed after merging parent branch %q, in its run and its one rerun (%s), so no test judged the code and no fixer runs; output: %s; way forward: `lym loom resume`, which runs the gate again, and when the runner crashes again, find the runtime's own fault in the output", LandingStopPrefix, what, parentBranch, result.Detail, result.Log), nil
		}
		watchdog := watchdogClause(result.Log)
		return fmt.Sprintf("%s failed after merging parent branch %q (exit code %d); output: %s%s; fix forward on the task branch, then resume", what, parentBranch, result.ExitCode, result.Log, watchdog), nil
	default:
		return "", fmt.Errorf("landingshed: %s: unknown %s status %q", producer, what, result.Status)
	}
	return "", nil
}
