// redundancy.go runs the redundancy report step a `--gate` key's reports list names, after the call's lints.
// It judges the diff's new and changed tests against the coverage maps inside one gate slot and a time budget.
// It writes its report; nothing it meets changes the call's result.

package gatecli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/testcover"
)

// redundancySite labels the slot holder and wait records of the redundancy report step.
const redundancySite = siteLabelPrefix + " redundancy"

// testKitDir is the module-relative directory of the testkits whose functions the spawn scan follows.
const testKitDir = "internal/testkit"

// Notes a report carries when the step judged nothing.
const (
	noteRedGate  = "a tier of the call was red, so the step judged nothing"
	noteNoHub    = "the worktree is in no hub, and the coverage maps live in a hub, so the step judged nothing"
	noteNoParent = "the worktree has no parent branch to diff from, so the step judged nothing"
	noteScoper   = "the hub's scoper is %s, not go, so the step judged nothing"
)

// Reasons a test the step took is not judged, beside the eligibility reasons of testcover.
const (
	reasonAboveMapped   = "a tier%d test, which no coverage map holds"
	reasonUndeclared    = "tier%d, which the hub does not declare"
	reasonKeep          = "kept: %s"
	reasonScanFailed    = "its package's keep or spawn scan failed"
	reasonPastCap       = "past redundancy_max_tests (%d)"
	reasonNoMap         = "its package has no coverage map"
	reasonMapFailure    = "its package's coverage map records a build failure"
	reasonMapUnreadable = "its package's coverage map is unreadable"
	reasonNoSlot        = "the step took no gate slot"
	reasonBudgetSpent   = "redundancy_budget_min was spent before it ran"
	reasonCancelled     = "the call was cancelled"
	reasonBuildFailed   = "its package's coverage build failed"
	reasonLymFailed     = "the lym build failed"
	reasonTimedOut      = "timed out after %s"
	reasonNotRun        = "it could not run"
	reasonUnkeyed       = "its covered blocks could not be keyed"
)

// redundancyStep is one call's redundancy report step, with what it reads from the call.
type redundancyStep struct {
	ctx      context.Context
	stderr   io.Writer
	cfg      gateslot.Config
	location *lymcwd.Location
	// root is the worktree root, and gateKey the key whose call runs the step.
	root, gateKey string
	// pool is the hub's slot pool; inherited is set when an enclosing gate run's slot is held, so the step takes none.
	pool      *gateslot.Pool
	inherited bool
	// env is the environment the step's children start from, before the slot's Go flags.
	env []string
	// goBinary is the go command the builds run.
	goBinary string
	// minuteUnit is the length of one minute of redundancy_budget_min and redundancy_build_timeout_min.
	// secondUnit is the length of one second of redundancy_test_timeout_sec.
	minuteUnit, secondUnit time.Duration
	// grace bounds the wait for a killed build's or test's exit and output.
	grace time.Duration
}

// redundancyStep returns the redundancy report step of the call e runs.
func (e *execution) redundancyStep() redundancyStep {
	return redundancyStep{
		ctx: e.ctx, stderr: e.request.stderr, cfg: e.cfg, location: e.location, root: e.request.root, gateKey: e.request.gate,
		pool: e.pool, inherited: e.inherited, env: e.env, goBinary: e.request.goBinary,
		minuteUnit: e.request.timeoutUnit, secondUnit: e.request.secondUnit, grace: e.request.pipeWaitDelay,
	}
}

// run writes the step's report into the worktree's gate directory and prints its summary line.
// red tells a call one of whose runs failed, and inHub a call inside a hub; either way the report then judges nothing.
// A report the step cannot write is logged at Warn, and the call's result stands.
func (s redundancyStep) run(red, inHub bool) {
	path := redundancyReportPath(s.location)
	var report RedundancyReport
	switch {
	case red:
		report = RedundancyReport{RedGate: true, Note: noteRedGate}
	case !inHub:
		report = RedundancyReport{Note: noteNoHub}
	case s.cfg.Scoper != "go":
		report = RedundancyReport{Note: fmt.Sprintf(noteScoper, s.cfg.Scoper)}
	default:
		judged, kept := s.judge(path)
		if kept {
			printLine(s.stderr, "gate test: redundancy report: the tree matches the report already written, so the step ran no build and no test; "+path)
			return
		}
		report = judged
	}
	report.GateKey = s.gateKey
	report.Written = time.Now()
	if err := writeRedundancyReport(path, report); err != nil {
		logger.Warn("gate test: could not write the redundancy report; the gate's result stands", "path", path, "cause", err)
		return
	}
	printLine(s.stderr, report.summaryLine(path))
}

