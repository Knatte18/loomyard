// scope.go computes the scope of one `lym gate test` call.
// It names the targets each listed tier runs over, and the guard tests the guard step runs.
// This holds for the computed, --base and --card forms.

package gatecli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"go/build"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
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

// moduleTargets holds the targets one module runs in a narrower scope.
// They are its Go package arguments, or the test project of a dotnet share.
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
	// skip names the `//lym:watch` tests of modules that the tier's diff leaves alone, sorted.
	skip []string
}

// scope is what a call's tiers run over.
type scope struct {
	// form labels the scope in each tier's summary line and run record.
	form string
	// whole means every listed tier runs over its all_targets.
	whole bool
	// modules are the per-module targets of a narrower scope; empty with whole and for the empty set, which runs nothing.
	modules []moduleTargets
	// noTests means the dotnet scoper found changes but no test class naming them; set from the tier's share.
	noTests bool
	// tiers holds the share of each listed tier of a diff scope, which derives one impacted set per base.
	// A tier it does not name takes whole and modules.
	tiers map[int]tierScope
	// guardStep means the scope form plans a guard step: a computed, --base or --card scope under the go scoper.
	// A --card scope under the card key plans none.
	guardStep bool
	// guardDir is the run directory of that guard step, relative to the worktree root.
	// Empty means the directory of each guard group's own tier.
	guardDir string
	// guards maps a tier number to each directory that holds guard tests of it for the guard step, and to their names.
	// The directories are relative to guardDir; nil when no scan has run.
	guards map[int]map[string][]string
	// listedPackages maps a tier number to the guard module's packages in which a --card scope's listed runs run its tests.
	// Each kept listed tier adds its share's packages under its own number and under every tier it covers.
	// A guard test outside the packages of its tier is left to the guard step, even in a package of the scope.
	listedPackages map[int]map[string]bool
	// tagTiers are the declared tiers, ascending, whose build tag adds a file to a --card scope's package.
	// A build without tags leaves that file out, so the vet step is planned over these tiers.
	tagTiers []int
	// skip names the `//lym:watch` tests of modules that the scope's changed files leave alone, sorted.
	// Each listed tier's run skips them.
	skip []string
	// changed holds the worktree-relative paths a --card scope's changes are read as, when changesRead is set.
	// The guard scan drops the watched guard tests they leave alone.
	changed     []string
	changesRead bool
}

// forTier returns s with the narrowing of tier number in place of its own whole, modules, noTests and skip.
func (s scope) forTier(number int) scope {
	if share, ok := s.tiers[number]; ok {
		s.whole, s.modules, s.noTests, s.skip = share.whole, share.modules, share.noTests, share.skip
	}
	return s
}

// withGateScope returns request with the scope form its --gate key names in place of the scope flags.
// A key of scope all is the --all form, and a key of scope diff is the computed form.
// A key of scope card keeps the --card ids; the other scopes drop them.
// Explicit targets replace the key's scope and a call with no key is returned unchanged.
func withGateScope(cfg gateslot.Config, request testRequest) testRequest {
	if request.gate == "" || len(request.targets) > 0 {
		return request
	}
	switch cfg.Gates[request.gate].Scope {
	case "all":
		request.all = true
		request.cards = nil
	case "diff", gateslot.ScopeMarked:
		request.cards = nil
	}
	return request
}

// isMarkedCall reports whether request runs a --gate key of scope marked, with no explicit targets to replace it.
func isMarkedCall(cfg gateslot.Config, request testRequest) bool {
	return request.gate != "" && len(request.targets) == 0 && cfg.Gates[request.gate].Scope == gateslot.ScopeMarked
}

// skippedInside returns the guard tests of inside whose tier is none of numbers, nor a tier one of them covers.
// The runs of numbers leave such a test out although its package is in their set, so the guard step takes it.
func skippedInside(cfg gateslot.Config, numbers []int, inside map[string][]impactset.GuardTest) map[string][]impactset.GuardTest {
	run := slices.Clone(numbers)
	for _, number := range numbers {
		if tier, err := cfg.Tier(number); err == nil {
			run = append(run, tier.Covers...)
		}
	}
	left := map[string][]impactset.GuardTest{}
	for dir, tests := range inside {
		for _, test := range tests {
			if !slices.Contains(run, test.Tier) {
				left[dir] = append(left[dir], test)
			}
		}
	}
	return left
}

