// review.go implements ResolveReview, the review segment's config-to-settings resolver:
// a pure composer that parses and resolves the review and fix roles' model-spec lists and pairs them with the review round timeout.
// Unlike DiscussionTable and PlanTable, it returns no seatengine.Table.
// There is no seat to build here, because the review segment's prompts are the Bouncer's own stencils, composed inside internal/shedadapters at call time.
// The caller threads ReviewSettings' values onto shedrecipe.Env instead.

package loomengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
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

// BouncerModels is one review segment's resolved seed and judge pair, each an already-resolved engine, model, effort and version.
type BouncerModels struct {
	Seed  shuttleengine.ModelChoice
	Judge shuttleengine.ModelChoice
}

// JudgeSettings is each review segment's resolved Bouncer models, resolved once from Config and threaded onto shedrecipe.Env per Bouncer row.
// It carries no timeout, since BouncerConfig carries none.
type JudgeSettings struct {
	Discussion BouncerModels
	Plan       BouncerModels
	Webster    BouncerModels
}

// ResolveJudge resolves each review segment's judge and seed models from cfg, returning the JudgeSettings the caller threads onto shedrecipe.Env.
// A segment's judge is its own judge key when non-empty, else judge.
// A segment's seed is its own seed key when non-empty, else seed when non-empty, else the segment's resolved judge.
// An error names the key the failing value came from.
func ResolveJudge(cfg Config, reg modelspec.Registry) (JudgeSettings, error) {
	discussion, err := resolveSegmentBouncerModels("discussion", cfg.DiscussionJudge, cfg.DiscussionSeed, cfg, reg)
	if err != nil {
		return JudgeSettings{}, err
	}
	plan, err := resolveSegmentBouncerModels("plan", cfg.PlanJudge, cfg.PlanSeed, cfg, reg)
	if err != nil {
		return JudgeSettings{}, err
	}
	webster, err := resolveSegmentBouncerModels("webster", cfg.WebsterJudge, cfg.WebsterSeed, cfg, reg)
	if err != nil {
		return JudgeSettings{}, err
	}
	return JudgeSettings{Discussion: discussion, Plan: plan, Webster: webster}, nil
}

// resolveSegmentBouncerModels resolves one segment's judge and seed pairs, applying the fallback order ResolveJudge documents.
// segment prefixes the per-segment key names in an error.
func resolveSegmentBouncerModels(segment, segmentJudge, segmentSeed string, cfg Config, reg modelspec.Registry) (BouncerModels, error) {
	judgeKey, judgeRaw := "judge", cfg.Judge
	if segmentJudge != "" {
		judgeKey, judgeRaw = segment+"_judge", segmentJudge
	}
	seedKey, seedRaw := judgeKey, judgeRaw
	switch {
	case segmentSeed != "":
		seedKey, seedRaw = segment+"_seed", segmentSeed
	case cfg.Seed != "":
		seedKey, seedRaw = "seed", cfg.Seed
	}

	judge, err := resolveKeyedModelChoice(judgeKey, judgeRaw, reg)
	if err != nil {
		return BouncerModels{}, err
	}
	seed, err := resolveKeyedModelChoice(seedKey, seedRaw, reg)
	if err != nil {
		return BouncerModels{}, err
	}
	return BouncerModels{Seed: seed, Judge: judge}, nil
}

// resolveKeyedModelChoice parses and resolves one model-spec through reg, naming key in an error.
func resolveKeyedModelChoice(key, raw string, reg modelspec.Registry) (shuttleengine.ModelChoice, error) {
	spec, err := modelspec.Parse(raw)
	if err != nil {
		return shuttleengine.ModelChoice{}, fmt.Errorf("loom: ResolveJudge: %s role model-spec: %w", key, err)
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return shuttleengine.ModelChoice{}, fmt.Errorf("loom: ResolveJudge: %s role model-spec: %w", key, err)
	}
	return shuttleengine.NewModelChoice(resolved), nil
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

	fanReview, err := resolveModelChoices("fan_review", cfg.FanReview, reg)
	if err != nil {
		return ReviewSettings{}, err
	}

	discussion, err := resolveSegmentModels("discussion", cfg.DiscussionReview, cfg.DiscussionFix, fannedReview(cfg.DiscussionFan, fanReview), runWide, reg)
	if err != nil {
		return ReviewSettings{}, err
	}
	plan, err := resolveSegmentModels("plan", cfg.PlanReview, cfg.PlanFix, fannedReview(cfg.PlanFan, fanReview), runWide, reg)
	if err != nil {
		return ReviewSettings{}, err
	}
	webster, err := resolveSegmentModels("webster", cfg.WebsterReview, cfg.WebsterFix, nil, runWide, reg)
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

// fannedReview returns fanReview when fan names a fan, else nil, which leaves a segment's reviewer list to its own keys.
func fannedReview(fan string, fanReview []shuttleengine.ModelChoice) []shuttleengine.ModelChoice {
	if fan == "" {
		return nil
	}
	return fanReview
}

// resolveSegmentModels resolves one review segment's reviewer and fixer lists, each falling back independently to runWide when its key is unset.
// A non-nil fanReview replaces the reviewer list outright, since a fanned segment's reviewer runs on fan_review; the fixer list is unaffected.
func resolveSegmentModels(segment string, reviewSpecs, fixSpecs ModelSpecList, fanReview []shuttleengine.ModelChoice, runWide burlerengine.RoundModels, reg modelspec.Registry) (burlerengine.RoundModels, error) {
	models := runWide
	if fanReview != nil {
		models.Review = fanReview
	} else if !isUnsetModelSpecList(reviewSpecs) {
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
func resolveModelChoices(key string, specs ModelSpecList, reg modelspec.Registry) ([]shuttleengine.ModelChoice, error) {
	choices := make([]shuttleengine.ModelChoice, 0, len(specs))
	for i, raw := range specs {
		spec, err := modelspec.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("loom: ResolveReview: %s entry %d: %w; set that entry in loom.yaml to a model-spec the registry defines", key, i+1, err)
		}
		resolved, err := reg.Resolve(spec)
		if err != nil {
			return nil, fmt.Errorf("loom: ResolveReview: %s entry %d: %w; set that entry in loom.yaml to a model-spec the registry defines", key, i+1, err)
		}
		choices = append(choices, shuttleengine.NewModelChoice(resolved))
	}
	return choices, nil
}