// judge returns the report for the worktree's diff, or the report at reportPath and true when that one still holds.
// The report at reportPath holds when it asks for no retry and the current tree matches it.
func (s redundancyStep) judge(reportPath string) (RedundancyReport, bool) {
	var report RedundancyReport
	module, err := redundancyModule(s.cfg, s.root)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report, false
	}
	report.ModuleDir = module.rel
	base, found, err := parentMergeBase(s.location)
	switch {
	case err != nil:
		report.Errors = append(report.Errors, fmt.Sprintf("read the merge base with the parent branch: %v", err))
		return report, false
	case !found:
		report.Note = noteNoParent
		return report, false
	}
	report.Base = base
	fabric, err := fabricengine.Open(s.location)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report, false
	}
	changed, err := impactset.ChangedPaths(s.ctx, s.root, base)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report, false
	}
	tree := workingTree{root: s.root}
	tests, err := changedTests(tree, fabric, base, module.rel, changed)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("pick the changed tests: %v", err))
		return report, false
	}
	if previous, found, err := readRedundancyReport(reportPath); err == nil && found && !previous.retries() && previous.ModuleDir == module.rel {
		if matched, err := previous.matches(tree, tests); err == nil && matched {
			return previous, true
		}
	}
	report.TestHashes = testHashesOf(tests)
	s.judgeTests(&report, module, base, fabric, tests)
	slices.SortFunc(report.Tests, func(a, b RedundancyTest) int {
		return cmp.Or(strings.Compare(a.File, b.File), strings.Compare(a.Name, b.Name))
	})
	return report, false
}

// judgeTests adds to report a line for each of tests, the tests the step took from the module.
// It drops the tests no map can judge and caps the rest.
// It judges each package's tests against its map inside one gate slot.
func (s redundancyStep) judgeTests(report *RedundancyReport, module goModule, base string, fabric *fabricengine.Fabric, tests []redundancyCandidate) {
	scans := map[string]packageScans{}
	var kept []redundancyCandidate
	for _, test := range tests {
		if reason := s.dropReason(report, module, test, scans); reason != "" {
			report.notJudged(test, reason)
			continue
		}
		kept = append(kept, test)
	}
	if limit := s.cfg.RedundancyMaxTests; len(kept) > limit {
		for _, test := range kept[limit:] {
			report.notJudged(test, fmt.Sprintf(reasonPastCap, limit))
		}
		kept = kept[:limit]
	}
	packages := s.mappedPackages(report, module, base, fabric, kept)
	if len(packages) == 0 {
		return
	}
	run := &redundancyRun{step: s, module: module, report: report}
	defer run.close()
	if err := run.holdSlot(); err != nil {
		report.Errors = append(report.Errors, err.Error())
		for _, p := range packages {
			report.notJudgedAll(p.tests, reasonNoSlot)
		}
		return
	}
	for _, p := range packages {
		run.judgePackage(p)
	}
}

// packageScans are a package's keep and spawn scans; failed marks a package whose scan failed.
type packageScans struct {
	keeps  map[string]string
	spawns map[string]testcover.SpawnVerdict
	failed bool
}

