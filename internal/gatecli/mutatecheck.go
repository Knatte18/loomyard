// mutatecheck.go runs the mutation check of `lym gate mutate` over the changed packages of a diff.
// It plans the mutants of each package's changed lines against the blocks its map's tests cover.
// It spreads the mutant cap over the changed functions in turn.
// It runs each mutant against the map's tests that cover its block until one fails.

package gatecli

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testreach"
)

// Reasons a mutant is left unjudged.
const (
	reasonMutantBudget       = "past mutation_budget_min; way forward: run the verb again, or raise mutation_budget_min with lym config gate from the prime"
	reasonMutantTimedOut     = "a covering test ran past its time bound with the mutant on"
	reasonMutantMemoryCapped = "a covering test ended at the memory cap with the mutant on"
	reasonMutantGuardKilled  = "the memory guard killed a covering test's run with the mutant on"
	reasonMutantBuild        = "the package's mutant binary did not build: %v"
)

// mutateMutant names one mutant by its worktree-relative file, line, operator and function.
type mutateMutant struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Operator string `json:"operator"`
	Function string `json:"function"`
}

// mutateSurvivor is a mutant every covering test passed.
type mutateSurvivor struct {
	mutateMutant
	// PossibleKillers are the tests of other packages that the maps show reach the mutant's function, a fixed number at most.
	// They are not run.
	PossibleKillers []string `json:"possible_killers"`
	// MorePossibleKillers counts those left out.
	MorePossibleKillers int `json:"more_possible_killers,omitempty"`
}

// mutateUnjudgedMutant is a mutant no verdict was reached for, with why.
type mutateUnjudgedMutant struct {
	mutateMutant
	Reason string `json:"reason"`
}

// mutateUncoveredBlock is a changed block no test of its package's map covers.
type mutateUncoveredBlock struct {
	Package  string `json:"package"`
	File     string `json:"file"`
	Function string `json:"function"`
}

// mutateNotJudged is a changed package the check judged nothing of, with why.
type mutateNotJudged struct {
	Package string `json:"package"`
	Reason  string `json:"reason"`
}

// mutateCounts is how many mutants the check planned, ran and decided.
type mutateCounts struct {
	// Run counts the mutants a covering test started on.
	Run int `json:"run"`
	// Killed and Survived count the mutants decided, and Unjudged those left without a verdict, run or not.
	Killed   int `json:"killed"`
	Survived int `json:"survived"`
	Unjudged int `json:"unjudged"`
	// Unbuildable counts the mutants dropped for a compile error.
	Unbuildable int `json:"unbuildable"`
	// NotRun counts the mutants over mutation_max_mutants.
	NotRun int `json:"not_run"`
}

// mutateOutcome is what a diffMutation found.
type mutateOutcome struct {
	survivors []mutateSurvivor
	unjudged  []mutateUnjudgedMutant
	uncovered []mutateUncoveredBlock
	// notJudged are the packages the check could not plan.
	notJudged []mutateNotJudged
	counts    mutateCounts
	// runs counts the test processes the check started.
	runs int
}

// newMutateOutcome returns an outcome whose lists are empty rather than nil, so each prints as a JSON list.
func newMutateOutcome() mutateOutcome {
	return mutateOutcome{
		survivors: []mutateSurvivor{}, unjudged: []mutateUnjudgedMutant{}, uncovered: []mutateUncoveredBlock{}, notJudged: []mutateNotJudged{},
	}
}

// mutantFate is what the covering tests decided about one mutant.
type mutantFate int

const (
	fateSurvived mutantFate = iota
	fateKilled
	fateUnjudged
)

// mutantVerdict is the outcome of running one mutant against the tests that cover its block.
type mutantVerdict struct {
	fate mutantFate
	// reason says why an unjudged mutant is.
	reason string
	// killer is the index among the mutant's covering tests of the test that killed it.
	killer int
}

// coveringTest is a test of a package's map that can kill a mutant, with the blocks it covers.
type coveringTest struct {
	test   testcover.MapTest
	blocks testcover.BlockSet
}

// mutatedPackage is one changed package with a fresh map, its planned mutants and the tests that can kill them.
type mutatedPackage struct {
	pkg   string
	check *mutationCheck
	// tests are the map's tests with a passing measurement whose check does not read source files, the fastest first.
	tests     []coveringTest
	plan      *testcover.MutantPlan
	uncovered []testcover.BlockKey
	// selected holds the IDs of the mutants the cap took.
	selected map[int]bool
}

