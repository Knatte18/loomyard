// testmap.go holds `lym gate testmap`, which keeps the per-package coverage maps current from the hub's prime worktree.
// It refuses a call outside the prime, plans the packages at HEAD, then measures the queued ones under a cap and audits.
// It prints what it measured, skipped and failed with its plan's counts, and writes its exit record as it exits.

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

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/envconfig"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/testcover"
)

// testMapSite labels the slot holder record of a `lym gate testmap` call.
const testMapSite = "lym gate testmap"

// defaultTestMapMax caps the packages a `lym gate testmap` call without --max measures.
const defaultTestMapMax = 5

// testMapAuditTests is how many passing entries the audit round that ends a pass re-measures, oldest measurement first.
const testMapAuditTests = 3

// testMapBuildLockSuffix ends the name of the lock a call holds on a package's map while it plans or builds the map.
const testMapBuildLockSuffix = ".build.lock"

// mappedTiers are the tiers whose tests a coverage map holds, lowest first.
var mappedTiers = []int{0, 1}

// testMapRequest is one `lym gate testmap` invocation after flag parsing.
type testMapRequest struct {
	// goBinary is the go command the builds run.
	goBinary string
	// maxPackages caps the packages the call measures.
	maxPackages int
	// packages are the package directories named on the command line, relative to the module root;
	// empty plans every package.
	packages []string
	// environ is the environment the call's children start from: the process's own in production.
	environ []string
	// secondUnit is the length of one `redundancy_test_timeout_sec` second.
	// minuteUnit is the length of one `redundancy_build_timeout_min` minute.
	secondUnit, minuteUnit time.Duration
	// grace bounds the wait for a timed-out build's or test's exit and output once its process group is killed.
	grace time.Duration
	// auditTests is how many passing entries the closing audit round re-measures: testMapAuditTests in production.
	auditTests int
}

// testMapCmd builds `lym gate testmap`.
// goBinary is the go command its builds run.
// minuteUnit is the length of one `redundancy_build_timeout_min` minute.
// grace is the wait for a killed build's or test's exit and output.
func testMapCmd(goBinary string, minuteUnit, grace time.Duration) *cobra.Command {
	var maxPackages int
	cmd := &cobra.Command{
		Use:         "testmap [<package>...]",
		Short:       "keep the per-package coverage maps current at the HEAD of the hub's prime worktree",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `testmap keeps the coverage map of every package current at the prime's HEAD, or rebuilds each named package's map whole.
A map records, for each tier0 and tier1 test of the package run alone under a coverage binary over every in-module package its test binary links, the blocks of the package's own files and the functions of the linked packages it covers.
With no package named, it first plans every package at HEAD, building nothing: the coverage selector compares each map with HEAD, as a gate compares it with the working tree.
A map moves to HEAD keeping every entry but those of affected tests and of tests no longer declared; the affected ones are queued.
A map the selector misses for stays as it is, and its package is queued for a whole rebuild.
It then measures the queued packages, at most --max per call, in package path order, holding one gate slot from its first build to its end.
A test that fails, times out, skips or covers nothing gets an entry naming that outcome; a test the spawn scan marks is recorded and never run.
A failed build keeps the map's entries and records the failure with HEAD.
Once nothing is left queued, it re-measures the oldest passing entries when a gate slot is free at once, and counts each that differs as an audit mismatch.
A package with a modified or untracked Go file, its own or a linked package's, is skipped; one whose map another call is building is skipped and counted as queued.
It prints the packages it measured, skipped and failed, the timed-out tests, the planned HEAD, the queued work left, the plan's counts, the mismatch counts and each measured package's build and run seconds.

Examples:

  lym gate testmap
  lym gate testmap --max 2
  lym gate testmap internal/foo`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), runTestMap(cmd.Context(), cmd.OutOrStdout(), testMapRequest{
				goBinary: goBinary, maxPackages: maxPackages, packages: args, environ: os.Environ(),
				secondUnit: time.Second, minuteUnit: minuteUnit, grace: grace, auditTests: testMapAuditTests,
			}))
			return nil
		},
	}
	cmd.Flags().IntVar(&maxPackages, "max", defaultTestMapMax, "the most packages the call measures")
	return cmd
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