// dropReason returns why the step drops test before any build, or "" for a test it goes on with.
// It drops a test of a tier no map holds and a test of a tier the hub does not declare.
// It also drops a test marked `//testtiming:keep`,
// and a test the spawn scan marks as possibly running module code in another process.
// A test that opens with no tiergate.Require counts as tier0.
func (s redundancyStep) dropReason(report *RedundancyReport, module goModule, test redundancyCandidate, scans map[string]packageScans) string {
	tier := test.fn.Tier
	if !slices.Contains(mappedTiers, tier) {
		return fmt.Sprintf(reasonAboveMapped, tier)
	}
	if _, err := s.cfg.Tier(tier); err != nil {
		return fmt.Sprintf(reasonUndeclared, tier)
	}
	scan, scanned := scans[test.pkg]
	if !scanned {
		scan = scanPackage(report, module, test.pkg)
		scans[test.pkg] = scan
	}
	if scan.failed {
		return reasonScanFailed
	}
	if reason, marked := scan.keeps[test.fn.Name]; marked {
		return fmt.Sprintf(reasonKeep, reason)
	}
	switch verdict := scan.spawns[test.fn.Name]; {
	case verdict.OutOfProcess:
		return testcover.ReasonOutOfProcess
	case verdict.Unresolved:
		return testcover.ReasonUnresolved
	}
	return ""
}

// scanPackage runs the keep and spawn scans over the package pkg of module.
// A failed scan adds an error line to report.
func scanPackage(report *RedundancyReport, module goModule, pkg string) packageScans {
	dir := filepath.Join(module.dir, filepath.FromSlash(pkg))
	keeps, err := testcover.ScanKeeps(dir)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("keep scan of %s: %v", pkg, err))
		return packageScans{failed: true}
	}
	spawns, err := testcover.ScanSpawns(module.path, filepath.Join(module.dir, filepath.FromSlash(testKitDir)), dir)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("spawn scan of %s: %v", pkg, err))
		return packageScans{failed: true}
	}
	return packageScans{keeps: keeps, spawns: spawns}
}

// judgedPackage is one package holding kept tests, with the coverage map they are judged against.
type judgedPackage struct {
	pkg   string
	tests []redundancyCandidate
	m     testcover.Map
}

// mappedPackages groups kept by package, in order, and returns the packages whose coverage map can judge them.
// It adds a line to report per package.
// A package with no map, an unreadable map or a map recording a build failure is not judged, and nor are its tests.
// A judged package's line names its map's commit and age,
// and the files whose text at the merge base base differs from the map's.
// Its Go files' current hashes go into the report.
func (s redundancyStep) mappedPackages(report *RedundancyReport, module goModule, base string, fabric *fabricengine.Fabric, kept []redundancyCandidate) []judgedPackage {
	mapsDir := testcover.Dir(fabricengine.BoltDir(s.location.HubPath))
	var order []string
	byPackage := map[string][]redundancyCandidate{}
	for _, test := range kept {
		if _, seen := byPackage[test.pkg]; !seen {
			order = append(order, test.pkg)
		}
		byPackage[test.pkg] = append(byPackage[test.pkg], test)
	}
	var packages []judgedPackage
	for _, pkg := range order {
		tests := byPackage[pkg]
		line := RedundancyPackage{Package: pkg}
		m, found, err := testcover.ReadMap(mapsDir, pkg)
		if err == nil && found {
			err = checkMapBlocks(m)
		}
		reason := ""
		switch {
		case err != nil:
			report.Errors = append(report.Errors, fmt.Sprintf("coverage map of %s: %v", pkg, err))
			line.Reason, reason = "its coverage map is unreadable", reasonMapUnreadable
		case !found:
			line.Reason, reason = "it has no coverage map", reasonNoMap
		case m.BuildFailure != "":
			line.Reason, reason = "its coverage map records a build failure: "+m.BuildFailure, reasonMapFailure
		}
		if reason != "" {
			report.Packages = append(report.Packages, line)
			report.notJudgedAll(tests, reason)
			continue
		}
		dir := packageDirOf(module.rel, pkg)
		line.Judged, line.MapCommit, line.MapBuiltAt = true, m.Commit, m.BuiltAt
		line.MapAge = time.Since(m.BuiltAt).Round(time.Minute).String()
		if line.Drift, err = mapDrift(fabric, base, dir, m.Files); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("read %s at the merge base: %v", pkg, err))
		}
		hashes, err := goFileHashes(workingTree{root: s.root}, dir)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("hash the Go files of %s: %v", pkg, err))
		} else {
			if report.FileHashes == nil {
				report.FileHashes = map[string]map[string]string{}
			}
			report.FileHashes[pkg] = hashes
		}
		report.Packages = append(report.Packages, line)
		packages = append(packages, judgedPackage{pkg: pkg, tests: tests, m: m})
	}
	return packages
}

