// loomrecipe.go implements New: the coherence guard across env and paths, then the parse of contracts/recipes.LoomRecipe, the Discussion-Write engine choice, and the delegation to shedbuild.NewShedFrom.
// NewShedFrom builds the recipe against a caller-supplied shedrecipe.Env and returns the assembled *shedengine.Shed.

package loomrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/loomshed"
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
// New runs the Discussion-Write row on the DiscussionSeats engine when env.DiscussionSeats is true, by changing the parsed recipe's engine for that row before the build; Routing never reads the choice.
//
// New sets MaxBounces on every row of a review segment (a segment holding a
// Bouncer row) after the build: the segment's Bouncer's env.RowMaxBounces entry when above 0, env.ReviewMaxBounces otherwise.
// It refuses an env.ReviewMaxBounces below 1 as its first act.
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

	if err := checkReviewBudget("ReviewMaxBounces", env.ReviewMaxBounces); err != nil {
		return nil, err
	}

	parsed, err := shedbuild.Parse(recipes.LoomRecipe)
	if err != nil {
		return nil, fmt.Errorf("loomrecipe: %w", err)
	}
	applyDiscussionProducer(&parsed, env.DiscussionSeats)

	shed, err := shedbuild.NewShedFrom(parsed, env, paths)
	if err != nil {
		return nil, fmt.Errorf("loomrecipe: %w", err)
	}

	segments, err := reviewSegments(recipes.LoomRecipe)
	if err != nil {
		return nil, err
	}
	if err := applyReviewBudget(shed.Producers, segments, env.ReviewMaxBounces, env.RowMaxBounces); err != nil {
		return nil, err
	}

	return shed, nil
}

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

// applyDiscussionProducer runs the Discussion-Write row on the seat-table engine when seats is true, and leaves recipe alone otherwise.
// Only the row's engine changes: its name, gates and routing stay, so the durable resume identity and the status file are the same whichever producer runs.
func applyDiscussionProducer(recipe *shedbuild.Recipe, seats bool) {
	if !seats {
		return
	}
	for i := range recipe.Producers {
		if recipe.Producers[i].Name == loomshed.NameDiscussionWrite {
			recipe.Producers[i].Engine = discussionSeatsEngine
		}
	}
}

// Routing projects the embedded loom recipe's routing without building any engine, so a caller
// that never builds a Shed (the status verb) can compute progress. A parse error is returned
// prefixed with "loomrecipe: ". Routing.MaxBounces is left 0.
//
// Every row of a review segment carries its Bouncer's rowMaxBounces entry, or reviewMaxBounces where the entry is absent or 0, as its MaxBounces, the same value New
// applies, so the projection never reports the engine default for a review row.
// A reviewMaxBounces below 1 is refused, naming ReviewMaxBounces, and a negative rowMaxBounces entry for a Bouncer row is refused, naming the row.
func Routing(reviewMaxBounces int, rowMaxBounces map[string]int) (shedengine.Routing, error) {
	if err := checkReviewBudget("ReviewMaxBounces", reviewMaxBounces); err != nil {
		return shedengine.Routing{}, err
	}

	routing, err := shedbuild.RoutingOf(recipes.LoomRecipe)
	if err != nil {
		return shedengine.Routing{}, fmt.Errorf("loomrecipe: %w", err)
	}

	segments, err := reviewSegments(recipes.LoomRecipe)
	if err != nil {
		return shedengine.Routing{}, err
	}
	if err := applyReviewBudget(routing.Producers, segments, reviewMaxBounces, rowMaxBounces); err != nil {
		return shedengine.Routing{}, err
	}

	return routing, nil
}

// checkReviewBudget refuses a review bounce budget below 1; name is the caller's own name for the value.
func checkReviewBudget(name string, budget int) error {
	if budget < 1 {
		return fmt.Errorf("loomrecipe: %s = %d; want at least 1 (set review_max_bounces in loom.yaml)", name, budget)
	}

	return nil
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

// applyReviewBudget sets MaxBounces on every producer whose segment is a review segment.
// A segment's rows take its Bouncer's rowMaxBounces entry when that is above 0, and fallback otherwise.
// A negative entry for a Bouncer row of segments is a wiring error naming that row.
func applyReviewBudget(producers []shedengine.ProducerDef, segments map[string]string, fallback int, rowMaxBounces map[string]int) error {
	for _, bouncer := range segments {
		if budget := rowMaxBounces[bouncer]; budget < 0 {
			return fmt.Errorf("loomrecipe: bounce budget of Bouncer row %q = %d; want 0 or more (set the perch's max_bounces key in loom.yaml)", bouncer, budget)
		}
	}

	for i := range producers {
		bouncer, ok := segments[producers[i].Segment]
		if !ok {
			continue
		}
		producers[i].MaxBounces = fallback
		if budget := rowMaxBounces[bouncer]; budget > 0 {
			producers[i].MaxBounces = budget
		}
	}

	return nil
}
