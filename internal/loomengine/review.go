// review.go implements ResolveReview, the review segment's config-to-settings resolver:
// a pure composer that parses and resolves each review segment's reviewer and fixer model-spec lists and pairs them with the review round timeout.
// Unlike DiscussionTable and PlanTable, it returns no seatengine.Table.
// There is no seat to build here, because the review segment's prompts are the Bouncer's own stencils, composed inside internal/shedadapters at call time.
// The caller threads ReviewSettings' values onto shedrecipe.Env instead.

package loomengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ReviewSettings is the review segment's run-wide model and timeout settings, resolved once from
// Config and threaded onto shedrecipe.Env for every review-segment row to fall back to.
type ReviewSettings struct {
	// Models holds the resolved shared reviewer and fixer model lists, the fallback of every segment whose own key is empty.
	Models burlerengine.RoundModels
	// Discussion, Plan and Code hold each loom review segment's resolved models: its own list where loom.yaml sets one, else the shared list in Models.
	Discussion burlerengine.RoundModels
	Plan       burlerengine.RoundModels
	Code       burlerengine.RoundModels
	// DiscussionKeys, PlanKeys and CodeKeys name the config keys each of those segments' models came from.
	DiscussionKeys SegmentKeys
	PlanKeys       SegmentKeys
	CodeKeys       SegmentKeys
	// Timeout is one review round's shuttle-run deadline, derived from cfg.ReviewTimeoutMin.
	Timeout time.Duration
	// DiscussionAssist, PlanAssist and CodeAssist hold each loom review segment's resolved assistant reviewer lists.
	DiscussionAssist AssistModels
	PlanAssist       AssistModels
	CodeAssist       AssistModels
}

// AssistModels is one review segment's resolved assistant reviewer lists: a model choice per round, the zero choice for an empty entry, and the fan list, with the keys they came from.
type AssistModels struct {
	Models   []shuttleengine.ModelChoice
	Fans     lenses.FanList
	ModelKey shuttleengine.ModelKey
	FanKey   string
}

