// redundant.go implements `lym gate redundant`.
// The verb judges one package's coverage map and confirms its candidates by mutation.
// It lists the candidates whose covering tests kill every mutant they kill, and sets apart the ones a mutant refutes.

package gatecli

import (
	"context"
	"fmt"
	"io"
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
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/testcover"
)

// redundantCandidate is one test a `lym gate redundant` answer confirms redundant.
type redundantCandidate struct {
	Name    string  `json:"name"`
	Tier    int     `json:"tier"`
	Seconds float64 `json:"seconds"`
	// Removable is set for a test of the set that can be deleted together:
	// every block stays covered, and a test left kills every mutant it killed.
	Removable bool `json:"removable"`
	// Covering names the tests whose mutant kills the candidate was judged against, largest overlap first.
	Covering []string `json:"covering"`
	// MutantsKilled counts the mutants the candidate killed, each killed by a covering test too.
	MutantsKilled int `json:"mutants_killed"`
}

// redundantRefuted is one test whose blocks other tests cover but that kills a mutant its covering tests do not.
type redundantRefuted struct {
	Name     string   `json:"name"`
	Tier     int      `json:"tier"`
	Seconds  float64  `json:"seconds"`
	Covering []string `json:"covering"`
	// Unique are the mutants it killed that none of its covering tests killed.
	Unique []redundantMutant `json:"unique_mutants"`
}

// redundantMutant names one mutant by its site and operator.
type redundantMutant struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Operator string `json:"operator"`
}

// redundantSkip is one test a `lym gate redundant` answer did not judge, with the keep reason or why not.
type redundantSkip struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// redundantRequest is one `lym gate redundant` invocation after argument parsing.
type redundantRequest struct {
	// pkg is the package directory named on the command line, relative to the module root.
	pkg string
	// goBinary is the go command the builds run.
	goBinary string
	// environ is the environment the call's children start from: the process's own in production.
	environ []string
	// secondUnit is the length of one `redundancy_test_timeout_sec` second.
	// minuteUnit is the length of one `redundancy_build_timeout_min` minute.
	secondUnit, minuteUnit time.Duration
	// grace bounds the wait for a killed build's or test's exit and output.
	grace time.Duration
	// cappedParallel returns how many of parallel test runs, each capped at limit bytes of memory, go side by side.
	cappedParallel func(parallel int, limit int64) int
}

// redundantCmd builds `lym gate redundant`.
// goBinary is the go command its builds run.
// minuteUnit is the length of one `redundancy_build_timeout_min` minute.
// grace is the wait for a killed build's or test's exit and output.
func redundantCmd(goBinary string, minuteUnit, grace time.Duration) *cobra.Command {
	return &cobra.Command{
		Use:         "redundant <package>",
		Short:       "list the tests of one package that its other tests make redundant, judged by coverage and confirmed by mutation",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `redundant judges the coverage map of one package, named by its directory relative to the module root, and confirms each candidate by mutation.
A candidate is a tier0 or tier1 test whose covered blocks of the package's own files the package's other judged tests cover.
Equal coverage is not equal checks, so the verb then mutates the candidates' blocks: it negates conditions and comparisons, drops statements and changes constants, one mutant at a time.
It runs each candidate with each mutant of its blocks on, and its covering tests with each mutant it kills.
A candidate stays listed only when a covering test kills every mutant it kills; one that kills a mutant they miss is listed as refuted, with those mutants.
One that kills no mutant, holds none in its blocks or does not pass unmutated is listed as not judged.
Each listed candidate comes with its standalone seconds, the tests that cover it and the mutants it killed; removable marks the set a greedy pass, slowest first, can delete together while every block stays covered and every mutant killed.
A test marked //testtiming:keep is neither a candidate nor a cover, and nor is a test the spawn scan marks or one whose entry records no passing run.
It reads the map as the last lym gate testmap call from this worktree left it.
It refuses a map built from other Go files of the package than this worktree holds.
It reads the keep markers from this worktree, and runs its builds and tests inside one gate slot, bounded by redundancy_build_timeout_min and redundancy_test_timeout_sec.
Each test run is capped at redundancy_test_memory_mb of memory; a mutant run past the cap or its time bound is a kill.
It prints the map's commit and build time, map_build_failure when a later coverage build failed, the candidates, the refuted tests, the kept tests with their reasons, the tests not judged with why, and what the mutation check cost.

Examples:

  lym gate redundant internal/foo`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), runRedundant(cmd.Context(), cmd.OutOrStdout(), redundantRequest{
				pkg: args[0], goBinary: goBinary, environ: os.Environ(), secondUnit: time.Second, minuteUnit: minuteUnit, grace: grace,
				cappedParallel: memoryCappedParallel,
			}))
			return nil
		},
	}
}

