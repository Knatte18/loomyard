// testmap.go holds the map run of `lym gate test --map`.
// It builds the per-package coverage maps on command at the HEAD of the worktree it runs in.
// It picks the worktree's maps directory, takes the packages of the call's scope, and measures each one whole.
// It prints what it measured, left unmeasured, failed and left out.

package gatecli

import (
	"context"
	"errors"
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
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/testcover"
)

// testMapSite labels the slot holder record of a `lym gate test --map` call.
const testMapSite = "lym gate test --map"

// mapFlag is the name of the flag that makes `lym gate test` build the coverage maps.
const mapFlag = "map"

// mappedTiers are the tiers whose tests a coverage map holds, lowest first.
var mappedTiers = []int{0, 1}

// testMapRequest is one `lym gate test --map` invocation after flag parsing.
type testMapRequest struct {
	// scope is the call whose scope flags pick the packages.
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
	// guardFor starts the memory guard over the call's process tree and returns its stop function and kill check.
	// Nil starts the guard the gate's own runs use.
	guardFor func(ctx context.Context, limit int64, out io.Writer) (stop func(), guardKilled func(pid int) bool)
}

// testMapSkip is a package the call left without a new map, with the reason.
type testMapSkip struct {
	Package string `json:"package"`
	Reason  string `json:"reason"`
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

// testMapJob is one measurement of a package, which rebuilds its map whole.
type testMapJob struct {
	pkg string
	// dir is the package's worktree-relative slash directory.
	dir string
	// files are the hashes of the Go files dir tracks at the call's HEAD, by name.
	files map[string]string
}

// testMapRun is one `lym gate test --map` call's state as it walks its packages.
type testMapRun struct {
	ctx      context.Context
	request  testMapRequest
	location *lymcwd.Location
	fabric   *fabricengine.Fabric
	pool     *gateslot.Pool
	// cfg is the hub's gate config, its default directory filled.
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
	// headNow is the HEAD the latest read found.
	headNow string
	// lease is the held slot; it stays nil before the first build and inside an enclosing gate run's slot.
	lease *gateslot.Lease
	// holderEnv is env with the slot's Go flags, nil until the call holds its slot.
	holderEnv []string
	// sideBySide is the most tests of each mapped tier run at once, by tier number, filled once by prepare.
	sideBySide map[int]int
	// tmp holds the coverage binaries and profiles, empty until the call holds its slot.
	tmp string
	// next is the job the walk measures after the current one, whose binary the current job builds ahead;
	// its pkg is empty when none follows.
	next testMapJob
	// ahead is the coverage build started for next, nil when none runs or waits to be taken up.
	ahead *coverageBuild
	// builds counts the coverage builds the call started, so each writes its own binary.
	builds int
	// toolchain holds the Go version and platform of the maps' environment stamp, nil until the call reads them.
	toolchain *testcover.EnvironmentStamp

	built []string
	// unmeasured holds each package the call wrote no map for, with the reason.
	unmeasured []testMapSkip
	failed     []testMapFailure
	timedOut   []string
	// failedTests holds each test whose written entry records a failed or timed-out outcome, as `<package>.<Test>`.
	failedTests []string
	// passedAlone holds each test that failed beside others and passed its rerun alone, as `<package>.<Test>`.
	passedAlone []string
	// leftOut holds the packages the call's targets named that the call left out of its set, with the cause.
	leftOut []testMapSkip
	// measures holds the build and run time of each package the call built.
	measures []testMapMeasure
}

// runTestMap runs one `lym gate test --map` call and returns its exit code.
// The code is 0 once it walked its packages and measured every one of them.
// It is 1 for a refusal or a failure that ends the call.
// It is 1 too for a walk after which a build failed, a package was left unmeasured,
// or an entry it wrote records a failed or timed-out test; the report is printed then too.
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
		built: []string{}, unmeasured: []testMapSkip{}, failed: []testMapFailure{}, timedOut: []string{}, leftOut: []testMapSkip{},
		failedTests: []string{}, passedAlone: []string{}, measures: []testMapMeasure{},
	}
	defer run.close()
	if refusal := run.prepare(); refusal != "" {
		return output.Err(out, refusal)
	}
	startGuard := request.guardFor
	if startGuard == nil {
		startGuard = guardOwnTests
	}
	stopGuard, guardKilled := startGuard(runCtx, run.memoryQuit, request.stderr)
	defer stopGuard()
	run.guardKilled = guardKilled
	sc, err := computeScope(runCtx, run.cfg, location, location.WorktreePath(), request.scope, nil)
	if err != nil {
		return output.Err(out, err.Error())
	}
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
	if len(run.passedAlone) > 0 {
		logger.Info("gate test --map: tests that failed beside others and passed alone", "tests", run.passedAlone)
	}
	code := output.Ok(out, map[string]any{
		"built":        run.built,
		"unmeasured":   run.unmeasured,
		"failed":       run.failed,
		"timed_out":    run.timedOut,
		"failed_tests": run.failedTests,
		"passed_alone": run.passedAlone,
		"left_out":     run.leftOut,
		"head":         run.head,
		"packages":     run.measures,
	})
	if len(run.failed) > 0 || len(run.failedTests) > 0 || len(run.unmeasured) > 0 {
		return 1
	}
	return code
}

