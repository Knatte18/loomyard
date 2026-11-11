// coveragescope.go narrows a computed diff share under a gate key of scope coverage.
// A changed package runs whole, and an importing package runs only the tests its coverage map selects.

package gatecli

import (
	"context"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testcover"
)

// The labels the scope report gives a package of a coverage-scoped tier, beside a miss's and a selection's own.
const (
	coverageChanged    = "changed"
	coverageLymBinary  = "whole: lym binary"
	coverageOverBudget = "whole: over budget"
	coverageNone       = "none selected"
)

// selectionSource returns the selection of the importing package at a module-relative slash directory.
type selectionSource func(pkg string) testcover.Selection

// coveragePackage is one package of a coverage-scoped tier, as the scope report names it.
type coveragePackage struct {
	// module is the directory of the package's module, and target its module-relative package argument.
	module, target string
	label          string
	// whole is set when the package runs every test of the tier.
	whole bool
	// tests are the names the package runs by, when it runs neither whole nor nothing.
	tests []string
}

// coverageTier returns the tier numbered number when a coverage scope narrows its share, and whether it does.
// A call that is not of scope coverage, or whose words after `--` carry a filter flag, narrows no tier.
// A tier the hub does not declare is not narrowed.
// Nor is a tier whose command lacks {args} or a target placeholder.
// Nor is a tier with carry: false, which runs whole.
func coverageTier(cfg gateslot.Config, request testRequest, number int) (gateslot.Tier, bool) {
	if !request.coverage || slices.ContainsFunc(request.args, isFilterFlag) {
		return gateslot.Tier{}, false
	}
	tier, err := cfg.Tier(number)
	if err != nil || tier.NoCarry || !gateslot.HoldsTargets(tier.Command) || !strings.Contains(tier.Command, "{args}") {
		return gateslot.Tier{}, false
	}
	return tier, true
}

// narrowCoverage returns share, the diff share targets derive, narrowed for the tier numbered number.
// It also returns the scope report's line for each package of the share.
// A changed package and a binary builder of targets run whole.
// An importing package runs the union of its Test functions of the tier that match one of four rules.
// selectionOf selects the function.
// The map does not record it.
// The map marks it out of process or unresolved.
// It carries `//lym:guard`.
// The union holds only the tests a run of the tier executes.
// Those are declared in a test file the host admits under the tier's tag.
// It drops the tests targets leave unwatched in the package.
// A selection miss, a test file that cannot be read, or a guard marker the scan refuses runs the package whole.
// A package whose union is empty runs nothing.
// A union whose anchored names pass greenCommandBudget runs every selected package whole.
// root is the worktree root the module directories resolve against.
func narrowCoverage(share tierScope, targets impactset.Targets, root string, number int, tier gateslot.Tier, selectionOf selectionSource) (tierScope, []string) {
	tiers := append([]int{number}, tier.Covers...)
	tagged := build.Default
	tagged.BuildTags = []string{tierName(number)}

	var packages []coveragePackage
	for _, module := range targets.Modules {
		for _, target := range module.Packages {
			entry := coveragePackage{module: module.Dir, target: target, label: coverageChanged, whole: true}
			switch {
			case slices.Contains(module.BinaryBuilders, target):
				entry.label = coverageLymBinary
			case slices.Contains(module.Importers, target):
				entry = importerPackage(entry, tagged, tiers, root, targets, selectionOf)
			}
			packages = append(packages, entry)
		}
	}

	var names []string
	for _, entry := range packages {
		names = append(names, entry.tests...)
	}
	slices.Sort(names)
	names = slices.Compact(names)
	if len(anchoredPattern(names)) > greenCommandBudget {
		for i := range packages {
			if len(packages[i].tests) > 0 {
				packages[i] = coveragePackage{module: packages[i].module, target: packages[i].target, label: coverageOverBudget, whole: true}
			}
		}
		names = nil
	}

	narrowed := tierScope{skip: share.skip, selectedTests: names}
	lines := make([]string, len(packages))
	for i, entry := range packages {
		lines[i] = fmt.Sprintf("gate test: %s coverage: %s %s", tierName(number), entry.target, entry.label)
		switch {
		case entry.whole:
			narrowed.modules = appendTarget(narrowed.modules, entry.module, entry.target)
		case len(entry.tests) > 0:
			narrowed.selected = appendTarget(narrowed.selected, entry.module, entry.target)
		}
	}
	return narrowed, lines
}

