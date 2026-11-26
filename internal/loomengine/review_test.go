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
		Review: ReviewConfig{
			ReviewerModel: burlerengine.ModelSpecList{"opus[effort=high]", "sonnet[medium]"},
			FixerModel:    burlerengine.ModelSpecList{"opus[effort=low]"},
			TimeoutMin:    240,
		},
		// Discussion sets only its fixer list, Plan only its reviewer list, and Code leaves both unset.
		// Code's unset lists hold one empty entry, as the template loads.
		Discussion: DiscussionConfig{FixerModel: burlerengine.ModelSpecList{"sonnet[effort=high]"}},
		Plan:       PlanConfig{ReviewerModel: burlerengine.ModelSpecList{"sonnet[effort=low]", "opus[effort=high]"}},
		Code:       CodeConfig{ReviewerModel: burlerengine.ModelSpecList{""}, FixerModel: burlerengine.ModelSpecList{""}},
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
	// With no split lists and no raddle key, the split perch keeps the run-wide fixer.
	if !reflect.DeepEqual(settings.Split, settings.Models) {
		t.Errorf("ResolveReview(...).Split = %+v; want the run-wide %+v, since raddle is empty", settings.Split, settings.Models)
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
		{"Discussion", settings.DiscussionKeys, SegmentKeys{Review: loomKey("review.reviewer_model"), Fix: loomKey("discussion.fixer_model")}},
		{"Plan", settings.PlanKeys, SegmentKeys{Review: tapestryKey("plan.reviewer_model"), Fix: loomKey("review.fixer_model")}},
		{"Code", settings.CodeKeys, SegmentKeys{Review: loomKey("review.reviewer_model"), Fix: loomKey("review.fixer_model")}},
		{"Split", settings.SplitKeys, SegmentKeys{Review: loomKey("review.reviewer_model"), Fix: loomKey("review.fixer_model")}},
	} {
		if tt.got != tt.want {
			t.Errorf("ResolveReview(...).%sKeys = %+v; want %+v", tt.segment, tt.got, tt.want)
		}
	}

	// A set fan key changes no model:
	// a fanned segment's reviewer list is still its own review list, else the run-wide one.
	cfg.Discussion.ReviewerFan = lenses.FanList{"standard"}
	cfg.Plan.ReviewerFan = lenses.FanList{"", "full"}
	cfg.Discussion.FixerFan = lenses.FanList{"fix-discussion"}
	cfg.Code.ReviewerFan = lenses.FanList{"review-code"}
	cfg.Code.FixerFan = lenses.FanList{"fix-code"}
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
	engineSettings, err := ResolveReview(Config{Review: ReviewConfig{ReviewerModel: burlerengine.ModelSpecList{"alpha", "beta"}, FixerModel: burlerengine.ModelSpecList{"beta"}}}, engineReg)
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

	// An unset split fixer list takes the raddle key's model while that key is set.
	raddleSettings, err := ResolveReview(Config{Review: ReviewConfig{ReviewerModel: burlerengine.ModelSpecList{"alpha"}, FixerModel: burlerengine.ModelSpecList{"alpha"}}, Raddle: RaddleConfig{Model: "beta"}}, engineReg)
	if err != nil {
		t.Fatalf("ResolveReview(raddle set) = _, %v; want nil error", err)
	}
	if got := raddleSettings.Split.Fix; len(got) != 1 || got[0].Engine != "other" {
		t.Errorf("ResolveReview(raddle set).Split.Fix = %+v; want one entry on engine other, the raddle key's model", got)
	}
	if want := (SegmentKeys{Review: loomKey("review.reviewer_model"), Fix: loomKey("raddle.model")}); raddleSettings.SplitKeys != want {
		t.Errorf("ResolveReview(raddle set).SplitKeys = %+v; want %+v", raddleSettings.SplitKeys, want)
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
			Discussion: DiscussionConfig{AssistReviewerModel: burlerengine.ModelSpecList{""}},
			Plan:       PlanConfig{AssistReviewerModel: burlerengine.ModelSpecList{""}},
			Code:       CodeConfig{AssistReviewerModel: burlerengine.ModelSpecList{""}},
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
			Discussion: DiscussionConfig{AssistReviewerModel: burlerengine.ModelSpecList{"luna", ""}, AssistReviewerFan: lenses.FanList{"review-discussion", ""}},
			Plan:       PlanConfig{AssistReviewerModel: burlerengine.ModelSpecList{"", "luna"}},
			Code:       CodeConfig{AssistReviewerModel: burlerengine.ModelSpecList{"luna"}},
		}
		settings, err := ResolveReview(cfg, reg)
		if err != nil {
			t.Fatalf("ResolveReview(...) = _, %v; want nil error", err)
		}
		want := map[string]AssistModels{
			"Discussion": {Models: []shuttleengine.ModelChoice{luna, zero}, Fans: lenses.FanList{"review-discussion", ""}, ModelKey: loomKey("discussion.assist_reviewer_model"), FanKey: "loom.discussion.assist_reviewer_fan"},
			"Plan":       {Models: []shuttleengine.ModelChoice{zero, luna}, ModelKey: tapestryKey("plan.assist_reviewer_model"), FanKey: "tapestry.plan.assist_reviewer_fan"},
			"Code":       {Models: []shuttleengine.ModelChoice{luna}, ModelKey: loomKey("code.assist_reviewer_model"), FanKey: "loom.code.assist_reviewer_fan"},
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
			{"malformed second discussion entry", Config{Discussion: DiscussionConfig{AssistReviewerModel: burlerengine.ModelSpecList{"luna", "opus[effort"}}}, []string{"discussion.assist_reviewer_model entry 2", "in loom.yaml"}},
			{"unknown plan alias", Config{Plan: PlanConfig{AssistReviewerModel: burlerengine.ModelSpecList{"ghost"}}}, []string{"plan.assist_reviewer_model entry 1", "in tapestry.yaml"}},
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
// The chair is review.timeout_min, or the run timeout when that is 0.
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
		{name: "a bound that fits is kept", cfg: Config{Review: ReviewConfig{TimeoutMin: 240, AssistReviewerTimeoutMin: 60}}, runTimeout: 30 * time.Minute, startAllowance: 3 * time.Minute, awaitCap: 5 * time.Minute, want: 60 * time.Minute},
		{name: "a bound equal to the room is kept", cfg: Config{Review: ReviewConfig{TimeoutMin: 60, AssistReviewerTimeoutMin: 50}}, startAllowance: 5 * time.Minute, awaitCap: 5 * time.Minute, want: 50 * time.Minute},
		{name: "a bound past the room is capped to chair less start allowance less await cap", cfg: Config{Review: ReviewConfig{TimeoutMin: 60, AssistReviewerTimeoutMin: 60}}, startAllowance: 3 * time.Minute, awaitCap: 7 * time.Minute, want: 50 * time.Minute},
		{name: "a zero bound takes the whole room", cfg: Config{Review: ReviewConfig{TimeoutMin: 60, AssistReviewerTimeoutMin: 0}}, startAllowance: 3 * time.Minute, awaitCap: 7 * time.Minute, want: 50 * time.Minute},
		{name: "a zero review timeout takes the run timeout as the chair", cfg: Config{Review: ReviewConfig{TimeoutMin: 0, AssistReviewerTimeoutMin: 120}}, runTimeout: 100 * time.Minute, startAllowance: 10 * time.Minute, awaitCap: 5 * time.Minute, want: 85 * time.Minute},
		{name: "an allowance and await cap that spend the chair leave no time", cfg: Config{Review: ReviewConfig{TimeoutMin: 10, AssistReviewerTimeoutMin: 60}}, startAllowance: 6 * time.Minute, awaitCap: 4 * time.Minute, want: 0, wantSkip: true},
		{name: "an allowance past the chair leaves no time", cfg: Config{Review: ReviewConfig{TimeoutMin: 5, AssistReviewerTimeoutMin: 60}}, startAllowance: 6 * time.Minute, awaitCap: 4 * time.Minute, want: 0, wantSkip: true},
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
			for _, key := range []string{"review.assist_reviewer_timeout_min", "review.timeout_min"} {
				if !strings.Contains(reason, key) {
					t.Errorf("CapAssistTimeout(...) reason = %q; want it to name %q", reason, key)
				}
			}
		})
	}
}