// testMapJob is one queued measurement of a package.
type testMapJob struct {
	pkg string
	// dir is the package's worktree-relative slash directory.
	dir string
	// files are the hashes of the Go files dir tracks at the call's HEAD, by name.
	files map[string]string
	// whole rebuilds the map from nothing; otherwise the job measures tests into the map the plan wrote at HEAD.
	whole bool
	// tests name the tests a job that is not whole measures, each replacing its entry.
	tests []string
	// audit marks a job of the closing audit round, which counts each replaced entry its fresh one differs from.
	audit bool
}

// testMapRun is one `lym gate testmap` call's state as it walks its packages.
type testMapRun struct {
	ctx      context.Context
	request  testMapRequest
	location *lymcwd.Location
	fabric   *fabricengine.Fabric
	pool     *gateslot.Pool
	// tiers holds each mapped tier the hub declares, by number.
	tiers map[int]gateslot.Tier
	// mapsDir is the hub's coverage map directory.
	mapsDir string
	// moduleDir is the module root, moduleRel its worktree-relative slash path and modulePath its import path.
	moduleDir, moduleRel, modulePath string
	// buildTimeout bounds each build, and testTimeout each test.
	buildTimeout, testTimeout time.Duration
	// env is the environment the call's children start from, before the slot's Go flags.
	env []string
	// hubEnv is the hub's env.yaml variables, which env holds.
	hubEnv envconfig.Config
	// inert reports whether a worktree-relative slash path matches the hub's inert_paths.
	inert func(rel string) bool
	// dirty holds the worktree-relative slash paths of the uncommitted and untracked files read before the walk, sorted.
	dirty []string

	// The fields below fill as the call goes.

	// head is the prime's HEAD the call plans and measures at, read once before its walk.
	head string
	// selector compares the maps with head; nil until the plan needs it.
	selector *testcover.Selector
	// lease is the held slot; it stays nil before the first build and inside an enclosing gate run's slot.
	lease *gateslot.Lease
	// holderEnv is env with the slot's Go flags, nil until the call holds its slot.
	holderEnv []string
	// parallel is the `-p` of the `lym` build, read with the slot.
	parallel int
	// tmp holds the coverage binaries and profiles, empty until the call holds its slot.
	tmp string
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
	// queued counts the queued work the call leaves: jobs past the cap and packages whose build lock another call held.
	queued int
	// plan counts the plan's verdicts.
	plan                                 testcover.PlanCounts
	selfCheckMismatches, auditMismatches int
	// measures holds the build and run time of each package the call built.
	measures []testcover.PackageMeasure
}

// runTestMap runs one `lym gate testmap` call and returns its exit code.
// The code is 0 once it walked its packages.
// It is 1 for a refusal or a failure that ends the call.
// It is 128 plus the signal number after a terminating signal.
func runTestMap(ctx context.Context, out io.Writer, request testMapRequest) int {
	if request.maxPackages < 1 {
		return output.Err(out, fmt.Sprintf("gate testmap: --max %d; way forward: pass --max 1 or more", request.maxPackages))
	}
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate testmap: cannot read the working directory: %v; way forward: re-run the same command", err))
	}
	location, refusal := testMapLocation(cwd)
	if refusal != "" {
		return output.Err(out, refusal)
	}

	runCtx, caught, stop := catchTerminatingSignals(ctx)
	defer stop()
	run := &testMapRun{
		ctx: runCtx, request: request, location: location,
		mapsDir: testcover.Dir(fabricengine.BoltDir(location.HubPath)),
		built:   []string{}, skipped: []testMapSkip{}, failed: []testMapFailure{}, timedOut: []string{},
		measures: []testcover.PackageMeasure{},
	}
	defer run.close()
	if refusal := run.prepare(); refusal != "" {
		return output.Err(out, refusal)
	}
	packages, named, refusal := run.packages()
	if refusal != "" {
		return output.Err(out, refusal)
	}
	if err := run.mapPackages(packages, named); err != nil {
		if runCtx.Err() != nil {
			output.Err(out, "gate testmap: the call was cancelled; way forward: re-run the same command")
			return exitAfterSignal(caught, 1)
		}
		return output.Err(out, fmt.Sprintf("gate testmap: %v; way forward: re-run the same command", err))
	}
	record := run.exitRecord()
	return output.Ok(out, map[string]any{
		"built":                 run.built,
		"skipped":               run.skipped,
		"failed":                run.failed,
		"timed_out":             run.timedOut,
		"planned_head":          record.PlannedHead,
		"queued":                record.Queued,
		"plan":                  record.Plan,
		"self_check_mismatches": record.SelfCheckMismatches,
		"audit_mismatches":      record.AuditMismatches,
		"packages":              run.measures,
	})
}

