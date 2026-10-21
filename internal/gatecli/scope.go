// scope.go computes the scope of one `lyx gate test` call: which targets each listed tier runs over, and which guard tests the guard step runs, for the computed, --base and --card forms.

package gatecli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// The scope forms computeScope adds to the explicit-targets and --all forms.
const (
	scopeComputed = "computed"
	scopeBase     = "base"
	scopeCard     = "card"
)

// moduleTargets is the Go package arguments one module holds in a narrower scope.
type moduleTargets struct {
	// dir is the module directory relative to the worktree root, "." for the root module.
	dir string
	// targets are the module-relative package arguments.
	targets []string
}

// scope is what a call's tiers run over.
type scope struct {
	// form labels the scope in each tier's summary line and run record.
	form string
	// whole means every listed tier runs over its all_targets.
	whole bool
	// modules are the per-module targets of a narrower scope; empty with whole, and empty for the empty set, which runs no listed tier's command.
	modules []moduleTargets
	// guardStep means the scope form plans a guard step: a computed, --base or --card scope under the go scoper.
	guardStep bool
	// guardDir is the run directory of that guard step, relative to the worktree root; empty means the guard tier's own dir.
	guardDir string
	// guards maps the directory of each package outside the scope to the names of its guard tests, keyed relative to guardDir; nil when no scan has run.
	guards map[string][]string
}

// computeScope returns the scope of request over the worktree at root, for the config cfg.
// Explicit targets and --all take no computation.
// Otherwise the form is --card when a card is named, else --base, else the computed form, whose base is the merge base of HEAD with the parent branch the pair's fabric origin record names.
// A --card is read from the plan first, under either scoper, and an unknown id or a worktree with no plan is refused before any slot.
// Under the none scoper each of the three forms is whole.
// Under the go scoper a computed or --base scope is the impacted set of the diff from its base to the working tree, and falls back to whole, naming the reason on request.stderr, wherever the set cannot be narrowed;
// a --card scope is the cards' packages per module.
// A guard scan error refuses with its file, line and way forward.
// ctx bounds the derivation, so a stalled `go list` is killed when it ends.
func computeScope(ctx context.Context, cfg gateslot.Config, location *lyxcwd.Location, root string, request testRequest) (scope, error) {
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
	var cardModules []planparser.ModulePackages
	if len(request.cards) > 0 {
		form = scopeCard
		var err error
		if cardModules, err = cardPackages(location, root, request.cards); err != nil {
			return scope{}, err
		}
	}
	if cfg.Scoper != "go" {
		return scope{form: form, whole: true}, nil
	}

	guardTier, err := cfg.Tier(cfg.GuardTier)
	if err != nil {
		return scope{}, fmt.Errorf("gate test: guard_tier: %w", err)
	}
	if form == scopeCard {
		modules := make([]moduleTargets, len(cardModules))
		for i, module := range cardModules {
			modules[i] = moduleTargets{dir: module.Dir, targets: module.Packages}
		}
		return scope{form: form, modules: modules, guardStep: true, guardDir: guardTier.Dir}, nil
	}

	base := request.base
	if form == scopeComputed {
		var found bool
		if base, found, err = parentMergeBase(location); err != nil {
			printLine(request.stderr, fmt.Sprintf("gate test: the computed scope falls back to whole: cannot read the merge base with the parent branch: %v", err))
			return scope{form: form, whole: true, guardStep: true, guardDir: guardTier.Dir}, nil
		}
		if !found {
			return scope{form: form, whole: true, guardStep: true, guardDir: guardTier.Dir}, nil
		}
	}
	targets, err := impactset.DeriveTargets(ctx, root, base)
	if err != nil {
		if impactset.IsGuardScanError(err) {
			return scope{}, errors.New(guardScanRefusal(root, err))
		}
		return scope{}, fmt.Errorf("gate test: cannot derive the impacted set of the diff from %s: %w; way forward: re-run the same command", base, err)
	}
	if targets.Whole {
		printLine(request.stderr, "gate test: the scope falls back to whole: "+targets.Fallback)
		return scope{form: form, whole: true, guardStep: true, guardDir: guardTier.Dir}, nil
	}
	modules := make([]moduleTargets, len(targets.Modules))
	for i, module := range targets.Modules {
		modules[i] = moduleTargets{dir: module.Dir, targets: module.Packages}
	}
	return scope{form: form, modules: modules, guardStep: true, guardDir: ".", guards: targets.Guards}, nil
}

// scanGuards fills sc.guards with the guard tests of the module at sc.guardDir outside the scope's packages, by a scan of that module.
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
	sc.guards = map[string][]string{}
	for dir, names := range all {
		if len(names) > 0 && !inside[path.Clean(dir)] {
			sc.guards[dir] = names
		}
	}
	return sc, nil
}

// cardPackages reads the plan at the worktree's anchor and returns the Go packages of the named cards per module.
// An id is a card's number, or its number and slug as `N-slug`, with leading zeros ignored.
// An unknown id is refused naming the plan's ids, and a worktree with no plan is refused naming --all and explicit targets.
func cardPackages(location *lyxcwd.Location, root string, ids []string) ([]planparser.ModulePackages, error) {
	planDir := planparser.PlanDir(location.AnchorPath())
	if _, err := os.Stat(planparser.PlanOverview(location.AnchorPath())); errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("gate test: --card needs the plan at %s, which this worktree does not hold; way forward: run the call from the task worktree that holds the plan, or use --all or explicit targets, then re-run the same command", planDir)
	}
	plan, err := planparser.ParsePlan(planDir)
	if err != nil {
		return nil, fmt.Errorf("gate test: cannot read the plan at %s: %w; way forward: fix the plan, or use --all or explicit targets, then re-run the same command", planDir, err)
	}
	var cards []planparser.Card
	for _, id := range ids {
		index := slices.IndexFunc(plan.Cards, func(card planparser.Card) bool { return cardMatches(card, id) })
		if index < 0 {
			known := make([]string, len(plan.Cards))
			for i, card := range plan.Cards {
				known[i] = card.ID()
			}
			return nil, fmt.Errorf("gate test: --card %q names no card of the plan; way forward: pass one of %s, then re-run the same command", id, strings.Join(known, ", "))
		}
		cards = append(cards, plan.Cards[index])
	}
	return planparser.CardPackages(plan, cards, root), nil
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

// parentMergeBase returns the merge base of HEAD with the parent branch the pair's fabric origin record names.
// A worktree with no origin record or no parent branch, such as the prime on main, reports false.
func parentMergeBase(location *lyxcwd.Location) (string, bool, error) {
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
