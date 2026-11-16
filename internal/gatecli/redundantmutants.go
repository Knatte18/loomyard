// redundantmutants.go runs the mutation check of `lym gate redundant` inside one gate slot.
// It builds the package's coverage binary to place the candidates' blocks in the source.
// It then builds one test binary holding every mutant of those blocks, each turned on by testcover.MutantEnv.
// It runs the candidates, and their covering tests, with each mutant on.
// It judges each candidate by the mutants it kills.

package gatecli

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/testcover"
)

// redundantSite labels the slot holder record of a `lym gate redundant` call.
const redundantSite = "lym gate redundant"

// mutantBuildRounds caps the builds of one call's mutant binary.
// Each refused build drops the mutants on the lines the compiler names, such as a constant a typed context needs.
const mutantBuildRounds = 5

// A test runs with a mutant on for at most mutantTimeoutFactor times its unmutated seconds.
// The bound is never less than mutantTimeoutFloor seconds, and stays within redundancy_test_timeout_sec.
// A mutant that makes a loop endless is then killed long before the test's own bound.
const (
	mutantTimeoutFactor = 10
	mutantTimeoutFloor  = 10
)

// mutationStats is what one call's mutation check built and ran.
type mutationStats struct {
	// Mutants counts the mutants the binary holds, and Unbuildable those dropped for a compile error.
	Mutants     int `json:"mutants"`
	Unbuildable int `json:"unbuildable"`
	// BuildSeconds is the time from the slot's hold to a built mutant binary, and RunSeconds the time of the test runs.
	BuildSeconds float64 `json:"build_seconds"`
	RunSeconds   float64 `json:"run_seconds"`
	// Runs counts the test processes the check started.
	Runs int `json:"runs"`
}

// mutationCheck is one `lym gate redundant` call's mutation check of one package.
type mutationCheck struct {
	ctx      context.Context
	request  redundantRequest
	location *lymcwd.Location
	pool     *gateslot.Pool
	// tiers holds each mapped tier the hub declares, by number.
	tiers map[int]gateslot.Tier
	// pkgDir is the package directory, importPath its import path and moduleDir the module root the builds run in.
	pkgDir, importPath, moduleDir string
	// buildTimeout bounds each build, and testTimeout each test.
	buildTimeout, testTimeout time.Duration
	// env is the environment the check's children start from, before the slot's Go flags.
	env []string

	// The fields below fill as the check goes.

	// lease is the held slot; it stays nil inside an enclosing gate run's slot.
	lease *gateslot.Lease
	// holderEnv is env with the slot's Go flags, and parallel the most tests run side by side.
	holderEnv []string
	parallel  int
	// tmp holds the binaries, profiles and overlays.
	tmp string
	// lymBin is the `lym` built for a tier with prebuilt_lym, empty when none was needed.
	lymBin string
	stats  mutationStats
}

// mutantJob is one run of a test with one mutant on, the unmutated code for mutant 0.
type mutantJob struct {
	test    testcover.MapTest
	mutant  int
	timeout time.Duration
}

// run checks contained, the verdicts of the map's contained tests, by mutation.
// A contained test's covering tests are those testcover.CoveringTests picks among the other tests covers names.
// It returns their mutation verdicts in the same order.
// It also returns the covering tests each verdict was judged against, in that order, and the mutants by ID.
// The error reports a slot, build or run failure, or the context's error once the call is cancelled.
func (c *mutationCheck) run(m testcover.Map, contained []testcover.Verdict, covers map[string]bool) ([]testcover.MutantVerdict, [][]string, map[int]testcover.Mutant, error) {
	entries := mapEntries(m)
	targets := testcover.BlockSet{}
	for _, verdict := range contained {
		for block := range mapBlocksOf(m, entries[verdict.Name]) {
			targets[block] = struct{}{}
		}
	}
	if err := c.holdSlot(); err != nil {
		return nil, nil, nil, err
	}
	started := time.Now()
	plan, err := c.planMutants(targets)
	if err != nil {
		return nil, nil, nil, err
	}
	mutants := map[int]testcover.Mutant{}
	var bin string
	if len(plan.Mutants) > 0 {
		var dropped map[int]bool
		if bin, dropped, err = c.buildMutants(plan); err != nil {
			return nil, nil, nil, err
		}
		for _, mutant := range plan.Mutants {
			if !dropped[mutant.ID] {
				mutants[mutant.ID] = mutant
			}
		}
		c.stats.Unbuildable = len(dropped)
	}
	c.stats.Mutants = len(mutants)
	c.stats.BuildSeconds = time.Since(started).Seconds()

	candidates := make([]testcover.MutantCandidate, len(contained))
	coverings := make([][]string, len(contained))
	for i, verdict := range contained {
		blocks := mapBlocksOf(m, entries[verdict.Name])
		var pool []testcover.Run
		for _, name := range slices.Sorted(maps.Keys(covers)) {
			if name != verdict.Name {
				pool = append(pool, testcover.Run{Name: name, Blocks: mapBlocksOf(m, entries[name])})
			}
		}
		candidates[i] = testcover.MutantCandidate{
			Name: verdict.Name, Elapsed: entries[verdict.Name].Seconds, Covering: testcover.CoveringTests(blocks, pool),
		}
		for id, mutant := range mutants {
			if _, covered := blocks[mutant.Block]; covered {
				candidates[i].Mutants = append(candidates[i].Mutants, id)
			}
		}
		slices.Sort(candidates[i].Mutants)
		coverings[i] = candidates[i].Covering
	}
	if len(mutants) == 0 {
		return testcover.JudgeMutants(candidates, nil), coverings, mutants, nil
	}
	started = time.Now()
	kills, err := c.runMutants(bin, m, entries, candidates, mutants)
	c.stats.RunSeconds = time.Since(started).Seconds()
	if err != nil {
		return nil, nil, nil, err
	}
	return testcover.JudgeMutants(candidates, kills), coverings, mutants, nil
}