// TestCapParentReviewWait verifies the parent's wait is kept when it fits, and capped when it does not.
// The room is the chair's timeout less the start allowance, one await cap and the consolidation margin;
// with no room left the wait is zero, and the reason names both keys and says to raise review.timeout_min.
// Each perch takes its own timeout key; a row with no perch is the discussion perch's.
func TestCapParentReviewWait(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	waits := func(review, discussion, code int) Config {
		return Config{
			Review:     ReviewConfig{TimeoutMin: review},
			Discussion: DiscussionConfig{ReviewParentTimeoutMin: discussion},
			Code:       CodeConfig{ReviewParentTimeoutMin: code},
		}
	}
	tests := []struct {
		name           string
		perch          Perch
		cfg            Config
		runTimeout     time.Duration
		startAllowance time.Duration
		awaitCap       time.Duration
		want           time.Duration
		wantSkip       bool
	}{
		{name: "the code perch takes code.review_parent_timeout_min", perch: PerchCode, cfg: waits(240, 30, 45), startAllowance: 3 * time.Minute, awaitCap: 5 * time.Minute, want: 45 * time.Minute},
		{name: "the code perch's no-room reason names its own key", perch: PerchCode, cfg: waits(13, 0, 30), startAllowance: 3 * time.Minute, awaitCap: 5 * time.Minute, wantSkip: true},
		{name: "a wait that fits is kept", cfg: waits(240, 30, 0), startAllowance: 3 * time.Minute, awaitCap: 5 * time.Minute, want: 30 * time.Minute},
		{name: "a wait past the room is capped to chair less start allowance, await cap and margin", cfg: waits(20, 30, 0), startAllowance: 3 * time.Minute, awaitCap: 5 * time.Minute, want: 7 * time.Minute},
		{name: "a zero review timeout takes the run timeout as the chair", cfg: waits(0, 60, 0), runTimeout: 40 * time.Minute, startAllowance: 5 * time.Minute, awaitCap: 5 * time.Minute, want: 25 * time.Minute},
		{name: "a room spent to exactly zero sends no ask", cfg: waits(13, 30, 0), startAllowance: 3 * time.Minute, awaitCap: 5 * time.Minute, wantSkip: true},
		{name: "an allowance past the chair sends no ask", cfg: waits(5, 30, 0), startAllowance: 6 * time.Minute, awaitCap: 4 * time.Minute, wantSkip: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			perch, key := PerchDiscussion, "loom.discussion.review_parent_timeout_min"
			if tt.perch == PerchCode {
				perch, key = PerchCode, "loom.code.review_parent_timeout_min"
			}
			got, reason := CapParentReviewWait(tt.cfg, perch, tt.runTimeout, tt.startAllowance, tt.awaitCap)
			if got != tt.want {
				t.Errorf("CapParentReviewWait(...) wait = %s; want %s", got, tt.want)
			}
			if !tt.wantSkip {
				if reason != "" {
					t.Errorf("CapParentReviewWait(...) reason = %q; want none", reason)
				}
				chair := chairTimeout(tt.cfg, tt.runTimeout)
				if slack := chair - tt.startAllowance - tt.awaitCap - got; slack < ParentConsolidationMargin {
					t.Errorf("CapParentReviewWait(...) leaves the chair %s after its wait and start; want at least %s", slack, ParentConsolidationMargin)
				}
				return
			}
			for _, want := range []string{key, "loom.review.timeout_min", "raise loom.review.timeout_min"} {
				if !strings.Contains(reason, want) {
					t.Errorf("CapParentReviewWait(...) reason = %q; want it to name %q", reason, want)
				}
			}
		})
	}
}

