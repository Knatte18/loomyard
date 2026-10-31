// loomrecipe.go implements New and Routing, the recipe-keyed builder and routing projection of the embedded loom recipes.
// New is the coherence guard across env and paths, then the parse of the embedded recipe the name selects, and the delegation to shedbuild.NewShedFrom.
// NewShedFrom builds the recipe against a caller-supplied shedrecipe.Env and returns the assembled *shedengine.Shed.

package loomrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// New parses the embedded recipe the name selects, builds it against env, and returns a *shedengine.Shed carrying
// the built []shedengine.ProducerDef plus paths' five fields.
// An empty name reads as the tapestry recipe, and a name that selects no embedded recipe is refused.
//
// New delegates its build work to shedbuild.NewShedFrom, wrapping a non-nil returned error
// with a "loomrecipe: " prefix and nothing more: shedbuild.NewShedFrom already names the offending
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
// New refuses a recipe that does not declare `contract: loom`, naming NewPawl and `lym shed` as the way forward.
//
// A recipe whose verify_source is tree_scope also takes env.DarnVerifyAttempts, the `verify_attempts` of `darn.yaml`:
// New refuses a value below 1 and stamps it onto the verify gate of its build row and of the Code-Review segment's BurlerFix row at every build.
//
// New makes one further check of its own, a coherence check across its two arguments rather than a validation of either:
// it returns an error when env.StatusPath != paths.StatusPath, and another when env.StatusLockPath != paths.StatusLockPath.
// Both run as New's first act, ahead of the delegation to shedbuild.NewShed.
// Each error names both sides' values so the divergence is readable without a debugger.
//
// StatusPath and StatusLockPath are deliberately told twice, once in env and once in paths (for Shed), because the two argument types New takes split them.
// The check keeps env's copy equal to Shed's paths: a divergent fill would let Shed persist its status to one file while a row reads another, which surfaces as a broken resume rather than as any error.
// New is the only place in the tree where both copies are visible at once, so no other layer can make this check.
func New(recipe string, env shedrecipe.Env, paths shedbuild.ShedPaths) (*shedengine.Shed, error) {
	if err := checkStatusPaths(env, paths); err != nil {
		return nil, err
	}

	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return nil, err
	}
	if parsed.Contract != loomContract {
		return nil, fmt.Errorf("loomrecipe: recipe %q does not declare contract: %s; way forward: build a pawl recipe with NewPawl, or drive any other recipe with \"lym shed\"", recipe, loomContract)
	}

	gatedRows := verifyGatedRows(parsed)
	if len(gatedRows) > 0 {
		if err := checkVerifyAttempts(env.DarnVerifyAttempts); err != nil {
			return nil, err
		}
	}
	segments := reviewSegmentsOf(parsed)
	budgets, err := segmentBudgets(segments, env, loomSegmentTableWayForward)
	if err != nil {
		return nil, err
	}
	for _, row := range gatedRows {
		if err := applyVerifyAttempts(&parsed, row, env.DarnVerifyAttempts); err != nil {
			return nil, err
		}
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

// Routing projects the routing of the embedded recipe the name selects without building any engine, so a caller
// that never builds a Shed (the status verb) can compute progress. A parse error is returned
// prefixed with "loomrecipe: ". Routing.MaxBounces is left 0.
// An empty name reads as the tapestry recipe, and a name that selects no embedded recipe is refused.
//
// Every row of a review segment carries budgets' entry for its segment as its MaxBounces, the same value New
// applies, so the projection never reports the engine default for a review row.
// The exception is a zero-round segment: its rows carry MaxBounces 0, which the progress projection reads as shedengine's inherited budget.
// A review segment budgets lacks is refused naming the segment and the loom wiring's segment-to-perch table, and a negative entry is refused naming the segment.
func Routing(recipe string, budgets map[string]int) (shedengine.Routing, error) {
	source, err := embeddedRecipeSource(recipe)
	if err != nil {
		return shedengine.Routing{}, err
	}
	routing, err := shedbuild.RoutingOf(source)
	if err != nil {
		return shedengine.Routing{}, fmt.Errorf("loomrecipe: %w", err)
	}

	segments, err := reviewSegments(source)
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

		return fmt.Errorf("loomrecipe: bounce budget of review segment %q = %d; want 0 or more; way forward: set the perch's <perch>_max_bounces, or review_max_bounces, in loom.yaml", segment, budget)
	}

	return nil
}

// negativeBudgetError refuses a negative budget of segment, naming the config key and file it resolved from.
func negativeBudgetError(segment string, budget int, key, file string) error {
	return fmt.Errorf("loomrecipe: bounce budget of review segment %q = %d from %s in %s; want 0 or more; way forward: set %s in %s", segment, budget, key, file, key, file)
}

// reviewSegments maps each segment of the recipe source that holds a Bouncer row to that Bouncer row's name.
func reviewSegments(source []byte) (map[string]string, error) {
	recipe, err := shedbuild.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("loomrecipe: %w", err)
	}

	return reviewSegmentsOf(recipe), nil
}

// reviewSegmentsOf maps each segment of the parsed recipe that holds a Bouncer row to that Bouncer row's name.
func reviewSegmentsOf(recipe shedbuild.Recipe) map[string]string {
	segments := map[string]string{}
	for _, row := range recipe.Producers {
		if row.Engine == bouncerEngine && row.Segment != "" {
			segments[row.Segment] = row.Name
		}
	}

	return segments
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
