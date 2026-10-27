// scope.go computes the scope of one `lym gate test` call: which targets each listed tier runs over, and which guard tests the guard step runs, for the computed, --base and --card forms.

package gatecli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"go/build"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// The scope forms computeScope adds to the explicit-targets and --all forms.
const (
	scopeComputed = "computed"
	scopeBase     = "base"
	scopeCard     = "card"
)

// moduleTargets is the Go package arguments one module holds in a narrower scope, or the test project one dotnet share holds.
type moduleTargets struct {
	// dir is the module directory relative to the worktree root, "." for the root module.
	dir string
	// targets are the module-relative package arguments, or the one project path of a dotnet share.
	targets []string
	// terms are the class filter terms of a dotnet share's project; nil under every other scoper.
	terms []string
}

// tierScope is the narrowing one tier runs under when a diff scope takes its base per tier.
type tierScope struct {
	// whole means the tier runs over its all_targets.
	whole bool
	// modules are the tier's per-module targets; empty for the empty set.
	modules []moduleTargets
	// noTests means the dotnet scoper found changes but no test class naming them.
	noTests bool
}

// scope is what a call's tiers run over.
type scope struct {
	// form labels the scope in each tier's summary line and run record.
	form string
	// whole means every listed tier runs over its all_targets.
	whole bool
	// modules are the per-module targets of a narrower scope; empty with whole, and empty for the empty set, which runs no listed tier's command.
	modules []moduleTargets
	// noTests means the dotnet scoper found changes but no test class naming them; set from the tier's share.
	noTests bool
	// tiers holds the share of each listed tier of a diff scope, which derives one impacted set per base; a tier it does not name takes whole and modules.
	tiers map[int]tierScope
	// guardStep means the scope form plans a guard step: a computed, --base or --card scope under the go scoper.
	guardStep bool
	// guardDir is the run directory of that guard step, relative to the worktree root; empty means the dir of each guard group's own tier.
	guardDir string
	// guards maps a tier number to the directory of each package outside the scope that holds guard tests of that tier, and to their names, keyed relative to guardDir; nil when no scan has run.
	guards map[int]map[string][]string
	// tagTiers are the declared tiers, ascending, whose build tag brings a file into a --card scope's package that a build without tags leaves out; the vet step is planned over them.
	tagTiers []int
}

// forTier returns s with the narrowing of tier number in place of its own whole, modules and noTests.
func (s scope) forTier(number int) scope {
	if share, ok := s.tiers[number]; ok {
		s.whole, s.modules, s.noTests = share.whole, share.modules, share.noTests
	}
	return s
}

// withGateScope returns request with the scope form its --gate key names in place of the scope flags.
// A key of scope all is the --all form, a key of scope diff is the computed form, and a key of scope card keeps the --card ids, so the --card ids beside the other two scopes are dropped.
// Explicit targets replace the key's scope and a call with no key is returned unchanged.
func withGateScope(cfg gateslot.Config, request testRequest) testRequest {
	if request.gate == "" || len(request.targets) > 0 {
		return request
	}
	switch cfg.Gates[request.gate].Scope {
	case "all":
		request.all = true
		request.cards = nil
	case "diff":
		request.cards = nil
	}
	return request
}