// computeScope returns the scope of request over the worktree at root, for the config cfg.
// Explicit targets take no computation, and --all takes only allScope's inert check.
// Otherwise the form is --card when a card is named, --base when a base is given, and the computed form otherwise.
// The computed form takes its base per listed tier.
// A computed tier's base is the newest ancestor of HEAD that green holds whole-green for the tier.
// Failing that, it is the merge base of HEAD with the parent branch the pair's fabric origin record names.
// A tier with neither runs whole.
// green is nil for a dirty worktree, which takes the merge base.
// A --card is read from the plan first, under either scoper.
// An unknown id, or a worktree with no plan, is refused before any slot.
// Under the none scoper each of the three forms is whole, except for a hub with gate.inert_paths.
// There, the tiers of a computed or --base group whose diff is inert-only get the empty share.
// That share plans no guard step.
// Under the dotnet scoper dotnetScope derives them.
// Under the go scoper a computed or --base scope is the impacted set of the diff from its base.
// The set is derived once per base and shared by the tiers that land on it.
// Each derivation is named on request.stderr.
// A base group whose diff is inert-only under gate.inert_paths gets the empty share and is not derived.
// It leaves the guard tests to the module scan, as a whole tier does.
// A set that cannot be narrowed falls back to whole, naming the reason on request.stderr.
// A --card scope is the cards' packages per module.
// Under the card key it holds them alone at every tier, plans no guard step and says so on request.stderr.
// Otherwise it holds every package of that module whose build or tests reach one of them too.
// With request.mainPackages it also holds every package main of tier0's module, listed once.
// Under the batch key, each listed tier above tier0 that does not cover tier0 runs the cards' packages alone.
// A tier that runs whole is not narrowed.
// That narrower set holds the main packages when asked, and request.stderr names the narrowed tiers.
// It adds the declared tiers whose build tag brings a file into one of those packages.
// A narrowed share, and a --card scope, skip the `//lym:watch` tests of their packages that no changed file concerns.
// A --card scope reads its changed files as the diff from the merge base with the parent branch.
// With no parent branch or an unreadable diff it skips none.
// A narrowed share's guard step also takes each guard test inside its set whose tier no tier of its base is or covers.
// A --gate key of scope marked takes markedScope instead, whatever the scoper.
// A guard or watch scan error refuses with its file, line and way forward.
// ctx bounds the derivation, so a stalled `go list` is killed when it ends.
func computeScope(ctx context.Context, cfg gateslot.Config, location *lymcwd.Location, root string, request testRequest, green *greenRun) (scope, error) {
	request = withGateScope(cfg, request)
	if isMarkedCall(cfg, request) {
		return markedScope(cfg, request)
	}
	switch {
	case len(request.targets) > 0:
		return scope{form: scopeTargets}, nil
	case request.all:
		return allScope(ctx, cfg, location, root, request), nil
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
		// The main packages are listed first, so a refused call names no narrowed scope.
		mainsDir := moduleKey(root, tier0.Dir)
		var mains []string
		if request.mainPackages {
			if mains, err = impactset.ModuleMainPackages(ctx, resolveRunDir(root, tier0.Dir)); err != nil {
				return scope{}, fmt.Errorf("gate test: --%s cannot list the main packages of the module at %s: %w; way forward: fix the module until \"go list ./...\" succeeds there, then re-run the same command", gateslot.MainPackagesFlag, mainsDir, err)
			}
		}
		// The batch key's call after the batch's last card runs the dependent packages and the guard step,
		// so the card key's call leaves both to it.
		ownOnly := request.gate == gateslot.CardGateKey
		modules := make([]moduleTargets, len(cardModules))
		own := make([]moduleTargets, len(cardModules))
		for i, module := range cardModules {
			modules[i] = moduleTargets{dir: module.Dir, targets: slices.Clone(module.Packages)}
			if !ownOnly {
				modules[i].targets = cardDependents(ctx, root, module, request.stderr)
			}
			own[i] = moduleTargets{dir: module.Dir, targets: slices.Clone(module.Packages)}
		}
		modules = withMainPackages(modules, mainsDir, mains)
		own = withMainPackages(own, mainsDir, mains)
		sc := scope{form: form, modules: modules, guardStep: !ownOnly, guardDir: tier0.Dir, tagTiers: tagTiers(cfg, root, modules)}
		if ownOnly {
			printLine(request.stderr, fmt.Sprintf("gate test: --gate %s runs the cards' packages alone and no guard step; --gate %s runs the packages that depend on them and the guard step", request.gate, gateslot.BatchGateKey))
		}
		ownTiers := ownPackageTiers(cfg, request)
		var ownSkip []string
		sc.changed, sc.changesRead = cardChanges(ctx, location, root)
		if sc.changesRead {
			if sc.skip, err = watchedSkip(root, existingDirs(root, modules), sc.changed); err != nil {
				return scope{}, err
			}
			if len(ownTiers) > 0 {
				if ownSkip, err = watchedSkip(root, existingDirs(root, own), sc.changed); err != nil {
					return scope{}, err
				}
			}
		}
		if len(ownTiers) > 0 {
			sc.tiers = map[int]tierScope{}
			for _, number := range ownTiers {
				sc.tiers[number] = tierScope{modules: own, skip: ownSkip}
			}
			printLine(request.stderr, fmt.Sprintf("gate test: --gate %s runs %s over the cards' packages alone, not the packages that depend on them", request.gate, tierNames(ownTiers)))
		}
		sc.listedPackages = listedPackages(cfg, request, sc)
		return sc, nil
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
			share = tierScope{modules: make([]moduleTargets, len(targets.Modules)), skip: skipNames(targets.Unwatched)}
			for i, module := range targets.Modules {
				share.modules[i] = moduleTargets{dir: module.Dir, targets: module.Packages}
			}
			guards = mergeGuards(mergeGuards(guards, targets.Guards), skippedInside(cfg, group.tiers, targets.InsideGuards))
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

// cardDependents returns module's packages and every package of that module whose build or tests reach one of them.
// A test outside the cards' packages that asserts on what a card changes then runs in the same call.
// When the module's packages cannot be listed it returns the cards' packages alone.
// It then names the narrowing and its cause in the log and on stderr.
func cardDependents(ctx context.Context, root string, module planparser.ModulePackages, stderr io.Writer) []string {
	dependents, err := impactset.DependentPackages(ctx, resolveRunDir(root, module.Dir), module.Packages)
	if err != nil {
		logger.Warn("gate test: could not list the packages that depend on the cards' packages; the card scope holds the cards' packages alone", "module", module.Dir, "cause", err)
		printLine(stderr, fmt.Sprintf("gate test: the card scope holds the cards' packages alone: cannot list the packages that depend on them in %s: %v", module.Dir, err))
		return module.Packages
	}
	return dependents
}

// ownPackageTiers returns the kept listed tiers of a batch key call that run the cards' packages alone, ascending.
// They are the tiers above tier0 that do not cover tier0, so tier0 alone runs the packages that depend on the cards.
// The slower tests of the dependent packages are left to the runs over the whole tree.
// Those happen once per plan, not once per batch.
// A tier that runs whole is not narrowed:
// a `carry: false` tier, every tier of a --no-carry call, and one whose command takes no targets.
// A call of any other key returns none.
func ownPackageTiers(cfg gateslot.Config, request testRequest) []int {
	if request.gate != gateslot.BatchGateKey {
		return nil
	}
	var numbers []int
	for _, number := range keptTiers(cfg, request) {
		tier, err := cfg.Tier(number)
		if err != nil || number == 0 || slices.Contains(tier.Covers, 0) {
			continue
		}
		if !tier.NoCarry && !request.noCarry && gateslot.HoldsTargets(tier.Command) {
			numbers = append(numbers, number)
		}
	}
	return numbers
}

// listedPackages returns, per tier number, the guard module's packages in which the listed runs of sc run its tests.
// A kept listed tier runs its own tests and those of every tier it covers, over its share of sc.
// The packages are module-relative directories.
func listedPackages(cfg gateslot.Config, request testRequest, sc scope) map[int]map[string]bool {
	byTier := map[int]map[string]bool{}
	for _, number := range keptTiers(cfg, request) {
		run := []int{number}
		if tier, err := cfg.Tier(number); err == nil {
			run = append(run, tier.Covers...)
		}
		for _, module := range sc.forTier(number).modules {
			if path.Clean(module.dir) != path.Clean(sc.guardDir) {
				continue
			}
			for _, target := range module.targets {
				dir := strings.TrimPrefix(path.Clean(target), "./")
				for _, covered := range run {
					if byTier[covered] == nil {
						byTier[covered] = map[string]bool{}
					}
					byTier[covered][dir] = true
				}
			}
		}
	}
	return byTier
}

// existingDirs returns the worktree-relative directories of the packages of modules that exist under root.
// A card may name a package it has yet to create, which is left out.
func existingDirs(root string, modules []moduleTargets) []string {
	var dirs []string
	for _, module := range modules {
		for _, target := range module.targets {
			dir := path.Join(module.dir, strings.TrimPrefix(target, "./"))
			if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir))); err == nil && info.IsDir() {
				dirs = append(dirs, dir)
			}
		}
	}
	return dirs
}