// checkMapBlocks returns an error for a map whose test names a block index outside its block table.
func checkMapBlocks(m testcover.Map) error {
	for _, test := range m.Tests {
		for _, index := range test.Blocks {
			if index < 0 || index >= len(m.Blocks) {
				return fmt.Errorf("test %s names block %d of a table of %d", test.Name, index, len(m.Blocks))
			}
		}
	}
	return nil
}

// mapDrift returns the sorted names of the Go files in the worktree-relative slash directory dir that drifted.
// A file drifted when its text at the commit base hashes otherwise than mapped records;
// a file on one side alone counts.
func mapDrift(fabric *fabricengine.Fabric, base, dir string, mapped map[string]string) ([]string, error) {
	atBase, err := goFileHashes(commitTree{fabric: fabric, rev: base}, dir)
	if err != nil {
		return nil, err
	}
	var drift []string
	for name, hash := range atBase {
		if mapped[name] != hash {
			drift = append(drift, name)
		}
	}
	for name := range mapped {
		if _, found := atBase[name]; !found {
			drift = append(drift, name)
		}
	}
	slices.Sort(drift)
	return drift, nil
}

// notJudged adds test to the report as not judged for reason.
func (r *RedundancyReport) notJudged(test redundancyCandidate, reason string) {
	r.Tests = append(r.Tests, RedundancyTest{Package: test.pkg, Name: test.fn.Name, File: test.file, Verdict: RedundancyNotJudged, Reason: reason})
}

// notJudgedAll adds each of tests to the report as not judged for reason.
func (r *RedundancyReport) notJudgedAll(tests []redundancyCandidate, reason string) {
	for _, test := range tests {
		r.notJudged(test, reason)
	}
}

// redundancyRun is the part of the step that holds its gate slot: the builds, the test runs and the verdicts.
type redundancyRun struct {
	step   redundancyStep
	module goModule
	report *RedundancyReport

	// lease is the held slot; it stays nil inside an enclosing gate run's slot.
	lease *gateslot.Lease
	// holderEnv is the step's env with the slot's Go flags, and parallel the `-p` of the `lym` build.
	holderEnv []string
	parallel  int
	// tmp holds the coverage binaries and profiles.
	tmp string
	// deadline is when redundancy_budget_min, counted from the slot's hold, is spent.
	deadline time.Time
	// lymBuilt is set once the run built `lym`; lymBin is the binary, and lymErr the build's failure.
	lymBuilt bool
	lymBin   string
	lymErr   error
}

// holdSlot takes one slot of the hub's pool for the rest of the step, unless an enclosing gate run's slot is held.
// Its wait has no bound but the call's context, and the budget counts from its end.
// It fills the slot holder's Go flags into the step's environment and creates the run's temporary directory.
func (r *redundancyRun) holdSlot() error {
	s := r.step
	limits, err := s.pool.Limits()
	if err != nil {
		return fmt.Errorf("read the hub's gate limits: %w", err)
	}
	if !s.inherited {
		waitPath, err := gateslot.WriteWait(gateslot.WaitDir(s.location.AnchorPath()), gateslot.Wait{Site: redundancySite, PID: os.Getpid(), Started: time.Now()})
		if err != nil {
			logger.Warn("gate test: could not write the gate wait record", "cause", err)
		} else {
			defer os.Remove(waitPath)
		}
		if r.lease, err = s.pool.Acquire(s.ctx, gateslot.Holder{Worktree: s.location.WorktreePath(), Site: redundancySite}); err != nil {
			return fmt.Errorf("acquire a gate slot: %w", err)
		}
	}
	r.deadline = time.Now().Add(time.Duration(s.cfg.RedundancyBudgetMin) * s.minuteUnit)
	if r.tmp, err = lymtemp.MkdirTemp("redundancy-"); err != nil {
		return fmt.Errorf("create the coverage binary directory: %w", err)
	}
	r.parallel = limits.Tier0Parallel
	if r.parallel < 1 {
		r.parallel = runtime.NumCPU()
	}
	r.holderEnv = gateslot.HolderEnv(s.env, r.lease, limits.Tier0Parallel)
	return nil
}

