// testmap.go holds the map run of `lym gate test --map`.
// It builds the per-package coverage maps at the HEAD of the worktree it runs in, over the packages of the call's scope.
// It picks the worktree's maps directory, then measures each package whole.
// It prints what it measured, skipped, failed and left out.

package gatecli

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/envconfig"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/testcover"
)

// testMapSite labels the slot holder record of a map run.
const testMapSite = "lym gate test --map"

// mappedTiers are the tiers whose tests a coverage map holds, lowest first.
var mappedTiers = []int{0, 1}

// testMapRequest is one map run after flag parsing.
type testMapRequest struct {
	// scope is the call, whose scope flags pick the packages.
	scope testRequest
	// goBinary is the go command the builds run.
	goBinary string
	// environ is the environment the call's children start from: the process's own in production.
	environ []string
	// secondUnit is the length of one `redundancy_test_timeout_sec` second.
	// minuteUnit is the length of one `redundancy_build_timeout_min` minute.
	secondUnit, minuteUnit time.Duration
	// grace bounds the wait for a timed-out build's or test's exit and output once its process group is killed.
	grace time.Duration
	// cappedParallel returns how many of parallel test runs, each capped at limit bytes of memory, go side by side.
	cappedParallel func(parallel int, limit int64) int
	// stderr receives the memory guard's line for each test binary it kills; nil prints none.
	stderr io.Writer
}

// testMapSkip is a package the call left without a new map, with the cause.
type testMapSkip struct {
	Package string `json:"package"`
	Cause   string `json:"cause"`
}

// testMapFailure is a package whose coverage build failed, with the failure.
type testMapFailure struct {
	Package string `json:"package"`
	Failure string `json:"failure"`
}

// testMapMeasure is the time one package's measurement took.
type testMapMeasure struct {
	Package string `json:"package"`
	// BuildSeconds is the coverage build's duration.
	BuildSeconds float64 `json:"build_seconds"`
	// RunSeconds is the duration of the package's test runs together.
	RunSeconds float64 `json:"run_seconds"`
	// Tests counts the test runs.
	Tests int `json:"tests"`
}

// testMapJob is one measurement of a package.
type testMapJob struct {
	pkg string
	// dir is the package's worktree-relative slash directory.
	dir string
	// files are the hashes of the Go files dir tracks at the call's HEAD, by name.
	files map[string]string
}

// testMapRun is one map run's state as it walks its packages.
type testMapRun struct {
	ctx      context.Context
	request  testMapRequest
	location *lymcwd.Location
	fabric   *fabricengine.Fabric
	pool     *gateslot.Pool
	// cfg is the hub's gate config.
	cfg gateslot.Config
	// tiers holds each mapped tier the hub declares, by number.
	tiers map[int]gateslot.Tier
	// mapsDir is the coverage map directory the call reads and writes.
	mapsDir string
	// moduleDir is the module root, moduleRel its worktree-relative slash path and modulePath its import path.
	moduleDir, moduleRel, modulePath string
	// buildTimeout bounds each build, and testTimeout each test.
	buildTimeout, testTimeout time.Duration
	// testMemory caps the bytes each test run may hold.
	testMemory int64
	// memoryQuit is the bytes a test binary's tree may hold before the memory guard kills it.
	memoryQuit int64
	// guardKilled reports whether the memory guard killed the process with a pid; nil means it killed none.
	guardKilled func(pid int) bool
	// env is the environment the call's children start from, before the slot's Go flags.
	env []string
	// hubEnv is the hub's env.yaml variables, which env holds.
	hubEnv envconfig.Config

	// The fields below fill as the call goes.

	// head is the worktree's HEAD the call measures at, read once before its walk.
	head string
	// headMoved is set once a later read of HEAD differs from head, which ends the call.
	headMoved bool
	// lease is the held slot; it stays nil before the first build and inside an enclosing gate run's slot.
	lease *gateslot.Lease
	// holderEnv is env with the slot's Go flags, nil until the call holds its slot.
	holderEnv []string
	// parallel is the `-p` of the `lym` build, read with the slot.
	parallel int
	// sideBySide is the most tests run at once.
	// It is parallel, or fewer when the memory limit leaves less than their caps together.
	sideBySide int
	// tmp holds the coverage binaries and profiles, empty until the call holds its slot.
	tmp string
	// next is the job the walk measures after the current one, whose binary the current job builds ahead;
	// its pkg is empty when none follows.
	next testMapJob
	// ahead is the coverage build started for next, nil when none runs or waits to be taken up.
	ahead *coverageBuild
	// builds counts the coverage builds the call started, so each writes its own binary.
	builds int
	// lymBuilt is set once the call built `lym`; lymBin is the binary, and lymErr the build's failure.
	lymBuilt bool
	lymBin   string
	lymErr   error
	// toolchain holds the Go version and platform of the maps' environment stamp, nil until the call reads them.
	toolchain *testcover.EnvironmentStamp

	built    []string
	skipped  []testMapSkip
	failed   []testMapFailure
	timedOut []string
	// leftOut holds the packages the command line names that the call does not map, each with the reason.
	leftOut []testMapSkip
	// queued counts the packages a HEAD move, an error or a terminating signal left unmeasured.
	queued int
	// measures holds the build and run time of each package the call built.
	measures []testMapMeasure
}

