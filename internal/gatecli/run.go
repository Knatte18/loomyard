// run.go runs one `lym gate test` call.
// It refuses a malformed call, resolves hub membership and the gate config, and plans the tiers.
// It takes each tier's gate slots in turn, runs the tier inside them, and turns a terminating signal into a clean exit.

package gatecli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/envconfig"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/proc"
)

// siteLabelPrefix starts every holder and wait record this verb writes.
// It lets `lym loom status` name the command that holds or waits for a slot.
const siteLabelPrefix = "lym gate test"

// runTest runs request in this process and returns the verb's exit code.
// The code is the first failing run's own, or 1 for a refusal or a timed-out tier.
// After a terminating signal it is 128 plus the signal number.
func runTest(ctx context.Context, out io.Writer, request testRequest) int {
	if refusal := flagShapeRefusal(request); refusal != "" {
		return output.Err(out, refusal)
	}
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate test: cannot read the working directory: %v; way forward: re-run the same command", err))
	}
	dir, err := absoluteDir(cwd, request.dir)
	if err != nil {
		return output.Err(out, err.Error())
	}
	if request.dir != "" {
		request.dir = dir
	}

	runCtx, caught, stop := catchTerminatingSignals(ctx)
	defer stop()
	preCtx, cancelPre := context.WithTimeout(runCtx, request.preTierCeiling)
	defer cancelPre()

	location, inHub, cfg, refusal := loadGate(dir)
	if refusal != "" {
		return output.Err(out, refusal)
	}
	hubEnv, err := envconfig.LoadConfig(configDirs(location, inHub))
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate test: cannot read the env config: %v; way forward: take the step it names, then re-run the same command", err))
	}
	if refusal := preTierExpiry(preCtx, runCtx, request, "loading the gate config"); refusal != "" {
		return output.Err(out, refusal)
	}
	if refusal := gateKeyRefusal(cfg, request); refusal != "" {
		return output.Err(out, refusal)
	}
	if request.gate != "" && len(cfg.Tiers) == 0 {
		printLine(request.stderr, fmt.Sprintf("gate test: the hub declares no tier, so --gate %q runs no tier, only its lints; once the repository has tests, declare tier0 with \"lym config gate\" from the prime", request.gate))
		request.root = buildRoot(location, dir)
		lints := runLints(request.root, cfg.Gates[request.gate].Lints)
		if len(lints) == 0 {
			return 0
		}
		labels := make([]string, len(lints))
		for i, lint := range lints {
			labels[i] = fmt.Sprintf("%s (%s)", lint.label, lint.outcome)
		}
		exe := &execution{ctx: runCtx, request: request, location: location, failed: lints}
		exe.printReport(inHub)
		printFailedRuns(request.stderr, labels, lints[0].label, false)
		return exitAfterSignal(caught, 1)
	}
	cfg = cfg.WithDefaultDir(location.AnchorRel)
	request.root = buildRoot(location, dir)
	request.busyQuit = cfg.Limits().BusyQuit
	request.memoryQuit = cfg.MemoryQuit()
	request.hubEnv = hubEnv
	request.sessionEnv = os.Environ()

	green := openGreen(gateslot.WaitDir(location.AnchorPath()), request.root, request.goBinary, cfg, request.gate)
	if request.checkoutRoot != "" {
		request.root = request.checkoutRoot
	}
	request, err = withOnlyTests(preCtx, cfg, request)
	if refusal := preTierExpiry(preCtx, runCtx, request, "scanning the //lym:only tests"); refusal != "" {
		return output.Err(out, refusal)
	}
	if err != nil {
		return output.Err(out, err.Error())
	}
	sc, err := computeScope(preCtx, cfg, location, request.root, request, green)
	if refusal := preTierExpiry(preCtx, runCtx, request, "computing the scope"); refusal != "" {
		return output.Err(out, refusal)
	}
	if err != nil {
		return output.Err(out, err.Error())
	}

	// The guard groups come from the scan's tiers, so the scan runs before the plan.
	if (request.guards || sc.guardStep) && cfg.Scoper == "go" && sc.guards == nil {
		if sc.guardDir, err = guardScanDir(cfg, request, sc); err != nil {
			return output.Err(out, err.Error())
		}
		if sc, err = scanGuards(sc, request.root); err != nil {
			return output.Err(out, err.Error())
		}
		if refusal := preTierExpiry(preCtx, runCtx, request, "scanning the guard tests"); refusal != "" {
			return output.Err(out, refusal)
		}
	}
	// A marked scope's guards are already the call's own tests per directory.
	// Dropping by bare name would lose one that shares its name with a held test elsewhere.
	if sc.form != scopeMarked {
		sc.guards = withoutHeld(sc.guards, request.held)
	}
	plan, err := planTierRuns(cfg, request, sc)
	if err != nil {
		return output.Err(out, err.Error())
	}
	if (request.guards || sc.guardStep) && len(sc.guards) == 0 {
		logger.Info("gate test: no guard test is left for the guard step; it runs nothing")
	}

	exe := &execution{
		ctx:       runCtx,
		out:       out,
		request:   request,
		cfg:       cfg,
		location:  location,
		runLogDir: runLogDirOf(location, inHub),
		tierList:  tierNumbers(plan),
		guards:    sc.guards,
		callID:    fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid()),
	}
	if inHub {
		exe.pool = hubgeom.GateSlots(location)
		exe.request.prebuiltStore = exe.pool.PrebuiltDir()
		exe.slug = originSlug(location)
		exe.priority = gateslot.SlotPriority(request.gate, holdsParentLock(location, exe.slug))
		if inherited := os.Getenv(gateslot.InheritEnv); inherited != "" && exe.pool.Inherited(inherited) {
			logger.Info("gate test: running inside the slot an enclosing gate run holds", "slot", inherited, "dir", dir)
			exe.inherited = true
		}
	} else {
		logger.Info("gate test: no hub bound applies; running unslotted", "dir", dir)
	}

	exe.green = greenForCall(green, request)

	env := gateEnv(os.Environ(), hubEnv)
	watching, reason := watchdogApplies(request.busyQuit, request.args, os.Getenv(enclosingGateEnv), proc.IsAncestor)
	logger.Info("gate test: CPU watchdog decision", "applies", watching, "reason", reason)
	exe.request.watching = watching
	guarding, guardReason := memoryGuardApplies(request.memoryQuit, os.Getenv(enclosingGateEnv), proc.IsAncestor)
	logger.Info("gate test: memory guard decision", "applies", guarding, "reason", guardReason)
	exe.request.memoryGuarding = guarding
	// A nested `lym gate test` reads the variable to learn this run's watchdog already sees its test binaries.
	exe.env = append(slices.DeleteFunc(env, func(entry string) bool { return strings.HasPrefix(entry, enclosingGateEnv+"=") }), runMarkerEntry(os.Getpid()))

	// Without --stop-on-fail a failed run does not stop the call, so every later tier and guard run stays checked.
	// That holds even when the change under test did not cause the failure.
	var failures []string
	code, firstFailed := 0, ""
	buildNamed := false
	var unjudged unjudgedRuns
	for i, run := range plan {
		var end runEnd
		if run.guardStep {
			end = exe.runGuard(run)
		} else {
			end = exe.runListed(run)
		}
		if end.blocked {
			unjudged.judged = true
			// The build is a failed run counted before the first tier it blocked.
			if !buildNamed {
				buildNamed = true
				failures = append(failures, fmt.Sprintf("build (exit %d)", end.code))
				if code == 0 {
					code, firstFailed = end.code, "build"
				}
			}
			failures = append(failures, runLabel(run)+" (blocked by the build)")
		} else if end.ran && end.code != 0 {
			failures = append(failures, failedRunLabel(run, end))
			unjudged.add(runLabel(run), end)
			if code == 0 {
				code, firstFailed = end.code, runLabel(run)
			}
		}
		if end.halt {
			if end.ran && code == 0 {
				printLine(request.stderr, "gate test: the call was cancelled while its last run ended without a failure, so it exits 1; way forward: re-run the same command")
			}
			exe.printReport(inHub)
			printFailedRuns(request.stderr, failures, firstFailed, true)
			return exitAfterSignal(caught, haltExitCode(end, code))
		}
		if request.stopOnFail && stopsCall(end) && i+1 < len(plan) {
			failed := runLabel(run)
			if end.blocked {
				failed = "build"
			}
			printLine(request.stderr, stopOnFailLine(failed, plan[i+1:]))
			break
		}
	}
	// The lint steps run after the tiers, so a call that fails both reports the two in one go.
	if request.gate != "" && runCtx.Err() == nil {
		for _, lint := range runLints(request.root, cfg.Gates[request.gate].Lints) {
			unjudged.judged = true
			exe.failed = append(exe.failed, lint)
			failures = append(failures, fmt.Sprintf("%s (%s)", lint.label, lint.outcome))
			if code == 0 {
				code, firstFailed = 1, lint.label
			}
		}
	}
	exe.printReport(inHub)
	printFailedRuns(request.stderr, failures, firstFailed, false)
	if line := unjudged.closingLine(); line != "" {
		printLine(request.stderr, line)
	}
	if code == 0 && sc.form == scopeCard {
		printNotRunTiers(request.stderr, notRunTiers(cfg, plan))
	}
	if cfg.Scoper == "dotnet" {
		for _, line := range unrunProjectLines(cfg, plan, sc) {
			printLine(request.stderr, line)
		}
	}
	return exitAfterSignal(caught, code)
}

