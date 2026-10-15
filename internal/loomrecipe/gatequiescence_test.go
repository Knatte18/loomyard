// gatequiescence_test.go pins the precondition the per-attempt done-signal narrowing rests on:
// a gate runs at a turn end that is not a wait, so no gate may run while a fork subagent is live.
// Batch 1's engine_test.go pins that the in-process Agent tool is denied by the shipped shuttle template.
// This file pins which gated rows may authorize shuttleengine.Spec.ForkSubagents at all.
//
// It parses the real embedded recipe and hand-authored recipes and reads no worktree, staying untagged and offline.

package loomrecipe

import (
	"fmt"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// gateQuiescenceFailureMessage is the shared failure text every case below reports on: a turn end whose Stop payload reports an outstanding fork is a wait, so no gate runs while a fork is live,
// and the burler fork audit's exact count is the backstop.
// A gated row that forks has no such wait to rest on, so the decision must be re-opened rather than this test updated to tolerate the new shape.
// A BurlerReview reviewer may fork because it carries no "gates" key, and a BurlerFix fixer may carry gates because it never forks.
const gateQuiescenceFailureMessage = "a turn end whose Stop payload reports an outstanding fork is a wait, so no gate runs while a fork is live, and the burler fork audit's exact count is the backstop; only a BurlerReview reviewer, whose only gate is the review-parse entry, may fork -- the decision must be re-opened, not this test"

// gatedRowFanViolations returns one message per gated row of r that breaks the fork rule.
// A row is gated when its config carries a non-empty "gates" list.
// A gated row receives a fan from its profile's "cluster-fan" or from fans, a row-name to fan map as Env.RowClusterFans carries them.
// A gated row must be a writer engine (DiscussionWrite, PlanWrite, Describe, PRRework) or BurlerFix, and none of them may receive a fan.
// A writer engine's spec factory never sets shuttleengine.Spec.ForkSubagents, and burlerengine.Engine.Review sets it from a non-empty fan alone, so a fan on the ungated BurlerReview row is the one legal fork.
func gatedRowFanViolations(r shedbuild.Recipe, fans map[string]string) []string {
	var violations []string
	for _, row := range r.Producers {
		if gates, ok := row.Config["gates"].([]any); !ok || len(gates) == 0 {
			continue
		}

		fan := fans[row.Name]
		if profile, ok := row.Config["profile"].(map[string]any); ok {
			if profileFan, has := profile["cluster-fan"]; has && profileFan != "" {
				fan = fmt.Sprint(profileFan)
			}
		}

		switch row.Engine {
		case "DiscussionWrite", "PlanWrite", "Describe", "PRRework", "BurlerFix":
			if fan != "" {
				violations = append(violations, fmt.Sprintf("row %q (engine %q): receives fan %q; want no fan on a gated row -- %s", row.Name, row.Engine, fan, gateQuiescenceFailureMessage))
			}
		default:
			violations = append(violations, fmt.Sprintf("row %q: carries a \"gates\" config list with engine %q, neither a writer engine (DiscussionWrite/PlanWrite/Describe/PRRework) nor BurlerFix -- %s", row.Name, row.Engine, gateQuiescenceFailureMessage))
		}
	}
	return violations
}

// TestNoGatedRowAuthorizesForkSubagents asserts the fork rule over the shipped recipe with fans set for the Discussion-Review and Plan-Review rows through the loomshed row names, and over hand-authored recipes.
// A gated writer or BurlerFix row receiving a fan, from its profile or from the map, fails; an ungated BurlerReview row with a fan passes.
// It also pins that no shipped row carries a literal "cluster-fan" key, the Webster rows included, since a fan reaches a row only through loom.yaml.
//
//testtiming:keep pins that no gated shipped row authorizes fork subagents outside an ungated BurlerReview reviewer, a guard that fires on the recipe file, which TestApproveSeam_FailsToBuild never reads
func TestNoGatedRowAuthorizesForkSubagents(t *testing.T) {
	shipped, err := shedbuild.Parse(recipes.LoomRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse(recipes.LoomRecipe) error = %v; want nil", err)
	}

	gatedRowFound := false
	for _, row := range shipped.Producers {
		if gates, ok := row.Config["gates"].([]any); ok && len(gates) > 0 {
			gatedRowFound = true
		}
		if profile, ok := row.Config["profile"].(map[string]any); ok {
			if _, has := profile["cluster-fan"]; has {
				t.Errorf("row %q (engine %q): profile carries a literal \"cluster-fan\" key; want none, a fan reaches a row only through loom.yaml", row.Name, row.Engine)
			}
		}
	}
	if !gatedRowFound {
		t.Fatalf("no row in the embedded recipe carries a \"gates\" config list; this test has nothing to assert -- the recipe or this test has drifted")
	}

	gatedWriter := func(profileFan any) shedbuild.Recipe {
		config := map[string]any{"gates": []any{map[string]any{"name": "discussion"}}}
		if profileFan != nil {
			config["profile"] = map[string]any{"cluster-fan": profileFan}
		}
		return shedbuild.Recipe{Producers: []shedbuild.Row{{Name: "Writer", Engine: "DiscussionWrite", Config: config}}}
	}
	burlerRow := func(engine string, gated bool) shedbuild.Recipe {
		config := map[string]any{"profile": map[string]any{"cluster-fan": "standard"}}
		if gated {
			config["gates"] = []any{map[string]any{"name": "verify"}}
		}
		return shedbuild.Recipe{Producers: []shedbuild.Row{{Name: "Burler", Engine: engine, Config: config}}}
	}

	reviewFans := map[string]string{
		loomshed.NameDiscussionBouncer:      "standard",
		loomshed.NameDiscussionBurlerReview: "standard",
		loomshed.NamePlanBouncer:            "full",
		loomshed.NamePlanBurlerReview:       "full",
	}

	tests := []struct {
		name           string
		recipe         shedbuild.Recipe
		fans           map[string]string
		wantViolations int
	}{
		{name: "shipped recipe with Discussion-Review and Plan-Review fans", recipe: shipped, fans: reviewFans},
		{name: "gated writer row receiving a fan from the map", recipe: gatedWriter(nil), fans: map[string]string{"Writer": "standard"}, wantViolations: 1},
		{name: "gated writer row receiving a fan from its profile", recipe: gatedWriter("standard"), wantViolations: 1},
		{name: "gated writer row with no fan", recipe: gatedWriter(nil)},
		{name: "ungated BurlerReview row with a fan", recipe: burlerRow("BurlerReview", false), fans: map[string]string{"Burler": "full"}},
		{name: "gated BurlerFix row with no fan", recipe: shedbuild.Recipe{Producers: []shedbuild.Row{{Name: "Fixer", Engine: "BurlerFix", Config: map[string]any{"gates": []any{map[string]any{"name": "verify"}}}}}}},
		{name: "gated BurlerFix row receiving a fan from the map", recipe: burlerRow("BurlerFix", true), fans: map[string]string{"Burler": "full"}, wantViolations: 1},
		{
			name: "gated row of an engine that is neither a writer nor BurlerFix",
			recipe: shedbuild.Recipe{Producers: []shedbuild.Row{{
				Name: "Other", Engine: "Webster", Config: map[string]any{"gates": []any{map[string]any{"name": "verify"}}},
			}}},
			wantViolations: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := gatedRowFanViolations(tt.recipe, tt.fans)
			if len(violations) != tt.wantViolations {
				t.Errorf("gatedRowFanViolations() = %q; want %d violation(s)", violations, tt.wantViolations)
			}
		})
	}
}

// TestWebsterRoundCarriesNoGateKey asserts the Webster round carries no "gates" key at all, so TestNoGatedRowAuthorizesForkSubagents's rule over gated rows is exhaustive without a case for it.
//
//testtiming:keep pins that the shipped Webster round has no gates key, a guard that fires on the recipe file, which TestApproveSeam_FailsToBuild never reads
func TestWebsterRoundCarriesNoGateKey(t *testing.T) {
	r, err := shedbuild.Parse(recipes.LoomRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse(recipes.LoomRecipe) error = %v; want nil", err)
	}

	found := false
	for _, row := range r.Producers {
		if row.Engine != "Webster" {
			continue
		}
		found = true
		if gates, hasGates := row.Config["gates"]; hasGates {
			t.Errorf("row %q (engine %q): config[\"gates\"] = %v; want absent -- %s", row.Name, row.Engine, gates, gateQuiescenceFailureMessage)
		}
	}
	if !found {
		t.Fatalf("no row in the embedded recipe has engine %q; this test has nothing to assert", "Webster")
	}
}
