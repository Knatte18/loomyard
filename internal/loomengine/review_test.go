// review_test.go — untagged Tier-1 unit tests for ResolveReview and LoomReviewsDir.
// ResolveReview's tests are pure Go over an in-memory Config and a temp-dir modelspec registry.
// They involve no live hub, reed, or network.
// LoomReviewsDir's test is pure path arithmetic over a hand-built lymcwd.Location.
// The accessor is durable, rooted under _lym.

package loomengine

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestResolveReview verifies ResolveReview returns the expected model/effort/version triples.
// It also checks the keys each segment's models came from, and the timeout.
func TestResolveReview(t *testing.T) {
	tiergate.Require(t, 0)
	cfg := Config{
		ReviewerModel:    burlerengine.ModelSpecList{"opus[effort=high]", "sonnet[medium]"},
		FixerModel:       burlerengine.ModelSpecList{"opus[effort=low]"},
		ReviewTimeoutMin: 240,
		// Discussion sets only its fixer list, Plan only its reviewer list, and Code leaves both unset.
		// Code's unset lists hold one empty entry, as the template loads.
		DiscussionFixerModel: burlerengine.ModelSpecList{"sonnet[effort=high]"},
		PlanReviewerModel:    burlerengine.ModelSpecList{"sonnet[effort=low]", "opus[effort=high]"},
		CodeReviewerModel:    burlerengine.ModelSpecList{""},
		CodeFixerModel:       burlerengine.ModelSpecList{""},
	}

	reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	settings, err := ResolveReview(cfg, reg)
	if err != nil {
		t.Fatalf("ResolveReview(...) = _, %v; want nil error", err)
	}
	if len(settings.Models.Review) != 2 || len(settings.Models.Fix) != 1 {
		t.Fatalf("ResolveReview(...).Models = %+v; want two review entries and one fix entry", settings.Models)
	}
	if settings.Models.Review[0].Model == "" {
		t.Error("ResolveReview(...).Models.Review[0].Model = \"\"; want non-empty")
	}
	if settings.Models.Review[0].Effort != "high" || settings.Models.Review[1].Effort != "medium" {
		t.Errorf("ResolveReview(...).Models.Review efforts = %q, %q; want high, medium", settings.Models.Review[0].Effort, settings.Models.Review[1].Effort)
	}
	if settings.Models.Fix[0].Effort != "low" {
		t.Errorf("ResolveReview(...).Models.Fix[0].Effort = %q; want %q", settings.Models.Fix[0].Effort, "low")
	}
	// A set segment key replaces the run-wide list, review and fix falling back independently;
	// an unset one takes the run-wide list.
	if got := settings.Discussion; !reflect.DeepEqual(got.Review, settings.Models.Review) || len(got.Fix) != 1 || got.Fix[0].Effort != "high" {
		t.Errorf("ResolveReview(...).Discussion = %+v; want the run-wide review list and a fix list of one high-effort entry", got)
	}
	if got := settings.Plan; len(got.Review) != 2 || got.Review[0].Effort != "low" || got.Review[1].Effort != "high" || !reflect.DeepEqual(got.Fix, settings.Models.Fix) {
		t.Errorf("ResolveReview(...).Plan = %+v; want a two-entry review list (low, high) and the run-wide fix list", got)
	}
	if !reflect.DeepEqual(settings.Code, settings.Models) {
		t.Errorf("ResolveReview(...).Code = %+v; want the run-wide %+v", settings.Code, settings.Models)
	}
	wantTimeout := 240 * time.Minute
	if settings.Timeout != wantTimeout {
		t.Errorf("ResolveReview(...).Timeout = %s; want %s", settings.Timeout, wantTimeout)
	}

	// Each segment records the key its models came from.
	// That is its own key when set, in tapestry.yaml for the plan segment and loom.yaml for the others;
	// otherwise it is loom.yaml's shared key.
	loomKey := func(key string) shuttleengine.ModelKey { return shuttleengine.ModelKey{Module: "loom", Key: key} }
	tapestryKey := func(key string) shuttleengine.ModelKey { return shuttleengine.ModelKey{Module: "tapestry", Key: key} }
	for _, tt := range []struct {
		segment   string
		got, want SegmentKeys
	}{
		{"Discussion", settings.DiscussionKeys, SegmentKeys{Review: loomKey("reviewer_model"), Fix: loomKey("discussion_fixer_model")}},
		{"Plan", settings.PlanKeys, SegmentKeys{Review: tapestryKey("plan_reviewer_model"), Fix: loomKey("fixer_model")}},
		{"Code", settings.CodeKeys, SegmentKeys{Review: loomKey("reviewer_model"), Fix: loomKey("fixer_model")}},
	} {
		if tt.got != tt.want {
			t.Errorf("ResolveReview(...).%sKeys = %+v; want %+v", tt.segment, tt.got, tt.want)
		}
	}

	// A set fan key changes no model:
	// a fanned segment's reviewer list is still its own review list, else the run-wide one.
	cfg.DiscussionReviewerFan = lenses.FanList{"standard"}
	cfg.PlanReviewerFan = lenses.FanList{"", "full"}
	cfg.DiscussionFixerFan = lenses.FanList{"fix-discussion"}
	cfg.CodeReviewerFan = lenses.FanList{"review-code"}
	cfg.CodeFixerFan = lenses.FanList{"fix-code"}
	fanned, err := ResolveReview(cfg, reg)
	if err != nil {
		t.Fatalf("ResolveReview(fanned cfg) = _, %v; want nil error", err)
	}
	if !reflect.DeepEqual(fanned, settings) {
		t.Errorf("ResolveReview(fanned cfg) = %+v; want the settings of the unfanned cfg, %+v", fanned, settings)
	}

	// Each entry carries the engine its alias resolves to, in list order.
	engineReg := modelspec.Registry{
		"alpha": {Engine: "claude", Model: "alpha-model"},
		"beta":  {Engine: "other", Model: "beta-model"},
	}
	engineSettings, err := ResolveReview(Config{ReviewerModel: burlerengine.ModelSpecList{"alpha", "beta"}, FixerModel: burlerengine.ModelSpecList{"beta"}}, engineReg)
	if err != nil {
		t.Fatalf("ResolveReview(engine registry) = _, %v; want nil error", err)
	}
	wantReview := []shuttleengine.ModelChoice{{Engine: "claude", Model: "alpha-model"}, {Engine: "other", Model: "beta-model"}}
	if !reflect.DeepEqual(engineSettings.Models.Review, wantReview) {
		t.Errorf("ResolveReview(engine registry).Models.Review = %+v; want %+v", engineSettings.Models.Review, wantReview)
	}
	if got := engineSettings.Models.Fix; len(got) != 1 || got[0].Engine != "other" {
		t.Errorf("ResolveReview(engine registry).Models.Fix = %+v; want one entry on engine other", got)
	}
}