// watchedSkip returns the sorted names of the `//lym:watch` tests in dirs that no path of changed concerns.
// A watch scan error is returned as the refusal.
func watchedSkip(root string, dirs, changed []string) ([]string, error) {
	watched, err := impactset.FindWatchedTests(root, dirs)
	if err != nil {
		return nil, errors.New(guardScanRefusal(root, err))
	}
	return skipNames(impactset.UnwatchedTests(watched, changed)), nil
}

// withMainPackages returns modules with mains, the main packages of the module at dir, added to that module's targets.
// A package the cards already name is not added twice, and a module the cards leave out joins the list last.
func withMainPackages(modules []moduleTargets, dir string, mains []string) []moduleTargets {
	if len(mains) == 0 {
		return modules
	}
	index := slices.IndexFunc(modules, func(module moduleTargets) bool { return path.Clean(module.dir) == dir })
	if index < 0 {
		return append(modules, moduleTargets{dir: dir, targets: mains})
	}
	for _, pkg := range mains {
		if !slices.Contains(modules[index].targets, pkg) {
			modules[index].targets = append(modules[index].targets, pkg)
		}
	}
	return modules
}

// noneScope returns the computed or --base scope under the none scoper for a hub with gate.inert_paths:
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

// allScope returns the scope of an --all call, which is whole.
// A --gate key of scope all gets the empty scope instead when every changed file is inert.
// The diff is taken from the merge base with the parent branch.
// A change of inert files alone then runs no tier at any gate key.
// A worktree with no parent branch, a merge base that cannot be read and an explicit --all stay whole.
func allScope(ctx context.Context, cfg gateslot.Config, location *lymcwd.Location, root string, request testRequest) scope {
	whole := scope{form: scopeAll, whole: true}
	if request.gate == "" || len(cfg.InertPaths) == 0 {
		return whole
	}
	base, ok, err := parentMergeBase(location)
	if err != nil {
		printLine(request.stderr, fmt.Sprintf("gate test: the scope of --gate %q stays whole: cannot read the merge base with the parent branch: %v", request.gate, err))
		return whole
	}
	if !ok || !allInertSince(ctx, cfg, root, base) {
		return whole
	}
	printLine(request.stderr, fmt.Sprintf("gate test: scope all of --gate %q: every file changed since %s is inert, so the scope is empty", request.gate, base))
	return scope{form: scopeAll}
}

