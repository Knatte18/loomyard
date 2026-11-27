// testmappackage.go measures one package in the map run of `lym gate test --map`, inside the call's slots.
// It runs the package's tests under a coverage binary over every in-module package the test binary links.
// Each test runs alone in its own process, tier0 before tier1, each tier up to its count side by side.
// It reads each run's profile once.
// It keys the blocks covered of the package's own files and names the functions covered of the others.
// It merges the runs into the package's map and writes it once.
// It writes nothing for a package when a linked package's files are dirty or HEAD moved.
// It starts the next package's coverage build while the current package's tests run.

package gatecli

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/envconfig"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/testcover"
)

// mappedRun is one mapped test's run alone, with the function it ran, reduced to what the map keeps of it.
type mappedRun struct {
	fn testcover.TestFunc
	// result is the run; its profile is dropped once the run is reduced.
	result testcover.RunResult
	// outcome is why the run records no coverage, empty for a passing run.
	outcome testcover.TestOutcome
	// covered holds the profile positions of the package's own blocks a passing run covered.
	covered testcover.BlockSet
	// funcs are the functions of the linked packages a passing run covered.
	funcs []testcover.FuncKey
}

// blockTable is what a fresh coverage build holds of a package, read from the first profile one of its runs wrote.
// Every profile of one coverage binary holds every block it counts, covered or not.
type blockTable struct {
	// keys are the content keys of the blocks of the package's own files, each position once, in profile order.
	keys []testcover.BlockKey
	// positions maps each of those blocks' profile positions to its index in keys.
	positions map[string]int
}

// packageRun is one job's state once its package's coverage binary is built.
type packageRun struct {
	job                     testMapJob
	pkgDir, importPath, bin string
	trees                   map[string]packageTree
	// indexes name the functions of each linked package but the job's own, by import path.
	indexes map[string]*testcover.FuncIndex
	// m is the package's map as the job started: its hashes and the entries of its spawn-marked tests.
	m testcover.Map
	// table is read from the first profile a run of the job wrote, nil until then.
	table *blockTable
	// measure holds the job's build and run time.
	measure testMapMeasure
}

// coverageBuild is one package's coverage binary build, which runs in the background and is taken up once done.
type coverageBuild struct {
	pkg string
	// bin is the binary the build writes, its own within the call's temporary directory.
	bin string
	// done closes once the build ended; linked, err and seconds are set by then.
	done chan struct{}
	// cancel ends the build early, for one whose binary the call will not use.
	cancel context.CancelFunc
	// linked are the in-module packages the test binary links, the package's own included, sorted;
	// nil when listing them failed.
	linked []string
	// err is the failure of the listing or of the build.
	err error
	// seconds is the build's duration.
	seconds float64
}

