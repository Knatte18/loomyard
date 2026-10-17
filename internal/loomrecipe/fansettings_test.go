// fansettings_test.go pins that the shipped recipes name fan and auto-accept settings consistently within each review segment, and that each row names the setting its perch's wiring has always read.

package loomrecipe

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/shedbuild"
)

// settingText returns the string value of row's Config key, "" when the key is absent.
func settingText(row *shedbuild.Row, key string) string {
	if row == nil {
		return ""
	}
	text, _ := row.Config[key].(string)
	return text
}

// fanSettingMismatches returns one message per fan-setting violation in r, grouping rows by segment among segments holding a Bouncer row.
// The rows of a segment naming reviewer_fan_setting name one reference, and likewise fixer_fan_setting.
// The BurlerReview row names reviewer_fan_setting exactly when the Bouncer does.
// The BurlerFix row names fixer_fan_setting exactly when the BurlerReview row names it or sets a profile fix-fan.
func fanSettingMismatches(r shedbuild.Recipe) []string {
	type segmentRows struct{ bouncer, review, fix *shedbuild.Row }
	var order []string
	bySegment := map[string]*segmentRows{}
	for i := range r.Producers {
		row := &r.Producers[i]
		if row.Segment == "" {
			continue
		}
		rows, ok := bySegment[row.Segment]
		if !ok {
			rows = &segmentRows{}
			bySegment[row.Segment] = rows
			order = append(order, row.Segment)
		}
		switch row.Engine {
		case "Bouncer":
			rows.bouncer = row
		case "BurlerReview":
			rows.review = row
		case "BurlerFix":
			rows.fix = row
		}
	}

	var mismatches []string
	for _, segment := range order {
		rows := bySegment[segment]
		if rows.bouncer == nil {
			continue
		}
		for _, key := range []string{"reviewer_fan_setting", "fixer_fan_setting"} {
			references := map[string]bool{}
			for _, row := range []*shedbuild.Row{rows.bouncer, rows.review, rows.fix} {
				if text := settingText(row, key); text != "" {
					references[text] = true
				}
			}
			if len(references) > 1 {
				mismatches = append(mismatches, fmt.Sprintf("segment %q: rows naming %s name %d different references", segment, key, len(references)))
			}
		}
		if (settingText(rows.bouncer, "reviewer_fan_setting") != "") != (settingText(rows.review, "reviewer_fan_setting") != "") {
			mismatches = append(mismatches, fmt.Sprintf("segment %q: the Bouncer and the BurlerReview row disagree on naming reviewer_fan_setting", segment))
		}
		fixWanted := settingText(rows.review, "fixer_fan_setting") != ""
		if rows.review != nil {
			if profile, ok := rows.review.Config["profile"].(map[string]any); ok {
				if fixFan, _ := profile["fix-fan"].(string); fixFan != "" {
					fixWanted = true
				}
			}
		}
		if fixWanted != (settingText(rows.fix, "fixer_fan_setting") != "") {
			mismatches = append(mismatches, fmt.Sprintf("segment %q: the BurlerFix row names fixer_fan_setting %t but the review row's fixer fan wants it %t", segment, settingText(rows.fix, "fixer_fan_setting") != "", fixWanted))
		}
	}
	return mismatches
}