// allInertSince reports whether every changed file matches the hub's gate.inert_paths.
// The changed files are those that differ between base and the working tree, untracked files included.
// A hub with no gate.inert_paths, an empty diff and a diff that cannot be read report false.
// A diff that cannot be read is logged at Warn, and the scope stays as it is.
func allInertSince(ctx context.Context, cfg gateslot.Config, root, base string) bool {
	if len(cfg.InertPaths) == 0 {
		return false
	}
	paths, err := impactset.ChangedPaths(ctx, root, base)
	if err != nil {
		logger.Warn("gate test: could not read the changed files to check them against "+keyName(inertPathsKey)+"; the scope stays as it is", "base", base, "cause", err)
		return false
	}
	return cfg.AllInert(paths)
}

// baseGroup is the tiers of a call that diff from one base.
type baseGroup struct {
	base  string
	tiers []int
}

// diffBaseGroups groups the tiers the request lists by the base their diff starts from.
// Groups keep the order in which each base first appears.
// The second result holds the tiers with no base, which run whole.
// A --base call gives every tier its one base.
// Under the computed form, a tier's base is the newest ancestor of HEAD that the green record holds whole-green for it.
// Only an entry that held back no test the request runs counts for this.
// With no such ancestor, the base is the merge base with the parent branch.
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
			base, ok = green.record.newestWholeAncestor(green.fingerprint(tier), moduleKey(root, tier.Dir), request.held, isAncestor)
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

// keptTiers returns the tiers the request lists that no other listed tier covers, in ascending order.
// It writes nothing to stderr.
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

// mergeGuards returns the union of the guard tests of a and b per directory, sorted by name and then tier.
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

