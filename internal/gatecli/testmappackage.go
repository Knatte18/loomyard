// testmappackage.go builds one package's coverage map for `lym gate testmap`.
// It runs each of the package's mapped tests alone, inside the call's slot.
// The tests run under a coverage binary over every in-module package the test binary links.
// It keys the blocks they cover of the package's own files and names the functions they cover of the others.
// It discards the map when a linked package's files are dirty or changed under its tests.

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

// mappedRun is one mapped test that passed alone, with the function it ran.
type mappedRun struct {
	fn     testcover.TestFunc
	result testcover.RunResult
}

// mapPackage builds and writes the coverage map of pkg, whose worktree-relative directory is dir.
// It builds from the commit head, whose Go files hash to before.
// A package with no mapped test gets a map of its file hashes and no tests, with no build;
// one whose build failed gets the same, naming the failure.
// A package gets no map and is reported skipped when a linked package's file is dirty just before its build;
// the first result is then true, since no build was spent on the package.
// It is skipped the same way, with the first result false, when such a file is dirty after its tests.
// It is also skipped when such a file differs at HEAD once they ended.
// The error reports a slot, read or write failure, or a cancelled call, that ends the call.
func (r *testMapRun) mapPackage(pkg, dir, head string, before map[string]string) (bool, error) {
	pkgDir := filepath.Join(r.location.WorktreePath(), filepath.FromSlash(dir))
	importPath := r.importPath(pkg)
	own, err := r.readPackageTree(head, dir)
	if err != nil {
		return false, err
	}
	m := testcover.Map{Commit: head, Files: before, OtherFiles: own.otherFiles()}
	funcs, err := testcover.ScanTestFuncs(pkgDir)
	if err != nil {
		return false, r.writeFailed(pkg, m, err)
	}
	var mapped []testcover.TestFunc
	for _, fn := range funcs {
		if _, declared := r.tiers[fn.Tier]; fn.Tiered && declared {
			mapped = append(mapped, fn)
		}
	}
	if len(mapped) == 0 {
		return false, r.write(pkg, m)
	}
	spawns, err := testcover.ScanSpawns(r.modulePath, filepath.Join(r.moduleDir, filepath.FromSlash(testKitDir)), pkgDir)
	if err != nil {
		return false, r.writeFailed(pkg, m, fmt.Errorf("spawn scan: %w", err))
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
	if err := builder.BuildBinary(importPath, strings.Join(linked, ","), "", bin); err != nil {
		// A terminating signal ends the build for a cause outside the package, so no failure map records it.
		if cancelled := r.ctx.Err(); cancelled != nil {
			return false, cancelled
		}
		return false, r.writeFailed(pkg, m, err)
	}
	if slices.ContainsFunc(mapped, func(fn testcover.TestFunc) bool { return r.tiers[fn.Tier].PrebuiltLym }) {
		if err := r.buildLym(); err != nil {
			if cancelled := r.ctx.Err(); cancelled != nil {
				return false, cancelled
			}
			return false, r.writeFailed(pkg, m, err)
		}
	}
	var passed []mappedRun
	for _, fn := range mapped {
		if err := r.ctx.Err(); err != nil {
			return false, err
		}
		if result, ok := r.runTest(pkg, pkgDir, bin, fn); ok {
			passed = append(passed, mappedRun{fn: fn, result: result})
		}
	}
	// A signal during the last test leaves that test out of passed, so the map would miss it.
	if err := r.ctx.Err(); err != nil {
		return false, err
	}

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
	var indexes []*testcover.FuncIndex
	for _, other := range linked {
		if other != importPath {
			indexes = append(indexes, testcover.NewFuncIndex(filepath.Join(r.location.WorktreePath(), filepath.FromSlash(trees[other].dir)), other))
		}
	}
	if err := fillMapTests(&m, pkgDir, importPath, passed, spawns, indexes); err != nil {
		return false, r.writeFailed(pkg, m, err)
	}
	return false, r.write(pkg, m)
}

// importPath returns the import path of the module-relative package directory pkg.
func (r *testMapRun) importPath(pkg string) string {
	if pkg == "." {
		return r.modulePath
	}
	return r.modulePath + "/" + pkg
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
		rel, inModule := strings.CutPrefix(other, r.modulePath+"/")
		if other == r.modulePath {
			rel, inModule = ".", true
		}
		if !inModule {
			return nil, fmt.Errorf("the test binary of %s links %s, which lies outside the module %s", importPath, other, r.modulePath)
		}
		tree, err := r.readPackageTree(head, path.Join(r.moduleRel, rel))
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

// holdSlot takes one slot of the hub's pool for the rest of the call.
// It takes none when it holds one already or runs inside the slot an enclosing gate run holds.
// It fills the slot holder's Go flags into the call's environment and creates the call's temporary directory.
// Its wait has no bound but the call's context.
func (r *testMapRun) holdSlot() error {
	if r.holderEnv != nil {
		return nil
	}
	limits, err := r.pool.Limits()
	if err != nil {
		return fmt.Errorf("read the hub's gate limits: %w", err)
	}
	if inherited := envValue(r.request.environ, gateslot.InheritEnv); inherited == "" || !r.pool.Inherited(inherited) {
		if r.lease, err = r.pool.Acquire(r.ctx, gateslot.Holder{Worktree: r.location.WorktreePath(), Site: testMapSite}); err != nil {
			return fmt.Errorf("acquire a gate slot: %w", err)
		}
	}
	if r.tmp, err = lymtemp.MkdirTemp("testmap-"); err != nil {
		return fmt.Errorf("create the coverage binary directory: %w", err)
	}
	r.parallel = limits.Tier0Parallel
	if r.parallel < 1 {
		r.parallel = runtime.NumCPU()
	}
	r.holderEnv = gateslot.HolderEnv(r.env, r.lease, limits.Tier0Parallel)
	return nil
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

// runTest runs the mapped test fn of pkg alone under the coverage binary bin in the package directory pkgDir.
// It runs with its tier's env and, for a tier with prebuilt_lym, the call's `lym` build.
// It returns the result and true for a test that passed.
// A test that timed out is listed on the call's report; one that failed, skipped or wrote no profile is left out.
func (r *testMapRun) runTest(pkg, pkgDir, bin string, fn testcover.TestFunc) (testcover.RunResult, bool) {
	tier := r.tiers[fn.Tier]
	env := slices.Concat(r.holderEnv, renderTierEnv(tier))
	if tier.PrebuiltLym && r.lymBin != "" {
		env = append(env, gateslot.PrebuiltLymEnv+"="+r.lymBin)
	}
	runner := testcover.Toolchain{Env: env, Ctx: r.ctx, TestTimeout: r.testTimeout, Grace: r.request.grace}
	result, err := runner.RunAlone(bin, pkgDir, filepath.Join(r.tmp, "cover.out"), fn.Name)
	switch {
	case err != nil:
		logger.Warn("gate testmap: a test is left out of the map", "package", pkg, "test", fn.Name, "cause", err)
		return result, false
	case result.TimedOut:
		r.timedOut = append(r.timedOut, pkg+"."+fn.Name)
		return result, false
	}
	return result, result.Action == testcover.ActionPass
}

// fillMapTests fills m's block key table from the blocks of the package at importPath, and its function table.
// It reads the blocks from the package directory pkgDir.
// It reads each linked package's functions through its index in indexes.
// It adds one test per run that covers a block of the package or a function of a linked package.
// Each test carries its verdict in spawns.
// Every run's profile holds every block of the package, covered or not, so the first run's profile gives the block table.
func fillMapTests(m *testcover.Map, pkgDir, importPath string, runs []mappedRun, spawns map[string]testcover.SpawnVerdict, indexes []*testcover.FuncIndex) error {
	if len(runs) == 0 {
		return nil
	}
	var blocks []testcover.ProfileBlock
	index := map[string]int{}
	for _, block := range testcover.PackageBlocks(runs[0].result.Profile, importPath) {
		if _, seen := index[block.Position()]; !seen {
			index[block.Position()] = len(blocks)
			blocks = append(blocks, block)
		}
	}
	keys, err := testcover.KeyBlocks(pkgDir, blocks)
	if err != nil {
		return fmt.Errorf("key the covered blocks: %w", err)
	}
	m.Blocks = keys

	runFuncs := make([][]testcover.FuncKey, len(runs))
	table := map[testcover.FuncKey]int{}
	for i, run := range runs {
		for _, linked := range indexes {
			covered, err := linked.CoveredFuncs(run.result.Profile)
			if err != nil {
				return fmt.Errorf("name the covered functions: %w", err)
			}
			runFuncs[i] = append(runFuncs[i], covered...)
		}
		for _, key := range runFuncs[i] {
			table[key] = 0
		}
	}
	m.Funcs = slices.SortedFunc(maps.Keys(table), testcover.CompareFuncKeys)
	for i, key := range m.Funcs {
		table[key] = i
	}

	for i, run := range runs {
		var covered []int
		for position := range testcover.CoveredPositions(testcover.PackageBlocks(run.result.Profile, importPath)) {
			if block, found := index[position]; found {
				covered = append(covered, block)
			}
		}
		var funcs []int
		for _, key := range runFuncs[i] {
			funcs = append(funcs, table[key])
		}
		if len(covered) == 0 && len(funcs) == 0 {
			continue
		}
		slices.Sort(covered)
		slices.Sort(funcs)
		m.Tests = append(m.Tests, testcover.MapTest{
			Name: run.fn.Name, Tier: run.fn.Tier, FuncHash: run.fn.Hash, Seconds: run.result.Elapsed,
			Blocks: covered, Funcs: funcs, Spawn: spawns[run.fn.Name],
		})
	}
	return nil
}

// write stamps m with the build time, writes it as the map of pkg and reports pkg built.
func (r *testMapRun) write(pkg string, m testcover.Map) error {
	if err := r.writeMap(pkg, m); err != nil {
		return err
	}
	r.built = append(r.built, pkg)
	return nil
}

// writeFailed writes the map of pkg with no tests and failure as its build failure, and reports pkg failed.
func (r *testMapRun) writeFailed(pkg string, m testcover.Map, failure error) error {
	m.BuildFailure = failure.Error()
	m.Blocks, m.Funcs, m.Tests = nil, nil, nil
	if err := r.writeMap(pkg, m); err != nil {
		return err
	}
	r.failed = append(r.failed, testMapFailure{Package: pkg, Failure: m.BuildFailure})
	return nil
}

// writeMap stamps m with the build time and its format marker, and writes it as the map of pkg.
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

// environmentStamp returns the environment stamp of a map built from the commit.
// It asks the go command for the toolchain's version and platform once per call.
func (r *testMapRun) environmentStamp(commit string) (testcover.EnvironmentStamp, error) {
	if r.toolchain == nil {
		reader := testcover.Toolchain{Go: r.request.goBinary, Env: r.env, Dir: r.moduleDir, Ctx: r.ctx, BuildTimeout: r.buildTimeout, Grace: r.request.grace}
		version, goos, goarch, err := reader.GoEnvironment()
		if err != nil {
			return testcover.EnvironmentStamp{}, fmt.Errorf("read the Go toolchain's environment: %w", err)
		}
		r.toolchain = &testcover.EnvironmentStamp{GoVersion: version, GOOS: goos, GOARCH: goarch}
	}
	stamp := stampEnvironment(*r.toolchain, r.tiers, r.hubEnv)
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
