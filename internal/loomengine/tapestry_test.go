// tapestry_test.go holds the tier0 tests for the tapestry config.
// They cover LoadTapestryConfig over a worktree-only layer stack, and the plan values Config.WithTapestry applies.

package loomengine

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// seedTapestryConfig writes <baseDir>/_lym/config/tapestry.yaml with the given contents.
func seedTapestryConfig(t *testing.T, baseDir, contents string) {
	t.Helper()
	configDir := filepath.Join(baseDir, "_lym", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	path := filepath.Join(configDir, "tapestry.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// seedLoomAndTapestry seeds baseDir's worktree layer with a loom.yaml and a tapestry.yaml.
// The loom.yaml is built from the shipped template with loomValues applied,
// and the tapestry.yaml is built from its template with tapestryValues applied.
func seedLoomAndTapestry(t *testing.T, baseDir string, loomValues, tapestryValues map[string]string) {
	t.Helper()
	seedLoomConfig(t, baseDir, loomTemplateWith(t, loomValues))
	seedTapestryConfig(t, baseDir, templateWith(t, TapestryConfigTemplate(), tapestryValues))
}

// TestLoadTapestryConfig verifies that the plan perch's values load from tapestry.yaml.
// They resolve against loom.yaml's run-wide keys, and Config.WithTapestry applies them.
// A null tapestry.plan.max_bounces and a zero tapestry.plan.hard_max_bounces take loom.yaml's budget and hard cap.
// An empty model list, judge and seed take the shared ones.
// Every malformed or out-of-bound value is refused, and the error names the tapestry key.
func TestLoadTapestryConfig(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("every moved key's new path is in the template and its old name is gone", func(t *testing.T) {
		t.Parallel()
		requireMovesIntoTemplate(t, tapestryMovedKeys(), TapestryConfigTemplate())
	})

	t.Run("the move table holds every renamed key", func(t *testing.T) {
		t.Parallel()
		want := map[string]string{
			"plan":                       "plan.model",
			"plan_timeout_min":           "plan.timeout_min",
			"plan_lenses":                "plan.lenses",
			"plan_gate_fixer":            "plan.gate_fixer",
			"plan_split":                 "plan.split",
			"plan_reviewer_model":        "plan.reviewer_model",
			"plan_fixer_model":           "plan.fixer_model",
			"plan_reviewer_fan":          "plan.reviewer_fan",
			"plan_fixer_fan":             "plan.fixer_fan",
			"plan_assist_reviewer_model": "plan.assist_reviewer_model",
			"plan_assist_reviewer_fan":   "plan.assist_reviewer_fan",
			"plan_judge":                 "plan.judge",
			"plan_seed":                  "plan.seed",
			"plan_max_bounces":           "plan.max_bounces",
			"plan_hard_max_bounces":      "plan.hard_max_bounces",
			"plan_auto_accept":           "plan.auto_accept",
			"parts":                      "parts.model",
			"parts_depth":                "parts.depth",
			"parts_max":                  "parts.max",
			"parts_chair":                "parts.chair",
			"parts_chair_timeout_min":    "parts.chair_timeout_min",
			"part_writer":                "parts.writer",
		}
		if got := tapestryMovedKeys(); !maps.Equal(got, want) {
			t.Errorf("tapestryMovedKeys() = %v; want %v", got, want)
		}
	})

	t.Run("an old key name in a local file is refused naming its new path", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		seedLoomAndTapestry(t, dir, nil, nil)
		path := configengine.LocalFile(dir, "tapestry")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte("part_writer: opus\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
		}

		_, err := LoadTapestryConfig(configengine.Dirs{Anchor: dir}, "tapestry")
		if err == nil || !strings.Contains(err.Error(), "tapestry.parts.writer") {
			t.Errorf("LoadTapestryConfig() error = %v; want a refusal naming tapestry.parts.writer", err)
		}
	})

	t.Run("the template loads and WithTapestry applies every plan value", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		seedLoomAndTapestry(t, dir, nil, map[string]string{
			"plan.model": "opus[effort=high]", "plan.timeout_min": "7", "plan.lenses": `""`,
			"plan.reviewer_model": "sonnet[low]", "plan.fixer_model": "sonnet[high]",
			"plan.reviewer_fan": "full", "plan.fixer_fan": "fix-plan",
			"plan.judge": "opus[low]", "plan.seed": "haiku[low]",
			"plan.max_bounces": "2", "plan.hard_max_bounces": "3", "plan.auto_accept": "true",
		})
		dirs := configengine.Dirs{Anchor: dir}

		loaded, err := LoadTapestryConfig(dirs, "tapestry")
		if err != nil {
			t.Fatalf("LoadTapestryConfig() = _, %v; want nil error", err)
		}
		want := TapestryConfig{
			Plan: PlanConfig{
				Model: "opus[effort=high]", TimeoutMin: 7, Lenses: "", GateFixer: "sonnet[medium]",
				ReviewerModel: burlerengine.ModelSpecList{"sonnet[low]"}, FixerModel: burlerengine.ModelSpecList{"sonnet[high]"},
				ReviewerFan: lenses.FanList{"full"}, FixerFan: lenses.FanList{"fix-plan"},
				AssistReviewerModel: burlerengine.ModelSpecList{""}, AssistReviewerFan: lenses.FanList{""},
				Judge: "opus[low]", Seed: "haiku[low]",
				MaxBounces: intPtr(2), HardMaxBounces: 3, AutoAccept: true,
				Split: false,
			},
			Parts: PartsConfig{Depth: "light", Max: 6, Model: "opus[medium]", Writer: "opus[medium]", Chair: "opus[medium]"},
		}
		if !reflect.DeepEqual(loaded, want) {
			t.Errorf("LoadTapestryConfig() = %+v; want %+v", loaded, want)
		}

		loom, err := LoadConfig(dirs, "loom")
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		if !reflect.DeepEqual(loom.Plan, PlanConfig{}) || !reflect.DeepEqual(loom.Parts, PartsConfig{}) {
			t.Errorf("LoadConfig() plan and parts areas = %+v/%+v; want the zero values, since loom.yaml never reads tapestry.yaml", loom.Plan, loom.Parts)
		}
		applied := loom.WithTapestry(loaded)
		if !reflect.DeepEqual(applied.Plan, want.Plan) || !reflect.DeepEqual(applied.Parts, want.Parts) {
			t.Errorf("WithTapestry() = %+v; want the plan and parts areas copied from %+v", applied, want)
		}
		if caps := applied.Caps(PerchPlan); caps.Soft != 2 || caps.SoftKey != "plan.max_bounces" || caps.Hard != 3 || caps.HardKey != "plan.hard_max_bounces" {
			t.Errorf("WithTapestry().Caps(PerchPlan) = %+v; want the plan perch's own budget and hard cap", caps)
		}
	})

	t.Run("the plan perch's own budget wins over loom.yaml's run-wide one", func(t *testing.T) {
		t.Parallel()
		for _, tt := range []struct {
			tapestry string
			want     int
		}{
			{"plan: {max_bounces: 2}\n", 2},
			{"plan: {}\n", 4},
		} {
			board, anchor := t.TempDir(), t.TempDir()
			seedLoomConfig(t, board, "review: {max_bounces: 4}\n")
			seedTapestryConfig(t, anchor, tt.tapestry)
			dirs := configengine.Dirs{Anchor: anchor, Board: board}
			seedTapestryConfig(t, board, "")

			tapestry, err := LoadTapestryConfig(dirs, "tapestry")
			if err != nil {
				t.Fatalf("LoadTapestryConfig(%q) = _, %v; want nil error", tt.tapestry, err)
			}
			loom, err := LoadConfig(dirs, "loom")
			if err != nil {
				t.Fatalf("LoadConfig() = _, %v; want nil error", err)
			}
			if got := loom.WithTapestry(tapestry).Caps(PerchPlan).Soft; got != tt.want {
				t.Errorf("Caps(PerchPlan).Soft with %q = %d; want %d", tt.tapestry, got, tt.want)
			}
		}
	})

	t.Run("an empty plan.gate_fixer loads the fixer off", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		seedLoomAndTapestry(t, dir, nil, map[string]string{"plan.gate_fixer": `""`})

		loaded, err := LoadTapestryConfig(configengine.Dirs{Anchor: dir}, "tapestry")
		if err != nil {
			t.Fatalf("LoadTapestryConfig() = _, %v; want nil error", err)
		}
		if loaded.Plan.GateFixer != "" {
			t.Errorf("LoadTapestryConfig().Plan.GateFixer = %q; want empty, the fixer off", loaded.Plan.GateFixer)
		}
	})

	t.Run("the plan assistant lists load per round and WithTapestry copies them", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		seedLoomAndTapestry(t, dir, nil, map[string]string{
			"plan.assist_reviewer_model": `["luna[low]", ""]`, "plan.assist_reviewer_fan": `["full", ""]`,
		})
		dirs := configengine.Dirs{Anchor: dir}

		loaded, err := LoadTapestryConfig(dirs, "tapestry")
		if err != nil {
			t.Fatalf("LoadTapestryConfig() = _, %v; want nil error", err)
		}
		wantModels, wantFans := burlerengine.ModelSpecList{"luna[low]", ""}, lenses.FanList{"full", ""}
		if !reflect.DeepEqual(loaded.Plan.AssistReviewerModel, wantModels) || !reflect.DeepEqual(loaded.Plan.AssistReviewerFan, wantFans) {
			t.Errorf("LoadTapestryConfig() assist lists = %v, %v; want %v, %v", loaded.Plan.AssistReviewerModel, loaded.Plan.AssistReviewerFan, wantModels, wantFans)
		}
		loom, err := LoadConfig(dirs, "loom")
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		applied := loom.WithTapestry(loaded)
		if !reflect.DeepEqual(applied.Plan.AssistReviewerModel, wantModels) || !reflect.DeepEqual(applied.Plan.AssistReviewerFan, wantFans) {
			t.Errorf("WithTapestry() assist lists = %v, %v; want %v, %v", applied.Plan.AssistReviewerModel, applied.Plan.AssistReviewerFan, wantModels, wantFans)
		}
	})

	t.Run("the default assistant keys give every round no assistant model", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		seedLoomAndTapestry(t, dir, nil, nil)
		dirs := configengine.Dirs{Anchor: dir}

		tapestry, err := LoadTapestryConfig(dirs, "tapestry")
		if err != nil {
			t.Fatalf("LoadTapestryConfig() = _, %v; want nil error", err)
		}
		loom, err := LoadConfig(dirs, "loom")
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
		if err != nil {
			t.Fatalf("modelspec.LoadRegistry() = _, %v; want nil error", err)
		}
		review, err := ResolveReview(loom.WithTapestry(tapestry), reg)
		if err != nil {
			t.Fatalf("ResolveReview() = _, %v; want nil error", err)
		}
		for name, got := range map[string]AssistModels{"Discussion": review.DiscussionAssist, "Plan": review.PlanAssist, "Code": review.CodeAssist} {
			for round, choice := range got.Models {
				if choice != (shuttleengine.ModelChoice{}) {
					t.Errorf("ResolveReview().%sAssist.Models[%d] = %+v; want the zero choice", name, round, choice)
				}
			}
		}
	})

	t.Run("a null budget, a zero hard cap and empty lists, judge and seed take loom.yaml's", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		seedLoomAndTapestry(t, dir,
			map[string]string{"review.max_bounces": "5", "review.hard_max_bounces": "6"},
			map[string]string{"plan.reviewer_model": `""`})
		dirs := configengine.Dirs{Anchor: dir}

		tapestry, err := LoadTapestryConfig(dirs, "tapestry")
		if err != nil {
			t.Fatalf("LoadTapestryConfig() = _, %v; want nil error", err)
		}
		loom, err := LoadConfig(dirs, "loom")
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		cfg := loom.WithTapestry(tapestry)
		if caps := cfg.Caps(PerchPlan); caps.Soft != 5 || caps.SoftKey != "review.max_bounces" || caps.Hard != 6 || caps.HardKey != "review.hard_max_bounces" {
			t.Errorf("Caps(PerchPlan) = %+v; want review.max_bounces 5 and review.hard_max_bounces 6", caps)
		}

		reg, err := modelspec.LoadRegistry(configengine.Dirs{Anchor: t.TempDir()})
		if err != nil {
			t.Fatalf("modelspec.LoadRegistry() = _, %v; want nil error", err)
		}
		review, err := ResolveReview(cfg, reg)
		if err != nil {
			t.Fatalf("ResolveReview() = _, %v; want nil error", err)
		}
		if review.PlanKeys != sharedSegmentKeys {
			t.Errorf("ResolveReview().PlanKeys = %+v; want the shared keys %+v", review.PlanKeys, sharedSegmentKeys)
		}
		if !reflect.DeepEqual(review.Plan, review.Models) {
			t.Errorf("ResolveReview().Plan = %+v; want the shared lists %+v", review.Plan, review.Models)
		}

		judge, err := ResolveJudge(cfg, reg)
		if err != nil {
			t.Fatalf("ResolveJudge() = _, %v; want nil error", err)
		}
		sharedJudge, err := resolveKeyedModelChoice("review.judge", cfg.Review.Judge, reg)
		if err != nil {
			t.Fatalf("resolveKeyedModelChoice() = _, %v; want nil error", err)
		}
		if judge.Plan.Judge != sharedJudge || judge.Plan.Seed != sharedJudge {
			t.Errorf("ResolveJudge().Plan = %+v; want loom.yaml's judge %+v for both passes", judge.Plan, sharedJudge)
		}
		if got := review.PlanKeys.Review; got != (shuttleengine.ModelKey{Module: "loom", Key: "review.reviewer_model"}) {
			t.Errorf("ResolveReview().PlanKeys.Review = %+v; want loom's shared review.reviewer_model", got)
		}
	})

	refusals := []struct {
		name   string
		loom   map[string]string
		key    string
		value  string
		wantIn []string
	}{
		{name: "malformed plan spec", key: "plan.model", value: `"opus[effort"`},
		{name: "malformed plan.reviewer_model spec", key: "plan.reviewer_model", value: `"opus[effort"`, wantIn: []string{"entry 1", "a non-empty list of model-specs"}},
		{name: "malformed plan.fixer_model spec", key: "plan.fixer_model", value: `"opus[effort"`, wantIn: []string{"entry 1", "a non-empty list of model-specs"}},
		{name: "malformed plan.assist_reviewer_model spec", key: "plan.assist_reviewer_model", value: `"opus[effort"`, wantIn: []string{"entry 1", "no assistant that round"}},
		{name: "malformed later plan.assist_reviewer_model entry", key: "plan.assist_reviewer_model", value: `["luna[low]", "opus[effort"]`, wantIn: []string{"entry 2", "no assistant that round"}},
		{name: "unknown plan.assist_reviewer_fan", key: "plan.assist_reviewer_fan", value: "nosuchfan", wantIn: []string{`"nosuchfan"`, "known fans: ", "entry 1"}},
		{name: "unknown plan.reviewer_fan", key: "plan.reviewer_fan", value: "nosuchfan", wantIn: []string{`"nosuchfan"`, "known fans: ", "full", "entry 1", `set the entry to ""`}},
		{name: "empty plan.reviewer_fan list", key: "plan.reviewer_fan", value: "[]", wantIn: []string{"the list is empty", `write ""`}},
		{name: "empty plan.fixer_fan list", key: "plan.fixer_fan", value: "[]", wantIn: []string{"the list is empty", `write ""`}},
		{name: "mapping inside a plan.reviewer_fan list", key: "plan.reviewer_fan", value: "[full, {model: opus}]", wantIn: []string{"line ", "neither a fan name nor a list of fan names", "a list of fan names"}},
		{name: "malformed plan.gate_fixer spec", key: "plan.gate_fixer", value: `"opus[effort"`, wantIn: []string{"lym config tapestry"}},
		{name: "malformed plan.judge spec", key: "plan.judge", value: `"opus[effort"`},
		{name: "malformed plan.seed spec", key: "plan.seed", value: `"opus[effort"`},
		{name: "negative plan.max_bounces", key: "plan.max_bounces", value: "-1", wantIn: []string{"must not be negative", "or to null to take loom.review.max_bounces"}},
		{name: "negative plan.hard_max_bounces", key: "plan.hard_max_bounces", value: "-1", wantIn: []string{"set it to 0 to take loom.review.hard_max_bounces", "lym config tapestry --set plan.hard_max_bounces=<n>", `lym loom resume`}},
		{name: "negative plan.timeout_min", key: "plan.timeout_min", value: "-1", wantIn: []string{"must not be negative"}},
		{name: "malformed parts spec", key: "parts.model", value: `"opus[effort"`},
		{name: "malformed parts.writer spec", key: "parts.writer", value: `"opus[effort"`},
		{name: "malformed parts.chair spec", key: "parts.chair", value: `"opus[effort"`},
		{name: "unknown parts.depth", key: "parts.depth", value: "deep", wantIn: []string{"light", "medium"}},
		{name: "zero parts.max", key: "parts.max", value: "0", wantIn: []string{"set it to 1 or more"}},
		{name: "parts.chair_timeout_min not above plan.timeout_min", key: "parts.chair_timeout_min", value: "120", wantIn: []string{"120", "not above tapestry.plan.timeout_min", "set it above that key", "null"}},
		{
			name: "plan.hard_max_bounces below the budget inherited from loom.yaml",
			loom: map[string]string{"review.max_bounces": "5", "review.hard_max_bounces": "6"},
			key:  "plan.hard_max_bounces", value: "2",
			wantIn: []string{"resolved hard cap 2", `"loom.review.max_bounces"'s resolved soft cap 5`, "lym config tapestry --set plan.hard_max_bounces=<n>", "lym config loom --set review.max_bounces=<n>"},
		},
	}
	for _, tt := range refusals {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			seedLoomAndTapestry(t, dir, tt.loom, map[string]string{tt.key: tt.value})

			_, err := LoadTapestryConfig(configengine.Dirs{Anchor: dir}, "tapestry")
			if err == nil {
				t.Fatalf("LoadTapestryConfig() error = nil; want a refusal for %s: %s", tt.key, tt.value)
			}
			for _, want := range append([]string{"tapestry", tt.key}, tt.wantIn...) {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("LoadTapestryConfig() error = %q; want it to contain %q", err.Error(), want)
				}
			}
		})
	}

	t.Run("a set parts.chair_timeout_min loads with a zero plan.timeout_min and above a set one", func(t *testing.T) {
		t.Parallel()
		for _, values := range []map[string]string{
			{"plan.timeout_min": "0", "parts.chair_timeout_min": "30"},
			{"plan.timeout_min": "60", "parts.chair_timeout_min": "61"},
		} {
			dir := t.TempDir()
			seedLoomAndTapestry(t, dir, nil, values)
			loaded, err := LoadTapestryConfig(configengine.Dirs{Anchor: dir}, "tapestry")
			if err != nil {
				t.Fatalf("LoadTapestryConfig(%v) = _, %v; want nil error", values, err)
			}
			if loaded.Parts.ChairTimeoutMin == nil {
				t.Errorf("LoadTapestryConfig(%v).Parts.ChairTimeoutMin = nil; want the set value", values)
			}
		}
	})

	t.Run("the timeout accessors resolve against the effective plan timeout", func(t *testing.T) {
		t.Parallel()
		const runTimeout = 90 * time.Minute
		tests := []struct {
			name         string
			planMin      int
			chairMin     *int
			wantPlan     time.Duration
			wantChair    time.Duration
			wantChairErr bool
		}{
			{name: "a set plan timeout and an unset chair", planMin: 60, wantPlan: 60 * time.Minute, wantChair: 120 * time.Minute},
			{name: "a zero plan timeout defers to the run timeout", planMin: 0, wantPlan: runTimeout, wantChair: 2 * runTimeout},
			{name: "a set chair above the effective plan timeout", planMin: 60, chairMin: intPtr(200), wantPlan: 60 * time.Minute, wantChair: 200 * time.Minute},
			{name: "a set chair not above the run timeout under a zero plan timeout", planMin: 0, chairMin: intPtr(90), wantPlan: runTimeout, wantChairErr: true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				cfg := Config{Plan: PlanConfig{TimeoutMin: tt.planMin}, Parts: PartsConfig{ChairTimeoutMin: tt.chairMin}}
				if got := cfg.PlanTimeout(runTimeout); got != tt.wantPlan {
					t.Errorf("PlanTimeout(%s) = %s; want %s", runTimeout, got, tt.wantPlan)
				}
				got, err := cfg.PartsChairTimeout(runTimeout)
				if tt.wantChairErr {
					if err == nil || !strings.Contains(err.Error(), "parts.chair_timeout_min") {
						t.Errorf("PartsChairTimeout(%s) = %s, %v; want an error naming parts.chair_timeout_min", runTimeout, got, err)
					}
					return
				}
				if err != nil || got != tt.wantChair {
					t.Errorf("PartsChairTimeout(%s) = %s, %v; want %s, nil", runTimeout, got, err, tt.wantChair)
				}
			})
		}
	})

	t.Run("an unreadable loom.yaml is refused", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		seedTapestryConfig(t, dir, TapestryConfigTemplate())

		if _, err := LoadTapestryConfig(configengine.Dirs{Anchor: dir}, "tapestry"); err == nil {
			t.Error("LoadTapestryConfig() error = nil; want a refusal when loom.yaml is absent, since the run-wide review keys cannot be resolved")
		}
	})
}