// unjudgedRuns collects a call's failures by whether each judged the code.
type unjudgedRuns struct {
	// crashed names the failed runs that crashed their test runner twice.
	crashed []string
	// pressured names the failed runs the memory guard stopped twice for a memory limit other load filled.
	pressured []string
	// judged is set once any other failure happened.
	judged bool
}

// add records the run labelled label, which ran and failed as end.
func (u *unjudgedRuns) add(label string, end runEnd) {
	switch {
	case end.crash != "":
		u.crashed = append(u.crashed, fmt.Sprintf("%s (exit %d, %s)", label, end.code, end.crash))
	case end.pressureStop:
		u.pressured = append(u.pressured, fmt.Sprintf("%s (exit %d)", label, end.code))
	default:
		u.judged = true
	}
}

// closingLine returns the line saying no failure of the call judged the code, or "" when one did or nothing failed.
// It is the runner crash line when every failed run crashed, and the memory pressure line when one was a pressure stop.
func (u unjudgedRuns) closingLine() string {
	switch {
	case u.judged:
		return ""
	case len(u.pressured) > 0:
		return gateslot.MemoryPressureLine(strings.Join(u.pressured, ", "))
	case len(u.crashed) > 0:
		return gateslot.RunnerCrashLine(strings.Join(u.crashed, ", "))
	}
	return ""
}