// computeScope returns the scope of request over the worktree at root, for the config cfg.
// Explicit targets and --all take no computation.
// Otherwise the form is --card when a card is named, else --base, else the computed form, whose base is taken per listed tier.
// A computed tier's base is the newest commit that is an ancestor of HEAD and that green holds whole-green for the tier, else the merge base of HEAD with the parent branch the pair's fabric origin record names, and a tier with neither runs whole.
// green is nil for a dirty worktree, which so takes the merge base.
// A --card is read from the plan first, under either scoper, and an unknown id or a worktree with no plan is refused before any slot.
// Under the none scoper each of the three forms is whole, except that a hub with inert_paths gives the tiers of a computed or --base base group whose diff is inert-only the empty share, which plans no guard step.
// Under the dotnet scoper dotnetScope derives them.
// Under the go scoper a computed or --base scope is the impacted set of the diff from its base to the working tree, derived once per base and shared by the tiers that land on it, each derivation named on request.stderr;
// a base group whose diff is inert-only under inert_paths gets the empty share and is not derived, and leaves the guard tests to the module scan as a whole tier does.
// A set that cannot be narrowed falls back to whole, naming the reason on request.stderr;
// a --card scope is the cards' packages per module, with the declared tiers whose build tag brings a file into one of them.
// A guard scan error refuses with its file, line and way forward.
// ctx bounds the derivation, so a stalled `go list` is killed when it ends.
func computeScope(ctx context.Context, cfg gateslot.Config, location *lymcwd.Location, root string, request testRequest, green *greenRun) (scope, error) {
	request = withGateScope(cfg, request)
	switch {
	case len(request.targets) > 0:
		return scope{form: scopeTargets}, nil
	case request.all:
		return scope{form: scopeAll, whole: true}, nil
	}
	form := scopeComputed
	if request.base != "" {
		form = scopeBase
	}
	var (
		cardModules []planparser.ModulePackages
		cardSeeds   []planparser.CSharpSeed
	)
	if len(request.cards) > 0 {
		form = scopeCard
		if cfg.Scoper == "dotnet" {
			plan, cards, err := readCards(location, request.cards)
			if err != nil {
				return scope{}, err
			}
			cardSeeds = planparser.CardCSharpSeeds(plan, cards)
		} else {
			var err error
			if cardModules, err = cardPackages(location, root, request.cards); err != nil {
				return scope{}, err
			}
		}
	}
	if cfg.Scoper == "dotnet" {
		return dotnetScope(ctx, cfg, location, root, request, form, cardSeeds, green)
	}
	if cfg.Scoper == "none" && form != scopeCard && len(cfg.InertPaths) > 0 {
		return noneScope(ctx, cfg, location, root, request, form, green), nil
	}
	if cfg.Scoper != "go" {
		return scope{form: form, whole: true}, nil
	}

	tier0, err := cfg.Tier(0)
	if err != nil {
		return scope{}, fmt.Errorf("gate test: %w", err)
	}
	if form == scopeCard {
		modules := make([]moduleTargets, len(cardModules))
		for i, module := range cardModules {
			modules[i] = moduleTargets{dir: module.Dir, targets: module.Packages}
		}
		return scope{form: form, modules: modules, guardStep: true, guardDir: tier0.Dir, tagTiers: tagTiers(cfg, root, modules)}, nil
	}

	groups, wholeTiers := diffBaseGroups(cfg, location, root, request, form, green)
	shares := map[int]tierScope{}
	var guards map[string][]impactset.GuardTest
	narrowed := false
	anyWhole := len(wholeTiers) > 0
	for _, group := range groups {
		if allInertSince(ctx, cfg, root, group.base) {
			printLine(request.stderr, fmt.Sprintf("gate test: diff scope from %s for %s: every changed file is inert, so the scope is empty", group.base, tierNames(group.tiers)))
			// An inert-only group counts as narrowed and, like a whole tier, leaves the guard tests to the module scan.
			narrowed, anyWhole = true, true
			for _, number := range group.tiers {
				shares[number] = tierScope{}
			}
			continue
		}
		targets, err := impactset.DeriveTargets(ctx, root, group.base)
		if err != nil {
			if impactset.IsGuardScanError(err) {
				return scope{}, errors.New(guardScanRefusal(root, err))
			}
			return scope{}, fmt.Errorf("gate test: cannot derive the impacted set of the diff from %s: %w; way forward: re-run the same command", group.base, err)
		}
		printLine(request.stderr, fmt.Sprintf("gate test: diff scope from %s for %s", group.base, tierNames(group.tiers)))
		share := tierScope{whole: true}
		if targets.Whole {
			anyWhole = true
			printLine(request.stderr, "gate test: the scope falls back to whole: "+targets.Fallback)
		} else {
			narrowed = true
			share = tierScope{modules: make([]moduleTargets, len(targets.Modules))}
			for i, module := range targets.Modules {
				share.modules[i] = moduleTargets{dir: module.Dir, targets: module.Packages}
			}
			guards = mergeGuards(guards, targets.Guards)
		}
		for _, number := range group.tiers {
			shares[number] = share
		}
	}
	for _, number := range wholeTiers {
		shares[number] = tierScope{whole: true}
	}
	switch {
	case !narrowed:
		return scope{form: form, whole: true, guardStep: true, guardDir: tier0.Dir}, nil
	case anyWhole:
		// A tier that runs whole leaves the guard tests outside the scope to the module scan.
		return scope{form: form, tiers: shares, guardStep: true, guardDir: tier0.Dir}, nil
	}
	return scope{form: form, tiers: shares, guardStep: true, guardDir: ".", guards: guardsByTier(guards)}, nil
}

