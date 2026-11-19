// mutate.go implements `lym gate mutate`.
// The verb mutates the lines the worktree's diff changes and lists the mutants no covering test kills.
// It refreshes the worktree's own coverage maps of the changed packages.
// It then runs the mutation check inside one gate slot.

package gatecli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/envconfig"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testreach"
)

// mutateSite labels the slot holder record of a `lym gate mutate` call.
const mutateSite = "lym gate mutate"

// Reasons a changed package is not judged beside those the reach step's refresh gives.
const (
	reasonNoMapToMutate     = "no coverage map holds it; way forward: build the maps with `lym gate test --map` from the prime"
	reasonSharedMapStale    = "its shared map no longer matches the worktree's files; way forward: rebuild the maps with `lym gate test --map` from the prime"
	reasonWorktreeMapStale  = "its worktree map no longer matches the worktree's files; way forward: commit the files and run the verb again"
	reasonRefreshWroteNoMap = "the refresh wrote no map of it, as the errors name; way forward: run the verb again"
)

// mutateRequest is one `lym gate mutate` invocation after flag parsing.
type mutateRequest struct {
	// base is the --base revision, empty for the merge base with the parent branch.
	base string
	// goBinary is the go command the builds run.
	goBinary string
	// environ is the environment the call's children start from: the process's own in production.
	environ []string
	// secondUnit is the length of one `redundancy_test_timeout_sec` second.
	// minuteUnit is the length of one minute of the budget and build bound keys.
	secondUnit, minuteUnit time.Duration
	// grace bounds the wait for a killed build's or test's exit and output.
	grace time.Duration
	// cappedParallel returns how many of parallel test runs, each capped at limit bytes of memory, go side by side.
	cappedParallel func(parallel int, limit int64) int
	// stderr receives the memory guard's line for each test binary it kills; nil prints none.
	stderr io.Writer
}

// checkRequest returns the request a mutation check reads its builds and bounds from.
func (r mutateRequest) checkRequest() redundantRequest {
	return redundantRequest{
		goBinary: r.goBinary, environ: r.environ, secondUnit: r.secondUnit, minuteUnit: r.minuteUnit, grace: r.grace,
		cappedParallel: r.cappedParallel, stderr: r.stderr,
	}
}

// mutateCmd builds `lym gate mutate`.
// goBinary is the go command its builds run.
// minuteUnit is the length of one minute of `reach_budget_min`, `mutation_budget_min` and `redundancy_build_timeout_min`.
// grace is the wait for a killed build's or test's exit and output.
func mutateCmd(goBinary string, minuteUnit, grace time.Duration) *cobra.Command {
	var base string
	cmd := &cobra.Command{
		Use:         "mutate",
		Short:       "mutate the lines the worktree's diff changes and list the mutants no test that covers them kills",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `mutate mutates the lines the worktree's diff changes and lists the mutants no test kills.
The diff is from --base, else the merge base with the parent branch; a worktree with neither, the prime's case, is refused.
It refreshes the worktree's own coverage maps of the packages holding a changed function under reach_budget_min; the prime refreshes nothing.
A package is judged only with a fresh map: one the refresh wrote, or a map whose file hashes match the worktree, the hub's shared one in the prime.
Any other changed package is listed as not judged, with its reason and way forward.
It places at most one mutant on each changed line of a block a test covers, up to mutation_max_mutants over the changed functions in turn.
It runs each mutant against the map's tests that cover its block until one fails, inside one gate slot bounded by mutation_budget_min.
A run past its time bound or the memory cap is unjudged, never a kill; a mutant every covering test passes is a survivor.
It prints the survivors with the tests of other packages that reach their function, the uncovered changed blocks, the packages not judged and the counts of mutants run, killed, survived and unjudged.
It exits 0 once it ran, whatever it found.

Examples:

  lym gate mutate
  lym gate mutate --base 3f2a9c1`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), runMutate(cmd.Context(), cmd.OutOrStdout(), mutateRequest{
				base: base, goBinary: goBinary, environ: os.Environ(), secondUnit: time.Second, minuteUnit: minuteUnit, grace: grace,
				cappedParallel: memoryCappedParallel, stderr: cmd.ErrOrStderr(),
			}))
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "commit whose diff to the working tree is mutated; default the merge base with the parent branch")
	return cmd
}

// diffBase returns the commit the verb diffs from, or a refusal when there is none.
func (r mutateRequest) diffBase(location *lymcwd.Location, fabric *fabricengine.Fabric) (string, string) {
	if r.base != "" {
		if !fabric.CommitExists(r.base) {
			return "", fmt.Sprintf("gate mutate: --base %q names no commit; way forward: pass the SHA of a commit the repository holds", r.base)
		}
		return r.base, ""
	}
	base, found, err := parentMergeBase(location)
	switch {
	case err != nil:
		return "", fmt.Sprintf("gate mutate: read the merge base with the parent branch: %v; way forward: re-run the same command", err)
	case !found:
		return "", "gate mutate: the worktree has no parent branch to diff from; way forward: pass --base with the commit to diff from"
	}
	return base, ""
}