// covering returns the tests whose blocks hold one of the mutant's, the fastest first.
func (p *mutatedPackage) covering(mutant testcover.Mutant) []coveringTest {
	var covering []coveringTest
	for _, candidate := range p.tests {
		if mutant.InBlocks(candidate.blocks) {
			covering = append(covering, candidate)
		}
	}
	return covering
}

// diffMutation is one `lym gate mutate` call's mutation check over the changed packages that have a fresh map.
type diffMutation struct {
	// holder is the check that holds the call's slot; each package's check is a copy of it.
	holder *mutationCheck
	module goModule
	// reader names the tests of other packages that reach a survivor's function.
	reader *testreach.Reader
	// fresh holds the fresh map of each package to judge, by module-relative path.
	fresh map[string]testcover.Map
	// changedLines holds the changed line numbers of each changed file, by module-relative package and file name.
	changedLines map[string]map[string][]int
	// maxMutants caps the mutants the call runs, and budget bounds the time from the slot's hold.
	maxMutants int
	budget     time.Duration
	// deadline is when the budget is spent, set once the slot is held.
	deadline time.Time
	// runs counts the test processes the check started.
	runs    atomic.Int64
	outcome mutateOutcome
}

// run judges every package of the fresh maps inside one gate slot and returns what it found.
// It waits for the slot, plans every package whatever the budget, takes the mutants the cap allows, and runs them.
// The error reports a slot, build or run failure, or the context's error once the call is cancelled.
func (d *diffMutation) run() (mutateOutcome, error) {
	if err := d.holder.holdSlot(); err != nil {
		return mutateOutcome{}, err
	}
	d.deadline = time.Now().Add(d.budget)
	var planned []*mutatedPackage
	for i, pkg := range slices.Sorted(maps.Keys(d.fresh)) {
		judged, err := d.planPackage(i, pkg)
		if ctxErr := d.holder.ctx.Err(); ctxErr != nil {
			return mutateOutcome{}, ctxErr
		}
		if err != nil {
			reason := fmt.Sprintf("%v; way forward: fix the failure it names, then run the verb again", err)
			d.outcome.notJudged = append(d.outcome.notJudged, mutateNotJudged{Package: pkg, Reason: reason})
			continue
		}
		planned = append(planned, judged)
		for _, key := range judged.uncovered {
			d.outcome.uncovered = append(d.outcome.uncovered, mutateUncoveredBlock{Package: pkg, File: key.File, Function: key.Func})
		}
	}
	d.selectWithinCap(planned)
	for _, judged := range planned {
		if err := d.runPackage(judged); err != nil {
			return mutateOutcome{}, err
		}
	}
	d.outcome.runs = int(d.runs.Load())
	return d.outcome, nil
}

// planPackage builds the coverage binary of the package pkg and plans the mutants of its changed lines.
// The mutants land in the blocks the package's map covers with the tests that can kill one.
// index tells the package's place in the call, which names its directory under the check's.
func (d *diffMutation) planPackage(index int, pkg string) (*mutatedPackage, error) {
	m := d.fresh[pkg]
	sourceChecks, err := sourceCheckingTests(d.holder.location.WorktreePath(), d.module, pkg)
	if err != nil {
		return nil, fmt.Errorf("the source scan failed: %w", err)
	}
	check := *d.holder
	check.lease = nil
	check.pkgDir = filepath.Join(d.module.dir, filepath.FromSlash(pkg))
	check.importPath = d.module.importPath(pkg)
	check.tmp = filepath.Join(d.holder.tmp, strconv.Itoa(index))
	if err := os.MkdirAll(check.tmp, 0o755); err != nil {
		return nil, err
	}
	judged := &mutatedPackage{pkg: pkg, check: &check}
	covered := testcover.BlockSet{}
	for _, test := range m.Tests {
		if !test.Measured() || sourceChecks[test.Name] {
			continue
		}
		blocks := mapBlocksOf(m, test)
		judged.tests = append(judged.tests, coveringTest{test: test, blocks: blocks})
		maps.Copy(covered, blocks)
	}
	slices.SortFunc(judged.tests, func(a, b coveringTest) int {
		return cmp.Or(cmp.Compare(a.test.Seconds, b.test.Seconds), strings.Compare(a.test.Name, b.test.Name))
	})
	profile, err := check.profileBlocks()
	if err != nil {
		return nil, err
	}
	if judged.plan, judged.uncovered, err = testcover.PlanLineMutants(check.pkgDir, check.importPath, profile, d.changedLines[pkg], covered); err != nil {
		return nil, fmt.Errorf("plan the mutants: %w", err)
	}
	return judged, nil
}

