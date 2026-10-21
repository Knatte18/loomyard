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
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/proc"
)

// siteLabelPrefix starts every holder and wait record this verb writes, so `lyx loom status` names the command that holds or waits for a slot.
const siteLabelPrefix = "lyx gate test"

// runTest runs request and returns the verb's exit code: the failing tier's own, SlotBusyExit for a hub whose slots stay held past request.slotWait, 128 plus the signal number after a terminating signal, and 1 for a refusal or a timed-out tier.
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
	request.root = buildRoot(location, dir)
	request.busyQuit = cfg.Limits().BusyQuit

	plan, err := planTierRuns(cfg, request)
	if err != nil {
		return output.Err(out, err.Error())
	}

	var guards map[string][]string
	if guard := guardRunOf(plan); guard != nil {
		moduleDir := resolveRunDir(request.root, guard.invocations[0].dir)
		if guards, err = impactset.ModuleGuardTests(moduleDir); err != nil {
			return output.Err(out, guardScanRefusal(moduleDir, err))
		}
		if refusal := preTierExpiry(preCtx, runCtx, request, "scanning the guard tests"); refusal != "" {
			return output.Err(out, refusal)
		}
	}

	exe := &execution{
		ctx:       runCtx,
		out:       out,
		request:   request,
		cfg:       cfg,
		location:  location,
		runLogDir: gateslot.RunLogDir(gateslot.WaitDir(location.WorktreePath())),
		tierList:  tierNumbers(plan),
		guards:    guards,
	}
	if inHub {
		exe.pool = hubgeom.GateSlots(location)
		exe.request.prebuiltStore = exe.pool.PrebuiltDir()
		exe.runLogDir = gateslot.RunLogDir(gateslot.Dir(fabricengine.BoardDir(location.HubPath)))
		exe.slug = originSlug(location)
		if inherited := os.Getenv(gateslot.InheritEnv); inherited != "" && exe.pool.Inherited(inherited) {
			logger.Info("gate test: running inside the slot an enclosing gate run holds", "slot", inherited, "dir", dir)
			exe.inherited = true
		}
	} else {
		logger.Info("gate test: no hub bound applies; running unslotted", "dir", dir)
	}

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

	code := 0
	for _, run := range plan {
		if run.guardStep {
			code = exe.runGuard(run)
			break
		}
		if code = exe.runListed(run); code != 0 {
			break
		}
	}
	return exitAfterSignal(caught, code)
}

// flagShapeRefusal returns the refusal for a malformed combination of flags, which needs no config and runs before hub resolution, or "" when the call is well-formed.
func flagShapeRefusal(request testRequest) string {
	if request.tags != "" {
		return fmt.Sprintf("gate test: --tags %q is refused, since a tier's own command sets its build tags; way forward: name the tier that compiles them with --tier, for example --tier 1, and re-run the same command", request.tags)
	}
	forms := 0
	for _, given := range []bool{len(request.targets) > 0, request.all, request.base != "", len(request.cards) > 0} {
		if given {
			forms++
		}
	}
	if request.dir != "" && len(request.targets) == 0 && forms > 0 {
		return "gate test: -C goes with explicit targets only; way forward: name the packages to run after -C, or drop -C and let the tier's own dir apply, then re-run the same command"
	}
	if forms > 1 {
		return "gate test: explicit targets, --all, --base and --card are each one scope form, and a call takes one; way forward: keep one scope form, then re-run the same command"
	}
	if request.base != "" || len(request.cards) > 0 {
		return "gate test: --base and --card are not supported yet; way forward: use explicit targets or --all, then re-run the same command"
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

// guardRunOf returns the guard step of plan, which is its last run, or nil when plan has none.
func guardRunOf(plan []tierRun) *tierRun {
	if len(plan) > 0 && plan[len(plan)-1].guardStep {
		return &plan[len(plan)-1]
	}
	return nil
}

// tierNumbers returns the tier numbers of the listed runs of plan, leaving out the guard step.
func tierNumbers(plan []tierRun) []int {
	var numbers []int
	for _, run := range plan {
		if !run.guardStep {
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
	guards    map[string][]string
	// lastStamp is the time of the latest run record, kept so two records of one process never share a stamped name.
	lastStamp time.Time
}

// runListed runs one listed tier with its slots held, and returns the exit code to stop on, or 0.
// A tier with no invocation takes no slot.
func (e *execution) runListed(run tierRun) int {
	if run.ignoresTargets {
		printLine(e.request.stderr, fmt.Sprintf("gate test: the command of %s holds no {targets}, so it runs whole", tierName(run.number)))
	}
	return e.runWithSlots(run, len(run.invocations) == 0, func(env []string) tierResult {
		return runTier(e.ctx, e.out, run, env, e.request)
	})
}

// runGuard runs the guard step with the guard tier's slots held, unless the module has no guard test.
func (e *execution) runGuard(run tierRun) int {
	moduleDir := resolveRunDir(e.request.root, run.invocations[0].dir)
	if packages, _ := guardPackages(e.guards); len(packages) == 0 {
		logger.Info("gate test: no guard test found in the module; the guard step runs nothing", "dir", moduleDir)
		return 0
	}
	return e.runWithSlots(run, false, func(env []string) tierResult {
		return runGuardStep(e.ctx, e.out, e.cfg, moduleDir, e.guards, env, e.request)
	})
}

// runWithSlots takes the slots of run's tier unless the call is unslotted, inherited or the run empty, runs body inside them with the environment the slots give, releases the slots and reports the tier's outcome.
func (e *execution) runWithSlots(run tierRun, empty bool, body func(env []string) tierResult) int {
	waitStart := time.Now()
	env := e.env
	if e.pool != nil && !e.inherited && !empty {
		lease, code := acquireSlot(e.ctx, e.out, e.pool, e.location, e.request, tierName(run.number), run.tier.Slots)
		if lease == nil {
			return code
		}
		defer releaseSlot(lease, e.request.root)
		env = lease.InheritEnv(env)
	}
	return e.report(run, body(env), waitStart)
}

// report writes the tier's run record and log, prints its summary line to stderr and returns its exit code; a refusal is answered instead.
// A record that cannot be written is logged and decides nothing.
func (e *execution) report(run tierRun, result tierResult, waitStart time.Time) int {
	if result.refusal != "" {
		return output.Err(e.out, result.refusal)
	}
	record := result.record
	record.Worktree = e.request.root
	record.Slug = e.slug
	record.TierList = e.tierList
	record.WaitStart = waitStart

	stamp := time.Now()
	if !stamp.After(e.lastStamp.Add(time.Second)) {
		stamp = e.lastStamp.Add(time.Second)
	}
	e.lastStamp = stamp
	logPath := "(not written)"
	path, err := gateslot.WriteRunRecord(e.runLogDir, record, result.log, stamp)
	if err != nil {
		logger.Warn("gate test: could not write the tier's run record", "tier", tierName(run.number), "cause", err)
	} else {
		logPath = strings.TrimSuffix(path, ".json") + ".log"
	}
	printLine(e.request.stderr, gateslot.TierSummaryLine(record, logPath))
	return record.ExitCode
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