// mapSideBySide returns how many tests of tier run at once in a map call.
// It is the tier's parallel, which already holds this machine's `tier_parallel` entry.
// The count is clamped by capped to what memory leaves.
// A tier that sets memory_mb also fits no more tests than its memory divides into caps of testMemory bytes.
func mapSideBySide(tier gateslot.Tier, testMemory int64, capped func(parallel int, limit int64) int) int {
	count := capped(tier.Parallel, testMemory)
	if tier.MemoryMB > 0 {
		count = min(count, int(tier.MemoryLimit()/testMemory))
	}
	return count
}

// mapSlotAsk returns how many gate slots a map call over tiers holds: the larger of the tiers' asks.
// A tier asking for every slot, Slots 0, ranks above any count and makes the answer 0.
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
// It returns a refusal when one of them cannot be read.
// It also returns one when the hub declares no mapped tier.
// It returns one too when a mapped tier does not count its tests as a Go test run does.
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
	for _, number := range slices.Sorted(maps.Keys(r.tiers)) {
		if counter := r.tiers[number].Counter; counter != counterGoJSON {
			return fmt.Sprintf("gate test --map: tier%d counts its tests with %s, and a coverage map exists only for a go-json tier; way forward: when tier%d runs Go tests, set its counter to go-json with lym config gate from the prime and re-run the same command; a hub whose tier%d runs no Go tests has no coverage map to build", number, counter, number, number)
		}
	}
	r.testMemory = r.cfg.RedundancyTestMemory()
	for _, number := range slices.Sorted(maps.Keys(r.tiers)) {
		if memoryMB := r.tiers[number].MemoryMB; memoryMB > 0 && r.tiers[number].MemoryLimit() < r.testMemory {
			return fmt.Sprintf("gate test --map: tier%d sets memory_mb %d, below redundancy_test_memory_mb %d, so no test process fits its memory; way forward: raise tier%d's memory_mb or lower redundancy_test_memory_mb with lym config gate from the prime, then re-run the same command", number, memoryMB, r.cfg.RedundancyTestMemoryMB, number)
		}
	}
	r.sideBySide = map[int]int{}
	for number, tier := range r.tiers {
		r.sideBySide[number] = mapSideBySide(tier, r.testMemory, r.request.cappedParallel)
	}
	r.moduleDir = resolveRunDir(root, r.tiers[slices.Min(slices.Collect(maps.Keys(r.tiers)))].Dir)
	rel, err := filepath.Rel(root, r.moduleDir)
	if r.modulePath = modulePathOf(r.moduleDir); r.modulePath == "" || err != nil || !filepath.IsLocal(rel) {
		return fmt.Sprintf("gate test --map: %s holds no go.mod naming a module inside the worktree; way forward: set the dir of the lowest of tier0 and tier1 to the Go module's root with lym config gate from the prime, then re-run the same command", r.moduleDir)
	}
	r.moduleRel = filepath.ToSlash(rel)
	r.buildTimeout = time.Duration(r.cfg.RedundancyBuildTimeoutMin) * r.request.minuteUnit
	r.testTimeout = time.Duration(r.cfg.RedundancyTestTimeoutSec) * r.request.secondUnit
	r.memoryQuit = r.cfg.MemoryQuit()
	r.env = gateEnv(r.request.environ, hubEnv)
	r.hubEnv = hubEnv

	if r.fabric, err = fabricengine.Open(r.location); err != nil {
		return fmt.Sprintf("gate test --map: %v; way forward: re-run the same command", err)
	}
	r.pool = hubgeom.GateSlots(r.location)
	return ""
}

