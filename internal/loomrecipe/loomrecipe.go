// loomrecipe.go implements New: the coherence guard across env and paths, then the parse of contracts/recipes.LoomRecipe, and the delegation to shedbuild.NewShedFrom.
// NewShedFrom builds the recipe against a caller-supplied shedrecipe.Env and returns the assembled *shedengine.Shed.

package loomrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// New parses recipes.LoomRecipe, builds it against env, and returns a *shedengine.Shed carrying
// the built []shedengine.ProducerDef plus paths' five fields.
//
// New delegates its parse-and-build work to shedbuild.NewShed, wrapping a non-nil returned error
// with a "loomrecipe: " prefix and nothing more: shedbuild.NewShed already names the offending
// row's zero-based index and name in every error it raises after decode, and the decoder keeps
// yaml line numbers, so no further position work is needed.
//
// New never calls shedbuild.Check -- internal/shedcheck/doc.go and internal/shedbuild/check.go
// both state that Check is authoring-time only, because a resumed run legitimately starts
// mid-graph and reachability-from-entry is the wrong production question.
//
// New performs no nil-guard or absolute-path check of its own on any Env field: each registry
// entry validates exactly the fields it reads, and preflightEntry's
// requireAbsRoot("Preflight", "Cwd", …) is what now covers the guard loomshed.New's nil-Preflight
// check used to.
//
// New sets MaxBounces on every row of a review segment (a segment holding a
// Bouncer row) after the build, to the segment's env.Segments[segment].MaxBounces.
// A segment whose budget is 0 has zero_rounds stamped on its Bouncer row before the build, so the Bouncer settles approved at once.
// It refuses a review segment with no env.Segments entry, naming the segment and the loom wiring's segment-to-perch table, and a negative budget, naming the segment, its MaxBouncesKey and ConfigFile.
// The recipe declares no max_bounces on those rows; the budget comes from loom.yaml.
//
// New makes one further check of its own, and it is a coherence check across its two arguments rather
// than a validation of either: it returns an error when env.StatusPath != paths.StatusPath, and
// another when env.StatusLockPath != paths.StatusLockPath. Both run as New's first act, ahead of
// the delegation to shedbuild.NewShed. The ordering is load-bearing, not stylistic:
// loomPreflightEntry calls requireAbsRoot("LoomPreflight", "StatusPath", …) during Build, so a
// divergence involving an empty or relative value would otherwise surface as that entry's
// absoluteness error and hide which of the two copies was wrong. Each error names both sides'
// values so the divergence is readable without a debugger.
//
// StatusPath and StatusLockPath are deliberately told twice -- once in env (for
// loomPreflightEntry) and once in paths (for Shed) -- and that duplication is inherent to the
// split between the two argument types New takes; it must not be collapsed. This check is the
// guard the duplication needs. loomshed.Deps carried one StatusPath field feeding both
// NewLoomPreflight and shedengine.Shed, so the two could not disagree; splitting it into an Env
// copy (read by loomPreflightEntry) and a shedbuild.ShedPaths copy (read by Shed) makes a
// divergent fill possible for the first time, and its consequence is silent: Shed would persist
// its status to one file while Loom-Preflight reads another, which surfaces as a broken resume
// rather than as any error. That is the same class of silent durable-identity hazard the
// row-name-authority-stays-with-the-go-constants Shared Decision exists to machine-check, and New
// is the only place in the tree where both copies are visible at once, so no other layer can make
// this check.
func New(env shedrecipe.Env, paths shedbuild.ShedPaths) (*shedengine.Shed, error) {
	if err := checkStatusPaths(env, paths); err != nil {
		return nil, err
	}

	parsed, err := shedbuild.Parse(recipes.LoomRecipe)
	if err != nil {
		return nil, fmt.Errorf("loomrecipe: %w", err)
	}

	segments, err := reviewSegments(recipes.LoomRecipe)
	if err != nil {
		return nil, err
	}
	budgets, err := segmentBudgets(segments, env, loomSegmentTableWayForward)
	if err != nil {
		return nil, err
	}
	stampZeroRounds(&parsed, segments, budgets)

	shed, err := shedbuild.NewShedFrom(parsed, env, paths)
	if err != nil {
		return nil, fmt.Errorf("loomrecipe: %w", err)
	}

	applyReviewBudget(shed.Producers, segments, budgets)

	return shed, nil
}

// loomSegmentTableWayForward is the way forward of a loom review segment missing from the wiring.
const loomSegmentTableWayForward = "add the segment's row to the loom wiring's segment-to-perch table"

// darnReviewSegment is the darn recipe's review segment.
const darnReviewSegment = "Darn-Review"

// darnBudgetWayForward is the way forward of a negative darn review budget.
const darnBudgetWayForward = `run "lyx config darn --set review_max_bounces=<n>" from the prime, then "lyx loom resume"`

// checkStatusPaths refuses an env whose status and status-lock paths differ from paths' copies of them.
func checkStatusPaths(env shedrecipe.Env, paths shedbuild.ShedPaths) error {
	if env.StatusPath != paths.StatusPath {
		return fmt.Errorf("loomrecipe: env.StatusPath %q != paths.StatusPath %q", env.StatusPath, paths.StatusPath)
	}
	if env.StatusLockPath != paths.StatusLockPath {
		return fmt.Errorf("loomrecipe: env.StatusLockPath %q != paths.StatusLockPath %q", env.StatusLockPath, paths.StatusLockPath)
	}

	return nil
}

