// reach.go runs the reach report step a `--gate` key's reports list names, after the call's lints.
// It finds the functions the worktree's diff changes and refreshes the worktree's own coverage maps of their packages.
// It records the tests that reach each function, or the caveats of a function none reaches.
// It writes its report and prints its note; nothing it meets changes the call's result.

package gatecli

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testreach"
)

// Notes and reasons of the reach report beside those the redundancy report shares.
const (
	noteNoMaps             = "no coverage map exists, so the step judged nothing; way forward: build the maps with `lym gate test --map` from the prime"
	reasonPrimeKeepsNoMaps = "the call runs in the prime, which keeps no worktree maps; way forward: rebuild the shared maps with `lym gate test --map`"
	reasonMapRunRefused    = "the map run could not start, as its error line says"
	reasonNoRefreshSlot    = "the step took no gate slot, as its error line says"
	reasonRefreshEnded     = "the refresh ended before it reached it, as its error line says"
	reasonHeadUnreadable   = "HEAD could not be read, as its error line says; way forward: run the gate again"
	reasonMapCheckFailed   = "its map or files could not be checked, as its error line says; way forward: run the gate again"
)

// reachStep is one call's reach report step, with what it reads from the call.
type reachStep struct {
	ctx      context.Context
	stderr   io.Writer
	cfg      gateslot.Config
	location *lymcwd.Location
	// root is the worktree root, and gateKey the key whose call runs the step.
	root, gateKey string
	// environ is the environment the refresh's map run starts its children from: the process's own in production.
	environ []string
	// goBinary is the go command the refresh's builds run.
	goBinary string
	// minuteUnit is the length of one minute of reach_budget_min and redundancy_build_timeout_min.
	// secondUnit is the length of one second of redundancy_test_timeout_sec.
	minuteUnit, secondUnit time.Duration
	// grace bounds the wait for a killed build's or test's exit and output.
	grace time.Duration
	// guardKilled reports whether the memory guard killed the process with a pid; nil means it killed none.
	guardKilled func(pid int) bool
}

// reachStep returns the reach report step of the call e runs.
func (e *execution) reachStep() reachStep {
	return reachStep{
		ctx: e.ctx, stderr: e.request.stderr, cfg: e.cfg, location: e.location, root: e.request.root, gateKey: e.request.gate,
		environ: os.Environ(), goBinary: e.request.goBinary,
		minuteUnit: e.request.timeoutUnit, secondUnit: e.request.secondUnit, grace: e.request.pipeWaitDelay,
	}
}

// run writes the step's report into the worktree's gate directory and prints its note to stderr.
// red tells a call one of whose runs failed, and inHub a call inside a hub; either way the report then judges nothing.
// A report the step cannot write is logged at Warn, and the call's result stands.
func (s reachStep) run(red, inHub bool) {
	path := reachReportPath(s.location)
	var report ReachReport
	switch {
	case red:
		report = ReachReport{RedGate: true, Note: noteRedGate}
	case !inHub:
		report = ReachReport{Note: noteNoHub}
	case s.cfg.Scoper != "go":
		report = ReachReport{Note: fmt.Sprintf(noteScoper, s.cfg.Scoper)}
	default:
		report = s.answer()
	}
	report.GateKey = s.gateKey
	report.Written = time.Now()
	if err := state.WriteJSON(path, lock.TrackedFileLockPath(path, ".lock"), report); err != nil {
		logger.Warn("gate test: could not write the reach report; the gate's result stands", "path", path, "cause", err)
		return
	}
	for _, line := range report.NoteLines(path) {
		printLine(s.stderr, line)
	}
}

// answer returns the report for the worktree's diff.
// The diff is from the merge base with the parent branch.
// The prime has none, so it is the working tree's changes from HEAD.
// A task worktree with no parent branch gets a report that judges nothing.
func (s reachStep) answer() ReachReport {
	var report ReachReport
	module, err := redundancyModule(s.cfg, s.root)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report
	}
	report.ModuleDir = module.rel
	fabric, err := fabricengine.Open(s.location)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report
	}
	prime, err := hubgeom.IsPrime(s.location)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report
	}
	report.Prime = prime
	base, err := s.diffBase(fabric, prime)
	switch {
	case err != nil:
		report.Errors = append(report.Errors, err.Error())
		return report
	case base == "":
		report.Note = noteNoParent
		return report
	}
	report.Base = base
	changed, err := impactset.ChangedPaths(s.ctx, s.root, base)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report
	}
	funcs, err := changedFuncsAt(workingTree{root: s.root}, fabric, base, module, changed)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("find the changed functions: %v", err))
		return report
	}
	if len(funcs) == 0 {
		return report
	}
	s.refreshMaps(&report, module, funcs, prime)
	reader, err := testreach.Open(reachModuleOf(module), testcover.Dir(fabricengine.BoltDir(s.location.HubPath)), s.worktreeMapsDir(prime))
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("read the coverage maps: %v", err))
		return report
	}
	if len(reader.Maps()) == 0 {
		report.Note = noteNoMaps
		return report
	}
	s.describe(&report, module, reader, funcs)
	return report
}