// mapPackage measures the job's package whole at the call's HEAD and replaces its map once its tests ran.
// It runs every mapped test but those the spawn scan marks, which enter with their verdict, unrun.
// A package with no mapped test gets a map of its file hashes and no tests, with no build.
// A failed build is recorded through writeFailed.
// A check before a write that finds HEAD moved or a covered file dirty leaves the package unmeasured, with no map.
// A package whose build compiled is checked before its tests run too.
// With the binary ready, it starts the coverage build of the package the walk measures next.
// The error reports a slot, read or write failure that ends the call.
// After a terminating signal it is the call context's error.
func (r *testMapRun) mapPackage(job testMapJob) error {
	pkg, head := job.pkg, r.head
	pkgDir := filepath.Join(r.location.WorktreePath(), filepath.FromSlash(job.dir))
	importPath := r.importPath(pkg)
	own, err := r.readPackageTree(head, job.dir)
	if err != nil {
		return err
	}
	m := testcover.Map{Commit: head, Files: job.files, OtherFiles: own.otherFiles()}
	// A package with no build has no linked set, so its writes check its own files.
	ownTrees := map[string]packageTree{importPath: own}
	mapped, err := r.mappedTests(pkgDir)
	if err != nil {
		return r.writeFailed(pkg, m, err, ownTrees)
	}
	if len(mapped) == 0 {
		return r.write(pkg, m, ownTrees)
	}
	spawns, err := r.scanSpawns(pkgDir)
	if err != nil {
		return r.writeFailed(pkg, m, fmt.Errorf("spawn scan: %w", err), ownTrees)
	}
	var toRun []testcover.TestFunc
	for _, fn := range mapped {
		if verdict := spawns[fn.Name]; verdict.OutOfProcess || verdict.Unresolved {
			m.Tests = append(m.Tests, testcover.MapTest{Name: fn.Name, Tier: fn.Tier, FuncHash: fn.Hash, Spawn: verdict, Commit: head, MeasuredAt: time.Now()})
			continue
		}
		toRun = append(toRun, fn)
	}
	if len(toRun) == 0 {
		return r.write(pkg, m, ownTrees)
	}

	if err := r.holdSlot(); err != nil {
		return err
	}
	build := r.takeBuild(pkg)
	defer os.Remove(build.bin)
	if build.linked == nil {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		return r.writeFailed(pkg, m, build.err, nil)
	}
	trees, err := r.linkedTrees(head, importPath, own, build.linked)
	if err != nil {
		return err
	}
	m.Linked = linkedHashes(trees, importPath)
	if build.err != nil {
		// A signal ends the build for a cause outside the package, so no failure is recorded.
		if err := r.ctx.Err(); err != nil {
			return err
		}
		return r.writeFailed(pkg, m, build.err, trees)
	}
	reason, err := r.checkClean(trees)
	if err != nil {
		return err
	}
	if reason != "" {
		r.skip(pkg, reason)
		return nil
	}
	if r.next.pkg != "" && r.ahead == nil {
		r.ahead = r.startBuild(r.next.pkg)
	}
	p := &packageRun{
		job: job, pkgDir: pkgDir, importPath: importPath, bin: build.bin, trees: trees, m: m,
		indexes: map[string]*testcover.FuncIndex{}, measure: testMapMeasure{Package: pkg, BuildSeconds: build.seconds},
	}
	for _, other := range build.linked {
		if other != importPath {
			p.indexes[other] = testcover.NewFuncIndex(filepath.Join(r.location.WorktreePath(), filepath.FromSlash(trees[other].dir)), other)
		}
	}
	return r.measureTests(p, toRun)
}

// startBuild starts the coverage build of pkg in the background.
// The binary is its own, in the call's temporary directory.
// It lists the in-module packages the test binary links, then builds the binary with a coverpkg naming them all.
// It runs under the slot's environment, bounded by the build timeout, and the call's context or its cancel ends it.
func (r *testMapRun) startBuild(pkg string) *coverageBuild {
	r.builds++
	ctx, cancel := context.WithCancel(r.ctx)
	build := &coverageBuild{
		pkg: pkg, bin: filepath.Join(r.tmp, fmt.Sprintf("pkg-%d.test", r.builds)), done: make(chan struct{}), cancel: cancel,
	}
	importPath := r.importPath(pkg)
	builder := testcover.Toolchain{
		Go: r.request.goBinary, Env: r.holderEnv, Dir: r.moduleDir, Ctx: ctx, BuildTimeout: r.buildTimeout, Grace: r.request.grace,
	}
	go func() {
		defer close(build.done)
		defer cancel()
		linked, err := builder.LinkedPackages(importPath)
		if err != nil {
			build.err = err
			return
		}
		build.linked = slices.Compact(slices.Sorted(slices.Values(append(linked, importPath))))
		start := time.Now()
		build.err = builder.BuildBinary(importPath, strings.Join(build.linked, ","), "", build.bin)
		build.seconds = time.Since(start).Seconds()
	}()
	return build
}

// takeBuild returns the ended coverage build of pkg: the one started ahead for it, or one it starts and waits for.
// A build started ahead for another package, whose job ended before it needed a binary, is cancelled and dropped.
func (r *testMapRun) takeBuild(pkg string) *coverageBuild {
	build := r.ahead
	r.ahead = nil
	if build != nil && build.pkg != pkg {
		build.cancel()
		<-build.done
		os.Remove(build.bin)
		build = nil
	}
	if build == nil {
		build = r.startBuild(pkg)
	}
	<-build.done
	return build
}

