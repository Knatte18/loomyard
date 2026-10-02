package planparser_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

func reviewClassCard(groups ...planparser.TargetGroup) planparser.Card {
	return planparser.Card{Number: 1, Slug: "a", TargetGroups: groups}
}

func reviewClassGroup(t planparser.CardType, refs ...string) planparser.TargetGroup {
	return planparser.TargetGroup{Type: t, Refs: refs}
}

func TestReviewExempt(t *testing.T) {
	prosa := planparser.CardTypeProsa
	tests := []struct {
		name string
		plan planparser.Plan
		want bool
	}{
		{"md and yaml prosa", planparser.Plan{Language: "go", Cards: []planparser.Card{
			reviewClassCard(reviewClassGroup(prosa, "docs/a.md#", "conf/b.yaml#")),
		}}, true},
		{"edit card", planparser.Plan{Language: "go", Cards: []planparser.Card{
			reviewClassCard(reviewClassGroup(planparser.CardTypeEdit, "docs/a.md#")),
		}}, false},
		{"create card", planparser.Plan{Language: "go", Cards: []planparser.Card{
			reviewClassCard(reviewClassGroup(planparser.CardTypeCreate, "docs/a.md#")),
		}}, false},
		{"delete card", planparser.Plan{Language: "go", Cards: []planparser.Card{
			reviewClassCard(reviewClassGroup(planparser.CardTypeDelete, "docs/a.md#")),
		}}, false},
		{"rename card", planparser.Plan{Language: "go", Cards: []planparser.Card{
			reviewClassCard(reviewClassGroup(planparser.CardTypeRename, "docs/a.md#")),
		}}, false},
		{"move card", planparser.Plan{Language: "go", Cards: []planparser.Card{
			reviewClassCard(reviewClassGroup(planparser.CardTypeMove, "docs/a.md#")),
		}}, false},
		{"custom card", planparser.Plan{Language: "go", Cards: []planparser.Card{
			reviewClassCard(reviewClassGroup(planparser.CardTypeCustom, "docs/a.md#")),
		}}, false},
		{"prosa plus edit", planparser.Plan{Language: "go", Cards: []planparser.Card{
			reviewClassCard(reviewClassGroup(prosa, "docs/a.md#"), reviewClassGroup(planparser.CardTypeEdit, "docs/b.md#")),
		}}, false},
		{"prosa on go file", planparser.Plan{Language: "go", Cards: []planparser.Card{
			reviewClassCard(reviewClassGroup(prosa, "internal/foo/a.go#")),
		}}, false},
		{"prosa on whole package", planparser.Plan{Language: "go", Cards: []planparser.Card{
			reviewClassCard(reviewClassGroup(prosa, "internal/foo#")),
		}}, false},
		{"prosa on unparseable ref", planparser.Plan{Language: "go", Cards: []planparser.Card{
			reviewClassCard(reviewClassGroup(prosa, "not a glyph")),
		}}, false},
		{"none: directory", planparser.Plan{Language: "none", Cards: []planparser.Card{
			reviewClassCard(reviewClassGroup(prosa, "docs/sub")),
		}}, false},
		{"none: file", planparser.Plan{Language: "none", Cards: []planparser.Card{
			reviewClassCard(reviewClassGroup(prosa, "docs/a.md")),
		}}, true},
		{"no cards", planparser.Plan{Language: "go"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := planparser.ReviewExempt(&tt.plan); got != tt.want {
				t.Errorf("ReviewExempt = %v, want %v", got, tt.want)
			}
		})
	}
}