// Routing projects the embedded loom recipe's routing without building any engine, so a caller
// that never builds a Shed (the status verb) can compute progress. A parse error is returned
// prefixed with "loomrecipe: ". Routing.MaxBounces is left 0.
//
// Every row of a review segment carries budgets' entry for its segment as its MaxBounces, the same value New
// applies, so the projection never reports the engine default for a review row.
// The exception is a zero-round segment: its rows carry MaxBounces 0, which the progress projection reads as shedengine's inherited budget.
// A review segment budgets lacks is refused naming the segment and the loom wiring's segment-to-perch table, and a negative entry is refused naming the segment.
func Routing(budgets map[string]int) (shedengine.Routing, error) {
	routing, err := shedbuild.RoutingOf(recipes.LoomRecipe)
	if err != nil {
		return shedengine.Routing{}, fmt.Errorf("loomrecipe: %w", err)
	}

	segments, err := reviewSegments(recipes.LoomRecipe)
	if err != nil {
		return shedengine.Routing{}, err
	}
	if err := checkSegmentBudgets(segments, budgets, loomSegmentTableWayForward); err != nil {
		return shedengine.Routing{}, err
	}
	applyReviewBudget(routing.Producers, segments, budgets)

	return routing, nil
}

// segmentBudgets maps each review segment of segments to env.Segments' MaxBounces for it.
// A segment with no env.Segments entry is refused naming the segment and wayForward, and a negative budget is refused naming the segment, its MaxBouncesKey and ConfigFile.
func segmentBudgets(segments map[string]string, env shedrecipe.Env, wayForward string) (map[string]int, error) {
	budgets := make(map[string]int, len(segments))
	for segment := range segments {
		settings, ok := env.Segments[segment]
		if !ok {
			return nil, fmt.Errorf("loomrecipe: review segment %q has no Env.Segments entry; way forward: %s", segment, wayForward)
		}
		if settings.MaxBounces < 0 {
			return nil, negativeBudgetError(segment, settings.MaxBounces, settings.MaxBouncesKey, settings.ConfigFile)
		}
		budgets[segment] = settings.MaxBounces
	}

	return budgets, nil
}

// checkSegmentBudgets refuses a review segment of segments that budgets lacks, naming the segment and wayForward, and a negative entry, naming the segment.
func checkSegmentBudgets(segments map[string]string, budgets map[string]int, wayForward string) error {
	for segment := range segments {
		budget, ok := budgets[segment]
		if !ok {
			return fmt.Errorf("loomrecipe: review segment %q has no budget entry; way forward: %s", segment, wayForward)
		}
		if budget >= 0 {
			continue
		}
		if segment == darnReviewSegment {
			return negativeBudgetError(segment, budget, "review_max_bounces", "darn.yaml")
		}

		return fmt.Errorf("loomrecipe: bounce budget of review segment %q = %d; want 0 or more; way forward: set the perch's <perch>_max_bounces, or review_max_bounces, in loom.yaml", segment, budget)
	}

	return nil
}

// negativeBudgetError refuses a negative budget of segment, naming the config key and file it resolved from.
// The darn segment's way forward is the runnable command pair; any other names the key to set.
func negativeBudgetError(segment string, budget int, key, file string) error {
	wayForward := fmt.Sprintf("set %s in %s", key, file)
	if segment == darnReviewSegment {
		wayForward = darnBudgetWayForward
	}

	return fmt.Errorf("loomrecipe: bounce budget of review segment %q = %d from %s in %s; want 0 or more; way forward: %s", segment, budget, key, file, wayForward)
}

// reviewSegments maps each segment of the recipe source that holds a Bouncer row to that Bouncer row's name.
func reviewSegments(source []byte) (map[string]string, error) {
	recipe, err := shedbuild.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("loomrecipe: %w", err)
	}

	segments := map[string]string{}
	for _, row := range recipe.Producers {
		if row.Engine == bouncerEngine && row.Segment != "" {
			segments[row.Segment] = row.Name
		}
	}

	return segments, nil
}

// applyReviewBudget sets MaxBounces on every producer whose segment is a review segment, to budgets' entry for the segment.
// It reads the map alone: every key of segments has already been checked.
func applyReviewBudget(producers []shedengine.ProducerDef, segments map[string]string, budgets map[string]int) {
	for i := range producers {
		if _, ok := segments[producers[i].Segment]; !ok {
			continue
		}
		producers[i].MaxBounces = budgets[producers[i].Segment]
	}
}

// stampZeroRounds sets zero_rounds on the Bouncer row of every review segment whose budget is 0.
// A stamped Bouncer settles approved at once exactly where its rows carry MaxBounces 0 under applyReviewBudget.
func stampZeroRounds(recipe *shedbuild.Recipe, segments map[string]string, budgets map[string]int) {
	for segment, bouncer := range segments {
		if budgets[segment] != 0 {
			continue
		}
		for i := range recipe.Producers {
			if recipe.Producers[i].Name != bouncer {
				continue
			}
			if recipe.Producers[i].Config == nil {
				recipe.Producers[i].Config = map[string]any{}
			}
			recipe.Producers[i].Config["zero_rounds"] = true
		}
	}
}