// measureTests runs queue, the job's tests, and writes the package's map once they ran.
// A tier's tests all run before the next tier's, so no two tiers' tests run at once.
// A check once the runs end that finds a linked package's file dirty or HEAD moved discards them.
// The package is then left unmeasured.
// The error reports a read or write failure, or a signal, that ends the job.
func (r *testMapRun) measureTests(p *packageRun, queue []testcover.TestFunc) error {
	start := time.Now()
	var runs []mappedRun
	for _, number := range mappedTiers {
		batch := slices.DeleteFunc(slices.Clone(queue), func(fn testcover.TestFunc) bool { return fn.Tier != number })
		batchRuns, err := r.runBatch(p, batch, r.sideBySide[number])
		if err != nil {
			if stop := r.ctx.Err(); stop != nil {
				return stop
			}
			return r.writeFailed(p.job.pkg, p.m, err, p.trees)
		}
		runs = append(runs, batchRuns...)
	}
	p.measure.RunSeconds = time.Since(start).Seconds()
	ended, err := r.endJob(p)
	if err != nil || ended {
		return err
	}
	fillMapTests(&p.m, p.table, runs, r.head, time.Now())
	built := len(r.built)
	if err := r.write(p.job.pkg, p.m, p.trees); err != nil || len(r.built) == built {
		return err
	}
	for _, run := range runs {
		if run.outcome == testcover.OutcomeFailed || run.outcome == testcover.OutcomeTimedOut {
			r.failedTests = append(r.failedTests, p.job.pkg+"."+run.fn.Name)
		}
	}
	p.measure.Tests = len(runs)
	r.addMeasure(p.measure)
	return nil
}

// addMeasure adds m, one package's measurement, to the call's measures.
func (r *testMapRun) addMeasure(m testMapMeasure) {
	r.measures = append(r.measures, m)
}

// runBatch runs each test of queue alone in its own process and returns their runs.
// Up to sideBySide of them run at once.
// The runs come back in the order of queue.
// A test that failed, timed out or was killed beside other tests runs once more alone, and that run stands.
// A test whose run alone passed, and a timed-out test, are listed on the call's report.
// The error is the call context's after a terminating signal, which discards the batch, or the first reduction failure.
func (r *testMapRun) runBatch(p *packageRun, queue []testcover.TestFunc, sideBySide int) ([]mappedRun, error) {
	runs, err := r.runSideBySide(p, queue, sideBySide)
	if err != nil {
		return nil, err
	}
	if len(queue) > 1 && sideBySide > 1 {
		for i, run := range runs {
			if run.outcome != testcover.OutcomeFailed && run.outcome != testcover.OutcomeTimedOut {
				continue
			}
			alone, err := r.runSideBySide(p, queue[i:i+1], sideBySide)
			if err != nil {
				return nil, err
			}
			runs[i] = alone[0]
			if alone[0].outcome == "" {
				r.passedAlone = append(r.passedAlone, p.job.pkg+"."+alone[0].fn.Name)
			}
		}
	}
	for _, run := range runs {
		if run.outcome == testcover.OutcomeTimedOut {
			r.timedOut = append(r.timedOut, p.job.pkg+"."+run.fn.Name)
		}
	}
	return runs, nil
}

// runSideBySide runs each of tests alone in its own process with its own profile.
// At most sideBySide of them run at once.
// It reduces each run as the run ends, so it holds no more profiles at once than runs side by side.
// The runs come back in the order of tests.
// The error is the call context's after a terminating signal, since a run it ended records nothing.
// Otherwise it is the first reduction failure.
func (r *testMapRun) runSideBySide(p *packageRun, tests []testcover.TestFunc, sideBySide int) ([]mappedRun, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	runs := make([]mappedRun, len(tests))
	indexes := make(chan int)
	ended := make(chan int)
	var workers sync.WaitGroup
	for worker := range min(sideBySide, len(tests)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			profile := filepath.Join(r.tmp, fmt.Sprintf("cover-%d.out", worker))
			for i := range indexes {
				runs[i] = r.runTest(p, tests[i], profile)
				ended <- i
			}
		}()
	}
	go func() {
		for i := range tests {
			indexes <- i
		}
		close(indexes)
		workers.Wait()
		close(ended)
	}()
	var reduceErr error
	for i := range ended {
		if reduceErr != nil {
			runs[i].result.Profile = nil
			continue
		}
		reduceErr = p.reduce(&runs[i])
	}
	// A signal during a run ends it as a failure, which must not enter the map.
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	return runs, reduceErr
}

// reduce keeps of run's profile what the map records, and drops the profile.
// It groups the profile's blocks by package once, so each linked package's index reads only its own blocks.
// The job's first profile also sets the job's block table.
// A passing run gets the positions of the package's own blocks it covered and the linked functions it covered.
func (p *packageRun) reduce(run *mappedRun) error {
	profile := run.result.Profile
	run.result.Profile = nil
	if profile == nil {
		return nil
	}
	groups := testcover.ProfileByPackage(profile)
	own := groups[p.importPath]
	if p.table == nil {
		table, err := p.newBlockTable(own)
		if err != nil {
			return err
		}
		p.table = table
	}
	if run.outcome != "" {
		return nil
	}
	run.covered = testcover.CoveredPositions(own)
	for importPath, index := range p.indexes {
		covered, err := index.CoveredFuncs(groups[importPath])
		if err != nil {
			return fmt.Errorf("name the covered functions: %w", err)
		}
		run.funcs = append(run.funcs, covered...)
	}
	return nil
}