// TestResolveReview_Assist verifies the assistant lists resolve per round.
// An empty entry, the default, is the zero choice; a model-spec resolves through the registry.
// Each segment names the keys its lists came from, the plan segment's in tapestry.yaml.
func TestResolveReview_Assist(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	reg := modelspec.Registry{"luna": {Engine: "codex", Model: "luna-model"}}
	luna := shuttleengine.ModelChoice{Engine: "codex", Model: "luna-model"}
	zero := shuttleengine.ModelChoice{}
	loomKey := func(key string) shuttleengine.ModelKey { return shuttleengine.ModelKey{Module: "loom", Key: key} }
	tapestryKey := func(key string) shuttleengine.ModelKey { return shuttleengine.ModelKey{Module: "tapestry", Key: key} }

	t.Run("the default empty entry gives no assistant model in any segment", func(t *testing.T) {
		t.Parallel()
		cfg := Config{
			DiscussionAssistReviewerModel: burlerengine.ModelSpecList{""},
			PlanAssistReviewerModel:       burlerengine.ModelSpecList{""},
			CodeAssistReviewerModel:       burlerengine.ModelSpecList{""},
		}
		settings, err := ResolveReview(cfg, reg)
		if err != nil {
			t.Fatalf("ResolveReview(...) = _, %v; want nil error", err)
		}
		for name, got := range map[string]AssistModels{"Discussion": settings.DiscussionAssist, "Plan": settings.PlanAssist, "Code": settings.CodeAssist} {
			if !reflect.DeepEqual(got.Models, []shuttleengine.ModelChoice{zero}) {
				t.Errorf("ResolveReview(...).%sAssist.Models = %+v; want one zero choice", name, got.Models)
			}
		}
	})

	t.Run("unset lists give no choices", func(t *testing.T) {
		t.Parallel()
		settings, err := ResolveReview(Config{}, reg)
		if err != nil {
			t.Fatalf("ResolveReview(Config{}) = _, %v; want nil error", err)
		}
		if got := settings.CodeAssist.Models; len(got) != 0 {
			t.Errorf("ResolveReview(Config{}).CodeAssist.Models = %+v; want none", got)
		}
	})

	t.Run("a per-round list resolves round by round with its keys and fans", func(t *testing.T) {
		t.Parallel()
		cfg := Config{
			DiscussionAssistReviewerModel: burlerengine.ModelSpecList{"luna", ""},
			DiscussionAssistReviewerFan:   lenses.FanList{"review-discussion", ""},
			PlanAssistReviewerModel:       burlerengine.ModelSpecList{"", "luna"},
			CodeAssistReviewerModel:       burlerengine.ModelSpecList{"luna"},
		}
		settings, err := ResolveReview(cfg, reg)
		if err != nil {
			t.Fatalf("ResolveReview(...) = _, %v; want nil error", err)
		}
		want := map[string]AssistModels{
			"Discussion": {Models: []shuttleengine.ModelChoice{luna, zero}, Fans: lenses.FanList{"review-discussion", ""}, ModelKey: loomKey("discussion_assist_reviewer_model"), FanKey: "discussion_assist_reviewer_fan"},
			"Plan":       {Models: []shuttleengine.ModelChoice{zero, luna}, ModelKey: tapestryKey("plan_assist_reviewer_model"), FanKey: "plan_assist_reviewer_fan"},
			"Code":       {Models: []shuttleengine.ModelChoice{luna}, ModelKey: loomKey("code_assist_reviewer_model"), FanKey: "code_assist_reviewer_fan"},
		}
		got := map[string]AssistModels{"Discussion": settings.DiscussionAssist, "Plan": settings.PlanAssist, "Code": settings.CodeAssist}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ResolveReview(...) assists = %+v; want %+v", got, want)
		}
	})

	t.Run("a bad entry is refused naming its key, entry and file", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name   string
			cfg    Config
			wantIn []string
		}{
			{"malformed second discussion entry", Config{DiscussionAssistReviewerModel: burlerengine.ModelSpecList{"luna", "opus[effort"}}, []string{"discussion_assist_reviewer_model entry 2", "in loom.yaml"}},
			{"unknown plan alias", Config{PlanAssistReviewerModel: burlerengine.ModelSpecList{"ghost"}}, []string{"plan_assist_reviewer_model entry 1", "in tapestry.yaml"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				_, err := ResolveReview(tt.cfg, reg)
				if err == nil {
					t.Fatal("ResolveReview(...) error = nil; want a refusal")
				}
				for _, want := range tt.wantIn {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("ResolveReview(...) error = %q; want it to contain %q", err.Error(), want)
					}
				}
			})
		}
	})
}