// noneScope returns the computed or --base scope under the none scoper for a hub with inert_paths:
// the tiers of a base group whose diff is inert-only get the empty share, and every other tier runs whole.
// A call with no inert-only group is the whole scope, and no scope of this scoper plans a guard step.
// A diff that cannot be read leaves its group whole, since allInertSince then reports false.
func noneScope(ctx context.Context, cfg gateslot.Config, location *lymcwd.Location, root string, request testRequest, form string, green *greenRun) scope {
	groups, wholeTiers := diffBaseGroups(cfg, location, root, request, form, green)
	shares := map[int]tierScope{}
	inertOnly := false
	for _, group := range groups {
		share := tierScope{whole: true}
		if allInertSince(ctx, cfg, root, group.base) {
			printLine(request.stderr, fmt.Sprintf("gate test: diff scope from %s for %s: every changed file is inert, so the scope is empty", group.base, tierNames(group.tiers)))
			share = tierScope{}
			inertOnly = true
		}
		for _, number := range group.tiers {
			shares[number] = share
		}
	}
	if !inertOnly {
		return scope{form: form, whole: true}
	}
	for _, number := range wholeTiers {
		shares[number] = tierScope{whole: true}
	}
	return scope{form: form, tiers: shares}
}

// allInertSince reports whether every file that differs between base and the working tree, untracked files included, matches the hub's inert_paths.
// A hub with no inert_paths, an empty diff and a diff that cannot be read report false, the last logged at Warn, so the scope stays as it would be.
func allInertSince(ctx context.Context, cfg gateslot.Config, root, base string) bool {
	if len(cfg.InertPaths) == 0 {
		return false
	}
	paths, err := impactset.ChangedPaths(ctx, root, base)
	if err != nil {
		logger.Warn("gate test: could not read the changed files to check them against inert_paths; the scope stays as it is", "base", base, "cause", err)
		return false
	}
	return cfg.AllInert(paths)
}

// baseGroup is the tiers of a call that diff from one base.
type baseGroup struct {
	base  string
	tiers []int
}

// diffBaseGroups returns the tiers request lists, grouped by the base their diff starts from in the order each base first appears, and the tiers with no base, which run whole.
// A --base call gives every tier its one base.
// Under the computed form a tier's base is the newest commit the green record holds whole-green for it that is an ancestor of HEAD, else the merge base with the parent branch, and a tier with neither runs whole.
// A dirty worktree holds no green record, so every tier takes the merge base.
func diffBaseGroups(cfg gateslot.Config, location *lymcwd.Location, root string, request testRequest, form string, green *greenRun) (groups []baseGroup, wholeTiers []int) {
	numbers := keptTiers(cfg, request)

	if form == scopeBase {
		return []baseGroup{{base: request.base, tiers: numbers}}, nil
	}

	var (
		parent     string
		parentRead bool
		parentOK   bool
	)
	parentMerge := func() (string, bool) {
		if parentRead {
			return parent, parentOK
		}
		parentRead = true
		var err error
		if parent, parentOK, err = parentMergeBase(location); err != nil {
			printLine(request.stderr, fmt.Sprintf("gate test: the computed scope falls back to whole: cannot read the merge base with the parent branch: %v", err))
			parentOK = false
		}
		return parent, parentOK
	}
	isAncestor := func(commit, of string) bool {
		held, err := gitrepo.New(root).IsAncestor(commit, of)
		return err == nil && held
	}

	if len(numbers) == 0 {
		if base, ok := parentMerge(); ok {
			groups = append(groups, baseGroup{base: base})
		}
		return groups, nil
	}
	for _, number := range numbers {
		var base string
		ok := false
		if tier, err := cfg.Tier(number); err == nil && green != nil {
			base, ok = green.record.newestWholeAncestor(green.fingerprint(tier), moduleKey(root, tier.Dir), isAncestor)
		}
		if !ok {
			base, ok = parentMerge()
		}
		if !ok {
			wholeTiers = append(wholeTiers, number)
			continue
		}
		if index := slices.IndexFunc(groups, func(group baseGroup) bool { return group.base == base }); index >= 0 {
			groups[index].tiers = append(groups[index].tiers, number)
		} else {
			groups = append(groups, baseGroup{base: base, tiers: []int{number}})
		}
	}
	return groups, wholeTiers
}

