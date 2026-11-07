// loomrecipe.go holds New and Routing, the recipe-keyed builder and routing projection of the embedded loom recipes.
// New builds the recipe a name selects against a caller-supplied shedrecipe.Env, and Routing projects its routing.

package loomrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// New parses the embedded recipe the name selects, expands its blocks, and builds it against env.
// It returns a *shedengine.Shed carrying the built []shedengine.ProducerDef and the path fields of paths.
// An empty name reads as the tapestry recipe, and a name that selects no embedded recipe is refused.
//
// New delegates its build work to shedbuild.NewShedFrom and wraps a non-nil error from it with a "loomrecipe: " prefix.
// Errors NewShedFrom raises after decode name the offending row by index and name,
// and the decoder keeps YAML line numbers, so New adds no position detail.
//
// New never calls shedbuild.Check, which is authoring-time only:
// a resumed run legitimately starts mid-graph, so reachability from the entry is the wrong question at run time.
//
// New performs no nil or absolute-path check of its own on any Env field:
// each registry entry validates exactly the fields it reads;
// the preflight entry requires absolute roots for Preflight and Cwd.
//
// A review segment is a segment holding a Bouncer row.
// New sets MaxBounces on every row of a review segment after the build, to its env.Segments[segment].MaxBounces.
// A segment whose budget is 0 has zero_rounds stamped on its Bouncer row before the build,
// so the Bouncer settles approved at once.
// It refuses a review segment with no env.Segments entry.
// The error names the segment and the loom wiring's segment-to-perch table.
// It also refuses a negative budget, naming the segment, its MaxBouncesKey and ConfigFile.
// The recipe declares no max_bounces on those rows; the budget comes from loom.yaml.
//
// New refuses a recipe that does not declare `contract: loom`, naming `lym shed` as the way forward.
//
// A recipe whose verify_source is tree_scope also takes env.DarnVerifyAttempts, the `verify_attempts` of `darn.yaml`:
// New refuses a value below 1.
// It stamps the value onto the verify gate of its build row and of the Code-Review segment's BurlerFix row.
//
// New checks that env and paths agree on the status paths before anything else.
// It returns an error when env.StatusPath != paths.StatusPath.
// It returns another when env.StatusLockPath != paths.StatusLockPath.
// Each error names both sides' values so the divergence is readable without a debugger.
//
// StatusPath and StatusLockPath each appear twice, once in env and once in paths, which Shed reads.
// The check keeps env's copy equal to paths' copy:
// a divergent fill would let Shed persist its status to one file while a row reads another;
// the mismatch surfaces as a broken resume rather than as any error.
// New is the only place where both copies are visible at once, so only New can make this check.
func New(recipe string, env shedrecipe.Env, paths shedbuild.ShedPaths) (*shedengine.Shed, error) {
	if err := checkStatusPaths(env, paths); err != nil {
		return nil, err
	}

	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return nil, err
	}
	if parsed.Contract != loomContract {
		return nil, fmt.Errorf("loomrecipe: recipe %q does not declare contract: %s; way forward: drive the recipe with \"lym shed\"", recipe, loomContract)
	}

	gatedRows := verifyGatedRows(parsed)
	if len(gatedRows) > 0 {
		if err := checkVerifyAttempts(env.DarnVerifyAttempts); err != nil {
			return nil, err
		}
	}
	segments := reviewSegments(parsed)
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

// Routing projects the routing of the embedded recipe the name selects, with its blocks expanded.
// It builds no engine, so a caller that never builds a Shed can still compute progress.
// A parse or expansion error is returned prefixed with "loomrecipe: ".
// Routing.MaxBounces is left 0.
// An empty name reads as the tapestry recipe, and a name that selects no embedded recipe is refused.
//
// Every row of a review segment carries the budget for its segment as its MaxBounces,
// so the projection never reports the engine default for a review row.
// The exception is a zero-round segment, whose rows carry MaxBounces 0.
// The progress projection reads that 0 as shedengine's inherited budget.
// A review segment that budgets lacks is refused, naming the segment and the loom wiring's segment-to-perch table.
// A negative entry is refused, naming the segment.
func Routing(recipe string, budgets map[string]int) (shedengine.Routing, error) {
	parsed, err := parseEmbeddedRecipe(recipe)
	if err != nil {
		return shedengine.Routing{}, err
	}
	routing := shedbuild.RoutingFrom(parsed)

	segments := reviewSegments(parsed)
	if err := checkSegmentBudgets(segments, budgets, loomSegmentTableWayForward); err != nil {
		return shedengine.Routing{}, err
	}
	applyReviewBudget(routing.Producers, segments, budgets)

	return routing, nil
}

// segmentBudgets maps each review segment of segments to its MaxBounces in env.Segments.
// A segment with no env.Segments entry is refused, naming the segment and wayForward.
// A negative budget is refused, naming the segment, its MaxBouncesKey and ConfigFile.
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

// checkSegmentBudgets refuses a review segment of segments that budgets lacks, naming the segment and wayForward.
// It also refuses a negative entry, naming the segment.
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

// reviewSegments maps each segment of the expanded recipe that holds a Bouncer row to that Bouncer row's name.
func reviewSegments(recipe shedbuild.Recipe) map[string]string {
	segments := map[string]string{}
	for _, row := range recipe.Producers {
		if row.Engine == bouncerEngine && row.Segment != "" {
			segments[row.Segment] = row.Name
		}
	}

	return segments
}

// applyReviewBudget sets MaxBounces on every producer of a review segment, to that segment's budget.
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
// A stamped Bouncer settles approved at once, the same budget of 0 that applyReviewBudget sets on its rows.
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