// runRedundant writes the redundancy candidates of the package the request names, confirmed by mutation.
// It reads them from the package's coverage map where `lym gate testmap` from the worktree of the seam cwd writes it.
// It returns the verb's exit code: 128 plus the signal number after a terminating signal.
func runRedundant(ctx context.Context, out io.Writer, request redundantRequest) int {
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate redundant: cannot read the working directory: %v; way forward: re-run the same command", err))
	}
	location, inHub, err := resolveHub(cwd)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate redundant: %v; way forward: re-run the same command", err))
	}
	if !inHub {
		return output.Err(out, fmt.Sprintf("gate redundant: %s is in no hub, and the coverage maps live in a hub; way forward: run lym gate redundant from a worktree of the hub", cwd))
	}
	dirs := fabricengine.ConfigDirs(location)
	cfg, err := gateslot.LoadConfig(dirs)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate redundant: cannot read the hub's gate config: %v; way forward: take the step it names, then re-run the same command", err))
	}
	cfg = cfg.WithDefaultDir(location.AnchorRel)
	module, err := redundancyModule(cfg, location.WorktreePath())
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate redundant: %v; way forward: declare tier0 or tier1 with its dir at the Go module's root with lym config gate from the prime, then re-run the same command", err))
	}

	mapsDir, err := coverageMapsDir(location)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate redundant: %v; way forward: re-run the same command", err))
	}

	pkg := path.Clean(filepath.ToSlash(request.pkg))
	rebuild := fmt.Sprintf("way forward: build it with lym gate testmap %s from this worktree, then re-run the same command", pkg)
	m, found, err := testcover.ReadMap(mapsDir, pkg)
	if err == nil && found {
		err = checkMapBlocks(m)
	}
	switch {
	case err != nil:
		return output.Err(out, fmt.Sprintf("gate redundant: the coverage map of %s is unreadable: %v; %s", pkg, err, rebuild))
	case !found:
		return output.Err(out, fmt.Sprintf("gate redundant: %s has no coverage map; %s", pkg, rebuild))
	case m.BuildFailure != "":
		return output.Err(out, fmt.Sprintf("gate redundant: the coverage map of %s records only a failed build: %s; %s", pkg, m.BuildFailure, rebuild))
	}
	pkgDir := filepath.Join(module.dir, filepath.FromSlash(pkg))
	keeps, err := testcover.ScanKeeps(pkgDir)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate redundant: the keep scan of %s failed: %v; way forward: fix the //testtiming:keep line it names, then re-run the same command", pkg, err))
	}

	sourceChecks, err := sourceCheckingTests(location.WorktreePath(), module, pkg)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate redundant: the source scan of %s failed: %v; way forward: fix the file or marker it names, then re-run the same command", pkg, err))
	}

	kept, notJudged := []redundantSkip{}, []redundantSkip{}
	verdicts := testcover.JudgeMap(m, keeps)
	var contained []int
	// covers holds the judged tests that can kill a mutant: every one but those whose check reads source files.
	covers := map[string]bool{}
	for i, verdict := range verdicts {
		test := m.Tests[i]
		if verdict.Reason == "" && !sourceChecks[test.Name] {
			covers[test.Name] = true
		}
		switch {
		case verdict.Reason == testcover.ReasonKept:
			kept = append(kept, redundantSkip{Name: test.Name, Reason: keeps[test.Name]})
		case verdict.Reason != "":
			notJudged = append(notJudged, redundantSkip{Name: test.Name, Reason: verdict.Reason})
		case verdict.Contained && sourceChecks[test.Name]:
			notJudged = append(notJudged, redundantSkip{Name: test.Name, Reason: testcover.ReasonSourceCheck})
		case verdict.Contained:
			contained = append(contained, i)
		}
	}
	answer := map[string]any{
		"package":      pkg,
		"map_commit":   m.Commit,
		"map_built_at": m.BuiltAt.Format(time.RFC3339),
		"tests":        len(m.Tests),
		"kept":         kept,
	}
	if m.Failure != nil {
		answer["map_build_failure"] = map[string]string{"commit": m.Failure.Commit, "error": m.Failure.Error}
	}
	candidates, refuted := []redundantCandidate{}, []redundantRefuted{}
	if len(contained) == 0 {
		answer["candidates"], answer["refuted"], answer["not_judged"] = candidates, refuted, notJudged
		return output.Ok(out, answer)
	}

	current, err := goFileHashes(workingTree{root: location.WorktreePath()}, packageDirOf(module.rel, pkg))
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate redundant: cannot hash the Go files of %s: %v; way forward: re-run the same command", pkg, err))
	}
	if changed := changedGoFiles(m.Files, current); len(changed) > 0 {
		return output.Err(out, fmt.Sprintf("gate redundant: the coverage map of %s was built from other Go files than this worktree holds: %s; %s", pkg, strings.Join(changed, ", "), rebuild))
	}
	hubEnv, err := envconfig.LoadConfig(dirs)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate redundant: cannot read the env config: %v; way forward: take the step it names, then re-run the same command", err))
	}

	runCtx, caught, stop := catchTerminatingSignals(ctx)
	defer stop()
	check := &mutationCheck{
		ctx: runCtx, request: request, location: location, pool: hubgeom.GateSlots(location), tiers: mappedTierConfigs(cfg),
		pkgDir: pkgDir, importPath: module.importPath(pkg), moduleDir: module.dir,
		buildTimeout: time.Duration(cfg.RedundancyBuildTimeoutMin) * request.minuteUnit,
		testTimeout:  time.Duration(cfg.RedundancyTestTimeoutSec) * request.secondUnit,
		testMemory:   cfg.RedundancyTestMemory(),
		env:          gateEnv(request.environ, hubEnv),
	}
	defer check.close()
	var judged []testcover.Verdict
	for _, i := range contained {
		judged = append(judged, verdicts[i])
	}
	mutantVerdicts, coverings, mutants, err := check.run(m, judged, covers)
	if err != nil {
		if runCtx.Err() != nil {
			output.Err(out, "gate redundant: the call was cancelled; way forward: re-run the same command")
			return exitAfterSignal(caught, 1)
		}
		return output.Err(out, fmt.Sprintf("gate redundant: the mutation check of %s failed: %v; way forward: fix the build or test failure it names, then re-run the same command", pkg, err))
	}

	for j, i := range contained {
		test, verdict, mutated := m.Tests[i], verdicts[i], mutantVerdicts[j]
		switch {
		case mutated.Reason != "":
			notJudged = append(notJudged, redundantSkip{Name: test.Name, Reason: mutated.Reason})
		case mutated.Redundant:
			candidates = append(candidates, redundantCandidate{
				Name: test.Name, Tier: test.Tier, Seconds: test.Seconds, Removable: verdict.Removable && mutated.Removable,
				Covering: coverings[j], MutantsKilled: len(mutated.Killed),
			})
		default:
			line := redundantRefuted{Name: test.Name, Tier: test.Tier, Seconds: test.Seconds, Covering: coverings[j]}
			for _, id := range mutated.Unique {
				line.Unique = append(line.Unique, redundantMutant{File: mutants[id].File, Line: mutants[id].Line, Operator: mutants[id].Operator})
			}
			refuted = append(refuted, line)
		}
	}
	answer["candidates"], answer["refuted"], answer["not_judged"] = candidates, refuted, notJudged
	answer["mutation"] = check.stats
	return output.Ok(out, answer)
}