// packages returns the packages the call measures, module-relative and in import path order, and a refusal.
// The scope picks the candidates.
// A candidate enters the set only when it has a map, or tracks a Go file at HEAD and holds a tier0 or tier1 test.
// A package the call's targets named by its directory that stays out is listed under left_out with its reason.
func (r *testMapRun) packages(sc scope) ([]string, string) {
	candidates, named, refusal := r.candidatePackages(sc)
	if refusal != "" {
		return nil, refusal
	}
	head, err := r.fabric.HeadSHA()
	if err != nil {
		return nil, fmt.Sprintf("gate test --map: read HEAD: %v; way forward: re-run the same command", err)
	}
	var set []string
	for _, pkg := range candidates {
		reason, err := r.leftOutCause(head, pkg)
		if err != nil {
			return nil, fmt.Sprintf("gate test --map: %v; way forward: re-run the same command", err)
		}
		switch {
		case reason == "":
			set = append(set, pkg)
		case named[pkg]:
			r.leftOut = append(r.leftOut, testMapSkip{Package: pkg, Reason: reason})
		}
	}
	return set, ""
}

// leftOutCause returns why the package pkg is left out of the call's set at the commit head.
// It returns "" for a package that stays in.
func (r *testMapRun) leftOutCause(head, pkg string) (string, error) {
	_, err := os.Stat(testcover.MapPath(r.mapsDir, pkg))
	if err == nil {
		return "", nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("read the map of %s: %w", pkg, err)
	}
	dir := path.Join(r.moduleRel, pkg)
	files, err := r.fileHashes(head, dir)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "no Go file tracked at HEAD", nil
	}
	mapped, err := r.mappedTests(filepath.Join(r.location.WorktreePath(), filepath.FromSlash(dir)))
	if err == nil && len(mapped) == 0 {
		return "no tier0 or tier1 test and no map", nil
	}
	// A package whose tests cannot be scanned stays in, so its map records the failure.
	return "", nil
}

// candidatePackages returns the packages the scope reaches before the map and test checks, sorted, each once.
// It also returns the packages the call's targets named by their directory.
// Explicit targets resolve against the module root, and `./...` and a `/...` suffix expand to the packages below.
// A whole scope, or a whole tier0 or tier1 share of it, reaches every package of the module.
// Any other scope reaches the union of the packages those tiers' shares name, those inside the module root.
// The scope's shares are read directly, since map mode ignores the tiers' commands.
func (r *testMapRun) candidatePackages(sc scope) ([]string, map[string]bool, string) {
	if sc.form == scopeTargets {
		return r.namedPackages(r.request.scope.targets)
	}
	shares := []scope{sc}
	for _, number := range slices.Sorted(maps.Keys(r.tiers)) {
		shares = append(shares, sc.forTier(number))
	}
	if slices.ContainsFunc(shares, func(share scope) bool { return share.whole }) {
		found, err := modulePackages(r.moduleDir)
		if err != nil {
			return nil, nil, fmt.Sprintf("gate test --map: cannot list the packages of %s: %v; way forward: make the module readable, then re-run the same command", r.moduleDir, err)
		}
		return found, nil, ""
	}
	var packages []string
	for _, share := range shares[1:] {
		for _, module := range share.modules {
			for _, target := range module.targets {
				if pkg, inModule := r.moduleRelative(path.Join(module.dir, strings.TrimPrefix(target, "./"))); inModule {
					packages = append(packages, pkg)
				}
			}
		}
	}
	slices.Sort(packages)
	return slices.Compact(packages), nil, ""
}

// moduleRelative returns the module-relative slash path of the worktree-relative slash path rel.
// It reports false for a path outside the module root.
func (r *testMapRun) moduleRelative(rel string) (string, bool) {
	if r.moduleRel == "." {
		return rel, filepath.IsLocal(filepath.FromSlash(rel))
	}
	if rel == r.moduleRel {
		return ".", true
	}
	return strings.CutPrefix(rel, r.moduleRel+"/")
}