// runTestMap runs one map run and returns its exit code.
// The code is 0 once it walked its packages.
// It is 1 for a refusal or a failure that ends the call.
// It is 128 plus the signal number after a terminating signal.
func runTestMap(ctx context.Context, out io.Writer, request testMapRequest) int {
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate test --map: cannot read the working directory: %v; way forward: re-run the same command", err))
	}
	location, mapsDir, refusal := testMapLocation(cwd)
	if refusal != "" {
		return output.Err(out, refusal)
	}

	runCtx, caught, stop := catchTerminatingSignals(ctx)
	defer stop()
	run := &testMapRun{
		ctx: runCtx, request: request, location: location, mapsDir: mapsDir,
		built: []string{}, skipped: []testMapSkip{}, failed: []testMapFailure{}, timedOut: []string{},
		leftOut: []testMapSkip{}, measures: []testMapMeasure{},
	}
	defer run.close()
	if refusal := run.prepare(); refusal != "" {
		return output.Err(out, refusal)
	}
	sc, err := computeScope(runCtx, run.cfg, location, location.WorktreePath(), request.scope, nil)
	if err != nil {
		return output.Err(out, err.Error())
	}
	stopGuard, guardKilled := guardOwnTests(runCtx, run.memoryQuit, request.stderr)
	defer stopGuard()
	run.guardKilled = guardKilled
	packages, refusal := run.packages(sc)
	if refusal != "" {
		return output.Err(out, refusal)
	}
	if err := run.mapPackages(packages); err != nil {
		if runCtx.Err() != nil {
			output.Err(out, "gate test --map: the call was cancelled; way forward: re-run the same command")
			return exitAfterSignal(caught, 1)
		}
		return output.Err(out, fmt.Sprintf("gate test --map: %v; way forward: re-run the same command", err))
	}
	return output.Ok(out, map[string]any{
		"built":     run.built,
		"skipped":   run.skipped,
		"failed":    run.failed,
		"timed_out": run.timedOut,
		"left_out":  run.leftOut,
		"head":      run.head,
		"queued":    run.queued,
		"packages":  run.measures,
	})
}

// testMapLocation resolves the hub worktree cwd lies in and the coverage map directory of a call from it.
// It returns a refusal for a cwd outside every hub.
func testMapLocation(cwd string) (*lymcwd.Location, string, string) {
	location, inHub, err := resolveHub(cwd)
	if err != nil {
		return nil, "", fmt.Sprintf("gate test --map: %v; way forward: re-run the same command", err)
	}
	if !inHub {
		return nil, "", fmt.Sprintf("gate test --map: %s is in no hub, and the coverage maps live in a hub; way forward: run lym gate test --map from a worktree of a hub", cwd)
	}
	mapsDir, err := coverageMapsDir(location)
	if err != nil {
		return nil, "", fmt.Sprintf("gate test --map: %v; way forward: re-run the same command", err)
	}
	return location, mapsDir, ""
}