// runMutants runs the tests of candidates with each mutant of their blocks on and returns the kills.
// It first runs every candidate and covering test unmutated, setting FailsUnmutated on a candidate that does not pass.
// A test that does not pass unmutated runs with no mutant.
// Each candidate then runs with each mutant of its blocks on.
// A candidate that killed a mutant has its covering tests run with that mutant on.
// Only a covering test whose entry covers the mutant's block runs.
// A covering test that kills beside other tests runs once more alone, and that run stands.
// That holds for a covering test that is itself a candidate, whose kill from the candidate runs is replaced by that run.
func (c *mutationCheck) runMutants(bin string, m testcover.Map, entries map[string]testcover.MapTest, candidates []testcover.MutantCandidate, mutants map[int]testcover.Mutant) (testcover.Kills, error) {
	var involved []string
	for _, candidate := range candidates {
		involved = append(involved, candidate.Name)
		involved = append(involved, candidate.Covering...)
	}
	slices.Sort(involved)
	involved = slices.Compact(involved)
	blocks := map[string]testcover.BlockSet{}
	for _, name := range involved {
		blocks[name] = mapBlocksOf(m, entries[name])
	}
	if err := c.buildLymFor(involved, entries); err != nil {
		return nil, err
	}
	var baseline []mutantJob
	for _, name := range involved {
		baseline = append(baseline, mutantJob{test: entries[name], timeout: c.testTimeout})
	}
	results, err := c.runJobs(bin, baseline, true)
	if err != nil {
		return nil, err
	}
	timeouts := map[string]time.Duration{}
	for i, result := range results {
		if result.Action == testcover.ActionPass {
			timeouts[involved[i]] = c.mutantTimeout(result.Elapsed)
		}
	}

	kills := testcover.Kills{}
	record := func(jobs []mutantJob, results []testcover.RunResult) {
		for i, result := range results {
			if result.Action == testcover.ActionFail {
				if kills[jobs[i].mutant] == nil {
					kills[jobs[i].mutant] = map[string]bool{}
				}
				kills[jobs[i].mutant][jobs[i].test.Name] = true
			}
		}
	}
	var jobs []mutantJob
	ran := map[int]map[string]bool{}
	for i, candidate := range candidates {
		if _, passes := timeouts[candidate.Name]; !passes {
			candidates[i].FailsUnmutated = true
			continue
		}
		for _, id := range candidate.Mutants {
			jobs = append(jobs, mutantJob{test: entries[candidate.Name], mutant: id, timeout: timeouts[candidate.Name]})
			if ran[id] == nil {
				ran[id] = map[string]bool{}
			}
			ran[id][candidate.Name] = true
		}
	}
	if results, err = c.runJobs(bin, jobs, false); err != nil {
		return nil, err
	}
	record(jobs, results)

	jobs = nil
	type coverRun struct {
		mutant int
		name   string
	}
	queued := map[coverRun]bool{}
	for _, id := range slices.Sorted(maps.Keys(kills)) {
		for _, candidate := range candidates {
			if !kills[id][candidate.Name] {
				continue
			}
			for _, name := range candidate.Covering {
				_, passes := timeouts[name]
				_, covers := blocks[name][mutants[id].Block]
				// A covering test that ran the mutant as a candidate and did not kill it has its answer already.
				if !passes || !covers || queued[coverRun{id, name}] || (ran[id][name] && !kills[id][name]) {
					continue
				}
				queued[coverRun{id, name}] = true
				jobs = append(jobs, mutantJob{test: entries[name], mutant: id, timeout: timeouts[name]})
			}
		}
	}
	// The rerun decides these kills, so a kill made beside other runs is cleared once every job is chosen.
	for _, job := range jobs {
		delete(kills[job.mutant], job.test.Name)
	}
	if results, err = c.runJobs(bin, jobs, true); err != nil {
		return nil, err
	}
	record(jobs, results)
	return kills, nil
}