// selectWithinCap takes up to maxMutants of the planned mutants, one changed function at a time in turn.
// The functions go in the order of their package, file and name, and a function's mutants in the order of its plan.
// The mutants left over count as not run.
func (d *diffMutation) selectWithinCap(planned []*mutatedPackage) {
	type group struct {
		judged   *mutatedPackage
		file     string
		function string
		ids      []int
	}
	var groups []group
	total := 0
	for _, judged := range planned {
		judged.selected = map[int]bool{}
		byFunction := map[[2]string][]int{}
		for _, mutant := range judged.plan.Mutants {
			key := [2]string{mutant.File, mutant.FuncOf()}
			byFunction[key] = append(byFunction[key], mutant.ID)
			total++
		}
		for key, ids := range byFunction {
			groups = append(groups, group{judged: judged, file: key[0], function: key[1], ids: ids})
		}
	}
	slices.SortFunc(groups, func(a, b group) int {
		return cmp.Or(strings.Compare(a.judged.pkg, b.judged.pkg), strings.Compare(a.file, b.file), strings.Compare(a.function, b.function))
	})
	taken := 0
	for round := 0; taken < d.maxMutants; round++ {
		progressed := false
		for _, g := range groups {
			if round < len(g.ids) && taken < d.maxMutants {
				g.judged.selected[g.ids[round]] = true
				taken++
				progressed = true
			}
		}
		if !progressed {
			break
		}
	}
	d.outcome.counts.NotRun = total - taken
}

// runPackage builds one binary holding the package's selected mutants and judges each against its covering tests.
// Past the budget it starts no build and leaves every mutant unjudged for it.
// A build that fails leaves the package's mutants unjudged for that.
// The error reports a failed `lym` build or test start, or the context's error once the call is cancelled.
func (d *diffMutation) runPackage(judged *mutatedPackage) error {
	check := judged.check
	judged.plan.Mutants = slices.DeleteFunc(judged.plan.Mutants, func(m testcover.Mutant) bool { return !judged.selected[m.ID] })
	if len(judged.plan.Mutants) == 0 {
		return nil
	}
	if !time.Now().Before(d.deadline) {
		for _, mutant := range judged.plan.Mutants {
			d.recordUnjudged(judged, mutant, reasonMutantBudget)
		}
		return nil
	}
	bin, dropped, err := check.buildMutants(judged.plan)
	if err != nil {
		if ctxErr := check.ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		for _, mutant := range judged.plan.Mutants {
			d.recordUnjudged(judged, mutant, fmt.Sprintf(reasonMutantBuild, err))
		}
		return nil
	}
	d.outcome.counts.Unbuildable += len(dropped)
	check.mutants = map[int]testcover.Mutant{}
	var built []testcover.Mutant
	for _, mutant := range judged.plan.Mutants {
		if !dropped[mutant.ID] {
			check.mutants[mutant.ID] = mutant
			built = append(built, mutant)
		}
	}
	coverings := make([][]coveringTest, len(built))
	entries := map[string]testcover.MapTest{}
	var names []string
	for i, mutant := range built {
		coverings[i] = judged.covering(mutant)
		for _, covering := range coverings[i] {
			entries[covering.test.Name] = covering.test
			names = append(names, covering.test.Name)
		}
	}
	if err := check.buildLymFor(names, entries); err != nil {
		return err
	}
	verdicts, err := d.judgeMutants(check, bin, built, coverings)
	if err != nil {
		return err
	}
	for i, mutant := range built {
		d.record(judged, mutant, verdicts[i])
	}
	return nil
}