// TestCapAssistTimeout verifies the assistant's bound is kept when it fits, and capped when it does not.
// The chair is review_timeout_min, or the run timeout when that is 0.
// The room is the chair's timeout less the start allowance and one await cap;
// with no room left the bound is zero, and the reason names both keys.
func TestCapAssistTimeout(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name           string
		cfg            Config
		runTimeout     time.Duration
		startAllowance time.Duration
		awaitCap       time.Duration
		want           time.Duration
		wantSkip       bool
	}{
		{name: "a bound that fits is kept", cfg: Config{ReviewTimeoutMin: 240, AssistReviewerTimeoutMin: 60}, runTimeout: 30 * time.Minute, startAllowance: 3 * time.Minute, awaitCap: 5 * time.Minute, want: 60 * time.Minute},
		{name: "a bound equal to the room is kept", cfg: Config{ReviewTimeoutMin: 60, AssistReviewerTimeoutMin: 50}, startAllowance: 5 * time.Minute, awaitCap: 5 * time.Minute, want: 50 * time.Minute},
		{name: "a bound past the room is capped to chair less start allowance less await cap", cfg: Config{ReviewTimeoutMin: 60, AssistReviewerTimeoutMin: 60}, startAllowance: 3 * time.Minute, awaitCap: 7 * time.Minute, want: 50 * time.Minute},
		{name: "a zero bound takes the whole room", cfg: Config{ReviewTimeoutMin: 60, AssistReviewerTimeoutMin: 0}, startAllowance: 3 * time.Minute, awaitCap: 7 * time.Minute, want: 50 * time.Minute},
		{name: "a zero review timeout takes the run timeout as the chair", cfg: Config{ReviewTimeoutMin: 0, AssistReviewerTimeoutMin: 120}, runTimeout: 100 * time.Minute, startAllowance: 10 * time.Minute, awaitCap: 5 * time.Minute, want: 85 * time.Minute},
		{name: "an allowance and await cap that spend the chair leave no time", cfg: Config{ReviewTimeoutMin: 10, AssistReviewerTimeoutMin: 60}, startAllowance: 6 * time.Minute, awaitCap: 4 * time.Minute, want: 0, wantSkip: true},
		{name: "an allowance past the chair leaves no time", cfg: Config{ReviewTimeoutMin: 5, AssistReviewerTimeoutMin: 60}, startAllowance: 6 * time.Minute, awaitCap: 4 * time.Minute, want: 0, wantSkip: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, reason := CapAssistTimeout(tt.cfg, tt.runTimeout, tt.startAllowance, tt.awaitCap)
			if got != tt.want {
				t.Errorf("CapAssistTimeout(...) bound = %s; want %s", got, tt.want)
			}
			if !tt.wantSkip {
				if reason != "" {
					t.Errorf("CapAssistTimeout(...) reason = %q; want none", reason)
				}
				return
			}
			for _, key := range []string{"assist_reviewer_timeout_min", "review_timeout_min"} {
				if !strings.Contains(reason, key) {
					t.Errorf("CapAssistTimeout(...) reason = %q; want it to name %q", reason, key)
				}
			}
		})
	}
}