// mutantTimeout returns the bound of a run with a mutant on of a test that passed unmutated in elapsed seconds.
func (c *mutationCheck) mutantTimeout(elapsed float64) time.Duration {
	timeout := max(time.Duration(mutantTimeoutFloor)*c.request.secondUnit, time.Duration(elapsed*mutantTimeoutFactor*float64(time.Second)))
	if c.testTimeout > 0 {
		timeout = min(timeout, c.testTimeout)
	}
	return timeout
}

// planMutants builds the package's coverage binary and reads its block table from a run of no test.
// It returns the mutants of the blocks targets holds.
func (c *mutationCheck) planMutants(targets testcover.BlockSet) (*testcover.MutantPlan, error) {
	bin := filepath.Join(c.tmp, "cover.test")
	if err := c.toolchain().BuildBinary(c.importPath, c.importPath, "", bin); err != nil {
		return nil, fmt.Errorf("coverage build: %w", err)
	}
	lowest := c.tiers[slices.Min(slices.Collect(maps.Keys(c.tiers)))]
	runner := testcover.Toolchain{Env: slices.Concat(c.holderEnv, renderTierEnv(lowest)), Ctx: c.ctx, TestTimeout: c.testTimeout, Grace: c.request.grace}
	profile, err := runner.ProfileBlocks(bin, c.pkgDir, filepath.Join(c.tmp, "blocks.out"))
	if err != nil {
		return nil, err
	}
	plan, err := testcover.PlanMutants(c.pkgDir, c.importPath, profile, targets)
	if err != nil {
		return nil, fmt.Errorf("plan the mutants: %w", err)
	}
	return plan, nil
}

// buildMutants builds the binary holding every mutant of plan and returns it with the mutants dropped on the way.
// A build the compiler refuses drops each mutant on a line it names and builds again, up to mutantBuildRounds builds.
// The error reports a build that failed otherwise, refused a line no mutant spans or still failed at the last round.
func (c *mutationCheck) buildMutants(plan *testcover.MutantPlan) (string, map[int]bool, error) {
	dropped := map[int]bool{}
	bin := filepath.Join(c.tmp, "mutants.test")
	for round := 1; ; round++ {
		dir := filepath.Join(c.tmp, "overlay-"+strconv.Itoa(round))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", nil, err
		}
		overlay, err := plan.WriteOverlay(dir, dropped)
		if err != nil {
			return "", nil, fmt.Errorf("write the mutants: %w", err)
		}
		err = c.toolchain().BuildMutantBinary(c.importPath, overlay, bin)
		if err == nil {
			return bin, dropped, nil
		}
		unbuildable := plan.UnbuildableMutants(err, dropped)
		if len(unbuildable) == 0 || round == mutantBuildRounds {
			return "", nil, err
		}
		for _, id := range unbuildable {
			dropped[id] = true
		}
	}
}

// toolchain returns the toolchain the check's builds run under.
func (c *mutationCheck) toolchain() testcover.Toolchain {
	return testcover.Toolchain{
		Go: c.request.goBinary, Env: c.holderEnv, Dir: c.moduleDir, Ctx: c.ctx, BuildTimeout: c.buildTimeout, Grace: c.request.grace,
	}
}

// runJobs runs each of jobs alone in its own process, up to the check's parallel side by side.
// It returns the runs in the order of jobs.
// With rerunFailed, a job that failed beside other jobs runs once more alone, and that run stands.
// The error reports a test binary that did not start, or the context's error once the call is cancelled.
func (c *mutationCheck) runJobs(bin string, jobs []mutantJob, rerunFailed bool) ([]testcover.RunResult, error) {
	results, err := c.runSideBySide(bin, jobs)
	if err != nil || !rerunFailed || len(jobs) < 2 || c.parallel < 2 {
		return results, err
	}
	for i, result := range results {
		if result.Action != testcover.ActionFail {
			continue
		}
		alone, err := c.runSideBySide(bin, jobs[i:i+1])
		if err != nil {
			return nil, err
		}
		results[i] = alone[0]
	}
	return results, nil
}