// haltExitCode returns the exit code of a call that the run end halted.
// firstFailureCode is the code of the first failed run, the halting run included, or 0 when none failed.
// A run that ran and halted the call exits with firstFailureCode, or 1 when that is 0, since a halt is never green.
// Any other halt keeps its own code.
func haltExitCode(end runEnd, firstFailureCode int) int {
	switch {
	case end.ran && firstFailureCode == 0:
		return 1
	case end.ran:
		return firstFailureCode
	}
	return end.code
}

// printFailedRuns writes the closing line naming every failed run to stderr, and nothing when failures is empty.
// A line written because a run halted the call says the later runs did not run.
func printFailedRuns(stderr io.Writer, failures []string, firstFailed string, halted bool) {
	if len(failures) == 0 {
		return
	}
	suffix := ""
	if halted {
		suffix = "; the call stopped there, so later runs did not run"
	}
	printLine(stderr, fmt.Sprintf("%s%s; the exit code is %s's%s", gateslot.FailedRunsPrefix, strings.Join(failures, ", "), firstFailed, suffix))
}

// stopsCall reports whether a run that ended so stops a --stop-on-fail call.
// A failed or blocked run does.
// A run that crashed its test runner in its run and its rerun judged no code, so it does not.
// Nor does a run the memory guard stopped in its run and its rerun for a memory limit other load filled.
func stopsCall(end runEnd) bool {
	return end.blocked || (end.ran && end.code != 0 && end.crash == "" && !end.pressureStop)
}

// stopOnFailLine returns the stderr line a --stop-on-fail call prints when the failed run stops it.
// The line names the failed run and every run of skipped.
func stopOnFailLine(failed string, skipped []tierRun) string {
	labels := make([]string, len(skipped))
	for i, run := range skipped {
		labels[i] = runLabel(run)
	}
	return fmt.Sprintf("gate test: --%s ended the call after %s failed, so %s did not run", gateslot.StopOnFailFlag, failed, strings.Join(labels, ", "))
}

// notRunSuffix closes the line a passing card-scope call prints naming the declared tiers it did not run.
const notRunSuffix = " did not run, so this pass is not green for them"

// printNotRunTiers writes to stderr the line naming the tiers in numbers.
// The line keeps a card-scope pass from reading as green for tiers it never ran.
// It writes nothing when numbers is empty.
func printNotRunTiers(stderr io.Writer, numbers []int) {
	if len(numbers) == 0 {
		return
	}
	printLine(stderr, "gate test: "+tierNames(numbers)+notRunSuffix)
}

// failedRunLabel names a run that ran and failed, for the closing line of failed runs.
// The label carries the run's exit code.
// A run that hit a resource shortage twice carries the marker and the way forward of re-running once the load drops.
// A run that crashed its test runner twice carries the signal and says that no test failed.
// A run the memory guard stopped twice for a memory limit other load filled says it is an infrastructure stop.
func failedRunLabel(run tierRun, end runEnd) string {
	switch {
	case end.shortage != "":
		return fmt.Sprintf("%s (exit %d, a resource shortage, %q, in its run and its rerun: an environment error, not a test failure; way forward: re-run the same command once the machine's process load drops)", runLabel(run), end.code, end.shortage)
	case end.crash != "":
		return fmt.Sprintf("%s (exit %d, a runner crash, %s, in its run and its rerun with no failing test: the code under test was not judged)", runLabel(run), end.code, end.crash)
	case end.pressureStop:
		return fmt.Sprintf("%s (exit %d, a memory guard kill in its run and its rerun for a memory limit other load filled: an infrastructure stop, not a test failure; way forward: re-run the same command once that load ends)", runLabel(run), end.code)
	}
	return fmt.Sprintf("%s (exit %d)", runLabel(run), end.code)
}

// runEnd is how one run of the plan ended: its exit code, and whether the call stops there.
// A run halts the call on a refusal, a slot wait that did not end with the slots held and a cancelled context;
// a run whose command failed does not.
type runEnd struct {
	code int
	halt bool
	// ran is whether the run's command reached an outcome, not a refusal or a slot wait that ended the call.
	ran bool
	// blocked is set when a build step the tier opts into failed, so no command ran; code is then the build's exit code.
	blocked bool
	// shortage is the resource-shortage marker when the run and its rerun both failed on one; empty otherwise.
	shortage string
	// crash is the signal name of a test runner that crashed in the run and its rerun with no failing test; empty otherwise.
	crash string
	// pressureStop is set when the memory guard stopped the run and its rerun for a memory limit other load filled.
	pressureStop bool
	// log is the log path of the run's record, empty for a run that wrote none.
	log string
}

// runLabel names a run in the closing line of failed runs: its tier.
// A guard run's label starts with `guard`, and a vet run's with `vet`.
func runLabel(run tierRun) string {
	switch {
	case run.guardStep:
		return "guard " + tierName(run.number)
	case run.vetStep:
		return "vet " + tierName(run.number)
	}
	return tierName(run.number)
}