// namedPackages resolves the command-line targets to module-relative package directories, sorted, each once.
// It also returns the packages a target named by their directory, which a pattern does not.
// A target is a directory relative to the module root that holds a Go file, or a pattern ending in `...`;
// any other name, or a pattern that matches no package, is a refusal.
func (r *testMapRun) namedPackages(targets []string) ([]string, map[string]bool, string) {
	var found []string
	literal := map[string]bool{}
	for _, target := range targets {
		pkg := path.Clean(filepath.ToSlash(target))
		prefix, pattern := strings.CutSuffix(pkg, "...")
		var matched []string
		if pattern {
			prefix = strings.TrimSuffix(prefix, "/")
			all, err := modulePackages(r.moduleDir)
			if err != nil {
				return nil, nil, fmt.Sprintf("gate test --map: cannot list the packages of %s: %v; way forward: make the module readable, then re-run the same command", r.moduleDir, err)
			}
			for _, candidate := range all {
				if prefix == "" || candidate == prefix || strings.HasPrefix(candidate, prefix+"/") {
					matched = append(matched, candidate)
				}
			}
		} else if filepath.IsLocal(filepath.FromSlash(pkg)) && holdsGoFile(filepath.Join(r.moduleDir, filepath.FromSlash(pkg))) {
			matched = []string{pkg}
			literal[pkg] = true
		}
		if len(matched) == 0 {
			return nil, nil, fmt.Sprintf("gate test --map: %q names no package directory of the module at %s; way forward: name a package by its directory relative to the module root, such as internal/foo", target, r.moduleDir)
		}
		found = append(found, matched...)
	}
	slices.Sort(found)
	return slices.Compact(found), literal, ""
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

// mapPackages reads HEAD and measures each package of packages whole, in order.
// Every map write checks HEAD itself, so the walk reads it only once, before the first package.
// Once a package's check saw HEAD move the walk ends,
// and every package it had not measured is listed as unmeasured with the HEAD-moved reason.
// The error reports a read, slot or write failure, or a cancelled call, that ends the walk.
func (r *testMapRun) mapPackages(packages []string) error {
	head, err := r.fabric.HeadSHA()
	if err != nil {
		return fmt.Errorf("read HEAD: %w", err)
	}
	r.head = head
	for i, pkg := range packages {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		dir := path.Join(r.moduleRel, pkg)
		files, err := r.fileHashes(r.head, dir)
		if err != nil {
			return err
		}
		r.next = testMapJob{}
		if i+1 < len(packages) {
			r.next = testMapJob{pkg: packages[i+1]}
		}
		if err := r.mapPackage(testMapJob{pkg: pkg, dir: dir, files: files}); err != nil {
			return err
		}
		if r.headMoved {
			for _, left := range packages[i+1:] {
				r.skip(left, r.headMovedReason())
			}
			break
		}
	}
	r.next = testMapJob{}
	return nil
}

// readHead reads the worktree's HEAD into headNow and sets headMoved once it differs from the HEAD the call measures at.
func (r *testMapRun) readHead() error {
	current, err := r.fabric.HeadSHA()
	if err != nil {
		return fmt.Errorf("read HEAD: %w", err)
	}
	r.headNow = current
	if current != r.head {
		r.headMoved = true
	}
	return nil
}

// headMovedReason returns why a package is left unmeasured once HEAD moved from the HEAD the call measures at.
func (r *testMapRun) headMovedReason() string {
	return fmt.Sprintf("HEAD moved from %s to %s; way forward: re-run the same command once HEAD stays put", r.head, r.headNow)
}

// checkClean reads HEAD and the worktree status afresh, and returns why the package may not be written, "" when it may.
// The package may go on while HEAD is the commit the call measures at.
// It also needs that no file a package of trees owns is modified or untracked.
// A nil trees, for a build whose listing of its linked packages failed, checks every file under the module root instead.
func (r *testMapRun) checkClean(trees map[string]packageTree) (string, error) {
	if err := r.readHead(); err != nil {
		return "", err
	}
	if r.headMoved {
		return r.headMovedReason(), nil
	}
	var file string
	var err error
	if trees == nil {
		file, err = r.dirtyModuleFile()
	} else {
		file, err = r.dirtyOwnedFile(trees)
	}
	if err != nil || file == "" {
		return "", err
	}
	return "modified or untracked file " + file + "; way forward: commit or revert it, then re-run the same command", nil
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

// skip lists pkg as unmeasured for reason.
func (r *testMapRun) skip(pkg, reason string) {
	r.unmeasured = append(r.unmeasured, testMapSkip{Package: pkg, Reason: reason})
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
