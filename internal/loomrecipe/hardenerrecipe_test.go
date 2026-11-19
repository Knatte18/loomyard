// hardenerrecipe_test.go covers the hardener recipe.
// It checks the loom contract, the on_done path and ring edges, the review and fix profiles,
// and the hardener.yaml settings the ring rows read.
// It also checks the routing under a Harden-Review budget.

package loomrecipe

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// hardenReviewSegment is the review segment of the hardener recipe's ring.
const hardenReviewSegment = "Harden-Review"

// hardenRing is the hardener recipe's ring rows in the order their on_stuck edges walk them.
var hardenRing = []string{loomshed.NameHardenBouncer, loomshed.NameHardenBurlerReview, loomshed.NameHardenBurlerFix}

// TestHardenerRecipe pins the hardener recipe's loom contract, its routing and the profiles of its ring rows.
//
//testtiming:keep pins the hardener recipe's on_done path, ring edges and review profile, which TestArrasRun_GotoIntoRaddleWrite never asserts
func TestHardenerRecipe(t *testing.T) {
	tiergate.Require(t, 0)
	parsed, err := parseEmbeddedRecipe(shedrun.RecipeHardener)
	if err != nil {
		t.Fatalf("parseEmbeddedRecipe(%q) error = %v; want nil", shedrun.RecipeHardener, err)
	}
	rows := make(map[string]shedbuild.Row, len(parsed.Producers))
	for _, row := range parsed.Producers {
		rows[row.Name] = row
	}

	t.Run("passes the loom contract", func(t *testing.T) {
		env, _ := testEnv(t)
		if _, err := shedbuild.Build(parsed, env); err != nil {
			t.Errorf("shedbuild.Build() error = %v; want nil, the loom contract held", err)
		}
	})

	t.Run("on_done runs from the entry through the ring's Bouncer to the landing", func(t *testing.T) {
		var got []string
		for name := parsed.Entry; name != "" && len(got) <= len(rows); name = rows[name].OnDone {
			got = append(got, name)
		}
		want := []string{
			loomshed.NamePreflight, loomshed.NameRecordWrite, loomshed.NameHardenBouncer,
			loomshed.NameDescribe, loomshed.NamePRReview, loomshed.NameFinalize, loomshed.NameFrictionReflect,
		}
		if !slices.Equal(got, want) {
			t.Errorf("on_done path = %v; want %v", got, want)
		}
	})

	t.Run("on_stuck edges close the ring Bouncer, review, fix, Bouncer", func(t *testing.T) {
		for i, name := range hardenRing {
			want := hardenRing[(i+1)%len(hardenRing)]
			if got := rows[name].OnStuck; got != want {
				t.Errorf("%s on_stuck = %q; want %q", name, got, want)
			}
		}
	})

	t.Run("the review row reads source with tools and the harden rubric, and the fix row declares no gates", func(t *testing.T) {
		profile, _ := rows[loomshed.NameHardenBurlerReview].Config["profile"].(map[string]any)
		if got := profile["tool-use"]; got != true {
			t.Errorf("review profile tool-use = %v; want true", got)
		}
		if got := profile["fix-scope"]; got != "source" {
			t.Errorf("review profile fix-scope = %v; want source", got)
		}
		if got := profile["rubric_stencil"]; got != "harden-rubric-review" {
			t.Errorf("review profile rubric_stencil = %v; want harden-rubric-review", got)
		}
		if _, present := rows[loomshed.NameHardenBurlerFix].Config["gates"]; present {
			t.Errorf("%s config has a gates key; want none, the engine fixes its gates", loomshed.NameHardenBurlerFix)
		}
	})

	t.Run("every setting a ring row reads comes from hardener.yaml", func(t *testing.T) {
		want := map[string]map[string]string{
			loomshed.NameHardenBouncer:      {"reviewer_fan_setting": "hardener.reviewer_fan", "auto_accept_setting": "hardener.auto_accept"},
			loomshed.NameHardenBurlerReview: {"reviewer_fan_setting": "hardener.reviewer_fan", "fixer_fan_setting": "hardener.fixer_fan"},
			loomshed.NameHardenBurlerFix:    {"fixer_fan_setting": "hardener.fixer_fan"},
		}
		for name, settings := range want {
			for key, reference := range settings {
				if got := rows[name].Config[key]; got != reference {
					t.Errorf("%s config %s = %v; want %q", name, key, got, reference)
				}
			}
		}
	})

	t.Run("routing gives the ring rows the segment and its budget", func(t *testing.T) {
		routing, err := Routing(shedrun.RecipeHardener, map[string]int{hardenReviewSegment: 2})
		if err != nil {
			t.Fatalf("Routing() error = %v; want nil", err)
		}
		seen := map[string]bool{}
		for _, p := range routing.Producers {
			if !slices.Contains(hardenRing, p.Name) {
				continue
			}
			seen[p.Name] = true
			if p.Segment != hardenReviewSegment || p.MaxBounces != 2 {
				t.Errorf("%s segment/MaxBounces = %q/%d; want %q/2", p.Name, p.Segment, p.MaxBounces, hardenReviewSegment)
			}
		}
		for _, name := range hardenRing {
			if !seen[name] {
				t.Errorf("Routing() has no row %q", name)
			}
		}
	})
}