// flagShapeRefusal returns the refusal for a malformed combination of flags, or "" when the call is well-formed.
// It needs no config and runs before hub resolution.
func flagShapeRefusal(request testRequest) string {
	if request.copies < 0 {
		return fmt.Sprintf("gate test: --copies %d names no count; way forward: pass 1 or more copies, or drop --copies, then re-run the same command", request.copies)
	}
	if request.copies > 0 && request.gate == "" {
		return "gate test: --copies runs the copies of one gate key's tier; way forward: name the gate key with --gate, then re-run the same command"
	}
	if request.copies > 0 && len(request.targets) == 0 {
		return "gate test: --copies runs the same explicit targets in every copy; way forward: name the packages to run as targets, then re-run the same command"
	}
	if request.tags != "" {
		return fmt.Sprintf("gate test: --tags %q is refused, since a tier's own command sets its build tags; way forward: name the tier that compiles them with --tier, for example --tier 1, and re-run the same command", request.tags)
	}
	if request.gate != "" {
		for _, other := range []struct {
			flag  string
			given bool
		}{{"--tier", len(request.tiers) > 0}, {"--upto", request.uptoGiven}, {"--all", request.all}, {"--base", request.base != ""}} {
			if other.given {
				return fmt.Sprintf("gate test: --gate %q takes its tiers and scope from gate.yaml, so it goes with none of --tier, --upto, --all and --base; way forward: drop %s, then re-run the same command", request.gate, other.flag)
			}
		}
	}
	if request.uptoGiven && request.upto < 0 {
		return fmt.Sprintf("gate test: --upto %d names no tier; way forward: pass the highest tier number to run, 0 or more, then re-run the same command", request.upto)
	}
	forms := 0
	for _, given := range []bool{len(request.targets) > 0, request.all, request.base != "", len(request.cards) > 0} {
		if given {
			forms++
		}
	}
	if request.dir != "" && len(request.targets) == 0 {
		return "gate test: -C goes with explicit targets only; way forward: name the packages to run after -C, or drop -C and let the tier's own dir apply, then re-run the same command"
	}
	if forms > 1 {
		return "gate test: explicit targets, --all, --base and --card are each one scope form, and a call takes one; way forward: keep one scope form, then re-run the same command"
	}
	if request.mainPackages && len(request.cards) == 0 {
		return fmt.Sprintf("gate test: --%s adds to a --card scope, and the call names no card; way forward: pass --card <id> once per card, or drop --%s, then re-run the same command", gateslot.MainPackagesFlag, gateslot.MainPackagesFlag)
	}
	if request.runAgent && runsTmuxTier(request) && !namesPackage(request) {
		return fmt.Sprintf("gate test: a run agent's call of tier%d, the tmux tier, names no package, so it would start every tmux test of its scope; way forward: run one test with lym gate test --tier %d <the package that holds the test> -- <a filter selecting one test>", gateslot.TmuxTier, gateslot.TmuxTier)
	}
	return ""
}

// runsTmuxTier reports whether the call's flags list the tmux tier, through --tier or an --upto range that reaches it.
func runsTmuxTier(request testRequest) bool {
	return slices.Contains(request.tiers, gateslot.TmuxTier) || (request.uptoGiven && request.upto >= gateslot.TmuxTier)
}

// namesPackage reports whether the call's explicit targets include one that is no wildcard.
// A wildcard such as ./... stands for many packages, so it names none.
func namesPackage(request testRequest) bool {
	return slices.ContainsFunc(request.targets, func(target string) bool { return !strings.Contains(target, "...") })
}

// gateKeyRefusal returns the refusal for a --gate key the config cannot run, or "" for a runnable key or none.
// An unknown key, an empty tier list and an undeclared tier carry the config's own way forward.
// A key of scope card with no --card and no explicit targets is refused, and the refusal names the key.
// So is --copies beside a key of scope marked, whose tests run only in the guard step, which runs no copies.
func gateKeyRefusal(cfg gateslot.Config, request testRequest) string {
	if request.gate == "" {
		return ""
	}
	key, err := cfg.Gate(request.gate)
	if err != nil {
		return fmt.Sprintf("gate test: %v", err)
	}
	if key.Scope == scopeCard && len(request.cards) == 0 && len(request.targets) == 0 {
		return fmt.Sprintf("gate test: --gate %q has scope card, which runs the packages of the cards named; way forward: pass --card <id> once per card, then re-run the same command", request.gate)
	}
	if request.copies > 0 && key.Scope == gateslot.ScopeMarked {
		return fmt.Sprintf("gate test: --copies runs copies of a listed tier's run, and --gate %q has scope marked, whose tests run only in the guard step, which runs no copies; way forward: drop --copies, or name a key of scope diff or all, then re-run the same command", request.gate)
	}
	if request.copies > 0 && (len(key.Tiers) > 1 || slices.Contains(key.Tiers, gateslot.BilledTier)) {
		return fmt.Sprintf("gate test: --copies runs one tier below tier%d, and --gate %q names tiers %v; way forward: set the key to one tier below tier3 with lym config gate --set from the prime%s", gateslot.BilledTier, request.gate, key.Tiers, roleClause)
	}
	return ""
}

// configDirs returns the directories a hub-wide module loads its config from for location.
// Inside a hub these are the hub's, with this machine's override; outside every hub they are the worktree's own.
func configDirs(location *lymcwd.Location, inHub bool) configengine.Dirs {
	if inHub {
		return fabricengine.ConfigDirs(location)
	}
	return configengine.Dirs{Anchor: location.AnchorPath()}
}