// coverageMapsDir returns the coverage map directory that `lym gate test --map` and `lym gate redundant` use in location.
// The hub's prime uses the hub's shared maps under its board directory.
// Any other worktree keeps its own under its anchor,
// so its maps record the HEAD only its own session moves and go with the worktree.
func coverageMapsDir(location *lymcwd.Location) (string, error) {
	prime, err := hubgeom.IsPrime(location)
	if err != nil {
		return "", err
	}
	if prime {
		return testcover.Dir(fabricengine.BoltDir(location.HubPath)), nil
	}
	return testcover.Dir(location.AnchorPath()), nil
}

// prepare loads the hub's gate and env config and finds the Go module.
// It returns a refusal when one of them cannot be read or the hub declares no mapped tier.
// It also returns one for a mapped tier that does not count its tests as go-json.
func (r *testMapRun) prepare() string {
	dirs := fabricengine.ConfigDirs(r.location)
	cfg, err := gateslot.LoadConfig(dirs)
	if err != nil {
		return fmt.Sprintf("gate test --map: cannot read the hub's gate config: %v; way forward: take the step it names, then re-run the same command", err)
	}
	hubEnv, err := envconfig.LoadConfig(dirs)
	if err != nil {
		return fmt.Sprintf("gate test --map: cannot read the env config: %v; way forward: take the step it names, then re-run the same command", err)
	}
	r.cfg = cfg.WithDefaultDir(r.location.AnchorRel)
	root := r.location.WorktreePath()
	r.tiers = mappedTierConfigs(r.cfg)
	if len(r.tiers) == 0 {
		return "gate test --map: the hub declares neither tier0 nor tier1, so no test of a package can be mapped; way forward: declare them with lym config gate from the prime, then re-run the same command"
	}
	for _, number := range mappedTiers {
		if tier, declared := r.tiers[number]; declared && tier.Counter != counterGoJSON {
			return fmt.Sprintf("gate test --map: tier%d counts its tests with %s, and a coverage map exists only for a go-json tier; way forward: when tier%d runs Go tests, set its counter to go-json with lym config gate from the prime and re-run the same command; a hub whose tier%d runs no Go tests has no coverage map to build", number, tier.Counter, number, number)
		}
	}
	r.moduleDir = resolveRunDir(root, r.tiers[slices.Min(slices.Collect(maps.Keys(r.tiers)))].Dir)
	rel, err := filepath.Rel(root, r.moduleDir)
	if r.modulePath = modulePathOf(r.moduleDir); r.modulePath == "" || err != nil || !filepath.IsLocal(rel) {
		return fmt.Sprintf("gate test --map: %s holds no go.mod naming a module inside the worktree; way forward: set the dir of the lowest of tier0 and tier1 to the Go module's root with lym config gate from the prime, then re-run the same command", r.moduleDir)
	}
	r.moduleRel = filepath.ToSlash(rel)
	r.buildTimeout = time.Duration(r.cfg.RedundancyBuildTimeoutMin) * r.request.minuteUnit
	r.testTimeout = time.Duration(r.cfg.RedundancyTestTimeoutSec) * r.request.secondUnit
	r.testMemory = r.cfg.RedundancyTestMemory()
	r.memoryQuit = r.cfg.MemoryQuit()
	r.env = gateEnv(r.request.environ, hubEnv)
	r.hubEnv = hubEnv

	if r.fabric, err = fabricengine.Open(r.location); err != nil {
		return fmt.Sprintf("gate test --map: %v; way forward: re-run the same command", err)
	}
	r.pool = hubgeom.GateSlots(r.location)
	return ""
}

// packages returns the packages the call maps, picked from the scope sc.
// They are relative to the module root and in import path order.
// A package enters when it has a map, or tracks a Go file at HEAD and holds a tier0 or tier1 test.
// A package the command line names that does not enter is recorded as left out, with the reason.
// It returns a refusal for a target that names no package directory of the module, or for a failed read.
func (r *testMapRun) packages(sc scope) ([]string, string) {
	candidates, named, refusal := r.scopePackages(sc)
	if refusal != "" {
		return nil, refusal
	}
	head, err := r.fabric.HeadSHA()
	if err != nil {
		return nil, fmt.Sprintf("gate test --map: read HEAD: %v; way forward: re-run the same command", err)
	}
	var packages []string
	for _, pkg := range candidates {
		reason, err := r.leftOutReason(head, pkg)
		if err != nil {
			return nil, fmt.Sprintf("gate test --map: %v; way forward: re-run the same command", err)
		}
		switch {
		case reason == "":
			packages = append(packages, pkg)
		case named[pkg]:
			r.leftOut = append(r.leftOut, testMapSkip{Package: pkg, Cause: reason})
		}
	}
	return packages, ""
}

