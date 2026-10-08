// review.go implements ResolveReview, the review segment's config-to-settings resolver:
// a pure composer that parses and resolves the review and fix roles' model-spec lists and pairs them with the review round timeout.
// Unlike DiscussionSpec and PlanSpec, it returns no shuttleengine.Spec -- there is no prompt to
// compose here, because the review segment's prompts are the Bouncer's own stencils, composed
// inside internal/shedadapters at call time. The caller threads ReviewSettings' values onto
// shedrecipe.Env instead.

package loomengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
)

// ReviewSettings is the review segment's run-wide model and timeout settings, resolved once from
// Config and threaded onto shedrecipe.Env for every review-segment row to fall back to.
type ReviewSettings struct {
	// Models holds the resolved review and fix model lists the burler round picks from per round.
	Models burlerengine.RoundModels
	// Discussion, Plan and Webster hold each review segment's resolved models: its own list where loom.yaml sets one, else the run-wide list in Models.
	Discussion burlerengine.RoundModels
	Plan       burlerengine.RoundModels
	Webster    burlerengine.RoundModels
	// Timeout is one review round's shuttle-run deadline, derived from cfg.ReviewTimeoutMin.
	Timeout time.Duration
}

// JudgeSettings is the Bouncer rows' run-wide model settings, resolved once from Config and threaded onto shedrecipe.Env for every Bouncer row to fall back to.
// It carries no timeout, since BouncerConfig carries none.
type JudgeSettings struct {
	// Model is the resolved judge role's provider-side model string.
	Model string
	// Effort is the resolved judge role's "effort" parameter, empty when unset.
	Effort string
	// Version is the resolved judge role's "version" parameter, empty when unset.
	Version string
}

// ResolveJudge parses and resolves the judge role's model-spec from cfg, returning the JudgeSettings the caller threads onto shedrecipe.Env.
func ResolveJudge(cfg Config, reg modelspec.Registry) (JudgeSettings, error) {
	spec, err := modelspec.Parse(cfg.Judge)
	if err != nil {
		return JudgeSettings{}, fmt.Errorf("loom: ResolveJudge: judge role model-spec: %w", err)
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return JudgeSettings{}, fmt.Errorf("loom: ResolveJudge: judge role model-spec: %w", err)
	}

	return JudgeSettings{
		Model:   resolved.Model,
		Effort:  resolved.Params["effort"],
		Version: resolved.Params["version"],
	}, nil
}

// ResolveReview resolves every entry of the review and fix model-spec lists and of each set per-segment list through reg,
// and pairs them with the review round timeout, returning the ReviewSettings the caller threads onto shedrecipe.Env.
// Every entry is resolved at run start, so a bad later-round entry fails before round 1.
func ResolveReview(cfg Config, reg modelspec.Registry) (ReviewSettings, error) {
	review, err := resolveModelChoices("review", cfg.Review, reg)
	if err != nil {
		return ReviewSettings{}, err
	}
	fix, err := resolveModelChoices("fix", cfg.Fix, reg)
	if err != nil {
		return ReviewSettings{}, err
	}

	runWide := burlerengine.RoundModels{Review: review, Fix: fix}

	discussion, err := resolveSegmentModels("discussion", cfg.DiscussionReview, cfg.DiscussionFix, runWide, reg)
	if err != nil {
		return ReviewSettings{}, err
	}
	plan, err := resolveSegmentModels("plan", cfg.PlanReview, cfg.PlanFix, runWide, reg)
	if err != nil {
		return ReviewSettings{}, err
	}
	webster, err := resolveSegmentModels("webster", cfg.WebsterReview, cfg.WebsterFix, runWide, reg)
	if err != nil {
		return ReviewSettings{}, err
	}

	return ReviewSettings{
		Models:     runWide,
		Discussion: discussion,
		Plan:       plan,
		Webster:    webster,
		Timeout:    time.Duration(cfg.ReviewTimeoutMin) * time.Minute,
	}, nil
}

// resolveSegmentModels resolves one review segment's reviewer and fixer lists, each falling back independently to runWide when its key is unset.
func resolveSegmentModels(segment string, reviewSpecs, fixSpecs ModelSpecList, runWide burlerengine.RoundModels, reg modelspec.Registry) (burlerengine.RoundModels, error) {
	models := runWide
	if !isUnsetModelSpecList(reviewSpecs) {
		review, err := resolveModelChoices(segment+"_review", reviewSpecs, reg)
		if err != nil {
			return burlerengine.RoundModels{}, err
		}
		models.Review = review
	}
	if !isUnsetModelSpecList(fixSpecs) {
		fix, err := resolveModelChoices(segment+"_fix", fixSpecs, reg)
		if err != nil {
			return burlerengine.RoundModels{}, err
		}
		models.Fix = fix
	}
	return models, nil
}

// resolveModelChoices parses and resolves each entry of specs, naming key and the 1-based entry index in an error.
func resolveModelChoices(key string, specs ModelSpecList, reg modelspec.Registry) ([]burlerengine.ModelChoice, error) {
	choices := make([]burlerengine.ModelChoice, 0, len(specs))
	for i, raw := range specs {
		spec, err := modelspec.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("loom: ResolveReview: %s entry %d: %w; set that entry in loom.yaml to a model-spec the registry defines", key, i+1, err)
		}
		resolved, err := reg.Resolve(spec)
		if err != nil {
			return nil, fmt.Errorf("loom: ResolveReview: %s entry %d: %w; set that entry in loom.yaml to a model-spec the registry defines", key, i+1, err)
		}
		choices = append(choices, burlerengine.ModelChoice{
			Model:   resolved.Model,
			Effort:  resolved.Params["effort"],
			Version: resolved.Params["version"],
		})
	}
	return choices, nil
}
