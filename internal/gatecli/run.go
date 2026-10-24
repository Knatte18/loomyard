// run.go runs one `lyx gate test`: it refuses a malformed call, resolves hub membership and the gate config, plans the tiers, takes each tier's gate slots in turn, runs the tier inside them and turns a terminating signal into a clean exit.

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
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/proc"
)

// siteLabelPrefix starts every holder and wait record this verb writes, so `lyx loom status` names the command that holds or waits for a slot.
const siteLabelPrefix = "lyx gate test"

// runTest runs request and returns the verb's exit code: the first failing run's own, SlotBusyExit for a hub whose slots stay held past request.slotWait when no earlier run failed, 128 plus the signal number after a terminating signal, and 1 for a refusal or a timed-out tier.
func runTest(ctx context.Context, out io.Writer, request testRequest) int {
	if refusal := flagShapeRefusal(request); refusal != "" {
		return output.Err(out, refusal)
	}
	cwd, err := lyxcwd.CwdFrom(ctx)
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
	if refusal := preTierExpiry(preCtx, runCtx, request, "loading the gate config"); refusal != "" {
		return output.Err(out, refusal)
	}
	if refusal := gateKeyRefusal(cfg, request); refusal != "" {
		return output.Err(out, refusal)
	}
	request.root = buildRoot(location, dir)
	request.busyQuit = cfg.Limits().BusyQuit

	green := openGreen(gateslot.WaitDir(location.AnchorPath()), request.root, request.goBinary, cfg)
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
	plan, err := planTierRuns(cfg, request, sc)
	if err != nil {
		return output.Err(out, err.Error())
	}
	if (request.guards || sc.guardStep) && len(sc.guards) == 0 {
		logger.Info("gate test: no guard test found outside the scope; the guard step runs nothing")
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
	}
	if inHub {
		exe.pool = hubgeom.GateSlots(location)
		exe.request.prebuiltStore = exe.pool.PrebuiltDir()
		exe.slug = originSlug(location)
		if inherited := os.Getenv(gateslot.InheritEnv); inherited != "" && exe.pool.Inherited(inherited) {
			logger.Info("gate test: running inside the slot an enclosing gate run holds", "slot", inherited, "dir", dir)
			exe.inherited = true
		}
	} else {
		logger.Info("gate test: no hub bound applies; running unslotted", "dir", dir)
	}

	exe.green = green

	// The tiers run without the strand name, so a test driving `lyx fabric` never meets a role guard that reads it.
	// They also run without this process's trace id, which is new at every run and would change the test cache key of every package that reads it.
	env := slices.DeleteFunc(os.Environ(), func(entry string) bool {
		return strings.HasPrefix(entry, agentname.StrandNameEnv+"=") || strings.HasPrefix(entry, logger.TraceIDEnv+"=")
	})
	env = gateslot.StripPrebuilt(env)
	watching, reason := watchdogApplies(request.busyQuit, request.args, os.Getenv(enclosingGateEnv), proc.IsAncestor)
	logger.Info("gate test: CPU watchdog decision", "applies", watching, "reason", reason)
	exe.request.watching = watching
	// A nested `lyx gate test` reads the variable to learn this run's watchdog already sees its test binaries.
	exe.env = append(slices.DeleteFunc(env, func(entry string) bool { return strings.HasPrefix(entry, enclosingGateEnv+"=") }), enclosingGateEnv+"="+strconv.Itoa(os.Getpid()))

	// A failed run does not stop the call, so a failure the change under test did not cause leaves every later tier and guard run checked.
	var failures []string
	code, firstFailed := 0, ""
	buildNamed := false
	for _, run := range plan {
		var end runEnd
		if run.guardStep {
			end = exe.runGuard(run)
		} else {
			end = exe.runListed(run)
		}
		if end.blocked {
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
			failures = append(failures, fmt.Sprintf("%s (exit %d)", runLabel(run), end.code))
			if code == 0 {
				code, firstFailed = end.code, runLabel(run)
			}
		}
		if end.halt {
			if end.ran && code == 0 {
				printLine(request.stderr, "gate test: the call was cancelled while its last run ended without a failure, so it exits 1; way forward: re-run the same command")
			}
			printFailedRuns(request.stderr, failures, firstFailed, true)
			return exitAfterSignal(caught, haltExitCode(end, code))
		}
	}
	printFailedRuns(request.stderr, failures, firstFailed, false)
	return exitAfterSignal(caught, code)
}

