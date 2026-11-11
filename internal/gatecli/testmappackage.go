// testmappackage.go measures one queued package for `lym gate testmap`, inside the call's slot.
// It runs the package's tests alone under a coverage binary over every in-module package the test binary links.
// It keys the blocks they cover of the package's own files and names the functions they cover of the others.
// It merges the runs into the package's map, and re-measures the kept entries the fresh build cannot place.
// It discards the runs when a linked package's files are dirty or changed under its tests.

package gatecli

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/envconfig"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/testcover"
)

// mappedRun is one mapped test's run alone, with the function it ran.
type mappedRun struct {
	fn     testcover.TestFunc
	result testcover.RunResult
	// outcome is why the run records no coverage, empty for a passing run.
	outcome testcover.TestOutcome
}

// mapPackage measures the job's package at the call's HEAD, writes its map and reports the package built.
// A whole job starts from an empty map at HEAD:
// it runs every mapped test but those the spawn scan marks, which enter with their verdict, unrun.
// Any other job runs its tests into the map the plan wrote at HEAD, each replacing its entry;
// the package is skipped and counted as queued when that map no longer stands at HEAD.
// Such a job also re-runs each kept passing entry whose block keys or linked functions the fresh build lacks.
// It counts each as a self-check mismatch.
// When none of its runs wrote a profile, it first re-runs one kept passing entry to read the fresh build's blocks.
// An audit job counts each re-run entry whose fresh coverage differs from the kept one as an audit mismatch.
// A package with no mapped test gets a map of its file hashes and no tests, with no build.
// A failed build is recorded through writeFailed.
// A package gets no map and is reported skipped when a linked package's file is dirty just before its build.
// The first result is then true, since no build was spent on the package.
// It is skipped the same way, with the first result false, when such a file is dirty after its tests.
// It is skipped so too when such a file differs at HEAD once they ended.
// The error reports a slot, read or write failure, or a cancelled call, that ends the call.
func (r *testMapRun) mapPackage(job testMapJob) (bool, error) {
	pkg, head := job.pkg, r.head
	pkgDir := filepath.Join(r.location.WorktreePath(), filepath.FromSlash(job.dir))
	importPath := r.importPath(pkg)
	own, err := r.readPackageTree(head, job.dir)
	if err != nil {
		return false, err
	}
	m := testcover.Map{Commit: head, Files: job.files, OtherFiles: own.otherFiles()}
	if !job.whole {
		planned, found, err := testcover.ReadMap(r.mapsDir, pkg)
		if err != nil || !found || planned.Commit != head {
			r.skip(pkg, "its map no longer stands at the planned HEAD")
			r.queued++
			return true, nil
		}
		m = planned
	}
	mapped, err := r.mappedTests(pkgDir)
	if err != nil {
		return false, r.writeFailed(pkg, m, err)
	}
	var toRun []testcover.TestFunc
	if job.whole {
		if len(mapped) == 0 {
			return false, r.write(pkg, m)
		}
		spawns, err := r.scanSpawns(pkgDir)
		if err != nil {
			return false, r.writeFailed(pkg, m, fmt.Errorf("spawn scan: %w", err))
		}
		for _, fn := range mapped {
			if verdict := spawns[fn.Name]; verdict.OutOfProcess || verdict.Unresolved {
				m.Tests = append(m.Tests, testcover.MapTest{Name: fn.Name, Tier: fn.Tier, FuncHash: fn.Hash, Spawn: verdict, Commit: head, MeasuredAt: time.Now()})
				continue
			}
			toRun = append(toRun, fn)
		}
		if len(toRun) == 0 {
			return false, r.write(pkg, m)
		}
	} else {
		for _, fn := range mapped {
			if slices.Contains(job.tests, fn.Name) {
				toRun = append(toRun, fn)
			}
		}
	}

	if err := r.holdSlot(); err != nil {
		return false, err
	}
	builder := testcover.Toolchain{
		Go: r.request.goBinary, Env: r.holderEnv, Dir: r.moduleDir, Ctx: r.ctx, BuildTimeout: r.buildTimeout, Grace: r.request.grace,
	}
	linked, err := builder.LinkedPackages(importPath)
	if err != nil {
		if cancelled := r.ctx.Err(); cancelled != nil {
			return false, cancelled
		}
		return false, r.writeFailed(pkg, m, err)
	}
	linked = slices.Compact(slices.Sorted(slices.Values(append(linked, importPath))))
	trees, err := r.linkedTrees(head, importPath, own, linked)
	if err != nil {
		return false, err
	}
	m.Linked = linkedHashes(trees, importPath)
	file, err := r.dirtyOwnedFile(trees)
	if err != nil {
		return false, err
	}
	if file != "" {
		r.skip(pkg, "modified or untracked file "+file)
		return true, nil
	}

	bin := filepath.Join(r.tmp, "pkg.test")
	defer os.Remove(bin)
	buildStart := time.Now()
	if err := builder.BuildBinary(importPath, strings.Join(linked, ","), "", bin); err != nil {
		// A terminating signal ends the build for a cause outside the package, so no failure is recorded.
		if cancelled := r.ctx.Err(); cancelled != nil {
			return false, cancelled
		}
		return false, r.writeFailed(pkg, m, err)
	}
	buildSeconds := time.Since(buildStart).Seconds()
	if slices.ContainsFunc(mapped, func(fn testcover.TestFunc) bool { return r.tiers[fn.Tier].PrebuiltLym }) {
		if err := r.buildLym(); err != nil {
			if cancelled := r.ctx.Err(); cancelled != nil {
				return false, cancelled
			}
			return false, r.writeFailed(pkg, m, err)
		}
	}
	var indexes []*testcover.FuncIndex
	for _, other := range linked {
		if other != importPath {
			indexes = append(indexes, testcover.NewFuncIndex(filepath.Join(r.location.WorktreePath(), filepath.FromSlash(trees[other].dir)), other))
		}
	}
	runStart := time.Now()
	runs, err := r.runTests(pkg, pkgDir, bin, toRun)
	if err != nil {
		return false, err
	}
	if !job.whole {
		if firstProfile(runs) == nil {
			probe, err := r.runFirstKeptEntry(pkg, pkgDir, bin, m, mapped, runs)
			if err != nil {
				return false, err
			}
			runs = append(runs, probe...)
		}
		if profile := firstProfile(runs); profile != nil {
			ran := map[string]bool{}
			for _, run := range runs {
				ran[run.fn.Name] = true
			}
			failing, err := selfCheck(m, pkgDir, importPath, profile, indexes, ran)
			if err != nil {
				return false, r.writeFailed(pkg, m, err)
			}
			r.selfCheckMismatches += len(failing)
			var again []testcover.TestFunc
			for _, fn := range mapped {
				if slices.Contains(failing, fn.Name) {
					again = append(again, fn)
				}
			}
			more, err := r.runTests(pkg, pkgDir, bin, again)
			if err != nil {
				return false, err
			}
			runs = append(runs, more...)
		}
	}
	runSeconds := time.Since(runStart).Seconds()

	file, err = r.dirtyOwnedFile(trees)
	if err != nil {
		return false, err
	}
	if file != "" {
		r.skip(pkg, "modified or untracked file "+file+" after its tests")
		return false, nil
	}
	changed, err := r.changedFiles(head, importPath, trees)
	if err != nil {
		return false, err
	}
	if len(changed) > 0 {
		r.skip(pkg, "files changed under its tests: "+strings.Join(changed, ", "))
		return false, nil
	}
	r.measures = append(r.measures, testcover.PackageMeasure{
		Package: pkg, BuildSeconds: buildSeconds, RunSeconds: runSeconds, Tests: len(runs),
	})
	before := m
	if err := fillMapTests(&m, pkgDir, importPath, runs, indexes, head, time.Now()); err != nil {
		return false, r.writeFailed(pkg, before, err)
	}
	if job.audit {
		for _, name := range job.tests {
			if entryCoverage(before, name) != entryCoverage(m, name) {
				r.auditMismatches++
			}
		}
	}
	m.Failure = nil
	return false, r.write(pkg, m)
}

