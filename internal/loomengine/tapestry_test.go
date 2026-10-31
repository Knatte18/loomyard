//go:build tier0

// tapestry_test.go — tier0 tests for the tapestry config module: LoadTapestryConfig over a worktree-only layer stack, and the plan values Config.WithTapestry applies.

package loomengine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
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

// tapestryTemplateWith returns the shipped tapestry template with each key in values replaced by its value.
func tapestryTemplateWith(t *testing.T, values map[string]string) string {
	t.Helper()
	var b strings.Builder
	replaced := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSuffix(TapestryConfigTemplate(), "\n"), "\n") {
		matched := false
		for key, value := range values {
			if strings.HasPrefix(line, key+":") {
				b.WriteString(key + ": " + value + "\n")
				replaced[key] = true
				matched = true
				break
			}
		}
		if !matched {
			b.WriteString(line + "\n")
		}
	}
	for key := range values {
		if !replaced[key] {
			t.Fatalf("key %q not present in TapestryConfigTemplate(); the fixture would silently test nothing", key)
		}
	}
	return b.String()
}

// seedLoomAndTapestry seeds baseDir's worktree layer with a loom.yaml built from the shipped template with loomValues applied, and a tapestry.yaml with tapestryValues applied.
func seedLoomAndTapestry(t *testing.T, baseDir string, loomValues, tapestryValues map[string]string) {
	t.Helper()
	if len(loomValues) == 0 {
		seedLoomConfig(t, baseDir, ConfigTemplate())
	} else {
		writeLoomConfigWithKeys(t, baseDir, loomValues)
	}
	seedTapestryConfig(t, baseDir, tapestryTemplateWith(t, tapestryValues))
}