// close releases the run's slot and removes its temporary directory.
func (r *redundancyRun) close() {
	if r.lease != nil {
		releaseSlot(r.lease, r.step.root)
	}
	if r.tmp != "" {
		os.RemoveAll(r.tmp)
	}
}

// stopReason returns why the run starts no further build or test, or "" while it may.
func (r *redundancyRun) stopReason() string {
	switch {
	case r.step.ctx.Err() != nil:
		return reasonCancelled
	case !time.Now().Before(r.deadline):
		return reasonBudgetSpent
	}
	return ""
}

// buildTimeout is the bound of each coverage build and of the `lym` build.
func (r *redundancyRun) buildTimeout() time.Duration {
	return time.Duration(r.step.cfg.RedundancyBuildTimeoutMin) * r.step.minuteUnit
}

// judgePackage builds p's coverage binary, runs each of its tests alone and judges them against p's map.
// A test the run does not reach before the budget is spent is listed as not judged for it.
func (r *redundancyRun) judgePackage(p judgedPackage) {
	s := r.step
	if reason := r.stopReason(); reason != "" {
		r.report.notJudgedAll(p.tests, reason)
		return
	}
	pkgDir := filepath.Join(r.module.dir, filepath.FromSlash(p.pkg))
	importPath := r.module.importPath(p.pkg)
	bin := filepath.Join(r.tmp, "pkg.test")
	defer os.Remove(bin)
	builder := testcover.Toolchain{Go: s.goBinary, Env: r.holderEnv, Dir: r.module.dir, Ctx: s.ctx, BuildTimeout: r.buildTimeout(), Grace: s.grace}
	if err := builder.BuildBinary(importPath, importPath, "", bin); err != nil {
		r.report.Errors = append(r.report.Errors, fmt.Sprintf("coverage build of %s: %v", p.pkg, err))
		r.report.notJudgedAll(p.tests, reasonBuildFailed)
		return
	}
	var ran []redundancyCandidate
	var runs []testcover.Run
	for _, test := range p.tests {
		if reason := r.stopReason(); reason != "" {
			r.report.notJudged(test, reason)
			continue
		}
		tier, _ := s.cfg.Tier(test.fn.Tier)
		if tier.PrebuiltLym && r.buildLym() != nil {
			r.report.notJudged(test, reasonLymFailed)
			continue
		}
		run, reason := r.runTest(p.pkg, pkgDir, importPath, bin, test, tier)
		if reason != "" {
			r.report.notJudged(test, reason)
			continue
		}
		ran = append(ran, test)
		runs = append(runs, run)
	}
	r.judgeRuns(p, ran, runs)
}

// buildLym builds `lym` from the worktree into the pool's prebuilt store, once per step, bounded by the build timeout.
// A failed build is kept and listed once as an error,
// so every later test that needs it is not judged, with no second build.
func (r *redundancyRun) buildLym() error {
	if r.lymBuilt {
		return r.lymErr
	}
	r.lymBuilt = true
	s := r.step
	ctx, cancel := context.WithTimeout(s.ctx, r.buildTimeout())
	defer cancel()
	// The store keeps the binary, so the cleanup prebuildLym returns for it removes nothing.
	bin, _, err := prebuildLym(ctx, s.goBinary, s.root, r.parallel, s.pool.PrebuiltDir(), s.grace, r.holderEnv)
	switch {
	case err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded):
		r.lymErr = fmt.Errorf("lym build: timed out after %s", r.buildTimeout())
	case err != nil:
		r.lymErr = fmt.Errorf("lym build: %w", err)
	}
	if r.lymErr != nil {
		r.report.Errors = append(r.report.Errors, r.lymErr.Error())
	}
	r.lymBin = bin
	return r.lymErr
}