// diffBase returns the commit the step diffs from: HEAD in the prime, otherwise the merge base with the parent branch.
// It returns "" for a task worktree with no parent branch.
func (s reachStep) diffBase(fabric *fabricengine.Fabric, prime bool) (string, error) {
	if prime {
		return fabric.HeadSHA()
	}
	base, found, err := parentMergeBase(s.location)
	if err != nil {
		return "", fmt.Errorf("read the merge base with the parent branch: %w", err)
	}
	if !found {
		return "", nil
	}
	return base, nil
}

// worktreeMapsDir returns the worktree's own maps directory, or "" in the prime, which keeps none.
func (s reachStep) worktreeMapsDir(prime bool) string {
	if prime {
		return ""
	}
	return testcover.Dir(s.location.AnchorPath())
}

// reachModuleOf returns module as the testreach package names it.
func reachModuleOf(module goModule) testreach.Module {
	return testreach.Module{Dir: module.dir, Rel: module.rel, Path: module.path}
}

// relativePackage returns the module-relative slash path of the package at the import path importPath.
// A package outside the module keeps its import path.
func (m goModule) relativePackage(importPath string) string {
	if importPath == m.path {
		return "."
	}
	if rel, inside := strings.CutPrefix(importPath, m.path+"/"); inside {
		return rel
	}
	return importPath
}

// changedFuncsAt returns the functions of the module's production Go files that head changes against the commit base.
// changed are the worktree-relative slash paths that differ between base and head.
// It takes each package directory holding a changed production Go file.
// It reads every production Go file of the directory at base, through fabric, and in head, and compares the two sets.
func changedFuncsAt(head redundancyTree, fabric *fabricengine.Fabric, base string, module goModule, changed []string) ([]testreach.ChangedFunc, error) {
	dirs := map[string]bool{}
	for _, rel := range changed {
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			continue
		}
		if dir := path.Dir(rel); isModulePackage(module.rel, dir) {
			dirs[dir] = true
		}
	}
	baseFiles, headFiles := testreach.FileSet{}, testreach.FileSet{}
	for _, dir := range slices.Sorted(maps.Keys(dirs)) {
		for _, side := range []struct {
			tree  redundancyTree
			files testreach.FileSet
		}{{commitTree{fabric: fabric, rev: base}, baseFiles}, {head, headFiles}} {
			names, err := side.tree.goFiles(dir)
			if err != nil {
				return nil, fmt.Errorf("list the Go files of %s: %w", dir, err)
			}
			for _, name := range names {
				if strings.HasSuffix(name, "_test.go") {
					continue
				}
				data, found, err := side.tree.file(path.Join(dir, name))
				if err != nil {
					return nil, fmt.Errorf("read %s: %w", path.Join(dir, name), err)
				}
				if found {
					side.files[path.Join(dir, name)] = data
				}
			}
		}
	}
	return testreach.ChangedFuncs(reachModuleOf(module), baseFiles, headFiles)
}

// changedPackages returns the module-relative slash path of each package holding one of funcs, sorted.
func changedPackages(module goModule, funcs []testreach.ChangedFunc) []string {
	packages := map[string]bool{}
	for _, fn := range funcs {
		packages[module.relativePackage(fn.Key.Package)] = true
	}
	return slices.Sorted(maps.Keys(packages))
}

