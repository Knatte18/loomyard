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
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// gateQuiescenceFailureMessage is the shared failure text every case below reports on: a turn end whose Stop payload reports an outstanding fork is a wait, so no gate runs while a fork is live.
// A BurlerReview reviewer forks under its cluster fan, and a gated BurlerFix fixer forks only under its segment's fixer fan, which reaches it through the review row's recorded fix_fan and never as a cluster fan.
// The backstop is the reviewer audit's exact count and the fixer audit's per-fork violation checks, the fixer's count being a warning.
// A gated row that forks outside those two has no such wait to rest on, so the decision must be re-opened rather than this test updated to tolerate the new shape.
const gateQuiescenceFailureMessage = "a turn end whose Stop payload reports an outstanding fork is a wait, so no gate runs while a fork is live; a BurlerReview reviewer forks under its cluster fan, and a gated BurlerFix fixer forks only under its segment's fixer fan, which reaches it through the review row's recorded fix_fan, never a cluster fan; the backstop is the reviewer audit's exact count and the fixer audit's per-fork violation checks, the fixer's count being a warning -- the decision must be re-opened, not this test"

// gatedRowFanViolations returns one message per gated row of r that breaks the fork rule.
// A row is gated when its config carries a non-empty "gates" list.
// A gated row receives a fan from its profile's "cluster-fan" or from fans, a row-name to fan-list map of the values the rows' fan settings would answer, when any entry of its list is non-empty.
// A gated row must be a writer engine (DiscussionWrite, PlanWrite, DarnWrite, Describe, PRRework) or BurlerFix, and none of them may receive a cluster fan.
// A writer engine's spec factory never sets shuttleengine.Spec.ForkSubagents, and burlerengine.Engine.Review sets it from a non-empty fan alone, so a cluster fan on the ungated BurlerReview row is the one legal reviewer fork.
// A gated BurlerFix fixer forks only under the fixer fan the review row recorded, so only a BurlerFix row may name "fixer_fan_setting".
func gatedRowFanViolations(r shedbuild.Recipe, fans map[string][]string) []string {
	var violations []string
	for _, row := range r.Producers {
		if gates, ok := row.Config["gates"].([]any); !ok || len(gates) == 0 {
			continue
		}

		if setting, has := row.Config["fixer_fan_setting"]; has && setting != "" && row.Engine != "BurlerFix" {
			violations = append(violations, fmt.Sprintf("row %q (engine %q): names fixer_fan_setting %q; want it only on a BurlerFix row -- %s", row.Name, row.Engine, setting, gateQuiescenceFailureMessage))
		}
		fan := ""
		for _, entry := range fans[row.Name] {
			if entry != "" {
				fan = entry
				break
			}
		}
		if profile, ok := row.Config["profile"].(map[string]any); ok {
			if profileFan, has := profile["cluster-fan"]; has && profileFan != "" {
				fan = fmt.Sprint(profileFan)
			}
		}

		switch row.Engine {
		case "DiscussionWrite", "PlanWrite", "DarnWrite", "Describe", "PRRework", "BurlerFix":
			if fan != "" {
				violations = append(violations, fmt.Sprintf("row %q (engine %q): receives fan %q; want no fan on a gated row -- %s", row.Name, row.Engine, fan, gateQuiescenceFailureMessage))
			}
		default:
			violations = append(violations, fmt.Sprintf("row %q: carries a \"gates\" config list with engine %q, neither a writer engine (DiscussionWrite/PlanWrite/DarnWrite/Describe/PRRework) nor BurlerFix -- %s", row.Name, row.Engine, gateQuiescenceFailureMessage))
		}
	}
	return violations
}

// TestNoGatedRowAuthorizesForkSubagents asserts the fork rule over the shipped loom and darn recipes with a fan set for every row that names a fan setting, and over hand-authored recipes.
// A gated writer or BurlerFix row receiving a fan, from its profile or from the map, fails; an ungated BurlerReview row with a fan passes.
// It also pins that no shipped loom row carries a literal "cluster-fan" key, since a fan reaches a row only through the hub config.
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
				t.Errorf("row %q (engine %q): profile carries a literal \"cluster-fan\" key; want none, a fan reaches a row only through the hub config", row.Name, row.Engine)
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

	shippedDarn, err := shedbuild.Parse(recipes.DarnRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse(recipes.DarnRecipe) error = %v; want nil", err)
	}
	// Every row that names a fan setting takes the fan its perch is assigned, except a BurlerFix row, whose fixer fan reaches it through the review row's record.
	perchFans := map[string]string{"Discussion-Review": "standard", "Plan-Review": "full"}
	fanSettingFans := func(r shedbuild.Recipe) map[string][]string {
		fans := map[string][]string{}
		for _, row := range r.Producers {
			_, namesReviewer := row.Config["reviewer_fan_setting"]
			_, namesFixer := row.Config["fixer_fan_setting"]
			if (!namesReviewer && !namesFixer) || row.Engine == "BurlerFix" {
				continue
			}
			fan := perchFans[row.Segment]
			if fan == "" {
				fan = "review-code"
			}
			fans[row.Name] = []string{fan}
		}
		return fans
	}
	reviewFans := fanSettingFans(shipped)
	darnFans := fanSettingFans(shippedDarn)
	if len(reviewFans) == 0 || len(darnFans) == 0 {
		t.Fatalf("no shipped row names a fan setting (loom %d rows, darn %d); this test has nothing to assert -- the recipe or this test has drifted", len(reviewFans), len(darnFans))
	}

	tests := []struct {
		name           string
		recipe         shedbuild.Recipe
		fans           map[string][]string
		wantViolations int
	}{
		{name: "shipped loom recipe with a fan on every review perch", recipe: shipped, fans: reviewFans},
		{name: "shipped darn recipe with a Darn-Review fan", recipe: shippedDarn, fans: darnFans},
		{name: "gated writer row naming a fixer fan setting", recipe: shedbuild.Recipe{Producers: []shedbuild.Row{{Name: "Writer", Engine: "DiscussionWrite", Config: map[string]any{"gates": []any{map[string]any{"name": "discussion"}}, "fixer_fan_setting": "loom.discussion_fixer_fan"}}}}, wantViolations: 1},
		{name: "gated writer row receiving a fan from the map", recipe: gatedWriter(nil), fans: map[string][]string{"Writer": {"", "standard"}}, wantViolations: 1},
		{name: "gated writer row receiving a fan from its profile", recipe: gatedWriter("standard"), wantViolations: 1},
		{name: "gated writer row with no fan", recipe: gatedWriter(nil)},
		{name: "gated writer row with an all-empty fan list", recipe: gatedWriter(nil), fans: map[string][]string{"Writer": {""}}},
		{name: "ungated BurlerReview row with a fan", recipe: burlerRow("BurlerReview", false), fans: map[string][]string{"Burler": {"full"}}},
		{name: "gated BurlerFix row with no fan", recipe: shedbuild.Recipe{Producers: []shedbuild.Row{{Name: "Fixer", Engine: "BurlerFix", Config: map[string]any{"gates": []any{map[string]any{"name": "verify"}}}}}}},
		{name: "gated BurlerFix row receiving a fan from the map", recipe: burlerRow("BurlerFix", true), fans: map[string][]string{"Burler": {"full"}}, wantViolations: 1},
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