// TestShippedRecipes_FanSettingsAgreeWithinSegment asserts the shipped loom and darn recipes have no fan-setting mismatch and that each shipped row names exactly the references its perch's wiring reads.
// It also runs hand-authored three-row recipes through fanSettingMismatches, so a rule that stops firing fails here.
//
//testtiming:keep pins the exact references of the shipped recipes, which a recipe edit pointing a row at another key must fail, and self-checks each rule of fanSettingMismatches over hand-authored segments
func TestShippedRecipes_FanSettingsAgreeWithinSegment(t *testing.T) {
	loom, err := shedbuild.Parse(recipes.LoomRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse(recipes.LoomRecipe) error = %v; want nil", err)
	}
	darn, err := shedbuild.Parse(recipes.DarnRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse(recipes.DarnRecipe) error = %v; want nil", err)
	}

	t.Run("ShippedRecipesAgree", func(t *testing.T) {
		for name, recipe := range map[string]shedbuild.Recipe{"loom": loom, "darn": darn} {
			if mismatches := fanSettingMismatches(recipe); len(mismatches) != 0 {
				t.Errorf("fanSettingMismatches(%s) = %q; want none", name, mismatches)
			}
		}
	})

	t.Run("ShippedRowsNameTheirPerchsReferences", func(t *testing.T) {
		type perch struct {
			recipe          shedbuild.Recipe
			segment         string
			reviewer, fixer string
			autoAccept      string
		}
		perches := []perch{
			{loom, "Discussion-Review", "loom.discussion_reviewer_fan", "loom.discussion_fixer_fan", "loom.discussion_auto_accept"},
			{loom, "Plan-Review", "loom.plan_reviewer_fan", "loom.plan_fixer_fan", "loom.plan_auto_accept"},
			{loom, "Webster-Review", "loom.webster_reviewer_fan", "loom.webster_fixer_fan", "loom.webster_auto_accept"},
			{loom, "Merge-Review", "loom.merge_reviewer_fan", "loom.merge_fixer_fan", "loom.merge_auto_accept"},
			{darn, "Darn-Review", "darn.darn_reviewer_fan", "darn.darn_fixer_fan", ""},
		}
		for _, p := range perches {
			want := map[string]map[string]string{
				"Bouncer":      {"reviewer_fan_setting": p.reviewer, "auto_accept_setting": p.autoAccept},
				"BurlerReview": {"reviewer_fan_setting": p.reviewer, "fixer_fan_setting": p.fixer},
				"BurlerFix":    {"fixer_fan_setting": p.fixer},
			}
			seen := map[string]bool{}
			for i := range p.recipe.Producers {
				row := &p.recipe.Producers[i]
				if row.Segment != p.segment || want[row.Engine] == nil {
					continue
				}
				seen[row.Engine] = true
				for _, key := range []string{"reviewer_fan_setting", "fixer_fan_setting", "auto_accept_setting"} {
					if got := settingText(row, key); got != want[row.Engine][key] {
						t.Errorf("row %q: %s = %q; want %q", row.Name, key, got, want[row.Engine][key])
					}
				}
			}
			if !reflect.DeepEqual(seen, map[string]bool{"Bouncer": true, "BurlerReview": true, "BurlerFix": true}) {
				t.Errorf("segment %q holds rows of engines %v; want a Bouncer, a BurlerReview and a BurlerFix row", p.segment, seen)
			}
		}
	})

	t.Run("HandAuthoredSegments", func(t *testing.T) {
		row := func(name, engine string, config map[string]any) shedbuild.Row {
			return shedbuild.Row{Name: name, Engine: engine, Segment: "S", Config: config}
		}
		agreeing := func() (bouncer, review, fix map[string]any) {
			return map[string]any{"reviewer_fan_setting": "loom.r"},
				map[string]any{"reviewer_fan_setting": "loom.r", "fixer_fan_setting": "loom.f"},
				map[string]any{"fixer_fan_setting": "loom.f"}
		}
		tests := []struct {
			name   string
			mutate func(bouncer, review, fix map[string]any)
			want   int
		}{
			{"AgreeingSegment", func(b, r, f map[string]any) {}, 0},
			{"ReviewerReferencesDifferBetweenBouncerAndReview", func(b, r, f map[string]any) { b["reviewer_fan_setting"] = "loom.other" }, 1},
			{"FixerReferencesDifferBetweenReviewAndFix", func(b, r, f map[string]any) { f["fixer_fan_setting"] = "loom.other" }, 1},
			{"ReviewNamesReviewerWithoutTheBouncer", func(b, r, f map[string]any) { delete(b, "reviewer_fan_setting") }, 1},
			{"BouncerNamesReviewerWithoutTheReview", func(b, r, f map[string]any) { delete(r, "reviewer_fan_setting") }, 1},
			{"FixMissingFixerBesideReviewNamingIt", func(b, r, f map[string]any) { delete(f, "fixer_fan_setting") }, 1},
			{"FixMissingFixerBesideReviewProfileFixFan", func(b, r, f map[string]any) {
				delete(r, "fixer_fan_setting")
				delete(f, "fixer_fan_setting")
				r["profile"] = map[string]any{"fix-fan": "recipe-fix"}
			}, 1},
			{"FixNamingFixerWithNeither", func(b, r, f map[string]any) { delete(r, "fixer_fan_setting") }, 1},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				bouncer, review, fix := agreeing()
				tt.mutate(bouncer, review, fix)
				recipe := shedbuild.Recipe{Producers: []shedbuild.Row{row("B", "Bouncer", bouncer), row("R", "BurlerReview", review), row("F", "BurlerFix", fix)}}
				if got := fanSettingMismatches(recipe); len(got) != tt.want {
					t.Errorf("fanSettingMismatches() = %q; want %d message(s)", got, tt.want)
				}
			})
		}
	})
}