// loadGate resolves the worktree that contains dir and loads its gate config.
// Inside a hub the config is the hub's, with this machine's override; outside every hub it is the worktree's own.
// A refusal is returned as its message, with the other results unset.
func loadGate(dir string) (location *lymcwd.Location, inHub bool, cfg gateslot.Config, refusal string) {
	location, inHub, err := resolveHub(dir)
	if err != nil {
		return nil, false, gateslot.Config{}, fmt.Sprintf("gate test: %v; way forward: re-run the same command", err)
	}
	if !inHub {
		location, err = lymcwd.ResolveWorktree(dir)
		if errors.Is(err, lymcwd.ErrNotAGitRepo) {
			return nil, false, gateslot.Config{}, fmt.Sprintf("gate test: %s is in no git worktree, so no gate.yaml applies; way forward: run \"lym config gate\" in a git worktree to write its own gate.yaml, then run the same command from there%s", dir, roleClause)
		}
		if err != nil {
			return nil, false, gateslot.Config{}, fmt.Sprintf("gate test: resolve the worktree of %s: %v; way forward: re-run the same command", dir, err)
		}
		cfg, err = gateslot.LoadConfig(configDirs(location, false))
		if err != nil {
			return nil, false, gateslot.Config{}, fmt.Sprintf("gate test: cannot read the worktree's gate config: %v; way forward: take the step it names, then re-run the same command", err)
		}
		return location, false, cfg, ""
	}
	cfg, err = gateslot.LoadConfig(configDirs(location, true))
	if errors.Is(err, gateslot.ErrConfigAbsent) {
		return nil, false, gateslot.Config{}, fmt.Sprintf("gate test: the hub has no gate config: %v; way forward: run \"lym fabric reconcile\", then re-run the same command; a session lym refuses the verb from reports status: FAILED and the orch runs it", err)
	}
	if err != nil {
		return nil, false, gateslot.Config{}, fmt.Sprintf("gate test: cannot read the hub's gate config: %v; way forward: take the step it names, then re-run the same command", err)
	}
	return location, true, cfg, ""
}

// preTierExpiry returns the refusal for the work before the first tier outliving request.preTierCeiling at step.
// It returns "" when the ceiling has not passed.
// A call a terminating signal ended is no expiry.
func preTierExpiry(preCtx, runCtx context.Context, request testRequest, step string) string {
	if runCtx.Err() != nil || !errors.Is(preCtx.Err(), context.DeadlineExceeded) {
		return ""
	}
	return fmt.Sprintf("gate test: the work before the first tier outlived its %s ceiling while %s; way forward: run the same command again, which a transient hang such as a stalled \"go list\" passes", request.preTierCeiling, step)
}

// tierNumbers returns the tier numbers of the listed runs of plan, each once, leaving out the vet and guard steps.
func tierNumbers(plan []tierRun) []int {
	var numbers []int
	for _, run := range plan {
		if !run.guardStep && !run.vetStep && !slices.Contains(numbers, run.number) {
			numbers = append(numbers, run.number)
		}
	}
	return numbers
}

// originSlug returns the task slug that the pair's fabric origin record names for location.
// It returns "" for the prime and for a pair with no record.
func originSlug(location *lymcwd.Location) string {
	_, found, err := fabricengine.ReadOrigin(location)
	if err != nil {
		logger.Warn("gate test: could not read the fabric origin record; the run record carries no slug", "cause", err)
		return ""
	}
	if !found {
		return ""
	}
	return location.WorktreeName
}

// holdsParentLock reports whether the run of slug holds a live parent lock of location's hub.
// A slug of "" holds none, and a lock store that cannot be read is logged and reads as held by none.
func holdsParentLock(location *lymcwd.Location, slug string) bool {
	if slug == "" {
		return false
	}
	store := &parentlock.Store{Dir: parentlock.Dir(fabriclayout.BoltDir(location.HubPath))}
	held, err := store.HeldBy(slug)
	if err != nil {
		logger.Warn("gate test: could not read the parent locks; the call takes its slots as a run holding none", "cause", err)
		return false
	}
	return held
}

// execution holds the state of one call's tier runs after planning.
// It records where slots come from, the environment the tiers get, and where their records go.
type execution struct {
	ctx      context.Context
	out      io.Writer
	request  testRequest
	cfg      gateslot.Config
	location *lymcwd.Location
	// pool is the hub's slot pool; nil outside a hub, where the tiers run unslotted.
	pool *gateslot.Pool
	// inherited is whether an enclosing gate run's slot is held, so no tier acquires one.
	inherited bool
	env       []string
	runLogDir string
	slug      string
	// priority is the rank every tier of the call waits for its slots with.
	priority gateslot.Priority
	tierList []int
	// guards maps tier to directory to guard names, which the plan's guard runs are built over.
	guards map[int]map[string][]string
	// green is the green record the call uses; nil when the worktree is dirty or it cannot apply, so every tier runs.
	green *greenRun
	// lastStamp is the time of the latest run record, kept so two records of one process never share a stamped name.
	lastStamp time.Time
	// buildDone is set once the build step starts, so only the first opted-in tier that acquires slots runs it.
	buildDone bool
	// buildFailure is the build step's log path once it failed or timed out; later opted-in tiers run no command.
	buildFailure string
	// buildExit is the exit code of the failed build.
	buildExit int
	// callID identifies this call; every record the call writes carries it.
	callID string
	// failed holds the call's failed runs in plan order, its lint steps last, for the failure report.
	failed []failedRun
	// slotClock reads the times that bound a tier's wait for its slots; nil reads time.Now.
	slotClock func() time.Time
}