// skipNames returns the test names of unwatched, sorted and without repeats.
func skipNames(unwatched map[string][]string) []string {
	var names []string
	for _, dirNames := range unwatched {
		names = append(names, dirNames...)
	}
	sort.Strings(names)
	return slices.Compact(names)
}

// cardChanges returns the worktree-relative paths that differ from the parent branch's merge base.
// The comparison is against the working tree.
// It reports false when the worktree has no parent branch or the diff cannot be read.
// Every watched test of the scope then runs.
func cardChanges(ctx context.Context, location *lymcwd.Location, root string) ([]string, bool) {
	base, ok, err := parentMergeBase(location)
	if err != nil {
		logger.Warn("gate test: could not read the merge base with the parent branch; every watched test of the card scope runs", "cause", err)
		return nil, false
	}
	if !ok {
		return nil, false
	}
	paths, err := impactset.ChangedPaths(ctx, root, base)
	if err != nil {
		logger.Warn("gate test: could not read the files changed since the merge base; every watched test of the card scope runs", "base", base, "cause", err)
		return nil, false
	}
	return paths, true
}

// moduleRelative returns the paths of changed that lie under the module at dir, made relative to it.
// changed and the result are slash-separated, and dir resolves against root.
func moduleRelative(root, dir string, changed []string) []string {
	prefix := moduleKey(root, dir)
	if prefix == "." {
		return changed
	}
	relative := []string{}
	for _, changedPath := range changed {
		if rest, ok := strings.CutPrefix(changedPath, prefix+"/"); ok {
			relative = append(relative, rest)
		}
	}
	return relative
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

// scanGuards fills sc.guards with the guard tests the scope's listed runs leave out, grouped by their file's tier.
// Those are the guard tests outside every package in which a listed run runs tests of their tier.
// It scans the module at sc.guardDir.
// A whole scope and an explicit-targets scope hold no package of that module, so every guard test counts as left out.
// It scans the guard packages for `//lym:watch` markers too.
// When the scope read its changes, it drops each watched guard test those changes leave alone.
// A scan error is returned as the refusal.
func scanGuards(sc scope, root string) (scope, error) {
	moduleDir := resolveRunDir(root, sc.guardDir)
	all, err := impactset.ModuleGuardTests(moduleDir)
	if err != nil {
		return scope{}, errors.New(guardScanRefusal(moduleDir, err))
	}
	leftOut := map[string][]impactset.GuardTest{}
	for dir, tests := range all {
		for _, test := range tests {
			if !sc.listedPackages[test.Tier][path.Clean(dir)] {
				leftOut[dir] = append(leftOut[dir], test)
			}
		}
	}
	watched, err := impactset.FindWatchedTests(moduleDir, slices.Sorted(maps.Keys(leftOut)))
	if err != nil {
		return scope{}, errors.New(guardScanRefusal(moduleDir, err))
	}
	if sc.changesRead {
		for dir, names := range impactset.UnwatchedTests(watched, moduleRelative(root, sc.guardDir, sc.changed)) {
			leftOut[dir] = slices.DeleteFunc(leftOut[dir], func(test impactset.GuardTest) bool { return slices.Contains(names, test.Name) })
		}
	}
	sc.guards = guardsByTier(leftOut)
	return sc, nil
}

// cardPackages returns the Go packages of the named cards per module, read from the plan at the worktree's anchor.
func cardPackages(location *lymcwd.Location, root string, ids []string) ([]planparser.ModulePackages, error) {
	plan, cards, err := readCards(location, ids)
	if err != nil {
		return nil, err
	}
	return planparser.CardPackages(plan, cards, root), nil
}

// readCards reads the plan at the worktree's anchor and returns it with the named cards, in the order of ids.
// An id is a card's number, or its number and slug as `N-slug`, with leading zeros ignored.
// An unknown id is refused naming the plan's ids,
// and a worktree with no plan is refused naming --all and explicit targets.
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

// tagTiers returns the tiers cfg declares, ascending, whose build tag `tierN` alone adds a `.go` file to a package.
// A file counts only if a build without tags leaves it out,
// and the match uses the host's GOOS and GOARCH.
// Package directories resolve against root.
// A directory or file that cannot be read or matched is left out, since the listed tiers' own compiles report it.
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

// tagAddsFile reports whether dir directly holds a `.go` file that matches the build tags of tagged.
// Only a file that a build without tags leaves out counts.
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