// leftOutReason returns why the package pkg is not mapped at the commit head, or "" for one that is.
func (r *testMapRun) leftOutReason(head, pkg string) (string, error) {
	if _, err := os.Stat(testcover.MapPath(r.mapsDir, pkg)); err == nil {
		return "", nil
	}
	dir := path.Join(r.moduleRel, pkg)
	files, err := r.fileHashes(head, dir)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "no Go file tracked at HEAD", nil
	}
	// A package whose tests do not scan enters, so its measurement records the failure.
	mapped, err := r.mappedTests(filepath.Join(r.location.WorktreePath(), filepath.FromSlash(dir)))
	if err == nil && len(mapped) == 0 {
		return "no tier0 or tier1 test and no map", nil
	}
	return "", nil
}

// scopePackages returns the package directories the call's scope reaches, relative to the module root, sorted,
// and the set of those the command line names one by one.
// Explicit targets resolve against the module root, and a whole scope or tier share reaches every package.
// Any other scope reaches the packages of the mapped tiers' shares that lie inside the module.
func (r *testMapRun) scopePackages(sc scope) ([]string, map[string]bool, string) {
	all, err := modulePackages(r.moduleDir)
	if err != nil {
		return nil, nil, fmt.Sprintf("gate test --map: cannot list the packages of %s: %v; way forward: make the module readable, then re-run the same command", r.moduleDir, err)
	}
	var candidates []string
	named := map[string]bool{}
	switch {
	case len(r.request.scope.targets) > 0:
		for _, target := range r.request.scope.targets {
			pkg := path.Clean(filepath.ToSlash(target))
			var found []string
			if below, pattern := ellipsisRoot(pkg); pattern {
				found = packagesBelow(all, below)
			} else if filepath.IsLocal(filepath.FromSlash(pkg)) && holdsGoFile(filepath.Join(r.moduleDir, filepath.FromSlash(pkg))) {
				found = []string{pkg}
				named[pkg] = true
			}
			if len(found) == 0 {
				return nil, nil, fmt.Sprintf("gate test --map: %q names no package directory of the module at %s; way forward: name a package by its directory relative to the module root, such as internal/foo", target, r.moduleDir)
			}
			candidates = append(candidates, found...)
		}
	case sc.whole || slices.ContainsFunc(mappedTiers, func(number int) bool { return sc.forTier(number).whole }):
		return all, named, ""
	default:
		for _, number := range mappedTiers {
			for _, module := range sc.forTier(number).modules {
				for _, target := range module.targets {
					candidates = append(candidates, r.modulePackagesAt(all, path.Join(module.dir, target))...)
				}
			}
		}
	}
	slices.Sort(candidates)
	return slices.Compact(candidates), named, ""
}

// modulePackagesAt returns the packages of all that the worktree-relative slash target reaches:
// one directory, or every package below it for a `/...` target.
// It returns none for a target outside the module.
func (r *testMapRun) modulePackagesAt(all []string, target string) []string {
	dir, pattern := ellipsisRoot(target)
	if !pattern {
		dir = target
	}
	switch rel, inside := strings.CutPrefix(dir, r.moduleRel+"/"); {
	case r.moduleRel == ".":
	case dir == r.moduleRel:
		dir = "."
	case inside:
		dir = rel
	default:
		return nil
	}
	if pattern {
		return packagesBelow(all, dir)
	}
	if slices.Contains(all, dir) {
		return []string{dir}
	}
	return nil
}

// ellipsisRoot returns the directory the clean slash pattern target reaches below, and whether it is a `...` pattern.
func ellipsisRoot(target string) (string, bool) {
	if target == "..." {
		return ".", true
	}
	return strings.CutSuffix(target, "/...")
}