// slotWait is when a tier's wait for its gate slots started, and when it ended with the tier free to run.
type slotWait struct {
	start, held time.Time
}

func (e *execution) slotNow() time.Time {
	if e.slotClock != nil {
		return e.slotClock()
	}
	return time.Now()
}

// printReport writes the failure report of the call's failed runs to stderr, and nothing when no run failed.
// The report names the files the change touches.
func (e *execution) printReport(inHub bool) {
	if len(e.failed) == 0 {
		return
	}
	printFailureReport(e.request.stderr, e.failed, touchedFiles(e.ctx, e.location, e.request.root, inHub))
}

// runListed runs one listed tier with its slots held, and returns how it ended.
// A tier with no invocation takes no slot.
// A tier the green record holds green for the current tree takes no slot either.
// It is reported as carried, naming the earlier run it reuses.
// A tier the record holds green in part runs only the rest, and a pass is recorded.
// A tier with only a build to run takes its slots for the build,
// and a tier that opts into a build that failed is reported blocked.
func (e *execution) runListed(run tierRun) runEnd {
	if run.tier.SharedBuild && e.buildFailure != "" {
		return e.reportBlocked(run)
	}
	if len(run.invocations) == 0 && !run.buildOnly {
		printLine(e.request.stderr, fmt.Sprintf("gate test: the scope is empty, so %s runs no command", tierName(run.number)))
		return e.runWithSlots(run, true, func(env []string, _ slotWait) tierResult {
			return runTier(e.ctx, e.out, run, env, e.request)
		})
	}
	return e.runGreenAware(run)
}

// runGuard runs one guard group with its tier's slots held, unless the green record holds the group green.
// It returns how the run ended.
func (e *execution) runGuard(planned tierRun) runEnd {
	moduleDir := resolveRunDir(e.request.root, planned.invocations[0].dir)
	run, err := guardStepRun(e.cfg, moduleDir, planned.number, e.guards[planned.number], e.request)
	if err != nil {
		return runEnd{code: output.Err(e.out, err.Error()), halt: true}
	}
	return e.runGreenAware(run)
}

// runGreenAware subtracts what the green record holds green from run.
// It reports run as carried when nothing is left.
// Otherwise it runs the rest inside the tier's slots and records a pass.
// A module carried over an inert-only diff gets its whole entry for the current tree before the run goes on.
// The first slotted tier that opts into the build runs the call's build step inside its slots, before its own command.
func (e *execution) runGreenAware(run tierRun) runEnd {
	var plan greenPlan
	if e.green != nil && !run.buildOnly {
		plan = e.green.subtract(e.ctx, run, e.env)
		e.green.recordInertCarried(run, plan, time.Now())
		if len(plan.invocations) == 0 {
			return e.reportCarried(run, plan.reusedFrom)
		}
		run.invocations = plan.invocations
	}
	end := e.runWithSlots(run, false, func(env []string, wait slotWait) tierResult {
		if run.tier.SharedBuild && !run.guardStep && !run.vetStep && !e.buildDone {
			if result, ok := e.runBuild(run, env); !ok {
				return result
			}
		}
		before, listed := untrackedBeforeRun(run, e.request.root)
		return retryTier(e.ctx, e.request.stderr, runLabel(run), e.request.rerunDelay, func() tierResult {
			return runTierChecked(e.ctx, e.out, run, env, e.request, before, listed)
		}, func(first tierResult) {
			first.record.WaitStart, first.record.SlotsHeld = wait.start, wait.held
			e.writeRecord(first.record, first.log, tierName(run.number))
		})
	})
	if end.code == 0 && !end.halt && e.green != nil {
		e.green.recordPass(e.ctx, run, run.invocations, plan, e.env, end.log, time.Now())
	}
	return end
}

// reportCarried writes the run record and summary line of a tier the green record held green.
// reusedFrom are the log paths of the earlier passes it reuses; the record carries them,
// and a line naming them precedes the summary line.
// The tier took no slot and ran no command, and reportCarried returns its pass.
func (e *execution) reportCarried(run tierRun, reusedFrom []string) runEnd {
	now := time.Now()
	record := gateslot.RunRecord{Tier: run.number, ScopeForm: run.scopeForm, Slots: run.tier.Slots, GuardStep: run.guardStep, Start: now, End: now, Outcome: "passed", Carried: true, ReusedFrom: reusedFrom}
	if len(reusedFrom) > 0 {
		printLine(e.request.stderr, reusedLine(runLabel(run), reusedFrom))
	}
	return e.report(run, tierResult{record: record}, slotWait{start: now, held: now})
}