// judgeMutants runs each of built against its covering tests, up to the check's sideBySide mutants at once.
// A kill made beside other runs is run again alone.
// A killer that then passes sends the mutant on to its next covering test.
func (d *diffMutation) judgeMutants(check *mutationCheck, bin string, built []testcover.Mutant, coverings [][]coveringTest) ([]mutantVerdict, error) {
	verdicts := make([]mutantVerdict, len(built))
	errs := make([]error, len(built))
	indexes := make(chan int)
	var workers sync.WaitGroup
	for range min(check.sideBySide, len(built)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range indexes {
				verdicts[i], errs[i] = d.scan(check, bin, built[i], coverings[i], 0)
			}
		}()
	}
	for i := range built {
		indexes <- i
	}
	close(indexes)
	workers.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	if check.sideBySide < 2 {
		return verdicts, nil
	}
	for i, verdict := range verdicts {
		if verdict.fate != fateKilled {
			continue
		}
		again, err := d.scan(check, bin, built[i], coverings[i], verdict.killer)
		if err != nil {
			return nil, err
		}
		verdicts[i] = again
	}
	return verdicts, nil
}

// scan runs the covering tests of the mutant from index from on, each alone in its own process with the mutant on.
// It stops at the first test that fails for a reason of its own: that test kills the mutant.
// A run past its time bound, at the memory cap or killed by the memory guard kills nothing, and neither does a pass.
// Past the budget it starts no further run.
// The error reports a test binary that did not start.
func (d *diffMutation) scan(check *mutationCheck, bin string, mutant testcover.Mutant, covering []coveringTest, from int) (mutantVerdict, error) {
	stopped := ""
	for i := from; i < len(covering); i++ {
		if !time.Now().Before(d.deadline) {
			stopped = reasonMutantBudget
			break
		}
		job := mutantJob{test: covering[i].test, mutant: mutant.ID, timeout: check.mutantTimeout(covering[i].test.Seconds)}
		result, err := check.runJob(bin, job)
		d.runs.Add(1)
		if err != nil {
			return mutantVerdict{}, err
		}
		switch {
		case result.Action != testcover.ActionFail:
		case result.GuardKilled:
			stopped = reasonMutantGuardKilled
		case result.MemoryCapped:
			stopped = reasonMutantMemoryCapped
		case result.TimedOut:
			stopped = reasonMutantTimedOut
		default:
			return mutantVerdict{fate: fateKilled, killer: i}, nil
		}
	}
	if stopped != "" {
		return mutantVerdict{fate: fateUnjudged, reason: stopped}, nil
	}
	return mutantVerdict{fate: fateSurvived}, nil
}

// record adds the verdict of a mutant that was run, or left unrun for the budget, to the outcome.
func (d *diffMutation) record(judged *mutatedPackage, mutant testcover.Mutant, verdict mutantVerdict) {
	counts := &d.outcome.counts
	switch verdict.fate {
	case fateKilled:
		counts.Run++
		counts.Killed++
	case fateSurvived:
		counts.Run++
		counts.Survived++
		d.outcome.survivors = append(d.outcome.survivors, d.survivor(judged, mutant))
	default:
		if verdict.reason != reasonMutantBudget {
			counts.Run++
		}
		d.recordUnjudged(judged, mutant, verdict.reason)
	}
}

// recordUnjudged adds a mutant left without a verdict to the outcome.
func (d *diffMutation) recordUnjudged(judged *mutatedPackage, mutant testcover.Mutant, reason string) {
	d.outcome.counts.Unjudged++
	d.outcome.unjudged = append(d.outcome.unjudged, mutateUnjudgedMutant{mutateMutant: d.site(judged, mutant), Reason: reason})
}

// site names the mutant by its worktree-relative file, line, operator and function.
func (d *diffMutation) site(judged *mutatedPackage, mutant testcover.Mutant) mutateMutant {
	return mutateMutant{
		File: path.Join(packageDirOf(d.module.rel, judged.pkg), mutant.File), Line: mutant.Line, Operator: mutant.Operator, Function: mutant.FuncOf(),
	}
}

// survivor returns the survivor entry of a mutant, with the tests of other packages that reach its function.
func (d *diffMutation) survivor(judged *mutatedPackage, mutant testcover.Mutant) mutateSurvivor {
	entry := mutateSurvivor{mutateMutant: d.site(judged, mutant), PossibleKillers: []string{}}
	reaches := d.reader.TestsReaching(testcover.FuncKey{Package: judged.check.importPath, File: mutant.File, Func: mutant.FuncOf()})
	for _, reach := range reaches {
		switch {
		case reach.Own:
		case len(entry.PossibleKillers) == maxReachTestsPerFunction:
			entry.MorePossibleKillers++
		default:
			entry.PossibleKillers = append(entry.PossibleKillers, d.module.relativePackage(reach.Package)+"."+reach.Test)
		}
	}
	return entry
}