// keptTiers returns the tiers request lists that no other listed tier covers, ascending, naming nothing on stderr.
func keptTiers(cfg gateslot.Config, request testRequest) []int {
	silent := request
	silent.stderr = nil
	listed := listedTiers(cfg, silent)
	declared := make(map[int]gateslot.Tier, len(listed))
	for _, number := range listed {
		if tier, err := cfg.Tier(number); err == nil {
			declared[number] = tier
		}
	}
	return dropCovered(listed, declared, silent)
}

// tierNames returns the names of the tiers numbers, joined by commas.
func tierNames(numbers []int) string {
	names := make([]string, len(numbers))
	for i, number := range numbers {
		names[i] = tierName(number)
	}
	return strings.Join(names, ", ")
}

// mergeGuards returns the union of the guard tests per directory of a and b, sorted by name.
func mergeGuards(a, b map[string][]impactset.GuardTest) map[string][]impactset.GuardTest {
	merged := map[string][]impactset.GuardTest{}
	for _, source := range []map[string][]impactset.GuardTest{a, b} {
		for dir, tests := range source {
			combined := slices.Concat(merged[dir], tests)
			slices.SortFunc(combined, func(x, y impactset.GuardTest) int {
				return cmp.Or(strings.Compare(x.Name, y.Name), x.Tier-y.Tier)
			})
			merged[dir] = slices.Compact(combined)
		}
	}
	return merged
}

// guardsByTier groups guard tests by their file's tier, each group keyed by package directory with the test names.
// The result is never nil, so a scope holding it counts as scanned.
func guardsByTier(guards map[string][]impactset.GuardTest) map[int]map[string][]string {
	byTier := map[int]map[string][]string{}
	for dir, tests := range guards {
		for _, test := range tests {
			if byTier[test.Tier] == nil {
				byTier[test.Tier] = map[string][]string{}
			}
			byTier[test.Tier][dir] = append(byTier[test.Tier][dir], test.Name)
		}
	}
	return byTier
}

// guardScanDir returns the directory of the module the guard scan lists, relative to the worktree root unless absolute:
// the scope's own guard directory, else the -C directory of explicit targets, else tier0's dir.
func guardScanDir(cfg gateslot.Config, request testRequest, sc scope) (string, error) {
	switch {
	case sc.guardDir != "":
		return sc.guardDir, nil
	case request.dir != "":
		return request.dir, nil
	}
	tier0, err := cfg.Tier(0)
	if err != nil {
		return "", fmt.Errorf("gate test: %w", err)
	}
	return tier0.Dir, nil
}

// scanGuards fills sc.guards with the guard tests of the module at sc.guardDir outside the scope's packages, grouped by their file's tier, by a scan of that module.
// A whole scope and an explicit-targets scope hold no package of the scan, so every guard test counts as outside.
// A scan error is returned as the refusal for it.
func scanGuards(sc scope, root string) (scope, error) {
	moduleDir := resolveRunDir(root, sc.guardDir)
	all, err := impactset.ModuleGuardTests(moduleDir)
	if err != nil {
		return scope{}, errors.New(guardScanRefusal(moduleDir, err))
	}
	inside := map[string]bool{}
	for _, module := range sc.modules {
		if path.Clean(module.dir) != path.Clean(sc.guardDir) {
			continue
		}
		for _, target := range module.targets {
			inside[strings.TrimPrefix(path.Clean(target), "./")] = true
		}
	}
	outside := map[string][]impactset.GuardTest{}
	for dir, tests := range all {
		if len(tests) > 0 && !inside[path.Clean(dir)] {
			outside[dir] = tests
		}
	}
	sc.guards = guardsByTier(outside)
	return sc, nil
}

// cardPackages returns the Go packages of the named cards per module, read from the plan at the worktree's anchor by readCards.
func cardPackages(location *lymcwd.Location, root string, ids []string) ([]planparser.ModulePackages, error) {
	plan, cards, err := readCards(location, ids)
	if err != nil {
		return nil, err
	}
	return planparser.CardPackages(plan, cards, root), nil
}