// runWithSlots takes the slots of run's tier unless the call is unslotted, inherited or the run empty.
// It runs body inside them with the environment the slots give and the times the wait for them started and ended.
// It releases the slots and reports the tier's outcome.
// A wait that does not end with the slots held halts the call.
// A --copies call takes one lease of the copies' slots together and runs the copies the pool holds inside it.
// A vet-step or guard-step run of a copies call runs once, in the slots of its own tier.
// It refuses to run inside a slot an enclosing gate run holds, since it needs a lease of its own.
func (e *execution) runWithSlots(run tierRun, empty bool, body func(env []string, wait slotWait) tierResult) runEnd {
	wait := slotWait{start: e.slotNow()}
	env := e.env
	copying := e.request.copies > 0 && !empty && !run.vetStep && !run.guardStep
	if copying && e.inherited {
		return runEnd{code: output.Err(e.out, "gate test: --copies needs a lease of its own, and this call runs inside a slot an enclosing gate run holds; way forward: run the call outside a held gate slot, then re-run the same command"), halt: true}
	}
	copiesRun := 0
	if copying {
		copiesRun = e.request.copies
	}
	if e.pool != nil && !e.inherited && !empty {
		count := run.tier.Slots
		if copying {
			limits, err := e.pool.Limits()
			if err != nil {
				return runEnd{code: output.Err(e.out, fmt.Sprintf("gate test: cannot read the gate slot limits: %v; way forward: re-run the same command", err)), halt: true}
			}
			count, copiesRun = copyLease(e.request.copies, run.tier.Slots, limits.Slots)
		}
		lease, code := acquireSlot(e.ctx, e.out, e.pool, e.location, e.request, tierName(run.number), count, e.priority)
		if lease == nil {
			return runEnd{code: code, halt: true}
		}
		defer releaseSlot(lease, e.request.root)
		env = lease.InheritEnv(env)
	}
	wait.held = e.slotNow()
	if copying && copiesRun < e.request.copies {
		printLine(e.request.stderr, copiesClampLine(e.request.copies, copiesRun, tierName(run.number)))
	}
	e.request.copiesRun = copiesRun
	defer func() { e.request.copiesRun = 0 }()
	if e.request.phase != nil {
		e.request.phase(runPhase{State: phaseRunning, Tier: tierName(run.number)})
	}
	if e.request.observer.Started != nil {
		e.request.observer.Started(run.number)
	}
	return e.report(run, body(env, wait), wait)
}

// report writes the tier's run record and log, and prints its summary line to stderr.
// It also prints the tier's `-trimpath` line when it has one, and a started line for the tier run, or for each copy.
// It returns the tier's exit code.
// A refusal is answered instead, so its stdout stays one JSON envelope, and it halts the call.
// A run that a terminating signal cancelled halts the call as well.
// A record that cannot be written is logged and decides nothing.
func (e *execution) report(run tierRun, result tierResult, wait slotWait) runEnd {
	if result.refusal != "" {
		return runEnd{code: output.Err(e.out, result.refusal), halt: true}
	}
	if result.buildFailed {
		return e.reportBlocked(run)
	}
	record := result.record
	record.WaitStart, record.SlotsHeld = wait.start, wait.held
	// The summary line shows the wait the call spent on the tier's slots, a rerun's line included.
	shown := record
	if record.Rerun {
		// The slot wait is on the first run's record, so the rerun's own record waited for nothing.
		record.WaitStart, record.SlotsHeld = record.Start, time.Time{}
	}
	logPath := e.writeRecord(record, result.log, tierName(run.number))
	if result.trimpathNote != "" {
		printLine(e.request.stderr, result.trimpathNote)
	}
	printLine(e.request.stderr, gateslot.TierSummaryLine(shown, logPath))
	e.printStartedLines(run, result)
	if record.ExitCode != 0 {
		e.addFailedRuns(run, result, logPath)
	}
	if e.request.observer.Ended != nil {
		e.request.observer.Ended(record)
	}
	return runEnd{code: record.ExitCode, halt: e.ctx.Err() != nil, ran: true, shortage: result.shortage, crash: result.crash, pressureStop: result.pressureStop, log: logPath}
}

// writeRecord adds the call's worktree, slug, tier list, gate key and call id to record.
// It marks the record abandoned when the call's context is cancelled.
// It writes the record with log under a stamp later than the previous record's.
// It returns the log's path, or "(not written)" when the write failed.
// run names the record in a failure.
func (e *execution) writeRecord(record gateslot.RunRecord, log []byte, run string) string {
	record.Worktree = e.request.root
	record.Slug = e.slug
	record.TierList = e.tierList
	record.GateKey = e.request.gate
	record.CallID = e.callID
	record.Abandoned = e.ctx.Err() != nil

	stamp := time.Now()
	if !stamp.After(e.lastStamp.Add(time.Second)) {
		stamp = e.lastStamp.Add(time.Second)
	}
	e.lastStamp = stamp
	path, err := gateslot.WriteRunRecord(e.runLogDir, record, log, stamp)
	if err != nil {
		logger.Warn("gate test: could not write the run record", "run", run, "cause", err)
		return "(not written)"
	}
	return strings.TrimSuffix(path, ".json") + ".log"
}

// resolveHub returns the location of the hub worktree that contains dir, and whether dir is inside a hub at all.
// A dir in a records worktree resolves to the code worktree it pairs with, since every gate verb acts on code.
// A dir outside every git worktree, or in a worktree whose hub carries no board, is outside every hub.
func resolveHub(dir string) (*lymcwd.Location, bool, error) {
	location, err := lymcwd.ResolveWorktree(dir)
	if errors.Is(err, lymcwd.ErrNotAGitRepo) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("resolve the worktree of %s: %w", dir, err)
	}
	if location, err = fabricengine.CodeLocation(location); err != nil {
		return nil, false, fmt.Errorf("resolve the code worktree of %s: %w", dir, err)
	}
	if !preflight.BoardLymPresent(location) {
		return nil, false, nil
	}
	return location, true, nil
}

// runMarkerEntry returns the environment entry that every process the tier run of pid starts carries,
// so a process can be traced to the run after it has left the run's process tree.
// A nested `lym gate test` replaces the entry for the processes it starts.
func runMarkerEntry(pid int) string {
	return enclosingGateEnv + "=" + strconv.Itoa(pid)
}