// testMapLocation resolves the hub worktree cwd lies in, and refuses one outside every hub or other than the hub's prime.
func testMapLocation(cwd string) (*lymcwd.Location, string) {
	location, inHub, err := resolveHub(cwd)
	if err != nil {
		return nil, fmt.Sprintf("gate testmap: %v; way forward: re-run the same command", err)
	}
	if !inHub {
		return nil, fmt.Sprintf("gate testmap: %s is in no hub, and the coverage maps live in a hub; way forward: run lym gate testmap from a hub's prime worktree", cwd)
	}
	prime, err := hubgeom.IsPrime(location)
	if err != nil {
		return nil, fmt.Sprintf("gate testmap: %v; way forward: re-run the same command", err)
	}
	if prime {
		return location, ""
	}
	primePath, err := hubgeom.PrimeWorktreePath(location)
	if err != nil {
		primePath = "the hub's prime worktree"
	}
	return nil, fmt.Sprintf("gate testmap: %s is not the hub's prime worktree, and a coverage map records the prime branch; way forward: run lym gate testmap from %s", location.WorktreePath(), primePath)
}

// prepare loads the hub's gate and env config, finds the Go module and reads the uncommitted and untracked files.
// It returns a refusal when one of them cannot be read, or when the hub declares no mapped tier.
func (r *testMapRun) prepare() string {
	dirs := fabricengine.ConfigDirs(r.location)
	cfg, err := gateslot.LoadConfig(dirs)
	if err != nil {
		return fmt.Sprintf("gate testmap: cannot read the hub's gate config: %v; way forward: take the step it names, then re-run the same command", err)
	}
	hubEnv, err := envconfig.LoadConfig(dirs)
	if err != nil {
		return fmt.Sprintf("gate testmap: cannot read the env config: %v; way forward: take the step it names, then re-run the same command", err)
	}
	cfg = cfg.WithDefaultDir(r.location.AnchorRel)
	root := r.location.WorktreePath()
	r.tiers = mappedTierConfigs(cfg)
	if len(r.tiers) == 0 {
		return "gate testmap: the hub declares neither tier0 nor tier1, so no test of a package can be mapped; way forward: declare them with lym config gate from the prime, then re-run the same command"
	}
	r.moduleDir = resolveRunDir(root, r.tiers[slices.Min(slices.Collect(maps.Keys(r.tiers)))].Dir)
	rel, err := filepath.Rel(root, r.moduleDir)
	if r.modulePath = modulePathOf(r.moduleDir); r.modulePath == "" || err != nil || !filepath.IsLocal(rel) {
		return fmt.Sprintf("gate testmap: %s holds no go.mod naming a module inside the worktree; way forward: set the dir of the lowest of tier0 and tier1 to the Go module's root with lym config gate from the prime, then re-run the same command", r.moduleDir)
	}
	r.moduleRel = filepath.ToSlash(rel)
	r.buildTimeout = time.Duration(cfg.RedundancyBuildTimeoutMin) * r.request.minuteUnit
	r.testTimeout = time.Duration(cfg.RedundancyTestTimeoutSec) * r.request.secondUnit
	r.env = gateEnv(r.request.environ, hubEnv)
	r.hubEnv = hubEnv
	r.inert = cfg.Inert

	if r.fabric, err = fabricengine.Open(r.location); err != nil {
		return fmt.Sprintf("gate testmap: %v; way forward: re-run the same command", err)
	}
	entries, err := r.fabric.Status()
	if err != nil {
		return fmt.Sprintf("gate testmap: %v; way forward: re-run the same command", err)
	}
	for _, entry := range entries {
		r.dirty = append(r.dirty, entry.Path)
	}
	slices.Sort(r.dirty)
	r.pool = hubgeom.GateSlots(r.location)
	return ""
}