// importerPackage returns entry, an importing package of targets, with the tests it runs and its label.
func importerPackage(entry coveragePackage, tagged build.Context, tiers []int, root string, targets impactset.Targets, selectionOf selectionSource) coveragePackage {
	pkg := path.Clean(strings.TrimPrefix(entry.target, "./"))
	dir := path.Join(entry.module, pkg)
	selection := selectionOf(pkg)
	if selection.Miss != nil {
		entry.label = "miss: " + selection.Miss.String()
		return entry
	}
	declared, err := tierTestNames(tagged, filepath.Join(root, filepath.FromSlash(dir)), tiers, targets.Unwatched[dir])
	var guards map[string][]impactset.GuardTest
	if err == nil {
		guards, err = impactset.FindGuardTests(root, []string{dir})
	}
	switch {
	case impactset.IsGuardScanError(err):
		entry.label = "miss: " + err.Error()
		return entry
	case err != nil:
		entry.label = "miss: " + testcover.Miss{Reason: testcover.MissParse, Detail: err.Error()}.String()
		return entry
	}
	entry.tests = coverageUnion(declared, selection, guards[dir])
	entry.whole = false
	entry.label = coverageNone
	if len(entry.tests) > 0 {
		entry.label = fmt.Sprintf("selected %d of %d", len(entry.tests), len(declared))
	}
	return entry
}

// coverageUnion returns the names of declared, sorted, that the selection selects.
// It adds the names its map does not record or marks out of process or unresolved, and the names guards holds.
func coverageUnion(declared []string, selection testcover.Selection, guards []impactset.GuardTest) []string {
	recorded, marked := map[string]bool{}, map[string]bool{}
	for _, test := range selection.Recorded {
		recorded[test.Name] = true
		marked[test.Name] = test.Spawn.OutOfProcess || test.Spawn.Unresolved
	}
	var union []string
	for _, name := range declared {
		guarded := slices.ContainsFunc(guards, func(guard impactset.GuardTest) bool { return guard.Name == name })
		if guarded || !recorded[name] || marked[name] || slices.Contains(selection.Tests, name) {
			union = append(union, name)
		}
	}
	return union
}

// tierTestNames returns the Test functions a run of tiers executes from the test files directly in dir, sorted.
// It reads only the files tagged admits on the host, and leaves out the names in skipped.
// A function opening with `tiergate.Require(t, N)` counts when tiers holds N, and any other counts always.
// A directory or test file that cannot be read, matched or parsed is an error.
func tierTestNames(tagged build.Context, dir string, tiers []int, skipped []string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, "_test.go") {
			continue
		}
		matches, err := tagged.MatchFile(dir, name)
		if err != nil {
			return nil, err
		}
		if !matches {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !isTestName(fn.Name.Name) || slices.Contains(skipped, fn.Name.Name) {
				continue
			}
			if tier, required := impactset.RequiredTier(file, fn); !required || slices.Contains(tiers, tier) {
				names = append(names, fn.Name.Name)
			}
		}
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}

// appendTarget returns modules with target added to the module at dir, which joins last when modules lacks it.
func appendTarget(modules []moduleTargets, dir, target string) []moduleTargets {
	index := slices.IndexFunc(modules, func(module moduleTargets) bool { return module.dir == dir })
	if index < 0 {
		return append(modules, moduleTargets{dir: dir, targets: []string{target}})
	}
	modules[index].targets = append(modules[index].targets, target)
	return modules
}