// newBlockTable reads the job's block table from one profile.
// own are the profile's blocks of the package's own files, keyed from the package directory.
func (p *packageRun) newBlockTable(own []testcover.ProfileBlock) (*blockTable, error) {
	var located []testcover.ProfileBlock
	positions := map[string]int{}
	for _, block := range own {
		if _, seen := positions[block.Position()]; !seen {
			positions[block.Position()] = len(located)
			located = append(located, block)
		}
	}
	keys, err := testcover.KeyBlocks(p.pkgDir, located)
	if err != nil {
		return nil, fmt.Errorf("key the covered blocks: %w", err)
	}
	return &blockTable{keys: keys, positions: positions}, nil
}

// endJob checks the job's trees once its tests ran.
// It reports whether the check ends the job with the package left unmeasured.
func (r *testMapRun) endJob(p *packageRun) (bool, error) {
	reason, err := r.checkClean(p.trees)
	if err != nil || reason == "" {
		return false, err
	}
	r.skip(p.job.pkg, reason)
	return true, nil
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

// mapSideBySide returns how many of the tier's tests a map run runs side by side, each capped at testMemory bytes.
// It is the tier's parallel as capped clamps it.
// When the tier sets a memory limit, it is also at most the number of such caps that limit holds.
func mapSideBySide(tier gateslot.Tier, testMemory int64, capped func(parallel int, limit int64) int) int {
	sideBySide := capped(tier.Parallel, testMemory)
	if tier.MemoryMB > 0 {
		sideBySide = min(sideBySide, int(tier.MemoryLimit()/testMemory))
	}
	return sideBySide
}

// mapSlotAsk returns how many gate slots a map run over tiers holds: the largest of their asks.
// An ask of 0 stands for every slot, ranks above any count and is returned as 0.
func mapSlotAsk(tiers map[int]gateslot.Tier) int {
	ask := 0
	for _, tier := range tiers {
		if tier.Slots == 0 {
			return 0
		}
		ask = max(ask, tier.Slots)
	}
	return ask
}

// holdSlot takes the call's gate slots for the rest of the call, waiting with no bound but the call's context.
func (r *testMapRun) holdSlot() error {
	return r.takeSlot(mapSlotAsk(r.tiers))
}

// takeSlot takes count slots of the hub's pool, 0 meaning every slot.
// It takes none when the call holds them already or runs inside the slot an enclosing gate run holds.
// It fills the slot holder's Go flags into the call's environment and creates the call's temporary directory once.
func (r *testMapRun) takeSlot(count int) error {
	if r.holderEnv != nil {
		return nil
	}
	limits, err := r.pool.Limits()
	if err != nil {
		return fmt.Errorf("read the hub's gate limits: %w", err)
	}
	if !r.inheritsSlot() {
		r.lease, err = r.pool.AcquireSlots(r.ctx, gateslot.Holder{Worktree: r.location.WorktreePath(), Site: testMapSite}, count)
		if err != nil {
			return fmt.Errorf("acquire the gate slots: %w", err)
		}
	}
	if r.tmp == "" {
		if r.tmp, err = lymtemp.MkdirTemp("testmap-"); err != nil {
			return fmt.Errorf("create the coverage binary directory: %w", err)
		}
	}
	r.holderEnv = gateslot.HolderEnv(r.env, r.lease, limits.Tier0Parallel)
	return nil
}

// inheritsSlot reports whether the call runs inside the slot an enclosing gate run holds, named by gateslot.InheritEnv.
func (r *testMapRun) inheritsSlot() bool {
	inherited := envValue(r.request.environ, gateslot.InheritEnv)
	return inherited != "" && r.pool.Inherited(inherited)
}

// releaseHeldSlot releases the slots the call holds, if any.
func (r *testMapRun) releaseHeldSlot() {
	if r.lease == nil {
		return
	}
	releaseSlot(r.lease, r.location.WorktreePath())
	r.lease, r.holderEnv = nil, nil
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

// runTest runs the mapped test fn of the job's package alone under its coverage binary, writing the profile to profile.
// It runs in the package directory with its tier's env.
// The run's outcome is set unless the test passed: a test that could not run counts as failed.
func (r *testMapRun) runTest(p *packageRun, fn testcover.TestFunc, profile string) mappedRun {
	env := slices.Concat(r.holderEnv, renderTierEnv(r.tiers[fn.Tier]))
	runner := testcover.Toolchain{
		Env: env, Ctx: r.ctx, TestTimeout: r.testTimeout, MemoryLimit: r.testMemory, GuardKilled: r.guardKilled, Grace: r.request.grace,
	}
	result, err := runner.RunAlone(p.bin, p.pkgDir, profile, fn.Name)
	run := mappedRun{fn: fn, result: result}
	switch {
	case err != nil:
		logger.Warn("gate testmap: a test that could not run is recorded as failed", "package", p.job.pkg, "test", fn.Name, "cause", err)
		run.outcome = testcover.OutcomeFailed
	case result.GuardKilled:
		logger.Warn("gate testmap: a test the memory guard killed is recorded as failed", "package", p.job.pkg, "test", fn.Name)
		run.outcome = testcover.OutcomeFailed
	case result.TimedOut:
		run.outcome = testcover.OutcomeTimedOut
	case result.Action == testcover.ActionSkip:
		run.outcome = testcover.OutcomeSkipped
	case result.Action != testcover.ActionPass:
		run.outcome = testcover.OutcomeFailed
	}
	return run
}

// ranNames returns the names of the tests runs ran.
func ranNames(runs []mappedRun) map[string]bool {
	ran := map[string]bool{}
	for _, run := range runs {
		ran[run.fn.Name] = true
	}
	return ran
}

// fillMapTests merges runs, the job's reduced runs, into m and sets its block key and function tables.
// An entry of m that a run names is replaced by the run's; every other entry is kept.
// The block table is table's keys, the fresh build's blocks of the package's own files;
// m's own table stands when table is nil, since no run wrote a profile.
// A kept entry is re-indexed by its blocks' keys and its functions,
// so its keys must lie in the fresh tables.
// The function table holds the kept entries' functions and those the passing runs cover.
// A passing run that covers a block of the package or a function of a linked package enters with that coverage;
// one that covers neither enters as OutcomeNoCoverage, and any other run with its outcome.
// Every entry a run gives carries commit and at.
func fillMapTests(m *testcover.Map, table *blockTable, runs []mappedRun, commit string, at time.Time) {
	ran := ranNames(runs)
	var kept []testcover.MapTest
	for _, test := range m.Tests {
		if !ran[test.Name] {
			kept = append(kept, test)
		}
	}
	blocks := m.Blocks
	if table != nil {
		blocks = table.keys
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
	for _, run := range runs {
		for _, key := range run.funcs {
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
	for _, run := range runs {
		entry := testcover.MapTest{
			Name: run.fn.Name, Tier: run.fn.Tier, FuncHash: run.fn.Hash, Seconds: run.result.Elapsed,
			Outcome: run.outcome, Commit: commit, MeasuredAt: at,
		}
		if run.outcome == "" {
			for position := range run.covered {
				if block, found := table.positions[position]; found {
					entry.Blocks = append(entry.Blocks, block)
				}
			}
			for _, key := range run.funcs {
				entry.Funcs = append(entry.Funcs, funcIndex[key])
			}
			slices.Sort(entry.Blocks)
			entry.Funcs = slices.Compact(slices.Sorted(slices.Values(entry.Funcs)))
			if len(entry.Blocks) == 0 && len(entry.Funcs) == 0 {
				entry.Outcome = testcover.OutcomeNoCoverage
			}
		}
		tests = append(tests, entry)
	}
	slices.SortStableFunc(tests, func(a, b testcover.MapTest) int { return strings.Compare(a.Name, b.Name) })
	m.Blocks, m.Funcs, m.Tests = blocks, funcs, tests
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

// write writes m as the map of pkg and reports pkg built.
// It first checks HEAD and the files of trees, a nil trees standing for the whole module;
// a failed check writes nothing and leaves pkg unmeasured.
func (r *testMapRun) write(pkg string, m testcover.Map, trees map[string]packageTree) error {
	reason, err := r.checkClean(trees)
	if err != nil {
		return err
	}
	if reason != "" {
		r.skip(pkg, reason)
		return nil
	}
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
// It first checks HEAD and the files of trees, a nil trees standing for the whole module;
// a failed check records nothing and leaves pkg unmeasured.
func (r *testMapRun) writeFailed(pkg string, m testcover.Map, failure error, trees map[string]packageTree) error {
	reason, err := r.checkClean(trees)
	if err != nil {
		return err
	}
	if reason != "" {
		r.skip(pkg, reason)
		return nil
	}
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