// TestCapParentJudgeWait verifies the judge's parent wait is capped by the run timeout its session runs under.
// It ignores loom.review.timeout_min.
// A room spent to zero sends no ask and gives a reason that says to raise shuttle.run_timeout_min.
func TestCapParentJudgeWait(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	cfg := Config{Review: ReviewConfig{TimeoutMin: 240}, Code: CodeConfig{ReviewParentTimeoutMin: 30}}
	tests := []struct {
		name       string
		runTimeout time.Duration
		want       time.Duration
		wantSkip   bool
	}{
		{name: "a wait that fits is kept", runTimeout: 120 * time.Minute, want: 30 * time.Minute},
		{name: "a wait past the run timeout's room is capped to it", runTimeout: 30 * time.Minute, want: 23 * time.Minute},
		{name: "a room spent to zero sends no ask", runTimeout: 7 * time.Minute, wantSkip: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, reason := CapParentJudgeWait(cfg, PerchCode, tt.runTimeout, time.Minute, time.Minute)
			if got != tt.want {
				t.Errorf("CapParentJudgeWait(...) wait = %s; want %s", got, tt.want)
			}
			if !tt.wantSkip {
				if reason != "" {
					t.Errorf("CapParentJudgeWait(...) reason = %q; want none", reason)
				}
				return
			}
			for _, want := range []string{"loom.code.review_parent_timeout_min", "shuttle.run_timeout_min", "raise shuttle.run_timeout_min"} {
				if !strings.Contains(reason, want) {
					t.Errorf("CapParentJudgeWait(...) reason = %q; want it to name %q", reason, want)
				}
			}
		})
	}
}