// readCards reads the plan at the worktree's anchor and returns it with the named cards, in the order of ids.
// An id is a card's number, or its number and slug as `N-slug`, with leading zeros ignored.
// An unknown id is refused naming the plan's ids, and a worktree with no plan is refused naming --all and explicit targets.
func readCards(location *lymcwd.Location, ids []string) (*planparser.Plan, []planparser.Card, error) {
	planDir := planparser.PlanDir(location.AnchorPath())
	if _, err := os.Stat(planparser.PlanOverview(location.AnchorPath())); errors.Is(err, fs.ErrNotExist) {
		return nil, nil, fmt.Errorf("gate test: --card needs the plan at %s, which this worktree does not hold; way forward: run the call from the task worktree that holds the plan, or use --all or explicit targets, then re-run the same command", planDir)
	}
	plan, err := planparser.ParsePlan(planDir)
	if err != nil {
		return nil, nil, fmt.Errorf("gate test: cannot read the plan at %s: %w; way forward: fix the plan, or use --all or explicit targets, then re-run the same command", planDir, err)
	}
	var cards []planparser.Card
	for _, id := range ids {
		index := slices.IndexFunc(plan.Cards, func(card planparser.Card) bool { return cardMatches(card, id) })
		if index < 0 {
			known := make([]string, len(plan.Cards))
			for i, card := range plan.Cards {
				known[i] = card.ID()
			}
			return nil, nil, fmt.Errorf("gate test: --card %q names no card of the plan; way forward: pass one of %s, then re-run the same command", id, strings.Join(known, ", "))
		}
		cards = append(cards, plan.Cards[index])
	}
	return plan, cards, nil
}

// cardMatches reports whether id names card: its number alone, or its number and slug as `N-slug`, leading zeros ignored.
func cardMatches(card planparser.Card, id string) bool {
	number, slug, hasSlug := strings.Cut(id, "-")
	parsed, err := strconv.Atoi(number)
	if err != nil || parsed != card.Number {
		return false
	}
	return !hasSlug || slug == card.Slug
}

// tagTiers returns the tiers cfg declares, ascending, whose build tag `tierN` alone brings a `.go` file of a package of modules into the build that a build without tags leaves out, on the host's GOOS and GOARCH.
// Package directories resolve against root, and a directory or file that cannot be read or matched is left out, since the listed tiers' own compiles report it.
func tagTiers(cfg gateslot.Config, root string, modules []moduleTargets) []int {
	var dirs []string
	for _, module := range modules {
		for _, target := range module.targets {
			dirs = append(dirs, filepath.Join(root, filepath.FromSlash(module.dir), filepath.FromSlash(target)))
		}
	}
	var numbers []int
	for key := range cfg.Tiers {
		number, err := strconv.Atoi(strings.TrimPrefix(key, "tier"))
		if err != nil {
			continue
		}
		tagged := build.Default
		tagged.BuildTags = []string{tierName(number)}
		if slices.ContainsFunc(dirs, func(dir string) bool { return tagAddsFile(tagged, dir) }) {
			numbers = append(numbers, number)
		}
	}
	slices.Sort(numbers)
	return numbers
}

// tagAddsFile reports whether dir directly holds a `.go` file that matches the build tags of tagged and that a build without tags leaves out.
func tagAddsFile(tagged build.Context, dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	plain := build.Default
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		withTag, err := tagged.MatchFile(dir, name)
		if err != nil || !withTag {
			continue
		}
		if withoutTag, err := plain.MatchFile(dir, name); err == nil && !withoutTag {
			return true
		}
	}
	return false
}

// parentMergeBase returns the merge base of HEAD with the parent branch the pair's fabric origin record names.
// A worktree with no origin record or no parent branch, such as the prime on main, reports false.
func parentMergeBase(location *lymcwd.Location) (string, bool, error) {
	origin, found, err := fabricengine.ReadOrigin(location)
	if err != nil {
		return "", false, err
	}
	if !found || origin.ParentBranch == "" {
		logger.Info("gate test: no parent branch to diff from", "worktree", location.WorktreePath())
		return "", false, nil
	}
	fabric, err := fabricengine.Open(location)
	if err != nil {
		return "", false, err
	}
	base, err := fabric.MergeBase(origin.ParentBranch)
	if err != nil {
		return "", false, fmt.Errorf("merge base with %s: %w", origin.ParentBranch, err)
	}
	return base, true, nil
}