// TestLoadTapestryConfig verifies the plan perch's values load from tapestry.yaml and resolve against loom.yaml's run-wide keys, and that Config.WithTapestry applies them:
// a null plan_max_bounces and a zero plan_hard_max_bounces take loom.yaml's budget and hard cap, an empty model list, judge and seed take the shared ones, and every malformed or out-of-bound value is refused naming the tapestry key.
func TestLoadTapestryConfig(t *testing.T) {
	t.Parallel()

	t.Run("the template loads and WithTapestry applies every plan value", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		seedLoomAndTapestry(t, dir, nil, map[string]string{
			"plan": "opus[effort=high]", "plan_timeout_min": "7", "plan_lenses": `""`,
			"plan_reviewer_model": "sonnet[low]", "plan_fixer_model": "sonnet[high]",
			"plan_reviewer_fan": "full", "plan_fixer_fan": "fix-plan",
			"plan_judge": "opus[low]", "plan_seed": "haiku[low]",
			"plan_max_bounces": "2", "plan_hard_max_bounces": "3", "plan_auto_accept": "true",
		})
		dirs := configengine.Dirs{Anchor: dir}

		loaded, err := LoadTapestryConfig(dirs, "tapestry")
		if err != nil {
			t.Fatalf("LoadTapestryConfig() = _, %v; want nil error", err)
		}
		want := TapestryConfig{
			Plan: "opus[effort=high]", PlanTimeoutMin: 7, PlanLenses: "",
			PlanReviewerModel: burlerengine.ModelSpecList{"sonnet[low]"}, PlanFixerModel: burlerengine.ModelSpecList{"sonnet[high]"},
			PlanReviewerFan: lenses.FanList{"full"}, PlanFixerFan: lenses.FanList{"fix-plan"},
			PlanJudge: "opus[low]", PlanSeed: "haiku[low]",
			PlanMaxBounces: intPtr(2), PlanHardMaxBounces: 3, PlanAutoAccept: true,
		}
		if !reflect.DeepEqual(loaded, want) {
			t.Errorf("LoadTapestryConfig() = %+v; want %+v", loaded, want)
		}

		loom, err := LoadConfig(dirs, "loom")
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		if loom.Plan != "" || loom.PlanMaxBounces != nil || loom.PlanReviewerModel != nil {
			t.Errorf("LoadConfig() plan fields = %q/%v/%v; want the zero values, since loom.yaml never reads tapestry.yaml", loom.Plan, loom.PlanMaxBounces, loom.PlanReviewerModel)
		}
		applied := loom.WithTapestry(loaded)
		if applied.Plan != want.Plan || applied.PlanTimeoutMin != 7 || applied.PlanLenses != "" || !reflect.DeepEqual(applied.PlanReviewerFan, want.PlanReviewerFan) ||
			!reflect.DeepEqual(applied.PlanFixerFan, want.PlanFixerFan) || applied.PlanJudge != want.PlanJudge || applied.PlanSeed != want.PlanSeed || !applied.PlanAutoAccept {
			t.Errorf("WithTapestry() = %+v; want every plan field copied from %+v", applied, want)
		}
		if caps := applied.Caps(PerchPlan); caps.Soft != 2 || caps.SoftKey != "plan_max_bounces" || caps.Hard != 3 || caps.HardKey != "plan_hard_max_bounces" {
			t.Errorf("WithTapestry().Caps(PerchPlan) = %+v; want the plan perch's own budget and hard cap", caps)
		}
	})

	t.Run("a null budget, a zero hard cap and empty lists, judge and seed take loom.yaml's", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		seedLoomAndTapestry(t, dir,
			map[string]string{"review_max_bounces": "5", "review_hard_max_bounces": "6"},
			map[string]string{"plan_reviewer_model": `""`})
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
		if caps := cfg.Caps(PerchPlan); caps.Soft != 5 || caps.SoftKey != "review_max_bounces" || caps.Hard != 6 || caps.HardKey != "review_hard_max_bounces" {
			t.Errorf("Caps(PerchPlan) = %+v; want review_max_bounces 5 and review_hard_max_bounces 6", caps)
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
		sharedJudge, err := resolveKeyedModelChoice("judge", cfg.Judge, reg)
		if err != nil {
			t.Fatalf("resolveKeyedModelChoice() = _, %v; want nil error", err)
		}
		if judge.Plan.Judge != sharedJudge || judge.Plan.Seed != sharedJudge {
			t.Errorf("ResolveJudge().Plan = %+v; want loom.yaml's judge %+v for both passes", judge.Plan, sharedJudge)
		}
		if got := review.PlanKeys.Review; got != (shuttleengine.ModelKey{Module: "loom", Key: "reviewer_model"}) {
			t.Errorf("ResolveReview().PlanKeys.Review = %+v; want loom's shared reviewer_model", got)
		}
	})

	refusals := []struct {
		name   string
		loom   map[string]string
		key    string
		value  string
		wantIn []string
	}{
		{name: "malformed plan spec", key: "plan", value: `"opus[effort"`},
		{name: "malformed plan_reviewer_model spec", key: "plan_reviewer_model", value: `"opus[effort"`, wantIn: []string{"entry 1", "a non-empty list of model-specs"}},
		{name: "malformed plan_fixer_model spec", key: "plan_fixer_model", value: `"opus[effort"`, wantIn: []string{"entry 1", "a non-empty list of model-specs"}},
		{name: "unknown plan_reviewer_fan", key: "plan_reviewer_fan", value: "nosuchfan", wantIn: []string{`"nosuchfan"`, "known fans: ", "full", "entry 1", `set the entry to ""`}},
		{name: "empty plan_reviewer_fan list", key: "plan_reviewer_fan", value: "[]", wantIn: []string{"the list is empty", `write ""`}},
		{name: "empty plan_fixer_fan list", key: "plan_fixer_fan", value: "[]", wantIn: []string{"the list is empty", `write ""`}},
		{name: "mapping inside a plan_reviewer_fan list", key: "plan_reviewer_fan", value: "\n  - full\n  - model: opus", wantIn: []string{"line ", "neither a fan name nor a list of fan names", "a list of fan names"}},
		{name: "malformed plan_judge spec", key: "plan_judge", value: `"opus[effort"`},
		{name: "malformed plan_seed spec", key: "plan_seed", value: `"opus[effort"`},
		{name: "negative plan_max_bounces", key: "plan_max_bounces", value: "-1", wantIn: []string{"must not be negative", "or to null to take review_max_bounces"}},
		{name: "negative plan_hard_max_bounces", key: "plan_hard_max_bounces", value: "-1", wantIn: []string{"set it to 0 to take review_hard_max_bounces", "lym config tapestry --set plan_hard_max_bounces=<n>", `lym loom resume`}},
		{name: "negative plan_timeout_min", key: "plan_timeout_min", value: "-1", wantIn: []string{"must not be negative"}},
		{
			name: "plan_hard_max_bounces below the budget inherited from loom.yaml",
			loom: map[string]string{"review_max_bounces": "5", "review_hard_max_bounces": "6"},
			key:  "plan_hard_max_bounces", value: "2",
			wantIn: []string{"resolved hard cap 2", `"review_max_bounces"'s resolved soft cap 5`, "lym config tapestry --set plan_hard_max_bounces=<n>", "lym config loom --set review_max_bounces=<n>"},
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

	t.Run("an unreadable loom.yaml is refused", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		seedTapestryConfig(t, dir, TapestryConfigTemplate())

		if _, err := LoadTapestryConfig(configengine.Dirs{Anchor: dir}, "tapestry"); err == nil {
			t.Error("LoadTapestryConfig() error = nil; want a refusal when loom.yaml is absent, since the run-wide review keys cannot be resolved")
		}
	})
}