// anchoredPattern returns the go test filter that matches exactly the top-level tests names.
func anchoredPattern(names []string) string {
	return "^(" + strings.Join(names, "|") + ")$"
}

// coverageSelections returns the selection source of a coverage scope over the worktree at root.
// It opens the selector at its first use, so a call with no importing package reads nothing,
// and it caches each package's selection, which every tier and base group of the call shares.
// A selector that cannot open, or a selection whose working files cannot be read, is a miss.
func coverageSelections(ctx context.Context, cfg gateslot.Config, location *lymcwd.Location, root string, request testRequest) selectionSource {
	cache := map[string]testcover.Selection{}
	var (
		selector  *testcover.Selector
		setupMiss *testcover.Miss
		opened    bool
	)
	return func(pkg string) testcover.Selection {
		if cached, ok := cache[pkg]; ok {
			return cached
		}
		if !opened {
			opened = true
			selector, setupMiss = openSelector(ctx, cfg, location, root, request)
		}
		selection := testcover.Selection{Miss: setupMiss}
		if selector != nil {
			var err error
			if selection, err = selector.Select(pkg); err != nil {
				selection = testcover.Selection{Miss: &testcover.Miss{Reason: testcover.MissMapUnreadable, Detail: err.Error()}}
			}
		}
		cache[pkg] = selection
		return selection
	}
}

// openSelector returns the selector over the hub's coverage maps and the worktree at root.
// It reads the working tree's files as HEAD's tree and the uncommitted and untracked paths, through fabric.
// It returns instead the miss every importing package takes: outside a hub there are no maps,
// and a working tree or toolchain that cannot be read leaves the maps nothing to compare with.
func openSelector(ctx context.Context, cfg gateslot.Config, location *lymcwd.Location, root string, request testRequest) (*testcover.Selector, *testcover.Miss) {
	if request.coverageMaps == "" {
		return nil, &testcover.Miss{Reason: testcover.MissNoMap, Detail: "the worktree is in no hub, which holds the coverage maps"}
	}
	unreadable := func(err error) (*testcover.Selector, *testcover.Miss) {
		return nil, &testcover.Miss{Reason: testcover.MissMapUnreadable, Detail: "cannot read the working tree to compare the maps with: " + err.Error()}
	}
	fabric, err := fabricengine.Open(location)
	if err != nil {
		return unreadable(err)
	}
	head, err := fabric.HeadSHA()
	if err != nil {
		return unreadable(err)
	}
	files, err := fabric.CodeTreeAt(head, ".")
	if err != nil {
		return unreadable(err)
	}
	changes, err := fabric.CodeWorktreeChanges()
	if err != nil {
		return unreadable(err)
	}
	files = append(files, changes...)
	slices.Sort(files)
	goRoot, err := goRootOf(ctx, request.goBinary, root)
	if err != nil {
		return unreadable(err)
	}
	return testcover.NewSelector(testcover.SelectorConfig{
		MapsDir:      request.coverageMaps,
		WorktreeRoot: root,
		ModuleRel:    ".",
		ModulePath:   modulePathOf(root),
		GoRoot:       goRoot,
		WorkingFiles: slices.Compact(files),
		Commits:      fabric,
		Inert:        cfg.Inert,
	}), nil
}

// goRootOf returns the root of the Go toolchain goBinary runs, as `go env GOROOT` prints it in dir.
func goRootOf(ctx context.Context, goBinary, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, goBinary, "env", "GOROOT")
	cmd.Dir = dir
	logger.Info("gate test: spawning go env GOROOT", "dir", dir)
	out, err := cmd.Output()
	logger.Info("gate test: go env GOROOT exited", "dir", dir, "err", err)
	if err != nil {
		return "", fmt.Errorf("go env GOROOT: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