// CapAssistTimeout returns the assistant reviewer's run bound: assist_reviewer_timeout_min, capped so the assistant's start with its start retries, its run and one await cap end before the chair's own timeout.
// chairTimeout is review_timeout_min, or shuttle's run_timeout_min when that key is 0; startAllowance is what shuttle grants a launch and its start retries, and awaitCap is the chair's keep-warm cap.
// A bound of 0 takes the whole room, so a zero never reaches a seat, where it would read as the chair's own timeout.
// A capped bound logs one Warn naming assist_reviewer_timeout_min and review_timeout_min.
// A cap that leaves no time returns a zero bound and a non-empty reason naming both keys, and logs a Warn.
func CapAssistTimeout(cfg Config, runTimeout, startAllowance, awaitCap time.Duration) (time.Duration, string) {
	chair := time.Duration(cfg.ReviewTimeoutMin) * time.Minute
	if cfg.ReviewTimeoutMin == 0 {
		chair = runTimeout
	}
	room := chair - startAllowance - awaitCap
	want := time.Duration(cfg.AssistReviewerTimeoutMin) * time.Minute
	if room <= 0 {
		reason := fmt.Sprintf("assist_reviewer_timeout_min leaves the assistant no time: the reviewer's timeout of %s (review_timeout_min) is spent by the assistant's start allowance of %s and one await cap of %s; raise review_timeout_min", chair, startAllowance, awaitCap)
		logger.Warn("loom: the assistant reviewer gets no time and does not run", "key", "assist_reviewer_timeout_min", "chairKey", "review_timeout_min", "chair", chair, "startAllowance", startAllowance, "awaitCap", awaitCap)
		return 0, reason
	}
	if want == 0 {
		return room, ""
	}
	if want > room {
		logger.Warn("loom: assist_reviewer_timeout_min is capped to end before the reviewer's own timeout", "key", "assist_reviewer_timeout_min", "chairKey", "review_timeout_min", "value", want, "capped", room)
		return room, ""
	}
	return want, ""
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

// loomSegmentModelSource returns the model lists of the review segment named segment, held by <segment>_reviewer_model and <segment>_fixer_model.
// The plan segment's keys are tapestry.yaml's and every other segment's are loom.yaml's, which the keys' module names.
func loomSegmentModelSource(segment string, review, fix burlerengine.ModelSpecList) segmentModelSource {
	module := loomModule
	if Perch(segment) == PerchPlan {
		module = tapestryModule
	}
	return segmentModelSource{
		reviewKey: shuttleengine.ModelKey{Module: module, Key: segment + "_reviewer_model"},
		fixKey:    shuttleengine.ModelKey{Module: module, Key: segment + "_fixer_model"},
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
	Code       BouncerModels
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
	code, err := resolveSegmentBouncerModels("code", cfg.CodeJudge, cfg.CodeSeed, cfg, reg)
	if err != nil {
		return JudgeSettings{}, err
	}
	return JudgeSettings{Discussion: discussion, Plan: plan, Code: code}, nil
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
		{loomSegmentModelSource("code", cfg.CodeReviewerModel, cfg.CodeFixerModel), &settings.Code, &settings.CodeKeys},
	} {
		*segment.models, *segment.keys, err = resolveSegmentModels(segment.source, settings.Models, reg)
		if err != nil {
			return ReviewSettings{}, err
		}
	}
	for _, assist := range []struct {
		segment string
		models  burlerengine.ModelSpecList
		fans    lenses.FanList
		into    *AssistModels
	}{
		{"discussion", cfg.DiscussionAssistReviewerModel, cfg.DiscussionAssistReviewerFan, &settings.DiscussionAssist},
		{"plan", cfg.PlanAssistReviewerModel, cfg.PlanAssistReviewerFan, &settings.PlanAssist},
		{"code", cfg.CodeAssistReviewerModel, cfg.CodeAssistReviewerFan, &settings.CodeAssist},
	} {
		*assist.into, err = resolveAssistModels(assist.segment, assist.models, assist.fans, reg)
		if err != nil {
			return ReviewSettings{}, err
		}
	}
	return settings, nil
}

// resolveAssistModels resolves one segment's assistant model list through reg and pairs it with the segment's assistant fan list and the keys both came from.
// An empty entry resolves to the zero choice, which runs no assistant that round.
func resolveAssistModels(segment string, models burlerengine.ModelSpecList, fans lenses.FanList, reg modelspec.Registry) (AssistModels, error) {
	module := loomModule
	if Perch(segment) == PerchPlan {
		module = tapestryModule
	}
	key := shuttleengine.ModelKey{Module: module, Key: segment + "_assist_reviewer_model"}
	choices := make([]shuttleengine.ModelChoice, 0, len(models))
	for i, raw := range models {
		if raw == "" {
			choices = append(choices, shuttleengine.ModelChoice{})
			continue
		}
		choice, err := resolveModelEntry(key, i, raw, reg)
		if err != nil {
			return AssistModels{}, err
		}
		choices = append(choices, choice)
	}
	return AssistModels{Models: choices, Fans: fans, ModelKey: key, FanKey: segment + "_assist_reviewer_fan"}, nil
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
		choice, err := resolveModelEntry(key, i, raw, reg)
		if err != nil {
			return nil, err
		}
		choices = append(choices, choice)
	}
	return choices, nil
}

// resolveModelEntry parses and resolves the entry raw at the 0-based index of key's list, naming key, its file and the 1-based entry index in an error.
func resolveModelEntry(key shuttleengine.ModelKey, index int, raw string, reg modelspec.Registry) (shuttleengine.ModelChoice, error) {
	spec, err := modelspec.Parse(raw)
	if err != nil {
		return shuttleengine.ModelChoice{}, fmt.Errorf("loom: ResolveReview: %s entry %d: %w; set that entry in %s.yaml to a model-spec the registry defines", key.Key, index+1, err, key.Module)
	}
	resolved, err := reg.Resolve(spec)
	if err != nil {
		return shuttleengine.ModelChoice{}, fmt.Errorf("loom: ResolveReview: %s entry %d: %w; set that entry in %s.yaml to a model-spec the registry defines", key.Key, index+1, err, key.Module)
	}
	return shuttleengine.NewModelChoice(resolved), nil
}