// TestResolveJudge verifies ResolveJudge resolves each segment's judge and seed through the fallback order:
// a segment's judge key, else judge; a segment's seed key, else seed, else the segment's resolved judge.
func TestResolveJudge(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	// segmentModels is the seed and judge model of one segment, in discussion, plan, code order.
	type segmentModels struct{ seed, judge string }
	tests := []struct {
		name string
		cfg  Config
		want [3]segmentModels
	}{
		{
			name: "all keys empty resolves every pass to judge",
			cfg:  Config{Judge: "sonnet[medium]"},
			want: [3]segmentModels{{"sonnet", "sonnet"}, {"sonnet", "sonnet"}, {"sonnet", "sonnet"}},
		},
		{
			name: "seed moves every seed",
			cfg:  Config{Judge: "sonnet[medium]", Seed: "haiku"},
			want: [3]segmentModels{{"haiku", "sonnet"}, {"haiku", "sonnet"}, {"haiku", "sonnet"}},
		},
		{
			name: "plan_judge alone moves plan's judge and seed together",
			cfg:  Config{Judge: "sonnet[medium]", PlanJudge: "opus"},
			want: [3]segmentModels{{"sonnet", "sonnet"}, {"opus", "opus"}, {"sonnet", "sonnet"}},
		},
		{
			name: "plan_judge with seed keeps plan's seed off the judge",
			cfg:  Config{Judge: "sonnet[medium]", PlanJudge: "opus", Seed: "haiku"},
			want: [3]segmentModels{{"haiku", "sonnet"}, {"haiku", "opus"}, {"haiku", "sonnet"}},
		},
		{
			name: "plan_judge with plan_seed keeps plan's seed off the judge",
			cfg:  Config{Judge: "sonnet[medium]", PlanJudge: "opus", PlanSeed: "haiku"},
			want: [3]segmentModels{{"sonnet", "sonnet"}, {"haiku", "opus"}, {"sonnet", "sonnet"}},
		},
		{
			name: "code_seed alone moves only code's seed",
			cfg:  Config{Judge: "sonnet[medium]", CodeSeed: "haiku"},
			want: [3]segmentModels{{"sonnet", "sonnet"}, {"sonnet", "sonnet"}, {"haiku", "sonnet"}},
		},
		{
			name: "plan_seed beats seed for plan alone",
			cfg:  Config{Judge: "sonnet[medium]", Seed: "haiku", PlanSeed: "opus"},
			want: [3]segmentModels{{"haiku", "sonnet"}, {"opus", "sonnet"}, {"haiku", "sonnet"}},
		},
		{
			name: "discussion_judge and code_judge move their own segments",
			cfg:  Config{Judge: "sonnet[medium]", DiscussionJudge: "opus", CodeJudge: "haiku"},
			want: [3]segmentModels{{"opus", "opus"}, {"sonnet", "sonnet"}, {"haiku", "haiku"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
			if err != nil {
				t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
			}

			settings, err := ResolveJudge(tt.cfg, reg)
			if err != nil {
				t.Fatalf("ResolveJudge(...) = _, %v; want nil error", err)
			}
			got := [3]BouncerModels{settings.Discussion, settings.Plan, settings.Code}
			for i, segment := range []string{"discussion", "plan", "code"} {
				if !strings.Contains(got[i].Seed.Model, tt.want[i].seed) {
					t.Errorf("ResolveJudge(...).%s seed model = %q; want a %s model", segment, got[i].Seed.Model, tt.want[i].seed)
				}
				if !strings.Contains(got[i].Judge.Model, tt.want[i].judge) {
					t.Errorf("ResolveJudge(...).%s judge model = %q; want a %s model", segment, got[i].Judge.Model, tt.want[i].judge)
				}
			}
		})
	}

	t.Run("effort of the judge key reaches the pair", func(t *testing.T) {
		t.Parallel()
		reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
		if err != nil {
			t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
		}
		settings, err := ResolveJudge(Config{Judge: "sonnet[medium]"}, reg)
		if err != nil {
			t.Fatalf("ResolveJudge(...) = _, %v; want nil error", err)
		}
		if settings.Plan.Judge.Effort != "medium" || settings.Plan.Seed.Effort != "medium" {
			t.Errorf("ResolveJudge(...).Plan = %+v; want effort medium on both passes", settings.Plan)
		}
	})

	t.Run("the judge and seed carry their aliases' engines, the seed falling back to the judge's", func(t *testing.T) {
		t.Parallel()
		reg := modelspec.Registry{
			"alpha": {Engine: "claude", Model: "alpha-model"},
			"beta":  {Engine: "other", Model: "beta-model"},
		}
		settings, err := ResolveJudge(Config{Judge: "alpha", PlanSeed: "beta"}, reg)
		if err != nil {
			t.Fatalf("ResolveJudge(...) = _, %v; want nil error", err)
		}
		if settings.Plan.Judge.Engine != "claude" || settings.Plan.Seed.Engine != "other" {
			t.Errorf("ResolveJudge(...).Plan = %+v; want judge engine claude and seed engine other", settings.Plan)
		}
		if settings.Discussion.Judge.Engine != "claude" || settings.Discussion.Seed.Engine != "claude" {
			t.Errorf("ResolveJudge(...).Discussion = %+v; want the seed fallen back to the judge, engine claude on both", settings.Discussion)
		}
	})
}

// TestResolveReviewAndJudge_MalformedSpec verifies an ungrammatical review, fix or judge model-spec is rejected.
// The rejection is an error naming its role, not a silent carry into a producer's spawn site.
// A bad later review entry or a bad fix entry also names its entry index and the way forward.
func TestResolveReviewAndJudge_MalformedSpec(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		role    string
		resolve func(Config, modelspec.Registry) error
		cfg     Config
		wantIn  []string
	}{
		{
			name: "reviewer_model",
			role: "reviewer_model",
			cfg:  Config{ReviewerModel: burlerengine.ModelSpecList{"opus[effort"}, ReviewTimeoutMin: 240},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveReview(cfg, reg)
				return err
			},
		},
		{
			name:   "later reviewer_model entry",
			role:   "reviewer_model",
			cfg:    Config{ReviewerModel: burlerengine.ModelSpecList{"sonnet[medium]", "opus[effort"}, FixerModel: burlerengine.ModelSpecList{"opus[medium]"}},
			wantIn: []string{"entry 2", "in loom.yaml", "a model-spec the registry defines"},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveReview(cfg, reg)
				return err
			},
		},
		{
			name:   "fixer_model entry",
			role:   "fixer_model",
			cfg:    Config{ReviewerModel: burlerengine.ModelSpecList{"sonnet[medium]"}, FixerModel: burlerengine.ModelSpecList{"opus[medium]", "opus[effort"}},
			wantIn: []string{"entry 2", "a model-spec the registry defines"},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveReview(cfg, reg)
				return err
			},
		},
		{
			name:   "code_fixer_model entry",
			role:   "code_fixer_model",
			cfg:    Config{ReviewerModel: burlerengine.ModelSpecList{"sonnet[medium]"}, FixerModel: burlerengine.ModelSpecList{"opus[medium]"}, CodeFixerModel: burlerengine.ModelSpecList{"ghost"}},
			wantIn: []string{"entry 1", "in loom.yaml"},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveReview(cfg, reg)
				return err
			},
		},
		{
			name: "judge",
			role: "judge",
			cfg:  Config{Judge: "sonnet[medium"},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveJudge(cfg, reg)
				return err
			},
		},
		{
			name: "seed",
			role: "seed",
			cfg:  Config{Judge: "sonnet[medium]", Seed: "haiku[medium"},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveJudge(cfg, reg)
				return err
			},
		},
		{
			name: "discussion_judge",
			role: "discussion_judge",
			cfg:  Config{Judge: "sonnet[medium]", DiscussionJudge: "opus[medium"},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveJudge(cfg, reg)
				return err
			},
		},
		{
			name: "plan_judge",
			role: "plan_judge",
			cfg:  Config{Judge: "sonnet[medium]", PlanJudge: "opus[medium"},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveJudge(cfg, reg)
				return err
			},
		},
		{
			name: "code_judge",
			role: "code_judge",
			cfg:  Config{Judge: "sonnet[medium]", CodeJudge: "opus[medium"},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveJudge(cfg, reg)
				return err
			},
		},
		{
			name: "discussion_seed",
			role: "discussion_seed",
			cfg:  Config{Judge: "sonnet[medium]", DiscussionSeed: "opus[medium"},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveJudge(cfg, reg)
				return err
			},
		},
		{
			name: "plan_seed",
			role: "plan_seed",
			cfg:  Config{Judge: "sonnet[medium]", PlanSeed: "opus[medium"},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveJudge(cfg, reg)
				return err
			},
		},
		{
			name: "code_seed",
			role: "code_seed",
			cfg:  Config{Judge: "sonnet[medium]", CodeSeed: "opus[medium"},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveJudge(cfg, reg)
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
			if err != nil {
				t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
			}

			err = tt.resolve(tt.cfg, reg)
			if err == nil {
				t.Fatalf("resolving the %s role = nil error; want non-nil error for a malformed spec", tt.role)
			}
			if !strings.Contains(err.Error(), tt.role) {
				t.Errorf("resolving the %s role error = %q; want it to name the role", tt.role, err.Error())
			}
			for _, want := range tt.wantIn {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("resolving the %s role error = %q; want it to contain %q", tt.role, err.Error(), want)
				}
			}
		})
	}
}

// TestLoomReviewsDir verifies that LoomReviewsDir's path is AnchorPath-anchored.
// It also verifies the path sits under the durable _lym tree rather than the ephemeral one,
// and that it equals the anchor joined with its Rel form.
// That Rel form's exact value is pinned too.
func TestLoomReviewsDir(t *testing.T) {
	tiergate.Require(t, 0)
	l := &lymcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-LYMHUB"),
		WorktreeName: "repo",
		// AnchorRel differs from "." so the accessor must follow the anchored subpath.
		// Returning the bare worktree root would fail the test.
		AnchorRel: filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), lymdirs.LymDirName, "reviews")
	if got := LoomReviewsDir(l); got != want {
		t.Errorf("LoomReviewsDir() = %q; want %q", got, want)
	}

	if got, rel := LoomReviewsDir(l), filepath.Join(l.AnchorPath(), LoomReviewsDirRel()); got != rel {
		t.Errorf("LoomReviewsDir() = %q; want it to equal %q", got, rel)
	}
	if got := LoomReviewsDirRel(); got != filepath.Join(lymdirs.LymDirName, "reviews") {
		t.Errorf("LoomReviewsDirRel() = %q; want %q", got, filepath.Join(lymdirs.LymDirName, "reviews"))
	}
}