// runMutate writes the mutation check of the worktree's diff.
// It returns the verb's exit code.
// The code is 0 once it ran, whatever it found, and 1 for a refusal or a failure that ends the call.
// After a terminating signal the code is 128 plus the signal number.
func runMutate(ctx context.Context, out io.Writer, request mutateRequest) int {
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate mutate: cannot read the working directory: %v; way forward: re-run the same command", err))
	}
	location, inHub, err := resolveHub(cwd)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate mutate: %v; way forward: re-run the same command", err))
	}
	if !inHub {
		return output.Err(out, fmt.Sprintf("gate mutate: %s is in no hub, and the coverage maps live in a hub; way forward: run lym gate mutate from a worktree of the hub", cwd))
	}
	dirs := fabricengine.ConfigDirs(location)
	cfg, err := gateslot.LoadConfig(dirs)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate mutate: cannot read the hub's gate config: %v; way forward: take the step it names, then re-run the same command", err))
	}
	cfg = cfg.WithDefaultDir(location.AnchorRel)
	if cfg.Scoper != "go" {
		return output.Ok(out, map[string]any{"note": fmt.Sprintf(noteScoper, cfg.Scoper)})
	}
	root := location.WorktreePath()
	module, err := redundancyModule(cfg, root)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate mutate: %v; way forward: declare tier0 or tier1 with its dir at the Go module's root with lym config gate from the prime, then re-run the same command", err))
	}
	fabric, err := fabricengine.Open(location)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate mutate: %v; way forward: re-run the same command", err))
	}
	prime, err := hubgeom.IsPrime(location)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate mutate: %v; way forward: re-run the same command", err))
	}
	base, refusal := request.diffBase(location, fabric)
	if refusal != "" {
		return output.Err(out, refusal)
	}
	hubEnv, err := envconfig.LoadConfig(dirs)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate mutate: cannot read the env config: %v; way forward: take the step it names, then re-run the same command", err))
	}

	runCtx, caught, stop := catchTerminatingSignals(ctx)
	defer stop()
	stopGuard, guardKilled := guardOwnTests(runCtx, cfg.MemoryQuit(), request.stderr)
	defer stopGuard()
	cancelled := func() int {
		output.Err(out, "gate mutate: the call was cancelled; way forward: re-run the same command")
		return exitAfterSignal(caught, 1)
	}

	changed, err := impactset.ChangedPaths(runCtx, root, base)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate mutate: %v; way forward: re-run the same command", err))
	}
	head := workingTree{root: root}
	funcs, err := changedFuncsAt(head, fabric, base, module, changed)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate mutate: find the changed functions: %v; way forward: fix the file it names, then re-run the same command", err))
	}
	lines, err := changedLinesAt(head, fabric, base, module, changed)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate mutate: find the changed lines: %v; way forward: re-run the same command", err))
	}
	answer := map[string]any{"base": base, "prime": prime, "changed_functions": len(funcs)}
	outcome := newMutateOutcome()
	if len(funcs) == 0 {
		answer["note"] = "the diff changes no function, so nothing was mutated"
		return output.Ok(out, fillMutateAnswer(answer, outcome, []mutateNotJudged{}, nil))
	}

	step := reachStep{
		ctx: runCtx, stderr: request.stderr, cfg: cfg, location: location, root: root, environ: request.environ, goBinary: request.goBinary,
		minuteUnit: request.minuteUnit, secondUnit: request.secondUnit, grace: request.grace, guardKilled: guardKilled,
	}
	var refresh ReachReport
	step.refreshMaps(&refresh, module, funcs, prime)
	if runCtx.Err() != nil {
		return cancelled()
	}
	mapsDir, err := coverageMapsDir(location)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate mutate: %v; way forward: re-run the same command", err))
	}
	fresh, notJudged, err := freshMaps(module, mapsDir, prime, changedPackages(module, funcs), refresh)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate mutate: %v", err))
	}
	if len(fresh) == 0 {
		return output.Ok(out, fillMutateAnswer(answer, outcome, notJudged, refresh.Errors))
	}
	reader, err := testreach.Open(reachModuleOf(module), testcover.Dir(fabricengine.BoltDir(location.HubPath)), step.worktreeMapsDir(prime))
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate mutate: read the coverage maps: %v; way forward: re-run the same command", err))
	}

	holder := &mutationCheck{
		ctx: runCtx, request: request.checkRequest(), location: location, pool: hubgeom.GateSlots(location), tiers: mappedTierConfigs(cfg),
		moduleDir:    module.dir,
		buildTimeout: time.Duration(cfg.RedundancyBuildTimeoutMin) * request.minuteUnit,
		testTimeout:  time.Duration(cfg.RedundancyTestTimeoutSec) * request.secondUnit,
		testMemory:   cfg.RedundancyTestMemory(),
		guardKilled:  guardKilled,
		env:          gateEnv(request.environ, hubEnv),
		site:         mutateSite,
	}
	defer holder.close()
	check := &diffMutation{
		holder: holder, module: module, reader: reader, fresh: fresh, changedLines: lines,
		maxMutants: cfg.MutationMaxMutants, budget: time.Duration(cfg.MutationBudgetMin) * request.minuteUnit,
		outcome: outcome,
	}
	outcome, err = check.run()
	if err != nil {
		if runCtx.Err() != nil {
			return cancelled()
		}
		return output.Err(out, fmt.Sprintf("gate mutate: the mutation check failed: %v; way forward: fix the build or test failure it names, then re-run the same command", err))
	}
	return output.Ok(out, fillMutateAnswer(answer, outcome, notJudged, refresh.Errors))
}

