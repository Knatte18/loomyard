// review.go implements ResolveReview, the review segment's config-to-settings resolver:
// a pure composer that parses and resolves each review segment's reviewer and fixer model-spec lists, Darn-Review's through ResolveDarnReview, and pairs them with the review round timeout.
// Unlike DiscussionTable and PlanTable, it returns no seatengine.Table.
// There is no seat to build here, because the review segment's prompts are the Bouncer's own stencils, composed inside internal/shedadapters at call time.
// The caller threads ReviewSettings' values onto shedrecipe.Env instead.

package loomengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ReviewSettings is the review segment's run-wide model and timeout settings, resolved once from
// Config and threaded onto shedrecipe.Env for every review-segment row to fall back to.
type ReviewSettings struct {
	// Models holds the resolved shared reviewer and fixer model lists, the fallback of every segment whose own key is empty.
	Models burlerengine.RoundModels
	// Discussion, Plan, Webster and Merge hold each loom review segment's resolved models: its own list where loom.yaml sets one, else the shared list in Models.
	Discussion burlerengine.RoundModels
	Plan       burlerengine.RoundModels
	Webster    burlerengine.RoundModels
	Merge      burlerengine.RoundModels
	// DiscussionKeys, PlanKeys, WebsterKeys and MergeKeys name the config keys each of those segments' models came from.
	DiscussionKeys SegmentKeys
	PlanKeys       SegmentKeys
	WebsterKeys    SegmentKeys
	MergeKeys      SegmentKeys
	// Timeout is one review round's shuttle-run deadline, derived from cfg.ReviewTimeoutMin.
	Timeout time.Duration
}

// SegmentKeys names the config keys one review segment's reviewer and fixer model lists came from:
// the segment's own key when it is set, else loom.yaml's shared key.
type SegmentKeys struct {
	Review shuttleengine.ModelKey
	Fix    shuttleengine.ModelKey
}

// segmentModelSource is one review segment's own reviewer and fixer model lists with the config keys holding them.
type segmentModelSource struct {
	reviewKey, fixKey shuttleengine.ModelKey
	review, fix       burlerengine.ModelSpecList
}

// loomSegmentModelSource returns the loom.yaml model lists of the review segment named segment, held by <segment>_reviewer_model and <segment>_fixer_model.
func loomSegmentModelSource(segment string, review, fix burlerengine.ModelSpecList) segmentModelSource {
	return segmentModelSource{
		reviewKey: shuttleengine.ModelKey{Module: loomModule, Key: segment + "_reviewer_model"},
		fixKey:    shuttleengine.ModelKey{Module: loomModule, Key: segment + "_fixer_model"},
		review:    review,
		fix:       fix,
	}
}

// sharedSegmentKeys names loom.yaml's shared reviewer and fixer model keys.
var sharedSegmentKeys = SegmentKeys{
	Review: shuttleengine.ModelKey{Module: loomModule, Key: "reviewer_model"},
	Fix:    shuttleengine.ModelKey{Module: loomModule, Key: "fixer_model"},
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

// ResolveReview resolves every entry of the shared reviewer and fixer model-spec lists and of each loom segment's set list through reg,
// and pairs them with the review round timeout, returning the ReviewSettings the caller threads onto shedrecipe.Env.
// Every entry is resolved at run start, so a bad later-round entry fails before round 1.
func ResolveReview(cfg Config, reg modelspec.Registry) (ReviewSettings, error) {
	review, err := resolveModelChoices(sharedSegmentKeys.Review, cfg.ReviewerModel, reg)
	if err != nil {
		return ReviewSettings{}, err
	}
	fix, err := resolveModelChoices(sharedSegmentKeys.Fix, cfg.FixerModel, reg)
	if err != nil {
		return ReviewSettings{}, err
	}

	settings := ReviewSettings{
		Models:  burlerengine.RoundModels{Review: review, Fix: fix},
		Timeout: time.Duration(cfg.ReviewTimeoutMin) * time.Minute,
	}
	for _, segment := range []struct {
		source segmentModelSource
		models *burlerengine.RoundModels
		keys   *SegmentKeys
	}{
		{loomSegmentModelSource("discussion", cfg.DiscussionReviewerModel, cfg.DiscussionFixerModel), &settings.Discussion, &settings.DiscussionKeys},
		{loomSegmentModelSource("plan", cfg.PlanReviewerModel, cfg.PlanFixerModel), &settings.Plan, &settings.PlanKeys},
		{loomSegmentModelSource("webster", cfg.WebsterReviewerModel, cfg.WebsterFixerModel), &settings.Webster, &settings.WebsterKeys},
		{loomSegmentModelSource("merge", cfg.MergeReviewerModel, cfg.MergeFixerModel), &settings.Merge, &settings.MergeKeys},
	} {
		*segment.models, *segment.keys, err = resolveSegmentModels(segment.source, settings.Models, reg)
		if err != nil {
			return ReviewSettings{}, err
		}
	}
	return settings, nil
}

// ResolveDarnReview resolves the Darn-Review perch's reviewer and fixer models from darn.yaml's darn_reviewer_model and darn_fixer_model through reg, and returns them with the config keys they came from.
// Each empty key takes its list in shared, the resolved ReviewSettings.Models.
func ResolveDarnReview(shared burlerengine.RoundModels, darn darnengine.Config, reg modelspec.Registry) (burlerengine.RoundModels, SegmentKeys, error) {
	return resolveSegmentModels(segmentModelSource{
		reviewKey: shuttleengine.ModelKey{Module: "darn", Key: darnengine.ReviewerModelKey},
		fixKey:    shuttleengine.ModelKey{Module: "darn", Key: darnengine.FixerModelKey},
		review:    darn.ReviewerModel,
		fix:       darn.FixerModel,
	}, shared, reg)
}

// resolveSegmentModels resolves one review segment's reviewer and fixer lists and returns them with the keys they came from.
// Each list falls back independently to shared when its key is unset.
func resolveSegmentModels(source segmentModelSource, shared burlerengine.RoundModels, reg modelspec.Registry) (burlerengine.RoundModels, SegmentKeys, error) {
	models, keys := shared, sharedSegmentKeys
	if !source.review.Unset() {
		review, err := resolveModelChoices(source.reviewKey, source.review, reg)
		if err != nil {
			return burlerengine.RoundModels{}, SegmentKeys{}, err
		}
		models.Review, keys.Review = review, source.reviewKey
	}
	if !source.fix.Unset() {
		fix, err := resolveModelChoices(source.fixKey, source.fix, reg)
		if err != nil {
			return burlerengine.RoundModels{}, SegmentKeys{}, err
		}
		models.Fix, keys.Fix = fix, source.fixKey
	}
	return models, keys, nil
}

// resolveModelChoices parses and resolves each entry of specs, naming key, its file and the 1-based entry index in an error.
func resolveModelChoices(key shuttleengine.ModelKey, specs burlerengine.ModelSpecList, reg modelspec.Registry) ([]shuttleengine.ModelChoice, error) {
	choices := make([]shuttleengine.ModelChoice, 0, len(specs))
	for i, raw := range specs {
		spec, err := modelspec.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("loom: ResolveReview: %s entry %d: %w; set that entry in %s.yaml to a model-spec the registry defines", key.Key, i+1, err, key.Module)
		}
		resolved, err := reg.Resolve(spec)
		if err != nil {
			return nil, fmt.Errorf("loom: ResolveReview: %s entry %d: %w; set that entry in %s.yaml to a model-spec the registry defines", key.Key, i+1, err, key.Module)
		}
		choices = append(choices, shuttleengine.NewModelChoice(resolved))
	}
	return choices, nil
}