// TestResolveJudge verifies ResolveJudge resolves each segment's judge and seed through the fallback order:
// a segment's judge key, else judge; a segment's seed key, else seed, else the segment's resolved judge.
// The split perch has no seed key, so its seed is seed, else its resolved judge.
func TestResolveJudge(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	// segmentModels is the seed and judge model of one segment, in discussion, plan, code order.
	type segmentModels struct{ seed, judge string }
	tests := []struct {
		name  string
		cfg   Config
		want  [3]segmentModels
		split segmentModels
	}{
		{
			name:  "all keys empty resolves every pass to judge",
			cfg:   Config{Review: ReviewConfig{Judge: "sonnet[medium]"}},
			want:  [3]segmentModels{{"sonnet", "sonnet"}, {"sonnet", "sonnet"}, {"sonnet", "sonnet"}},
			split: segmentModels{"sonnet", "sonnet"},
		},
		{
			name:  "seed moves every seed",
			cfg:   Config{Review: ReviewConfig{Judge: "sonnet[medium]", Seed: "haiku"}},
			want:  [3]segmentModels{{"haiku", "sonnet"}, {"haiku", "sonnet"}, {"haiku", "sonnet"}},
			split: segmentModels{"haiku", "sonnet"},
		},
		{
			name:  "plan.judge alone moves plan's judge and seed together",
			cfg:   Config{Review: ReviewConfig{Judge: "sonnet[medium]"}, Plan: PlanConfig{Judge: "opus"}},
			want:  [3]segmentModels{{"sonnet", "sonnet"}, {"opus", "opus"}, {"sonnet", "sonnet"}},
			split: segmentModels{"sonnet", "sonnet"},
		},
		{
			name:  "plan.judge with seed keeps plan's seed off the judge",
			cfg:   Config{Review: ReviewConfig{Judge: "sonnet[medium]", Seed: "haiku"}, Plan: PlanConfig{Judge: "opus"}},
			want:  [3]segmentModels{{"haiku", "sonnet"}, {"haiku", "opus"}, {"haiku", "sonnet"}},
			split: segmentModels{"haiku", "sonnet"},
		},
		{
			name:  "plan.judge with plan.seed keeps plan's seed off the judge",
			cfg:   Config{Review: ReviewConfig{Judge: "sonnet[medium]"}, Plan: PlanConfig{Judge: "opus", Seed: "haiku"}},
			want:  [3]segmentModels{{"sonnet", "sonnet"}, {"haiku", "opus"}, {"sonnet", "sonnet"}},
			split: segmentModels{"sonnet", "sonnet"},
		},
		{
			name:  "code.seed alone moves only code's seed",
			cfg:   Config{Review: ReviewConfig{Judge: "sonnet[medium]"}, Code: CodeConfig{Seed: "haiku"}},
			want:  [3]segmentModels{{"sonnet", "sonnet"}, {"sonnet", "sonnet"}, {"haiku", "sonnet"}},
			split: segmentModels{"sonnet", "sonnet"},
		},
		{
			name:  "plan.seed beats seed for plan alone",
			cfg:   Config{Review: ReviewConfig{Judge: "sonnet[medium]", Seed: "haiku"}, Plan: PlanConfig{Seed: "opus"}},
			want:  [3]segmentModels{{"haiku", "sonnet"}, {"opus", "sonnet"}, {"haiku", "sonnet"}},
			split: segmentModels{"haiku", "sonnet"},
		},
		{
			name:  "discussion.judge and code.judge move their own segments",
			cfg:   Config{Review: ReviewConfig{Judge: "sonnet[medium]"}, Discussion: DiscussionConfig{Judge: "opus"}, Code: CodeConfig{Judge: "haiku"}},
			want:  [3]segmentModels{{"opus", "opus"}, {"sonnet", "sonnet"}, {"haiku", "haiku"}},
			split: segmentModels{"sonnet", "sonnet"},
		},
		{
			name:  "split.judge alone moves split's judge and seed together",
			cfg:   Config{Review: ReviewConfig{Judge: "sonnet[medium]"}, Split: SplitConfig{Judge: "opus"}},
			want:  [3]segmentModels{{"sonnet", "sonnet"}, {"sonnet", "sonnet"}, {"sonnet", "sonnet"}},
			split: segmentModels{"opus", "opus"},
		},
		{
			name:  "split.judge with seed keeps split's seed off the judge",
			cfg:   Config{Review: ReviewConfig{Judge: "sonnet[medium]", Seed: "haiku"}, Split: SplitConfig{Judge: "opus"}},
			want:  [3]segmentModels{{"haiku", "sonnet"}, {"haiku", "sonnet"}, {"haiku", "sonnet"}},
			split: segmentModels{"haiku", "opus"},
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
			if !strings.Contains(settings.Split.Seed.Model, tt.split.seed) {
				t.Errorf("ResolveJudge(...).Split seed model = %q; want a %s model", settings.Split.Seed.Model, tt.split.seed)
			}
			if !strings.Contains(settings.Split.Judge.Model, tt.split.judge) {
				t.Errorf("ResolveJudge(...).Split judge model = %q; want a %s model", settings.Split.Judge.Model, tt.split.judge)
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
		settings, err := ResolveJudge(Config{Review: ReviewConfig{Judge: "sonnet[medium]"}}, reg)
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
		settings, err := ResolveJudge(Config{Review: ReviewConfig{Judge: "alpha"}, Plan: PlanConfig{Seed: "beta"}}, reg)
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
			role: "review.reviewer_model",
			cfg:  Config{Review: ReviewConfig{ReviewerModel: burlerengine.ModelSpecList{"opus[effort"}, TimeoutMin: 240}},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveReview(cfg, reg)
				return err
			},
		},
		{
			name:   "later reviewer_model entry",
			role:   "review.reviewer_model",
			cfg:    Config{Review: ReviewConfig{ReviewerModel: burlerengine.ModelSpecList{"sonnet[medium]", "opus[effort"}, FixerModel: burlerengine.ModelSpecList{"opus[medium]"}}},
			wantIn: []string{"entry 2", "in loom.yaml", "a model-spec the registry defines"},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveReview(cfg, reg)
				return err
			},
		},
		{
			name:   "fixer_model entry",
			role:   "review.fixer_model",
			cfg:    Config{Review: ReviewConfig{ReviewerModel: burlerengine.ModelSpecList{"sonnet[medium]"}, FixerModel: burlerengine.ModelSpecList{"opus[medium]", "opus[effort"}}},
			wantIn: []string{"entry 2", "a model-spec the registry defines"},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveReview(cfg, reg)
				return err
			},
		},
		{
			name:   "code.fixer_model entry",
			role:   "code.fixer_model",
			cfg:    Config{Review: ReviewConfig{ReviewerModel: burlerengine.ModelSpecList{"sonnet[medium]"}, FixerModel: burlerengine.ModelSpecList{"opus[medium]"}}, Code: CodeConfig{FixerModel: burlerengine.ModelSpecList{"ghost"}}},
			wantIn: []string{"entry 1", "in loom.yaml"},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveReview(cfg, reg)
				return err
			},
		},
		{
			name: "judge",
			role: "review.judge",
			cfg:  Config{Review: ReviewConfig{Judge: "sonnet[medium"}},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveJudge(cfg, reg)
				return err
			},
		},
		{
			name: "seed",
			role: "review.seed",
			cfg:  Config{Review: ReviewConfig{Judge: "sonnet[medium]", Seed: "haiku[medium"}},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveJudge(cfg, reg)
				return err
			},
		},
		{
			name: "discussion.judge",
			role: "discussion.judge",
			cfg:  Config{Review: ReviewConfig{Judge: "sonnet[medium]"}, Discussion: DiscussionConfig{Judge: "opus[medium"}},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveJudge(cfg, reg)
				return err
			},
		},
		{
			name: "plan.judge",
			role: "plan.judge",
			cfg:  Config{Review: ReviewConfig{Judge: "sonnet[medium]"}, Plan: PlanConfig{Judge: "opus[medium"}},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveJudge(cfg, reg)
				return err
			},
		},
		{
			name: "code.judge",
			role: "code.judge",
			cfg:  Config{Review: ReviewConfig{Judge: "sonnet[medium]"}, Code: CodeConfig{Judge: "opus[medium"}},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveJudge(cfg, reg)
				return err
			},
		},
		{
			name: "discussion.seed",
			role: "discussion.seed",
			cfg:  Config{Review: ReviewConfig{Judge: "sonnet[medium]"}, Discussion: DiscussionConfig{Seed: "opus[medium"}},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveJudge(cfg, reg)
				return err
			},
		},
		{
			name: "plan.seed",
			role: "plan.seed",
			cfg:  Config{Review: ReviewConfig{Judge: "sonnet[medium]"}, Plan: PlanConfig{Seed: "opus[medium"}},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveJudge(cfg, reg)
				return err
			},
		},
		{
			name: "code.seed",
			role: "code.seed",
			cfg:  Config{Review: ReviewConfig{Judge: "sonnet[medium]"}, Code: CodeConfig{Seed: "opus[medium"}},
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