// refreshMaps refreshes the worktree's own map of each package of funcs that needs one.
// It lists the packages it does not refresh as unrefreshed.
// A package needs one when it has no map or its map is not fresh against the worktree's files.
// It is refreshed only when no file it or a linked package owns is uncommitted, and only inside reach_budget_min.
// The prime refreshes nothing: it keeps no worktree maps, and its shared maps are the map run's to build.
func (s reachStep) refreshMaps(report *ReachReport, module goModule, funcs []testreach.ChangedFunc, prime bool) {
	packages := changedPackages(module, funcs)
	unrefreshed := func(pkg, reason string) {
		report.Unrefreshed = append(report.Unrefreshed, fmt.Sprintf("%s: not refreshed, %s", pkg, reason))
	}
	if prime {
		for _, pkg := range packages {
			unrefreshed(pkg, reasonPrimeKeepsNoMaps)
		}
		return
	}
	run := &testMapRun{ctx: s.ctx, request: s.mapRequest(), location: s.location, mapsDir: testcover.Dir(s.location.AnchorPath())}
	defer run.close()
	if refusal := run.prepare(); refusal != "" {
		report.Errors = append(report.Errors, refusal)
		for _, pkg := range packages {
			unrefreshed(pkg, reasonMapRunRefused)
		}
		return
	}
	run.guardKilled = s.guardKilled
	head, err := run.fabric.HeadSHA()
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("read HEAD: %v", err))
		for _, pkg := range packages {
			unrefreshed(pkg, reasonHeadUnreadable)
		}
		return
	}
	var jobs []string
	for _, pkg := range packages {
		linked, stale, err := s.staleLinked(run, module, pkg)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("read the map of %s: %v", pkg, err))
			unrefreshed(pkg, reasonMapCheckFailed)
			continue
		}
		if !stale {
			continue
		}
		reason, err := s.dirtyReason(run, module, head, pkg, linked)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("check the files of %s: %v", pkg, err))
			unrefreshed(pkg, reasonMapCheckFailed)
			continue
		}
		if reason != "" {
			unrefreshed(pkg, reason)
			continue
		}
		jobs = append(jobs, pkg)
	}
	if len(jobs) == 0 {
		return
	}
	if err := run.holdSlot(); err != nil {
		report.Errors = append(report.Errors, err.Error())
		for _, pkg := range jobs {
			unrefreshed(pkg, reasonNoRefreshSlot)
		}
		return
	}
	run.deadline = time.Now().Add(time.Duration(s.cfg.ReachBudgetMin) * s.minuteUnit)
	if err := run.mapPackages(jobs); err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("refresh the maps: %v", err))
	}
	for _, pkg := range jobs {
		if !slices.Contains(run.built, pkg) {
			unrefreshed(pkg, refreshLeftReason(run, pkg))
		}
	}
}

// refreshLeftReason returns why the map run's walk wrote no new map of the package pkg.
func refreshLeftReason(run *testMapRun, pkg string) string {
	if i := slices.IndexFunc(run.unmeasured, func(skip testMapSkip) bool { return skip.Package == pkg }); i >= 0 {
		return run.unmeasured[i].Reason
	}
	if i := slices.IndexFunc(run.failed, func(failure testMapFailure) bool { return failure.Package == pkg }); i >= 0 {
		return "its coverage build failed: " + run.failed[i].Failure
	}
	return reasonRefreshEnded
}

// mapRequest returns the map run request of the step's refresh.
func (s reachStep) mapRequest() testMapRequest {
	return testMapRequest{
		goBinary: s.goBinary, environ: s.environ, secondUnit: s.secondUnit, minuteUnit: s.minuteUnit, grace: s.grace,
		cappedParallel: memoryCappedParallel, stderr: s.stderr,
	}
}

// staleLinked reports whether the worktree's own map of the package pkg is absent or no longer fresh.
// It returns the import paths of the in-module packages linked by the package's test binary:
// those its map records, or for a package with no map those the go command lists.
func (s reachStep) staleLinked(run *testMapRun, module goModule, pkg string) (linked []string, stale bool, err error) {
	m, found, err := testreach.ReadPackageMap(run.mapsDir, pkg)
	if err != nil {
		return nil, false, err
	}
	if found {
		fresh, err := testreach.Fresh(reachModuleOf(module), pkg, m)
		if err != nil || fresh {
			return nil, false, err
		}
		return slices.Sorted(maps.Keys(m.Linked)), true, nil
	}
	toolchain := testcover.Toolchain{Go: s.goBinary, Env: run.env, Dir: run.moduleDir, Ctx: s.ctx, BuildTimeout: run.buildTimeout, Grace: s.grace}
	linked, err = toolchain.LinkedPackages(module.importPath(pkg))
	if err != nil {
		return nil, false, err
	}
	return linked, true, nil
}