// haltExitCode returns the exit code of a call the run end halted, given the code of the first earlier or halting failed run, 0 when none failed.
// A run that ran and halted the call exits with that first failure's code, or 1 when none failed, since a halt is never green.
// A busy hub after a failed run exits with that failure's code, since SlotBusyExit reads as no failure; any other halt keeps its own code.
func haltExitCode(end runEnd, firstFailureCode int) int {
	switch {
	case end.ran && firstFailureCode == 0:
		return 1
	case end.ran || end.code == SlotBusyExit && firstFailureCode != 0:
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
	printLine(stderr, fmt.Sprintf("gate test: failed runs: %s; the exit code is %s's%s", strings.Join(failures, ", "), firstFailed, suffix))
}

// runEnd is how one run of the plan ended: its exit code, and whether the call stops there instead of going on to the next run.
// A run halts the call on a refusal, a slot wait that did not end with the slots held and a cancelled context; a run whose command failed does not.
type runEnd struct {
	code int
	halt bool
	// ran is whether the run's command ran to an outcome, as opposed to a refusal or a slot wait that ended the call before it.
	ran bool
	// blocked is set for a tier that ran no command since the build step it opts into failed; code is then the build's exit code.
	blocked bool
}

// runLabel names run in the closing line of failed runs: its tier, prefixed `guard` for a guard run and `vet` for a vet run.
func runLabel(run tierRun) string {
	switch {
	case run.guardStep:
		return "guard " + tierName(run.number)
	case run.vetStep:
		return "vet " + tierName(run.number)
	}
	return tierName(run.number)
}

// flagShapeRefusal returns the refusal for a malformed combination of flags, which needs no config and runs before hub resolution, or "" when the call is well-formed.
func flagShapeRefusal(request testRequest) string {
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
	return ""
}

// gateKeyRefusal returns the refusal for a --gate key the config cannot run, or "" when request names no key or a runnable one.
// An unknown key, an empty tier list and an undeclared tier carry the config's own way forward, and a key of scope card with no --card and no explicit targets is refused naming the key.
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
	return ""
}

// loadGate resolves the worktree that contains dir and loads its gate config:
// the hub's, with this machine's override, inside a hub, and the worktree's own outside every hub.
// A refusal is returned as its message, with the other results unset.
func loadGate(dir string) (location *lyxcwd.Location, inHub bool, cfg gateslot.Config, refusal string) {
	location, inHub, err := resolveHub(dir)
	if err != nil {
		return nil, false, gateslot.Config{}, fmt.Sprintf("gate test: %v; way forward: re-run the same command", err)
	}
	if !inHub {
		location, err = lyxcwd.ResolveWorktree(dir)
		if errors.Is(err, lyxcwd.ErrNotAGitRepo) {
			return nil, false, gateslot.Config{}, fmt.Sprintf("gate test: %s is in no git worktree, so no gate.yaml applies; way forward: run \"lyx config gate\" in a git worktree to write its own gate.yaml, then run the same command from there%s", dir, roleClause)
		}
		if err != nil {
			return nil, false, gateslot.Config{}, fmt.Sprintf("gate test: resolve the worktree of %s: %v; way forward: re-run the same command", dir, err)
		}
		cfg, err = gateslot.LoadConfig(configengine.Dirs{Anchor: location.AnchorPath()})
		if err != nil {
			return nil, false, gateslot.Config{}, fmt.Sprintf("gate test: cannot read the worktree's gate config: %v; way forward: take the step it names, then re-run the same command", err)
		}
		return location, false, cfg, ""
	}
	cfg, err = gateslot.LoadConfig(fabricengine.ConfigDirs(location))
	if errors.Is(err, gateslot.ErrConfigAbsent) {
		return nil, false, gateslot.Config{}, fmt.Sprintf("gate test: the hub has no gate config: %v; way forward: run \"lyx fabric reconcile\", then re-run the same command; a session lyx refuses the verb from reports status: FAILED and the orch runs it", err)
	}
	if err != nil {
		return nil, false, gateslot.Config{}, fmt.Sprintf("gate test: cannot read the hub's gate config: %v; way forward: take the step it names, then re-run the same command", err)
	}
	return location, true, cfg, ""
}