// importPath returns the import path of the module-relative package directory pkg.
func (r *testMapRun) importPath(pkg string) string {
	if pkg == "." {
		return r.modulePath
	}
	return r.modulePath + "/" + pkg
}

// moduleDirOf returns the worktree-relative slash directory of the package at importPath.
// It reports false for a package outside the module.
func (r *testMapRun) moduleDirOf(importPath string) (string, bool) {
	if importPath == r.modulePath {
		return r.moduleRel, true
	}
	rel, inModule := strings.CutPrefix(importPath, r.modulePath+"/")
	if !inModule {
		return "", false
	}
	return path.Join(r.moduleRel, rel), true
}

// linkedTrees returns the package tree at the commit head of each package of linked, by import path.
// own is the tree of the package at importPath, which linked holds.
// A linked package outside the module is an error.
func (r *testMapRun) linkedTrees(head, importPath string, own packageTree, linked []string) (map[string]packageTree, error) {
	trees := map[string]packageTree{importPath: own}
	for _, other := range linked {
		if other == importPath {
			continue
		}
		dir, inModule := r.moduleDirOf(other)
		if !inModule {
			return nil, fmt.Errorf("the test binary of %s links %s, which lies outside the module %s", importPath, other, r.modulePath)
		}
		tree, err := r.readPackageTree(head, dir)
		if err != nil {
			return nil, err
		}
		trees[other] = tree
	}
	return trees, nil
}

