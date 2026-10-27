// dotnetscope.go computes the scope of a call under the dotnet scoper: the test projects and class filter terms each listed tier narrows to.

package gatecli

import (
	"context"
	"fmt"

	"github.com/Knatte18/loomyard/internal/dotnetimpact"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// dirGroup is the tiers of a call that run in one directory.
type dirGroup struct {
	dir   string
	tiers []int
}

// dotnetScope returns the scope of the computed, --base or --card form under the dotnet scoper.
// A computed or --base scope groups the listed tiers by base through diffBaseGroups and derives once per base and tier dir;
// a --card scope derives once per tier dir from seeds, the C# seeds of the named cards.
// A derivation that cannot narrow gives its tiers a whole share, naming the reason on request.stderr.
// A narrowed share is one moduleTargets per selected project, and the empty scope an empty share.
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

// groupByDir returns numbers grouped by the directory their tier runs in, in the order each directory first appears.
// A tier the config does not declare is left out, since planTierRuns refuses it.
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

// assignShare gives every tier of group the share derived describes, naming a whole fallback's reason on request.stderr.
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