// preTierExpiry returns the refusal for the work before the first tier outliving request.preTierCeiling at step, or "" when the ceiling has not passed.
// A call a terminating signal ended is no expiry.
func preTierExpiry(preCtx, runCtx context.Context, request testRequest, step string) string {
	if runCtx.Err() != nil || !errors.Is(preCtx.Err(), context.DeadlineExceeded) {
		return ""
	}
	return fmt.Sprintf("gate test: the work before the first tier outlived its %s ceiling while %s; way forward: run the same command again, which a transient hang such as a stalled \"go list\" passes", request.preTierCeiling, step)
}

// tierNumbers returns the tier numbers of the listed runs of plan, leaving out the vet and guard steps.
func tierNumbers(plan []tierRun) []int {
	var numbers []int
	for _, run := range plan {
		if !run.guardStep && !run.vetStep {
			numbers = append(numbers, run.number)
		}
	}
	return numbers
}

// originSlug returns the task slug the pair's fabric origin record names for location, and "" for the prime or a pair with no record.
func originSlug(location *lyxcwd.Location) string {
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

// execution is the state of one call's tier runs after planning: where slots come from, the environment the tiers get and where their records go.
type execution struct {
	ctx      context.Context
	out      io.Writer
	request  testRequest
	cfg      gateslot.Config
	location *lyxcwd.Location
	// pool is the hub's slot pool; nil outside a hub, where the tiers run unslotted.
	pool *gateslot.Pool
	// inherited is whether an enclosing gate run's slot is held, so no tier acquires one.
	inherited bool
	env       []string
	runLogDir string
	slug      string
	tierList  []int
	// guards holds the guard tests per tier, tier to directory to guard names, the guard runs of the plan are built over.
	guards map[int]map[string][]string
	// green is the call's use of the green record; nil when the worktree is dirty or the record cannot apply, so every tier runs.
	green *greenRun
	// lastStamp is the time of the latest run record, kept so two records of one process never share a stamped name.
	lastStamp time.Time
	// buildDone is set once the call's build step has started, so the first opted-in tier whose run acquires slots runs it and no later tier does.
	buildDone bool
	// buildFailure is the log path of the build step once it failed or timed out; every opted-in tier after it runs no command.
	buildFailure string
	// buildExit is the exit code of the failed build.
	buildExit int
}

// runListed runs one listed tier with its slots held, and returns how it ended.
// A tier with no invocation takes no slot, and neither does a tier the green record holds green for the current tree: it is reported as carried.
// A tier the record holds green in part runs only the rest, and a pass is recorded.
// A tier with only a build to run takes its slots for the build, and a tier that opts into a build that failed is reported blocked.
func (e *execution) runListed(run tierRun) runEnd {
	if run.tier.SharedBuild && e.buildFailure != "" {
		return e.reportBlocked(run)
	}
	if run.ignoresTargets {
		printLine(e.request.stderr, fmt.Sprintf("gate test: the command of %s holds no {targets}, so it runs whole", tierName(run.number)))
	}
	if len(run.invocations) == 0 && !run.buildOnly {
		printLine(e.request.stderr, fmt.Sprintf("gate test: the scope is empty, so %s runs no command", tierName(run.number)))
		return e.runWithSlots(run, true, func(env []string) tierResult {
			return runTier(e.ctx, e.out, run, env, e.request)
		})
	}
	return e.runGreenAware(run)
}

// runGuard runs one guard group with its tier's slots held, unless the green record holds the group green, and returns how it ended.
func (e *execution) runGuard(planned tierRun) runEnd {
	moduleDir := resolveRunDir(e.request.root, planned.invocations[0].dir)
	run, err := guardStepRun(e.cfg, moduleDir, planned.number, e.guards[planned.number], e.request)
	if err != nil {
		return runEnd{code: output.Err(e.out, err.Error()), halt: true}
	}
	return e.runGreenAware(run)
}

// runGreenAware subtracts what the green record holds green from run, then reports run carried when nothing is left, else runs the rest inside the tier's slots and records a pass.
// A module carried over an inert-only diff gets its whole entry for the current tree before the run goes on.
// The first tier that opts into the build and whose run takes slots runs the call's build step inside them, before its own command.
func (e *execution) runGreenAware(run tierRun) runEnd {
	var plan greenPlan
	if e.green != nil && !run.buildOnly {
		plan = e.green.subtract(e.ctx, run, e.env)
		e.green.recordInertCarried(run, plan, time.Now())
		if len(plan.invocations) == 0 {
			return e.reportCarried(run)
		}
		run.invocations = plan.invocations
	}
	end := e.runWithSlots(run, false, func(env []string) tierResult {
		if run.tier.SharedBuild && !run.guardStep && !run.vetStep && !e.buildDone {
			if result, ok := e.runBuild(run, env); !ok {
				return result
			}
		}
		return runTier(e.ctx, e.out, run, env, e.request)
	})
	if end.code == 0 && !end.halt && e.green != nil {
		e.green.recordPass(e.ctx, run, run.invocations, plan, e.env, time.Now())
	}
	return end
}

// reportCarried writes the run record and summary line of a tier the green record held green, which took no slot and ran no command, and returns its pass.
func (e *execution) reportCarried(run tierRun) runEnd {
	now := time.Now()
	record := gateslot.RunRecord{Tier: run.number, ScopeForm: run.scopeForm, Slots: run.tier.Slots, GuardStep: run.guardStep, Start: now, End: now, Outcome: "passed", Carried: true}
	return e.report(run, tierResult{record: record}, now)
}

// runWithSlots takes the slots of run's tier unless the call is unslotted, inherited or the run empty, runs body inside them with the environment the slots give, releases the slots and reports the tier's outcome.
// A wait that does not end with the slots held halts the call.
func (e *execution) runWithSlots(run tierRun, empty bool, body func(env []string) tierResult) runEnd {
	waitStart := time.Now()
	env := e.env
	if e.pool != nil && !e.inherited && !empty {
		lease, code := acquireSlot(e.ctx, e.out, e.pool, e.location, e.request, tierName(run.number), run.tier.Slots)
		if lease == nil {
			return runEnd{code: code, halt: true}
		}
		defer releaseSlot(lease, e.request.root)
		env = lease.InheritEnv(env)
	}
	return e.report(run, body(env), waitStart)
}

// report writes the tier's run record and log, prints its `-trimpath` line, when it has one, and its summary line to stderr and returns the tier's exit code.
// A refusal is answered instead, so its stdout stays one JSON envelope, and halts the call, as does a run a terminating signal cancelled.
// A record that cannot be written is logged and decides nothing.
func (e *execution) report(run tierRun, result tierResult, waitStart time.Time) runEnd {
	if result.refusal != "" {
		return runEnd{code: output.Err(e.out, result.refusal), halt: true}
	}
	if result.buildFailed {
		return e.reportBlocked(run)
	}
	record := result.record
	record.WaitStart = waitStart
	logPath := e.writeRecord(record, result.log, tierName(run.number))
	if result.trimpathNote != "" {
		printLine(e.request.stderr, result.trimpathNote)
	}
	printLine(e.request.stderr, gateslot.TierSummaryLine(record, logPath))
	return runEnd{code: record.ExitCode, halt: e.ctx.Err() != nil, ran: true}
}

// writeRecord adds the call's worktree, slug, tier list and gate key to record, writes it with log under a stamp later than the previous record's, and returns the log's path, "(not written)" when the write failed.
// run names the record in a failure.
func (e *execution) writeRecord(record gateslot.RunRecord, log []byte, run string) string {
	record.Worktree = e.request.root
	record.Slug = e.slug
	record.TierList = e.tierList
	record.GateKey = e.request.gate

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
// A dir outside every git worktree, or in a worktree whose hub carries no board, is outside every hub.
func resolveHub(dir string) (*lyxcwd.Location, bool, error) {
	location, err := lyxcwd.ResolveWorktree(dir)
	if errors.Is(err, lyxcwd.ErrNotAGitRepo) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("resolve the worktree of %s: %w", dir, err)
	}
	if !preflight.BoardLyxPresent(location) {
		return nil, false, nil
	}
	return location, true, nil
}

// acquireSlot takes count slots of pool for tier, keeping a wait record in the worktree's wait directory until the acquire returns.
// Without request.wait the wait lasts at most request.slotWait; with it the call blocks until the slots free.
// It returns the held lease, or a nil lease and the exit code to return: SlotBusyExit with the holders named once the wait bound passes, and 1 for any other failure, a cancelled ctx included.
func acquireSlot(ctx context.Context, out io.Writer, pool *gateslot.Pool, location *lyxcwd.Location, request testRequest, tier string, count int) (*gateslot.Lease, int) {
	site := strings.TrimSpace(siteLabelPrefix + " " + tier + " " + strings.Join(request.targets, " "))
	waitPath, err := gateslot.WriteWait(gateslot.WaitDir(location.AnchorPath()), gateslot.Wait{Site: site, PID: os.Getpid(), Started: time.Now()})
	if err != nil {
		logger.Warn("gate test: could not write the gate wait record", "cause", err)
	} else {
		defer os.Remove(waitPath)
	}

	waitCtx := ctx
	if !request.wait {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, request.slotWait)
		defer cancel()
	}
	lease, err := pool.AcquireSlots(waitCtx, gateslot.Holder{Worktree: location.WorktreePath(), Site: site, Tier: tier}, count)
	switch {
	case err == nil:
		return lease, 0
	case ctx.Err() != nil:
		return nil, 1
	case errors.Is(err, context.DeadlineExceeded):
		return nil, answerSlotBusy(out, pool, request.slotWait)
	default:
		return nil, output.Err(out, fmt.Sprintf("gate test: cannot acquire a gate slot: %v; way forward: re-run the same command", err))
	}
}