// packages returns the packages the call walks, in package path order, and whether the command line named them.
// A named package is a directory relative to the module root that holds a Go file; any other name is a refusal.
func (r *testMapRun) packages() ([]string, bool, string) {
	if len(r.request.packages) == 0 {
		found, err := modulePackages(r.moduleDir)
		if err != nil {
			return nil, false, fmt.Sprintf("gate testmap: cannot list the packages of %s: %v; way forward: make the module readable, then re-run the same command", r.moduleDir, err)
		}
		return found, false, ""
	}
	var named []string
	for _, arg := range r.request.packages {
		pkg := path.Clean(filepath.ToSlash(arg))
		if !filepath.IsLocal(filepath.FromSlash(pkg)) || !holdsGoFile(filepath.Join(r.moduleDir, filepath.FromSlash(pkg))) {
			return nil, true, fmt.Sprintf("gate testmap: %q names no package directory of the module at %s; way forward: name a package by its directory relative to the module root, such as internal/foo", arg, r.moduleDir)
		}
		named = append(named, pkg)
	}
	slices.Sort(named)
	return slices.Compact(named), true, ""
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

// mapPackages reads HEAD, plans every package at it and measures the queued jobs in order until maxPackages are measured.
// A job skipped before its build does not count against the cap.
// A job whose build lock another call holds is skipped and counted as queued, and so is each job past the cap.
// With nothing left queued, the closing audit round runs.
// The error reports a read, slot or write failure, or a cancelled call, that ends the walk.
func (r *testMapRun) mapPackages(packages []string, named bool) error {
	head, err := r.fabric.HeadSHA()
	if err != nil {
		return fmt.Errorf("read HEAD: %w", err)
	}
	r.head = head
	var jobs []testMapJob
	for _, pkg := range packages {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		job, queued, err := r.planPackage(pkg, named)
		if err != nil {
			return err
		}
		if queued {
			jobs = append(jobs, job)
		}
	}
	measured := 0
	for i, job := range jobs {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		if measured >= r.request.maxPackages {
			r.queued += len(jobs) - i
			break
		}
		buildLock, held, err := r.lockPackage(job.pkg)
		if err != nil {
			return err
		}
		if !held {
			r.skip(job.pkg, "another call holds its map build lock")
			r.queued++
			continue
		}
		measured++
		skippedBeforeBuild, err := r.mapPackage(job)
		releaseBuildLock(buildLock, job.pkg)
		if err != nil {
			return err
		}
		if skippedBeforeBuild {
			measured--
		}
	}
	if r.queued > 0 {
		return nil
	}
	return r.audit(packages)
}

// releaseBuildLock releases the build lock of the map of pkg, logging a failure.
func releaseBuildLock(buildLock *lock.FileLock, pkg string) {
	if err := buildLock.Release(); err != nil {
		logger.Warn("gate testmap: could not release a map build lock", "package", pkg, "cause", err)
	}
}

// dirtyGoFile returns the first uncommitted or untracked Go file directly in the worktree-relative directory dir, or "".
func (r *testMapRun) dirtyGoFile(dir string) string {
	for _, file := range r.dirty {
		if path.Dir(file) == dir && strings.HasSuffix(file, ".go") {
			return file
		}
	}
	return ""
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

// testMapBuildLockPath returns the path of the lock a call holds on the map of pkg while it plans or builds it.
// The path lies beside the map under the maps directory mapsDir.
func testMapBuildLockPath(mapsDir, pkg string) string {
	return lock.TrackedFileLockPath(testcover.MapPath(mapsDir, pkg), testMapBuildLockSuffix)
}

// lockPackage takes the build lock of the map of pkg without waiting, and reports whether it got it.
func (r *testMapRun) lockPackage(pkg string) (*lock.FileLock, bool, error) {
	lockPath := testMapBuildLockPath(r.mapsDir, pkg)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, false, fmt.Errorf("create the map directory of %s: %w", pkg, err)
	}
	return lock.TryAcquireWriteLock(lockPath)
}

// skip reports pkg skipped for cause.
func (r *testMapRun) skip(pkg, cause string) {
	r.skipped = append(r.skipped, testMapSkip{Package: pkg, Cause: cause})
}

// exitRecord returns the call's exit record as its state stands.
func (r *testMapRun) exitRecord() testcover.ExitRecord {
	return testcover.ExitRecord{
		PID: os.Getpid(), PlannedHead: r.head, Queued: r.queued, Plan: r.plan,
		SelfCheckMismatches: r.selfCheckMismatches, AuditMismatches: r.auditMismatches, Packages: r.measures,
	}
}

// close writes the call's exit record and releases its slot and its temporary directory.
// A failed write is logged.
func (r *testMapRun) close() {
	if err := testcover.WriteExitRecord(r.mapsDir, r.exitRecord()); err != nil {
		logger.Warn("gate testmap: could not write the exit record", "dir", r.mapsDir, "cause", err)
	}
	if r.lease != nil {
		releaseSlot(r.lease, r.location.WorktreePath())
	}
	if r.tmp != "" {
		os.RemoveAll(r.tmp)
	}
}