// linkedHashes returns the file hashes of every package of trees but the one at importPath, by import path.
// It returns nil for none.
func linkedHashes(trees map[string]packageTree, importPath string) map[string]map[string]string {
	hashes := map[string]map[string]string{}
	for linked, tree := range trees {
		if linked != importPath {
			hashes[linked] = tree.hashes
		}
	}
	if len(hashes) == 0 {
		return nil
	}
	return hashes
}

// holdSlot takes one slot of the hub's pool for the rest of the call, waiting with no bound but the call's context.
func (r *testMapRun) holdSlot() error {
	_, err := r.takeSlot(true)
	return err
}

// takeSlot takes one slot of the hub's pool for the rest of the call, and reports whether the call holds one.
// It takes none when it holds one already or runs inside the slot an enclosing gate run holds.
// With wait set it waits for a slot with no bound but the call's context;
// otherwise it takes one only when one is free at once, and reports false when none is.
// It fills the slot holder's Go flags into the call's environment and creates the call's temporary directory.
func (r *testMapRun) takeSlot(wait bool) (bool, error) {
	if r.holderEnv != nil {
		return true, nil
	}
	limits, err := r.pool.Limits()
	if err != nil {
		return false, fmt.Errorf("read the hub's gate limits: %w", err)
	}
	if inherited := envValue(r.request.environ, gateslot.InheritEnv); inherited == "" || !r.pool.Inherited(inherited) {
		holder := gateslot.Holder{Worktree: r.location.WorktreePath(), Site: testMapSite}
		if wait {
			r.lease, err = r.pool.Acquire(r.ctx, holder)
		} else {
			r.lease, err = r.pool.TryAcquire(holder)
		}
		if err != nil {
			return false, fmt.Errorf("acquire a gate slot: %w", err)
		}
		if r.lease == nil {
			return false, nil
		}
	}
	if r.tmp, err = lymtemp.MkdirTemp("testmap-"); err != nil {
		return false, fmt.Errorf("create the coverage binary directory: %w", err)
	}
	r.parallel = limits.Tier0Parallel
	if r.parallel < 1 {
		r.parallel = runtime.NumCPU()
	}
	r.holderEnv = gateslot.HolderEnv(r.env, r.lease, limits.Tier0Parallel)
	return true, nil
}

// envValue returns the value of the last entry of env named key, or "".
func envValue(env []string, key string) string {
	value := ""
	for _, entry := range env {
		if name, rest, found := strings.Cut(entry, "="); found && name == key {
			value = rest
		}
	}
	return value
}

