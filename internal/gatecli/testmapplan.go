// testmapplan.go plans `lym gate testmap`'s packages at the prime's HEAD, building nothing and running no test.
// It asks the coverage selector what changed since each map's commit.
// It writes what still holds at HEAD, and queues the tests and packages left to measure.

package gatecli

import (
	"fmt"
	"path"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/testcover"
)

// planPackage plans the package pkg at the call's HEAD, and returns its job and whether it queued one.
// A package with an uncommitted or untracked Go file is skipped.
// So is a named package that tracks no Go file at HEAD.
// A named package is queued for a whole rebuild.
// Any other is planned under its build lock; one whose lock another call holds is skipped and counted as queued.
// A package whose map records a failed build at HEAD queues nothing.
// A selector miss queues a whole rebuild and leaves the map as it is.
// Otherwise the plan writes the map at HEAD when it changed, and queues the declared tests it holds no entry for.
// A test or spawn scan that fails queues a whole rebuild, which records the failure.
// The error reports a read or write failure that ends the call.
func (r *testMapRun) planPackage(pkg string, named bool) (testMapJob, bool, error) {
	dir := path.Join(r.moduleRel, pkg)
	if file := r.dirtyGoFile(dir); file != "" {
		r.skip(pkg, "modified or untracked file "+file)
		return testMapJob{}, false, nil
	}
	files, err := r.fileHashes(r.head, dir)
	if err != nil {
		return testMapJob{}, false, err
	}
	if len(files) == 0 {
		if named {
			r.skip(pkg, "no Go file tracked at HEAD")
		}
		return testMapJob{}, false, nil
	}
	whole := testMapJob{pkg: pkg, dir: dir, files: files, whole: true}
	if named {
		return whole, true, nil
	}
	buildLock, held, err := r.lockPackage(pkg)
	if err != nil {
		return testMapJob{}, false, err
	}
	if !held {
		r.skip(pkg, "another call holds its map build lock")
		r.queued++
		return testMapJob{}, false, nil
	}
	defer releaseBuildLock(buildLock, pkg)

	m, found, err := testcover.ReadMap(r.mapsDir, pkg)
	if err == nil && found && m.FailedAt(r.head) {
		return testMapJob{}, false, nil
	}
	selector, err := r.headSelector()
	if err != nil {
		return testMapJob{}, false, err
	}
	selection, err := selector.Select(pkg)
	if err != nil {
		return testMapJob{}, false, fmt.Errorf("select the tests of %s at HEAD: %w", pkg, err)
	}
	pkgDir := filepath.Join(r.location.WorktreePath(), filepath.FromSlash(dir))
	declared, err := r.mappedTests(pkgDir)
	if err != nil {
		return whole, true, nil
	}
	tree, err := r.headTree(dir, files, m)
	if err != nil {
		return testMapJob{}, false, err
	}
	plan, err := testcover.PlanUpkeep(testcover.UpkeepInput{
		Map: m, Selection: selection, Commit: r.head, At: time.Now(), Tree: tree, Declared: declared,
		Spawns: func() (map[string]testcover.SpawnVerdict, error) { return r.scanSpawns(pkgDir) },
	})
	if err != nil {
		return whole, true, nil
	}
	switch plan.Verdict {
	case testcover.UpkeepMiss:
		if r.plan.Misses == nil {
			r.plan.Misses = map[testcover.MissReason]int{}
		}
		r.plan.Misses[plan.Miss.Reason]++
		return whole, true, nil
	case testcover.UpkeepAffected:
		r.plan.Affected++
	default:
		r.plan.Current++
	}
	if plan.Changed {
		if err := r.writeMap(pkg, plan.Map); err != nil {
			return testMapJob{}, false, err
		}
	}
	if len(plan.Queue) == 0 {
		return testMapJob{}, false, nil
	}
	return testMapJob{pkg: pkg, dir: dir, files: files, tests: plan.Queue}, true, nil
}

// headSelector returns the selector that compares the maps with the call's HEAD, opening it at its first call.
// Its environment stamp is built as a map build stamps one,
// and it reads the toolchain's root under the call's environment.
func (r *testMapRun) headSelector() (*testcover.Selector, error) {
	if r.selector != nil {
		return r.selector, nil
	}
	toolchain, err := r.readToolchain()
	if err != nil {
		return nil, err
	}
	reader := testcover.Toolchain{Go: r.request.goBinary, Env: r.env, Dir: r.moduleDir, Ctx: r.ctx, BuildTimeout: r.buildTimeout, Grace: r.request.grace}
	goRoot, err := reader.GoRoot()
	if err != nil {
		return nil, fmt.Errorf("read the Go toolchain's root: %w", err)
	}
	files, err := r.fabric.CodeTreeAt(r.head, ".")
	if err != nil {
		return nil, err
	}
	r.selector = testcover.NewSelector(testcover.SelectorConfig{
		MapsDir:      r.mapsDir,
		WorktreeRoot: r.location.WorktreePath(),
		ModuleRel:    r.moduleRel,
		ModulePath:   r.modulePath,
		GoRoot:       goRoot,
		Commits:      r.fabric,
		Inert:        r.inert,
		Environment:  stampEnvironment(toolchain, r.tiers, r.hubEnv),
	}, testcover.Target{Commit: r.head, Files: files})
	return r.selector, nil
}

// headTree returns the file hashes at the call's HEAD of the package in the worktree-relative slash directory dir.
// Its Go files hash to files there.
// The result also holds the file hashes of each package m links.
func (r *testMapRun) headTree(dir string, files map[string]string, m testcover.Map) (testcover.UpkeepTree, error) {
	own, err := r.readPackageTree(r.head, dir)
	if err != nil {
		return testcover.UpkeepTree{}, err
	}
	tree := testcover.UpkeepTree{Files: files, OtherFiles: own.otherFiles()}
	for importPath := range m.Linked {
		linkedDir, _ := r.moduleDirOf(importPath)
		linked, err := r.readPackageTree(r.head, linkedDir)
		if err != nil {
			return testcover.UpkeepTree{}, err
		}
		if tree.Linked == nil {
			tree.Linked = map[string]map[string]string{}
		}
		tree.Linked[importPath] = linked.hashes
	}
	return tree, nil
}

// mappedTests returns the Test functions of the package directory pkgDir.
// It keeps those whose tiergate.Require names a mapped tier the hub declares.
func (r *testMapRun) mappedTests(pkgDir string) ([]testcover.TestFunc, error) {
	funcs, err := testcover.ScanTestFuncs(pkgDir)
	if err != nil {
		return nil, err
	}
	var mapped []testcover.TestFunc
	for _, fn := range funcs {
		if _, declared := r.tiers[fn.Tier]; fn.Tiered && declared {
			mapped = append(mapped, fn)
		}
	}
	return mapped, nil
}

// scanSpawns returns the spawn scan's verdict on each test of the package directory pkgDir.
func (r *testMapRun) scanSpawns(pkgDir string) (map[string]testcover.SpawnVerdict, error) {
	return testcover.ScanSpawns(r.modulePath, filepath.Join(r.moduleDir, filepath.FromSlash(testKitDir)), pkgDir)
}