// dirtyReason returns why the package pkg is not refreshed for an uncommitted file, or "" when none blocks it.
// The file may belong to the package itself or to one of the packages linked by its test binary, at the commit head.
func (s reachStep) dirtyReason(run *testMapRun, module goModule, head, pkg string, linked []string) (string, error) {
	trees := map[string]packageTree{}
	for _, importPath := range append([]string{module.importPath(pkg)}, linked...) {
		dir, inModule := run.moduleDirOf(importPath)
		if !inModule {
			continue
		}
		tree, err := run.readPackageTree(head, dir)
		if err != nil {
			return "", err
		}
		trees[importPath] = tree
	}
	file, err := run.dirtyOwnedFile(trees)
	if err != nil || file == "" {
		return "", err
	}
	owner := "its own"
	for _, importPath := range slices.Sorted(maps.Keys(trees)) {
		if trees[importPath].owns(file) && importPath != module.importPath(pkg) {
			owner = "linked package " + module.relativePackage(importPath) + "'s"
		}
	}
	return fmt.Sprintf("the uncommitted file %s is %s file; way forward: commit it and run the gate again", file, owner), nil
}

// describe records, for each of funcs, the tests of reader that reach it, or why it may still be run unseen.
// It also records the maps and the Go file hashes of the packages the report names.
func (s reachStep) describe(report *ReachReport, module goModule, reader *testreach.Reader, funcs []testreach.ChangedFunc) {
	named := map[string]bool{}
	for _, fn := range funcs {
		pkg := module.relativePackage(fn.Key.Package)
		named[pkg] = true
		line := ReachFunction{Package: pkg, File: path.Join(packageDirOf(module.rel, pkg), fn.Key.File), Func: fn.Key.Func, Hash: fn.Hash, New: fn.New}
		reaches := reader.TestsReaching(fn.Key)
		for i, reach := range reaches {
			if i == maxReachTestsPerFunction {
				line.MoreTests = len(reaches) - i
				break
			}
			testPkg := module.relativePackage(reach.Package)
			named[testPkg] = true
			line.Tests = append(line.Tests, ReachTest{Package: testPkg, Test: reach.Test})
		}
		if len(reaches) == 0 {
			line.Caveats = s.caveats(report, module, reader, pkg, fn)
		}
		report.Functions = append(report.Functions, line)
	}
	for _, source := range reader.Maps() {
		if named[source.Package] {
			report.Maps = append(report.Maps, ReachMap{Package: source.Package, Commit: source.Commit, BuiltAt: source.BuiltAt, Worktree: source.Worktree})
		}
	}
	for _, pkg := range slices.Sorted(maps.Keys(named)) {
		hashes, err := goFileHashes(workingTree{root: s.root}, packageDirOf(module.rel, pkg))
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("hash the Go files of %s: %v", pkg, err))
			continue
		}
		if report.FileHashes == nil {
			report.FileHashes = map[string]map[string]string{}
		}
		report.FileHashes[pkg] = hashes
	}
}

// caveats returns the reasons a test may run fn, which no mapped test reaches, without the maps showing it.
// One is a test of its package above tier1, or an entry its map marks as running code in another process.
// The other, for a function new since the base, is a package linking its package whose map is older than its files.
func (s reachStep) caveats(report *ReachReport, module goModule, reader *testreach.Reader, pkg string, fn testreach.ChangedFunc) []string {
	var caveats []string
	tests, err := testcover.ScanTestFuncs(filepath.Join(module.dir, filepath.FromSlash(pkg)))
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("scan the tests of %s: %v", pkg, err))
	}
	aboveMapped := slices.ContainsFunc(tests, func(test testcover.TestFunc) bool { return !slices.Contains(mappedTiers, test.Tier) })
	if aboveMapped || reader.SpawnMarked(pkg) {
		caveats = append(caveats, fmt.Sprintf("a test of %s that no map holds, or that may run code in another process, may run it unseen", pkg))
	}
	if !fn.New {
		return caveats
	}
	stale, err := reader.StaleLinkers(fn.Key.Package)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("find the packages linking %s: %v", pkg, err))
		return caveats
	}
	if len(stale) > 0 {
		caveats = append(caveats, fmt.Sprintf("the tests of %s may reach it: their maps recorded the files of %s before they changed", strings.Join(stale, ", "), pkg))
	}
	return caveats
}
