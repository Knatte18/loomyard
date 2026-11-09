// testmappackage.go builds one package's coverage map for `lym gate testmap`.
// It runs each of the package's mapped tests alone under a coverage binary inside the call's slot.
// It keys the blocks they cover, and discards the map when the package's files changed under its tests.

package gatecli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

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
// A package whose Go files differ at HEAD once its tests ended gets no map and is reported skipped.
// The error reports a slot, read or write failure, or a cancelled call, that ends the call.
func (r *testMapRun) mapPackage(pkg, dir, head string, before map[string]string) error {
	pkgDir := filepath.Join(r.location.WorktreePath(), filepath.FromSlash(dir))
	importPath := r.importPath(pkg)
	m := testcover.Map{Commit: head, Files: before}
	funcs, err := testcover.ScanTestFuncs(pkgDir)
	if err != nil {
		return r.writeFailed(pkg, m, err)
	}
	var mapped []testcover.TestFunc
	for _, fn := range funcs {
		if _, declared := r.tiers[fn.Tier]; fn.Tiered && declared {
			mapped = append(mapped, fn)
		}
	}
	if len(mapped) == 0 {
		return r.write(pkg, m)
	}

	if err := r.holdSlot(); err != nil {
		return err
	}
	bin := filepath.Join(r.tmp, "pkg.test")
	defer os.Remove(bin)
	builder := testcover.Toolchain{
		Go: r.request.goBinary, Env: r.holderEnv, Dir: r.moduleDir, Ctx: r.ctx, BuildTimeout: r.buildTimeout, Grace: r.request.grace,
	}
	if err := builder.BuildBinary(importPath, importPath, "", bin); err != nil {
		// A terminating signal ends the build for a cause outside the package, so no failure map records it.
		if cancelled := r.ctx.Err(); cancelled != nil {
			return cancelled
		}
		return r.writeFailed(pkg, m, err)
	}
	if slices.ContainsFunc(mapped, func(fn testcover.TestFunc) bool { return r.tiers[fn.Tier].PrebuiltLym }) {
		if err := r.buildLym(); err != nil {
			if cancelled := r.ctx.Err(); cancelled != nil {
				return cancelled
			}
			return r.writeFailed(pkg, m, err)
		}
	}
	var passed []mappedRun
	for _, fn := range mapped {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		if result, ok := r.runTest(pkg, pkgDir, bin, fn); ok {
			passed = append(passed, mappedRun{fn: fn, result: result})
		}
	}
	// A signal during the last test leaves that test out of passed, so the map would miss it.
	if err := r.ctx.Err(); err != nil {
		return err
	}

	changed, err := r.changedFiles(dir, before)
	if err != nil {
		return err
	}
	if len(changed) > 0 {
		r.skip(pkg, "files changed under its tests: "+strings.Join(changed, ", "))
		return nil
	}
	if err := fillMapTests(&m, pkgDir, importPath, passed); err != nil {
		return r.writeFailed(pkg, m, err)
	}
	return r.write(pkg, m)
}

// importPath returns the import path of the module-relative package directory pkg.
func (r *testMapRun) importPath(pkg string) string {
	if pkg == "." {
		return r.modulePath
	}
	return r.modulePath + "/" + pkg
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

// changedFiles returns the sorted names of the Go files whose hash at the current HEAD differs from before.
// The files are those of the worktree-relative directory dir.
// A file added or removed since counts as changed.
func (r *testMapRun) changedFiles(dir string, before map[string]string) ([]string, error) {
	head, err := r.fabric.HeadSHA()
	if err != nil {
		return nil, fmt.Errorf("read HEAD: %w", err)
	}
	after, err := r.fileHashes(head, dir)
	if err != nil {
		return nil, err
	}
	var changed []string
	for name, hash := range before {
		if after[name] != hash {
			changed = append(changed, name)
		}
	}
	for name := range after {
		if _, found := before[name]; !found {
			changed = append(changed, name)
		}
	}
	slices.Sort(changed)
	return changed, nil
}

// fillMapTests fills m's block key table from the blocks of the package at importPath.
// It reads the blocks from the package directory pkgDir, and adds one test per run that covers one of them.
// Every run's profile holds every block of the package, covered or not, so the first run's profile gives the table.
func fillMapTests(m *testcover.Map, pkgDir, importPath string, runs []mappedRun) error {
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
	for _, run := range runs {
		var covered []int
		for position := range testcover.CoveredPositions(testcover.PackageBlocks(run.result.Profile, importPath)) {
			if i, found := index[position]; found {
				covered = append(covered, i)
			}
		}
		if len(covered) == 0 {
			continue
		}
		slices.Sort(covered)
		m.Tests = append(m.Tests, testcover.MapTest{Name: run.fn.Name, Tier: run.fn.Tier, FuncHash: run.fn.Hash, Seconds: run.result.Elapsed, Blocks: covered})
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
	m.Blocks, m.Tests = nil, nil
	if err := r.writeMap(pkg, m); err != nil {
		return err
	}
	r.failed = append(r.failed, testMapFailure{Package: pkg, Failure: m.BuildFailure})
	return nil
}

// writeMap stamps m with the build time and writes it as the map of pkg.
func (r *testMapRun) writeMap(pkg string, m testcover.Map) error {
	m.BuiltAt = time.Now()
	if err := testcover.WriteMap(r.mapsDir, pkg, m); err != nil {
		return fmt.Errorf("write the coverage map of %s: %w", pkg, err)
	}
	return nil
}
