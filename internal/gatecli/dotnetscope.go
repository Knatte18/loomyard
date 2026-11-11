// dotnetscope.go computes the scope of a call under the dotnet scoper: the test projects and class filter terms each listed tier narrows to.
// It also names the selected test projects that no run of the call reaches.

package gatecli

import (
	"context"
	"fmt"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/dotnetimpact"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// dirGroup holds the tiers of a call that run in one directory, and that directory.
type dirGroup struct {
	dir   string
	tiers []int
}

// dotnetScope returns the scope of the computed, --base or --card form under the dotnet scoper.
// A computed or --base scope groups the listed tiers by base and derives once per base and tier directory;
// a --card scope derives once per tier directory from seeds, the C# seeds of the named cards.
// A derivation that cannot narrow gives its tiers a whole share, naming the reason on request.stderr.
// A narrowed share holds one module target per selected project;
// a derivation that finds no test class gives its tiers an empty share.
// The scope plans no guard step and no vet step.
// A cancelled ctx is returned as the derivation's error.
func dotnetScope(ctx context.Context, cfg gateslot.Config, location *lymcwd.Location, root string, request testRequest, form string, seeds []planparser.CSharpSeed, green *greenRun) (scope, error) {
	shares := map[int]tierScope{}
	if form == scopeCard {
		for _, group := range groupByDir(cfg, keptTiers(cfg, request)) {
			derived, err := dotnetimpact.DeriveCards(ctx, root, group.dir, seeds, cfg.Inert)
			if err != nil {
				return scope{}, fmt.Errorf("gate test: cannot derive the test projects of the cards: %w; way forward: re-run the same command", err)
			}
			assignShare(shares, group, derived, request)
		}
		return scope{form: form, tiers: shares}, nil
	}

	groups, wholeTiers := diffBaseGroups(cfg, location, root, request, form, green)
	for _, baseGroup := range groups {
		for _, group := range groupByDir(cfg, baseGroup.tiers) {
			derived, err := dotnetimpact.DeriveDiff(ctx, root, group.dir, baseGroup.base, cfg.Inert)
			if err != nil {
				return scope{}, fmt.Errorf("gate test: cannot derive the test projects of the diff from %s: %w; way forward: re-run the same command", baseGroup.base, err)
			}
			printLine(request.stderr, fmt.Sprintf("gate test: diff scope from %s for %s", baseGroup.base, tierNames(group.tiers)))
			assignShare(shares, group, derived, request)
		}
	}
	for _, number := range wholeTiers {
		shares[number] = tierScope{whole: true}
	}
	return scope{form: form, tiers: shares}, nil
}

// groupByDir groups numbers by the directory their tier runs in, in the order each directory first appears.
// A tier the config does not declare is left out, since the run planning refuses such a tier.
func groupByDir(cfg gateslot.Config, numbers []int) []dirGroup {
	var groups []dirGroup
	for _, number := range numbers {
		tier, err := cfg.Tier(number)
		if err != nil {
			continue
		}
		found := false
		for i := range groups {
			if groups[i].dir == tier.Dir {
				groups[i].tiers = append(groups[i].tiers, number)
				found = true
			}
		}
		if !found {
			groups = append(groups, dirGroup{dir: tier.Dir, tiers: []int{number}})
		}
	}
	return groups
}

// assignShare gives every tier of group the share that derived describes.
// On a fallback to whole, it prints the reason on request.stderr.
func assignShare(shares map[int]tierScope, group dirGroup, derived dotnetimpact.Scope, request testRequest) {
	share := tierScope{}
	switch {
	case derived.Whole:
		share.whole = true
		printLine(request.stderr, "gate test: the scope falls back to whole: "+derived.Fallback)
	case derived.NoTestClass:
		share.noTests = true
	default:
		for _, project := range derived.Projects {
			share.modules = append(share.modules, moduleTargets{dir: group.dir, targets: []string{project.Path}, terms: project.Terms})
		}
	}
	for _, number := range group.tiers {
		shares[number] = share
	}
}

// The extensions of the .NET files a tier command or target names, matched without regard to case.
var (
	projectFileExtensions   = []string{".csproj", ".fsproj", ".vbproj"}
	solutionFileExtensions  = []string{".sln", ".slnx"}
	builtAssemblyExtensions = []string{".dll", ".exe"}
)

// unrunProjectLines returns one stderr line per test project of sc that no run of runs reaches, in path order.
// It reads only the projects a narrowed share selected for a run whose command holds no target placeholder.
// Such a run runs whole and is passed none of them.
// Each line says the pass is not green for its project.
// Its way forward runs the lowest declared tier, not manual, whose command reaches the project.
// With no such tier it names the prime's `lym config gate` step of adding {targets} to a tier's command.
func unrunProjectLines(cfg gateslot.Config, runs []tierRun, sc scope) []string {
	var projects []string
	for _, run := range runs {
		if !gateslot.HoldsTargets(run.tier.Command) {
			for _, module := range sc.forTier(run.number).modules {
				projects = append(projects, module.targets...)
			}
		}
	}
	slices.Sort(projects)
	var lines []string
	for _, project := range slices.Compact(projects) {
		if slices.ContainsFunc(runs, func(run tierRun) bool { return runReachesProject(run, project) }) {
			continue
		}
		wayForward := "no declared tier's command reaches it, so the prime adds {targets} to a tier's command with \"lym config gate\", and the same command runs again"
		if number, holdsTargets, found := lowestTierReaching(cfg, project); found {
			call := "lym gate test --tier " + strconv.Itoa(number)
			if holdsTargets {
				call += " " + project
			}
			wayForward = "run \"" + call + "\""
		}
		lines = append(lines, fmt.Sprintf("gate test: the scope selected tests in %s, and no tier this call runs reaches that project, so this pass is not green for it; way forward: %s", project, wayForward))
	}
	return lines
}

// runReachesProject reports whether run runs the test project at projectPath.
// A run with no invocation runs nothing.
// A run whose command takes targets runs the projects its invocations' targets name;
// any other run runs those its command's words name.
func runReachesProject(run tierRun, projectPath string) bool {
	if len(run.invocations) == 0 {
		return false
	}
	if !gateslot.HoldsTargets(run.tier.Command) {
		return namesProject(strings.Fields(run.tier.Command), projectPath)
	}
	return slices.ContainsFunc(run.invocations, func(inv invocation) bool { return namesProject(inv.targets, projectPath) })
}

// lowestTierReaching returns the lowest declared tier, not manual, whose command reaches the project at projectPath.
// holdsTargets reports whether that command takes targets, which reach every project.
// found is false when no such tier exists.
func lowestTierReaching(cfg gateslot.Config, projectPath string) (number int, holdsTargets, found bool) {
	numbers := make([]int, 0, len(cfg.Tiers))
	for key := range cfg.Tiers {
		parsed, _ := strconv.Atoi(strings.TrimPrefix(key, "tier"))
		numbers = append(numbers, parsed)
	}
	sort.Ints(numbers)
	for _, candidate := range numbers {
		tier := cfg.Tiers[tierName(candidate)]
		switch {
		case tier.Manual:
		case gateslot.HoldsTargets(tier.Command):
			return candidate, true, true
		case namesProject(strings.Fields(tier.Command), projectPath):
			return candidate, false, true
		}
	}
	return 0, false, false
}

// namesProject reports whether words, a command's words or a run's targets, run the test project at projectPath.
// Words naming a solution file, or naming no project, solution or built assembly file, count as running it:
// lym reads neither a solution, a built assembly's project, nor the directory a bare `dotnet test` runs in.
// A built assembly names a specific target of its own and runs the project it is named after,
// so a command naming only other assemblies runs no project.
// Otherwise the words run it when one of the project files they name is that project.
func namesProject(words []string, projectPath string) bool {
	project := strings.ToLower(strings.ReplaceAll(projectPath, `\`, "/"))
	namesAny := false
	for _, word := range words {
		word = strings.ToLower(strings.ReplaceAll(strings.Trim(word, `"'`), `\`, "/"))
		extension := path.Ext(word)
		switch {
		case slices.Contains(solutionFileExtensions, extension):
			return true
		case slices.Contains(builtAssemblyExtensions, extension):
			namesAny = true
			if path.Base(strings.TrimSuffix(word, extension)) == strings.TrimSuffix(path.Base(project), path.Ext(project)) {
				return true
			}
		case slices.Contains(projectFileExtensions, extension):
			namesAny = true
			if isSameProjectFile(word, project) {
				return true
			}
		}
	}
	return !namesAny
}

// isSameProjectFile reports whether word, a project file named by a command, is the project at projectPath.
// Both are lowercase and slash-separated.
// A bare file name matches the project's file name.
// A word with a directory part matches when one path is the other, or ends at a directory boundary with it.
// A command word and a project path may be relative to different directories.
func isSameProjectFile(word, projectPath string) bool {
	word, projectPath = path.Clean(word), path.Clean(projectPath)
	if !strings.Contains(word, "/") {
		return path.Base(projectPath) == word
	}
	return word == projectPath || strings.HasSuffix(projectPath, "/"+word) || strings.HasSuffix(word, "/"+projectPath)
}