// fillMutateAnswer fills answer with what the check found and returns it.
// notJudged are the changed packages left out before the check, and each package the check could not plan joins them.
// refreshErrors are the error lines of the maps' refresh.
func fillMutateAnswer(answer map[string]any, outcome mutateOutcome, notJudged []mutateNotJudged, refreshErrors []string) map[string]any {
	answer["errors"] = append([]string{}, refreshErrors...)
	answer["mutants"] = outcome.counts
	answer["runs"] = outcome.runs
	answer["survivors"] = outcome.survivors
	answer["unjudged_mutants"] = outcome.unjudged
	answer["uncovered_blocks"] = outcome.uncovered
	answer["not_judged"] = append(notJudged, outcome.notJudged...)
	return answer
}

// freshMaps returns the map of each of packages whose map is fresh against the worktree's files, by module-relative path.
// It also returns why each other package is not judged.
// A task worktree reads its own maps, and a package the refresh left unrefreshed is not judged for the refresh's reason.
// The prime reads the hub's shared maps, which the refresh never touches.
// The error is a map that cannot be read.
func freshMaps(module goModule, mapsDir string, prime bool, packages []string, refresh ReachReport) (map[string]testcover.Map, []mutateNotJudged, error) {
	fresh := map[string]testcover.Map{}
	notJudged := []mutateNotJudged{}
	for _, pkg := range packages {
		if reason := unrefreshedReason(refresh.Unrefreshed, pkg); reason != "" && !prime {
			notJudged = append(notJudged, mutateNotJudged{Package: pkg, Reason: reason})
			continue
		}
		m, found, err := testreach.ReadPackageMap(mapsDir, pkg)
		if err != nil {
			return nil, nil, err
		}
		reason := reasonRefreshWroteNoMap
		if prime {
			reason = reasonNoMapToMutate
		}
		if found {
			isFresh, err := testreach.Fresh(reachModuleOf(module), pkg, m)
			if err != nil {
				return nil, nil, err
			}
			if isFresh {
				fresh[pkg] = m
				continue
			}
			reason = reasonWorktreeMapStale
			if prime {
				reason = reasonSharedMapStale
			}
		}
		notJudged = append(notJudged, mutateNotJudged{Package: pkg, Reason: reason})
	}
	return fresh, notJudged, nil
}

// unrefreshedReason returns why the refresh left the package pkg unrefreshed, or "" when it did not.
func unrefreshedReason(unrefreshed []string, pkg string) string {
	for _, line := range unrefreshed {
		if reason, named := strings.CutPrefix(line, pkg+": "); named {
			return reason
		}
	}
	return ""
}

// changedLinesAt returns the changed lines of each changed production Go file of the module that head holds.
// changed are the worktree-relative slash paths that differ between the commit base and head.
// The lines are those of the head file that a line diff against the file at base marks.
// They are keyed by module-relative package and file name.
func changedLinesAt(head redundancyTree, fabric *fabricengine.Fabric, base string, module goModule, changed []string) (map[string]map[string][]int, error) {
	lines := map[string]map[string][]int{}
	for _, rel := range changed {
		dir := path.Dir(rel)
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") || !isModulePackage(module.rel, dir) {
			continue
		}
		headData, found, err := head.file(rel)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		if !found {
			continue
		}
		baseData, _, err := fabric.CodeFileAt(base, rel)
		if err != nil {
			return nil, fmt.Errorf("read %s at %s: %w", rel, base, err)
		}
		marked := testreach.ChangedLines(baseData, headData)
		if len(marked) == 0 {
			continue
		}
		pkg := modulePackageOf(module.rel, dir)
		if lines[pkg] == nil {
			lines[pkg] = map[string][]int{}
		}
		lines[pkg][path.Base(rel)] = marked
	}
	return lines, nil
}