// answerSlotBusy writes the ok answer that no slot freed within waited, naming each holder's worktree, site and tier and telling the caller to run the same command again, and returns SlotBusyExit.
func answerSlotBusy(out io.Writer, pool *gateslot.Pool, waited time.Duration) int {
	holders, err := pool.Holders()
	if err != nil {
		logger.Warn("gate test: could not read the gate holders", "cause", err)
	}
	named := make([]map[string]string, 0, len(holders))
	described := make([]string, 0, len(holders))
	for _, holder := range holders {
		entry := map[string]string{"worktree": holder.Worktree, "site": holder.Site}
		if holder.Tier != "" {
			entry["tier"] = holder.Tier
		}
		named = append(named, entry)
		described = append(described, fmt.Sprintf("%s (%s)", holder.Worktree, holder.Site))
	}
	message := fmt.Sprintf("gate test: no gate slot is free yet after %s; held by %s; run the same command again in a turn that does nothing else, so the prompt cache stays warm between calls", waited, strings.Join(described, ", "))
	output.Ok(out, map[string]any{"slot_free": false, "holders": named, "message": message})
	return SlotBusyExit
}

func releaseSlot(lease *gateslot.Lease, dir string) {
	if err := lease.Release(); err != nil {
		logger.Warn("gate test: could not release the gate slot", "dir", dir, "cause", err)
	}
}

// catchTerminatingSignals returns a context cancelled by the first terminating signal, the signal's number once one was caught (zero before), and the function that stops the catching.
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

// exitAfterSignal returns 128 plus the caught signal's number when a terminating signal ended the run, and code otherwise.
func exitAfterSignal(caught *atomic.Int32, code int) int {
	if number := caught.Load(); number != 0 {
		return 128 + int(number)
	}
	return code
}

// exitCodeOf returns the exit code of a finished child: its own, 1 for a child that did not exit normally, and 0 for none.
func exitCodeOf(runErr error) int {
	if runErr == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) && exitErr.ExitCode() >= 0 {
		return exitErr.ExitCode()
	}
	return 1
}