// sourceCheckingTests returns the tests of the module package pkg whose check reads the module's source files:
// those marked `//lym:guard`, and those that reach the module's scan kit.
// root is the worktree root.
func sourceCheckingTests(root string, module goModule, pkg string) (map[string]bool, error) {
	readers, err := testcover.ScanSourceReaders(module.path, filepath.Join(module.dir, filepath.FromSlash(pkg)))
	if err != nil {
		return nil, err
	}
	dir := packageDirOf(module.rel, pkg)
	guards, err := impactset.FindGuardTests(root, []string{dir})
	if err != nil {
		return nil, err
	}
	for _, guard := range guards[dir] {
		readers[guard.Name] = true
	}
	return readers, nil
}

// changedGoFiles returns the sorted names of the Go files whose hash differs between the map and the worktree.
// A file on one side alone is included.
func changedGoFiles(mapped, current map[string]string) []string {
	var changed []string
	for name, hash := range current {
		if mapped[name] != hash {
			changed = append(changed, name)
		}
	}
	for name := range mapped {
		if _, found := current[name]; !found {
			changed = append(changed, name)
		}
	}
	slices.Sort(changed)
	return changed
}

// mapBlocksOf returns the rendered keys of the blocks the map's entry for test covers.
func mapBlocksOf(m testcover.Map, test testcover.MapTest) testcover.BlockSet {
	blocks := testcover.BlockSet{}
	for _, index := range test.Blocks {
		blocks[m.Blocks[index].String()] = struct{}{}
	}
	return blocks
}

// mapEntries returns the entries of m by test name.
func mapEntries(m testcover.Map) map[string]testcover.MapTest {
	entries := map[string]testcover.MapTest{}
	for _, test := range m.Tests {
		entries[test.Name] = test
	}
	return entries
}