// runSideBySide runs each of jobs alone in its own process, at most the check's parallel at once.
// It returns the runs in the order of jobs.
// The error is the first run's that did not start, or the context's once the call is cancelled.
func (c *mutationCheck) runSideBySide(bin string, jobs []mutantJob) ([]testcover.RunResult, error) {
	results := make([]testcover.RunResult, len(jobs))
	errs := make([]error, len(jobs))
	indexes := make(chan int)
	var workers sync.WaitGroup
	for range min(c.parallel, len(jobs)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range indexes {
				results[i], errs[i] = c.runJob(bin, jobs[i])
			}
		}()
	}
	for i := range jobs {
		indexes <- i
	}
	close(indexes)
	workers.Wait()
	c.stats.Runs += len(jobs)
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	return results, errors.Join(errs...)
}

// runJob runs the job's test alone under bin in the package directory, with its tier's env and its mutant on.
// A run past its timeout comes back failed and TimedOut, which counts as a kill.
func (c *mutationCheck) runJob(bin string, job mutantJob) (testcover.RunResult, error) {
	tier := c.tiers[job.test.Tier]
	env := slices.Concat(c.holderEnv, renderTierEnv(tier), []string{testcover.MutantEnv + "=" + strconv.Itoa(job.mutant)})
	if tier.PrebuiltLym && c.lymBin != "" {
		env = append(env, gateslot.PrebuiltLymEnv+"="+c.lymBin)
	}
	runner := testcover.Toolchain{Env: env, Ctx: c.ctx, TestTimeout: job.timeout, Grace: c.request.grace}
	result, err := runner.RunAlone(bin, c.pkgDir, "", job.test.Name)
	if err != nil {
		return testcover.RunResult{}, fmt.Errorf("run %s with mutant %d: %w", job.test.Name, job.mutant, err)
	}
	return result, nil
}

// buildLymFor builds `lym` into the pool's prebuilt store when a test of names has a tier with prebuilt_lym.
func (c *mutationCheck) buildLymFor(names []string, entries map[string]testcover.MapTest) error {
	if !slices.ContainsFunc(names, func(name string) bool { return c.tiers[entries[name].Tier].PrebuiltLym }) {
		return nil
	}
	bounded, cancel := context.WithTimeout(c.ctx, c.buildTimeout)
	defer cancel()
	// The store keeps the binary, so the cleanup prebuildLym returns for it removes nothing.
	bin, _, err := prebuildLym(bounded, c.request.goBinary, c.location.WorktreePath(), c.parallel, c.pool.PrebuiltDir(), c.request.grace, c.holderEnv)
	switch {
	case err != nil && errors.Is(bounded.Err(), context.DeadlineExceeded) && c.ctx.Err() == nil:
		return fmt.Errorf("lym build: timed out after %s", c.buildTimeout)
	case err != nil:
		return fmt.Errorf("lym build: %w", err)
	}
	c.lymBin = bin
	return nil
}

// holdSlot takes one slot of the hub's pool for the rest of the check, waiting with no bound but the call's context.
// Inside the slot an enclosing gate run holds, named by gateslot.InheritEnv, it takes none.
// It fills the slot holder's Go flags into the check's environment and creates the check's temporary directory.
func (c *mutationCheck) holdSlot() error {
	limits, err := c.pool.Limits()
	if err != nil {
		return fmt.Errorf("read the hub's gate limits: %w", err)
	}
	inherited := envValue(c.request.environ, gateslot.InheritEnv)
	if inherited == "" || !c.pool.Inherited(inherited) {
		if c.lease, err = c.pool.Acquire(c.ctx, gateslot.Holder{Worktree: c.location.WorktreePath(), Site: redundantSite}); err != nil {
			return fmt.Errorf("acquire a gate slot: %w", err)
		}
	}
	if c.tmp, err = lymtemp.MkdirTemp("redundant-"); err != nil {
		return fmt.Errorf("create the mutation check's directory: %w", err)
	}
	c.parallel = limits.Tier0Parallel
	if c.parallel < 1 {
		c.parallel = runtime.NumCPU()
	}
	c.holderEnv = gateslot.HolderEnv(c.env, c.lease, limits.Tier0Parallel)
	return nil
}

// close releases the check's slot and removes its temporary directory.
func (c *mutationCheck) close() {
	if c.lease != nil {
		releaseSlot(c.lease, c.location.WorktreePath())
	}
	if c.tmp != "" {
		os.RemoveAll(c.tmp)
	}
}