// acquireSlot takes count slots of pool for tier, waiting with priority.
// It keeps a wait record in the worktree's wait directory until the acquire returns.
// The wait has no cap of its own: it ends when the slots free or ctx is cancelled.
// It returns the held lease, or a nil lease and the exit code to return, 1 for any failure, a cancelled ctx included.
func acquireSlot(ctx context.Context, out io.Writer, pool *gateslot.Pool, location *lymcwd.Location, request testRequest, tier string, count int, priority gateslot.Priority) (*gateslot.Lease, int) {
	site := strings.TrimSpace(siteLabelPrefix + " " + tier + " " + strings.Join(request.targets, " "))
	if request.phase != nil {
		request.phase(runPhase{State: phaseSlots, Tier: tier})
	}
	waitPath, err := gateslot.WriteWait(gateslot.WaitDir(location.AnchorPath()), gateslot.Wait{Site: site, PID: os.Getpid(), Started: time.Now()})
	if err != nil {
		logger.Warn("gate test: could not write the gate wait record", "cause", err)
	} else {
		defer os.Remove(waitPath)
	}

	holder := gateslot.Holder{Worktree: location.WorktreePath(), Site: site, Tier: tier, Marker: runMarkerEntry(os.Getpid()), Priority: priority}
	lease, err := pool.AcquireSlots(ctx, holder, count)
	switch {
	case err == nil:
		return lease, 0
	case ctx.Err() != nil:
		return nil, 1
	default:
		return nil, output.Err(out, fmt.Sprintf("gate test: cannot acquire a gate slot: %v; way forward: re-run the same command", err))
	}
}

func releaseSlot(lease *gateslot.Lease, dir string) {
	if err := lease.Release(); err != nil {
		logger.Warn("gate test: could not release the gate slot", "dir", dir, "cause", err)
	}
}

// catchTerminatingSignals returns a context that the first terminating signal cancels.
// It also returns the signal's number once one was caught (zero before), and the function that stops the catching.
func catchTerminatingSignals(ctx context.Context) (context.Context, *atomic.Int32, func()) {
	runCtx, cancel := context.WithCancel(ctx)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, terminatingSignalSet...)
	var caught atomic.Int32
	go func() {
		select {
		case received := <-signals:
			if number, ok := received.(syscall.Signal); ok {
				caught.Store(int32(number))
			}
			cancel()
		case <-runCtx.Done():
		}
	}()
	return runCtx, &caught, func() {
		signal.Stop(signals)
		cancel()
	}
}

// exitAfterSignal returns 128 plus the caught signal's number when a terminating signal ended the run.
// Otherwise it returns code.
func exitAfterSignal(caught *atomic.Int32, code int) int {
	if number := caught.Load(); number != 0 {
		return 128 + int(number)
	}
	return code
}

// exitCodeOf returns the exit code of a finished child.
// It is the child's own code, 1 for a child that did not exit normally, and 0 when runErr is nil.
func exitCodeOf(runErr error) int {
	if runErr == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if !errors.As(runErr, &exitErr) {
		return 1
	}
	if code := exitErr.ExitCode(); code >= 0 {
		return code
	}
	return 1
}

// tierExitCodeOf returns the exit code of a finished tier command.
// A child that a crash signal ended reads as 128 plus the signal's number, as a shell reports it.
// So a tier whose test runner died of a signal reads as a runner crash.
// Any other run reads as its own exit code, 1 when it did not exit normally, and 0 when runErr is nil.
func tierExitCodeOf(runErr error) int {
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) && exitErr.ExitCode() < 0 {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			if _, crash := crashSignals[status.Signal()]; crash {
				return 128 + int(status.Signal())
			}
		}
	}
	return exitCodeOf(runErr)
}

// gateEnv returns the environment a gate call's children start from.
// It is base without the strand name, this process's trace id and an inherited prebuilt `lym`.
// The hub env module's variables follow, then the build-server switches.
// Without the strand name a test driving `lym fabric` never meets a role guard that reads it.
// The trace id is new at every run, and it would change the test cache key of every package that reads it.
// The module's variables replace inherited ones of the same name.
// A tier's or a build's own env, appended later, replaces them.
// The build-server switches come last, so a variable of the module never undoes them.
func gateEnv(base []string, hubEnv envconfig.Config) []string {
	env := slices.DeleteFunc(slices.Clone(base), func(entry string) bool {
		return strings.HasPrefix(entry, agentname.StrandNameEnv+"=") || strings.HasPrefix(entry, logger.TraceIDEnv+"=")
	})
	env = gateslot.StripPrebuilt(env)
	env = append(env, hubEnv.Entries()...)
	return withBuildServerOff(env)
}

// withBuildServerOff returns env with every proc.BuildServerOffEnv variable set to its value.
// It replaces any inherited value of the same name.
// This keeps any .NET build a run starts from leaving MSBuild servers behind.
func withBuildServerOff(env []string) []string {
	env = slices.DeleteFunc(env, func(entry string) bool {
		return slices.ContainsFunc(proc.BuildServerOffEnv, func(v proc.EnvVar) bool { return strings.HasPrefix(entry, v.Name+"=") })
	})
	for _, v := range proc.BuildServerOffEnv {
		env = append(env, v.Name+"="+v.Value)
	}
	return env
}