// packagesBelow returns the packages of all that are dir or lie below it; "." reaches every one.
func packagesBelow(all []string, dir string) []string {
	var below []string
	for _, pkg := range all {
		if dir == "." || pkg == dir || strings.HasPrefix(pkg, dir+"/") {
			below = append(below, pkg)
		}
	}
	return below
}

// modulePackages returns the slash path, relative to moduleDir, of every package of the module rooted there, sorted.
// A package is a directory holding a Go file, found as `go list ./...` finds one:
// testdata, a directory whose name opens with a dot or an underscore, and a nested module are left out.
func modulePackages(moduleDir string) ([]string, error) {
	var packages []string
	err := filepath.WalkDir(moduleDir, func(dir string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if dir != moduleDir {
			name := entry.Name()
			if name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return filepath.SkipDir
			}
		}
		if !holdsGoFile(dir) {
			return nil
		}
		rel, err := filepath.Rel(moduleDir, dir)
		if err != nil {
			return err
		}
		packages = append(packages, filepath.ToSlash(rel))
		return nil
	})
	slices.Sort(packages)
	return packages, err
}

// holdsGoFile reports whether the directory dir holds a Go file directly; an unreadable directory holds none.
func holdsGoFile(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(entries, func(entry fs.DirEntry) bool {
		return !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go")
	})
}

// mapPackages reads HEAD and measures each of packages whole at it, in order.
// It reads HEAD before each package, and ends once HEAD moved, counting the packages left as queued.
// The error reports a read, slot or write failure, or a cancelled call, that ends the walk.
// The packages the walk had not measured by then count as queued.
func (r *testMapRun) mapPackages(packages []string) error {
	head, err := r.fabric.HeadSHA()
	if err != nil {
		return fmt.Errorf("read HEAD: %w", err)
	}
	r.head = head
	jobs := make([]testMapJob, 0, len(packages))
	for _, pkg := range packages {
		dir := path.Join(r.moduleRel, pkg)
		files, err := r.fileHashes(head, dir)
		if err != nil {
			r.queued += len(packages)
			return err
		}
		jobs = append(jobs, testMapJob{pkg: pkg, dir: dir, files: files})
	}
	for i, job := range jobs {
		if err := r.ctx.Err(); err != nil {
			r.queued += len(jobs) - i
			return err
		}
		if err := r.readHead(); err != nil {
			r.queued += len(jobs) - i
			return err
		}
		if r.headMoved {
			r.queued += len(jobs) - i
			return nil
		}
		r.next = testMapJob{}
		if i+1 < len(jobs) {
			r.next = jobs[i+1]
		}
		if err := r.mapPackage(job); err != nil {
			r.queued += len(jobs) - i
			return err
		}
	}
	return nil
}

// readHead reads the worktree's HEAD and sets headMoved once it differs from the HEAD the call measures at.
func (r *testMapRun) readHead() error {
	current, err := r.fabric.HeadSHA()
	if err != nil {
		return fmt.Errorf("read HEAD: %w", err)
	}
	if current != r.head {
		r.headMoved = true
	}
	return nil
}

// fileHashes returns the ContentHash of each Go file, by name, that the directory dir tracks at the commit head.
// The directory is relative to the worktree.
func (r *testMapRun) fileHashes(head, dir string) (map[string]string, error) {
	names, err := r.fabric.CodeDirAt(head, dir)
	if err != nil {
		return nil, err
	}
	hashes := map[string]string{}
	for _, name := range names {
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		data, found, err := r.fabric.CodeFileAt(head, path.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if found {
			hashes[name] = testcover.ContentHash(data)
		}
	}
	return hashes, nil
}

// skip reports pkg skipped for cause.
func (r *testMapRun) skip(pkg, cause string) {
	r.skipped = append(r.skipped, testMapSkip{Package: pkg, Cause: cause})
}

// close cancels a coverage build still running ahead and waits for it to end.
// It then releases the call's slot and its temporary directory.
func (r *testMapRun) close() {
	if r.ahead != nil {
		r.ahead.cancel()
		<-r.ahead.done
		r.ahead = nil
	}
	r.releaseHeldSlot()
	if r.tmp != "" {
		os.RemoveAll(r.tmp)
	}
}