// buildLym builds `lym` from the worktree into the pool's prebuilt store, once per call, bounded by the build timeout.
// A failed build is kept, so every later package that needs it fails the same way without building again.
// A build ended by a terminating signal is not kept and returns the call context's error.
func (r *testMapRun) buildLym() error {
	if r.lymBuilt {
		return r.lymErr
	}
	r.lymBuilt = true
	ctx, cancel := context.WithTimeout(r.ctx, r.buildTimeout)
	defer cancel()
	// The store keeps the binary, so the cleanup prebuildLym returns for it removes nothing.
	bin, _, err := prebuildLym(ctx, r.request.goBinary, r.location.WorktreePath(), r.parallel, r.pool.PrebuiltDir(), r.request.grace, r.holderEnv)
	if cancelled := r.ctx.Err(); cancelled != nil {
		r.lymBuilt = false
		return cancelled
	}
	switch {
	case err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded):
		r.lymErr = fmt.Errorf("lym build: timed out after %s", r.buildTimeout)
	case err != nil:
		r.lymErr = fmt.Errorf("lym build: %w", err)
	}
	r.lymBin = bin
	return r.lymErr
}

// runTests runs each of funcs alone, in order, and returns their runs.
// The error is the call context's once it is done, since a run it ended records nothing.
func (r *testMapRun) runTests(pkg, pkgDir, bin string, funcs []testcover.TestFunc) ([]mappedRun, error) {
	var runs []mappedRun
	for _, fn := range funcs {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		runs = append(runs, r.runTest(pkg, pkgDir, bin, fn))
	}
	// A signal during the last test ends it as a failure, which must not enter the map.
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

// runTest runs the mapped test fn of pkg alone under the coverage binary bin in the package directory pkgDir.
// It runs with its tier's env and, for a tier with prebuilt_lym, the call's `lym` build.
// The run's outcome is set unless the test passed: a test that could not run counts as failed.
// A test that timed out is also listed on the call's report.
func (r *testMapRun) runTest(pkg, pkgDir, bin string, fn testcover.TestFunc) mappedRun {
	tier := r.tiers[fn.Tier]
	env := slices.Concat(r.holderEnv, renderTierEnv(tier))
	if tier.PrebuiltLym && r.lymBin != "" {
		env = append(env, gateslot.PrebuiltLymEnv+"="+r.lymBin)
	}
	runner := testcover.Toolchain{Env: env, Ctx: r.ctx, TestTimeout: r.testTimeout, Grace: r.request.grace}
	result, err := runner.RunAlone(bin, pkgDir, filepath.Join(r.tmp, "cover.out"), fn.Name)
	run := mappedRun{fn: fn, result: result}
	switch {
	case err != nil:
		logger.Warn("gate testmap: a test that could not run is recorded as failed", "package", pkg, "test", fn.Name, "cause", err)
		run.outcome = testcover.OutcomeFailed
	case result.TimedOut:
		r.timedOut = append(r.timedOut, pkg+"."+fn.Name)
		run.outcome = testcover.OutcomeTimedOut
	case result.Action == testcover.ActionSkip:
		run.outcome = testcover.OutcomeSkipped
	case result.Action != testcover.ActionPass:
		run.outcome = testcover.OutcomeFailed
	}
	return run
}

// firstProfile returns the profile of the first of runs that wrote one, or nil when none did.
// Every profile of one coverage binary holds every block it counts, covered or not.
func firstProfile(runs []mappedRun) []testcover.ProfileBlock {
	for _, run := range runs {
		if run.result.Profile != nil {
			return run.result.Profile
		}
	}
	return nil
}

// runFirstKeptEntry runs alone the first passing entry of m that runs holds no run of and mapped still declares.
// It returns no run when there is none.
// A pass whose own runs wrote no profile calls it to get the fresh build's blocks for the self-check.
func (r *testMapRun) runFirstKeptEntry(pkg, pkgDir, bin string, m testcover.Map, mapped []testcover.TestFunc, runs []mappedRun) ([]mappedRun, error) {
	ran := map[string]bool{}
	for _, run := range runs {
		ran[run.fn.Name] = true
	}
	for _, test := range m.Tests {
		if ran[test.Name] || !test.Measured() {
			continue
		}
		if i := slices.IndexFunc(mapped, func(fn testcover.TestFunc) bool { return fn.Name == test.Name }); i >= 0 {
			return r.runTests(pkg, pkgDir, bin, mapped[i:i+1])
		}
	}
	return nil, nil
}

// packageBlockTable returns the blocks of the package at importPath in profile, each position once.
// The blocks come in profile order, with the index of each position among them.
func packageBlockTable(profile []testcover.ProfileBlock, importPath string) ([]testcover.ProfileBlock, map[string]int) {
	var blocks []testcover.ProfileBlock
	index := map[string]int{}
	for _, block := range testcover.PackageBlocks(profile, importPath) {
		if _, seen := index[block.Position()]; !seen {
			index[block.Position()] = len(blocks)
			blocks = append(blocks, block)
		}
	}
	return blocks, index
}

// selfCheck returns the names of m's passing entries that a fresh build, whose profile is profile, cannot place.
// An entry fails when one of its block keys is absent from the profile's blocks of the package's own files.
// Those blocks are keyed from the package directory pkgDir.
// It also fails when one of its linked functions is absent from the profile.
// The profile's functions are those it holds of the linked packages of indexes.
// The entries ran names are measured afresh anyway, so they are not checked.
func selfCheck(m testcover.Map, pkgDir, importPath string, profile []testcover.ProfileBlock, indexes []*testcover.FuncIndex, ran map[string]bool) ([]string, error) {
	located, _ := packageBlockTable(profile, importPath)
	keys, err := testcover.KeyBlocks(pkgDir, located)
	if err != nil {
		return nil, fmt.Errorf("key the fresh build's blocks: %w", err)
	}
	blocks := map[string]bool{}
	for _, key := range keys {
		blocks[key.String()] = true
	}
	funcs := map[testcover.FuncKey]bool{}
	for _, index := range indexes {
		profiled, err := index.ProfiledFuncs(profile)
		if err != nil {
			return nil, fmt.Errorf("name the fresh build's functions: %w", err)
		}
		for _, key := range profiled {
			funcs[key] = true
		}
	}
	var failing []string
	for _, test := range m.Tests {
		if ran[test.Name] || !test.Measured() {
			continue
		}
		lostBlock := slices.ContainsFunc(test.Blocks, func(i int) bool { return !blocks[m.Blocks[i].String()] })
		lostFunc := slices.ContainsFunc(test.Funcs, func(i int) bool { return !funcs[m.Funcs[i]] })
		if lostBlock || lostFunc {
			failing = append(failing, test.Name)
		}
	}
	return failing, nil
}

// fillMapTests merges runs, the runs of the package at importPath, into m and sets its block key and function tables.
// An entry of m that a run names is replaced by the run's; every other entry is kept.
// The block table holds every block of the package's own files in the first profile a run wrote, keyed from pkgDir;
// m's own table stands when no run wrote one.
// A kept entry is re-indexed by its blocks' keys and its functions.
// Its keys must therefore lie in the fresh tables, which the self-check makes sure of.
// The function table holds the kept entries' functions.
// It also holds those the passing runs cover of each linked package of indexes.
// A passing run that covers a block of the package or a function of a linked package enters with that coverage;
// one that covers neither enters as OutcomeNoCoverage, and any other run with its outcome.
// Every entry a run gives carries commit and at.
func fillMapTests(m *testcover.Map, pkgDir, importPath string, runs []mappedRun, indexes []*testcover.FuncIndex, commit string, at time.Time) error {
	ran := map[string]bool{}
	for _, run := range runs {
		ran[run.fn.Name] = true
	}
	var kept []testcover.MapTest
	for _, test := range m.Tests {
		if !ran[test.Name] {
			kept = append(kept, test)
		}
	}
	blocks := m.Blocks
	var positions map[string]int
	if profile := firstProfile(runs); profile != nil {
		var located []testcover.ProfileBlock
		located, positions = packageBlockTable(profile, importPath)
		keys, err := testcover.KeyBlocks(pkgDir, located)
		if err != nil {
			return fmt.Errorf("key the covered blocks: %w", err)
		}
		blocks = keys
	}
	blockIndex := map[string]int{}
	for i, key := range blocks {
		blockIndex[key.String()] = i
	}

	funcIndex := map[testcover.FuncKey]int{}
	for _, test := range kept {
		for _, i := range test.Funcs {
			funcIndex[m.Funcs[i]] = 0
		}
	}
	runFuncs := make([][]testcover.FuncKey, len(runs))
	for i, run := range runs {
		if run.outcome != "" {
			continue
		}
		for _, linked := range indexes {
			covered, err := linked.CoveredFuncs(run.result.Profile)
			if err != nil {
				return fmt.Errorf("name the covered functions: %w", err)
			}
			runFuncs[i] = append(runFuncs[i], covered...)
		}
		for _, key := range runFuncs[i] {
			funcIndex[key] = 0
		}
	}
	funcs := slices.SortedFunc(maps.Keys(funcIndex), testcover.CompareFuncKeys)
	for i, key := range funcs {
		funcIndex[key] = i
	}

	tests := make([]testcover.MapTest, 0, len(kept)+len(runs))
	for _, test := range kept {
		test.Blocks = reindexed(test.Blocks, func(i int) int { return blockIndex[m.Blocks[i].String()] })
		test.Funcs = reindexed(test.Funcs, func(i int) int { return funcIndex[m.Funcs[i]] })
		tests = append(tests, test)
	}
	for i, run := range runs {
		entry := testcover.MapTest{
			Name: run.fn.Name, Tier: run.fn.Tier, FuncHash: run.fn.Hash, Seconds: run.result.Elapsed,
			Outcome: run.outcome, Commit: commit, MeasuredAt: at,
		}
		if run.outcome == "" {
			for position := range testcover.CoveredPositions(testcover.PackageBlocks(run.result.Profile, importPath)) {
				if block, found := positions[position]; found {
					entry.Blocks = append(entry.Blocks, block)
				}
			}
			for _, key := range runFuncs[i] {
				entry.Funcs = append(entry.Funcs, funcIndex[key])
			}
			slices.Sort(entry.Blocks)
			slices.Sort(entry.Funcs)
			if len(entry.Blocks) == 0 && len(entry.Funcs) == 0 {
				entry.Outcome = testcover.OutcomeNoCoverage
			}
		}
		tests = append(tests, entry)
	}
	slices.SortStableFunc(tests, func(a, b testcover.MapTest) int { return strings.Compare(a.Name, b.Name) })
	m.Blocks, m.Funcs, m.Tests = blocks, funcs, tests
	return nil
}

// reindexed returns indexes mapped through to, sorted, or nil for none.
func reindexed(indexes []int, to func(int) int) []int {
	var mapped []int
	for _, i := range indexes {
		mapped = append(mapped, to(i))
	}
	slices.Sort(mapped)
	return mapped
}

// entryCoverage renders what m's entry for the test name records as comparable text:
// its outcome, its block keys and its linked functions, each sorted; it is "" when m holds no entry for the test.
func entryCoverage(m testcover.Map, name string) string {
	at := slices.IndexFunc(m.Tests, func(test testcover.MapTest) bool { return test.Name == name })
	if at < 0 {
		return ""
	}
	test := m.Tests[at]
	var blocks, funcs []string
	for _, i := range test.Blocks {
		blocks = append(blocks, m.Blocks[i].String())
	}
	for _, i := range test.Funcs {
		key := m.Funcs[i]
		funcs = append(funcs, key.Package+" "+key.File+" "+key.Func)
	}
	slices.Sort(blocks)
	slices.Sort(funcs)
	return strings.Join(slices.Concat([]string{"outcome " + string(test.Outcome)}, blocks, funcs), "\n")
}

// write writes m as the map of pkg and reports pkg built.
func (r *testMapRun) write(pkg string, m testcover.Map) error {
	if err := r.writeMap(pkg, m); err != nil {
		return err
	}
	r.built = append(r.built, pkg)
	return nil
}

// writeFailed records failure as the failed coverage build of pkg at the call's HEAD, and reports pkg failed.
// A readable map of pkg that holds test entries is kept with its commit, entries and marker.
// It records the failure and HEAD beside them.
// Otherwise m is written with its file hashes, no tests and failure as its build failure.
func (r *testMapRun) writeFailed(pkg string, m testcover.Map, failure error) error {
	r.failed = append(r.failed, testMapFailure{Package: pkg, Failure: failure.Error()})
	kept, found, err := testcover.ReadMap(r.mapsDir, pkg)
	if err == nil && found && len(kept.Tests) > 0 {
		kept.Failure = &testcover.MapFailure{Commit: r.head, Error: failure.Error()}
		if err := testcover.WriteMap(r.mapsDir, pkg, kept); err != nil {
			return fmt.Errorf("write the coverage map of %s: %w", pkg, err)
		}
		return nil
	}
	m.BuildFailure = failure.Error()
	m.Blocks, m.Funcs, m.Tests, m.Failure = nil, nil, nil, nil
	return r.writeMap(pkg, m)
}

// writeMap stamps m with the time of the write and its format marker, and writes it as the map of pkg.
// The marker takes the map's commit and time and the current environment stamp.
func (r *testMapRun) writeMap(pkg string, m testcover.Map) error {
	m.BuiltAt = time.Now()
	stamp, err := r.environmentStamp(m.Commit)
	if err != nil {
		return err
	}
	m.Format = &testcover.MapFormat{Version: testcover.MapFormatVersion, Commit: m.Commit, BuiltAt: m.BuiltAt, Environment: stamp}
	if err := testcover.WriteMap(r.mapsDir, pkg, m); err != nil {
		return fmt.Errorf("write the coverage map of %s: %w", pkg, err)
	}
	return nil
}

// readToolchain returns the Go version and platform the call's go command reports, asking it once per call.
func (r *testMapRun) readToolchain() (testcover.EnvironmentStamp, error) {
	if r.toolchain == nil {
		reader := testcover.Toolchain{Go: r.request.goBinary, Env: r.env, Dir: r.moduleDir, Ctx: r.ctx, BuildTimeout: r.buildTimeout, Grace: r.request.grace}
		version, goos, goarch, err := reader.GoEnvironment()
		if err != nil {
			return testcover.EnvironmentStamp{}, fmt.Errorf("read the Go toolchain's environment: %w", err)
		}
		r.toolchain = &testcover.EnvironmentStamp{GoVersion: version, GOOS: goos, GOARCH: goarch}
	}
	return *r.toolchain, nil
}

// environmentStamp returns the environment stamp of a map built from the commit.
func (r *testMapRun) environmentStamp(commit string) (testcover.EnvironmentStamp, error) {
	toolchain, err := r.readToolchain()
	if err != nil {
		return testcover.EnvironmentStamp{}, err
	}
	stamp := stampEnvironment(toolchain, r.tiers, r.hubEnv)
	return testcover.StampModuleFiles(stamp, r.moduleRel, func(rel string) ([]byte, bool, error) {
		return r.fabric.CodeFileAt(commit, rel)
	})
}

// stampEnvironment returns toolchain, a stamp holding the Go version and platform, with its two digests set.
// tiers are the mapped tiers the hub declares, by number, and hubEnv the hub's env.yaml variables.
// The tiers digest covers each tier's command and env as configured, before the `{parallel}` substitution,
// so a changed parallel count leaves it as it was.
// A map build and a coverage scope both stamp through it, so their stamps agree on one hub.
func stampEnvironment(toolchain testcover.EnvironmentStamp, tiers map[int]gateslot.Tier, hubEnv envconfig.Config) testcover.EnvironmentStamp {
	var lines []string
	for _, number := range slices.Sorted(maps.Keys(tiers)) {
		tier := tiers[number]
		lines = append(lines, fmt.Sprintf("tier%d command %s", number, tier.Command))
		for _, key := range slices.Sorted(maps.Keys(tier.Env)) {
			lines = append(lines, fmt.Sprintf("tier%d env %s=%s", number, key, tier.Env[key]))
		}
	}
	toolchain.TiersDigest = testcover.ContentHash([]byte(strings.Join(lines, "\n")))
	toolchain.HubEnvDigest = testcover.ContentHash([]byte(strings.Join(hubEnv.Entries(), "\n")))
	return toolchain
}

// mappedTierConfigs returns each of mappedTiers that cfg declares, by number.
func mappedTierConfigs(cfg gateslot.Config) map[int]gateslot.Tier {
	tiers := map[int]gateslot.Tier{}
	for _, number := range mappedTiers {
		if tier, err := cfg.Tier(number); err == nil {
			tiers[number] = tier
		}
	}
	return tiers
}