// runTest runs test alone under the coverage binary bin in the package directory pkgDir with its tier's env.
// It returns the test's run with the keys of the package blocks it covered.
// It returns the reason the test is not judged instead when the call was cancelled.
// It does so too when the test timed out, could not run or had blocks that could not be keyed.
func (r *redundancyRun) runTest(pkg, pkgDir, importPath, bin string, test redundancyCandidate, tier gateslot.Tier) (testcover.Run, string) {
	s := r.step
	env := slices.Concat(r.holderEnv, renderTierEnv(tier))
	if tier.PrebuiltLym && r.lymBin != "" {
		env = append(env, gateslot.PrebuiltLymEnv+"="+r.lymBin)
	}
	timeout := time.Duration(s.cfg.RedundancyTestTimeoutSec) * s.secondUnit
	runner := testcover.Toolchain{Env: env, Ctx: s.ctx, TestTimeout: timeout, Grace: s.grace}
	result, err := runner.RunAlone(bin, pkgDir, filepath.Join(r.tmp, "cover.out"), test.fn.Name)
	// A cancelled call kills the test in flight, which then reads as a failed test; it is listed as cancelled instead,
	// so the report asks for a retry.
	if s.ctx.Err() != nil {
		return testcover.Run{}, reasonCancelled
	}
	switch {
	case err != nil:
		r.report.Errors = append(r.report.Errors, fmt.Sprintf("run %s.%s: %v", pkg, test.fn.Name, err))
		return testcover.Run{}, reasonNotRun
	case result.TimedOut:
		return testcover.Run{}, fmt.Sprintf(reasonTimedOut, timeout)
	}
	run := testcover.Run{Name: test.fn.Name, Elapsed: result.Elapsed, Action: result.Action}
	if result.Action == testcover.ActionPass {
		blocks, err := testcover.CoveredKeys(pkgDir, importPath, result.Profile)
		if err != nil {
			r.report.Errors = append(r.report.Errors, fmt.Sprintf("key the blocks %s.%s covered: %v", pkg, test.fn.Name, err))
			return testcover.Run{}, reasonUnkeyed
		}
		run.Blocks = blocks
	}
	return run, ""
}

// judgeRuns gives each of runs, the runs of the tests ran of p, its verdict against the tests of p's map that still hold.
// A map test holds when the package still declares a test of its name with the same function hash,
// and no run bears its name.
// The greedy pass keeps every map test that holds, and a run it drops is contained.
func (r *redundancyRun) judgeRuns(p judgedPackage, ran []redundancyCandidate, runs []testcover.Run) {
	if len(runs) == 0 {
		return
	}
	current, err := testcover.ScanTestFuncs(filepath.Join(r.module.dir, filepath.FromSlash(p.pkg)))
	if err != nil {
		r.report.Errors = append(r.report.Errors, fmt.Sprintf("read the tests of %s: %v", p.pkg, err))
		r.report.notJudgedAll(ran, reasonScanFailed)
		return
	}
	hashes := map[string]string{}
	for _, fn := range current {
		hashes[fn.Name] = fn.Hash
	}
	judged := map[string]bool{}
	for _, test := range ran {
		judged[test.fn.Name] = true
	}
	var kept []testcover.Run
	for _, mapped := range p.m.Tests {
		if judged[mapped.Name] || hashes[mapped.Name] != mapped.FuncHash {
			continue
		}
		blocks := testcover.BlockSet{}
		for _, index := range mapped.Blocks {
			blocks[p.m.Blocks[index].String()] = struct{}{}
		}
		kept = append(kept, testcover.Run{Name: mapped.Name, Elapsed: mapped.Seconds, Action: testcover.ActionPass, Blocks: blocks})
	}
	for i, verdict := range testcover.JudgeAgainst(runs, kept) {
		line := RedundancyTest{Package: p.pkg, Name: ran[i].fn.Name, File: ran[i].file}
		switch {
		case verdict.Reason != "":
			line.Verdict, line.Reason = RedundancyNotJudged, verdict.Reason
		case verdict.Removable:
			line.Verdict, line.Covering = RedundancyContained, verdict.Covering
		default:
			line.Verdict = RedundancyUnique
		}
		r.report.Tests = append(r.report.Tests, line)
	}
}
