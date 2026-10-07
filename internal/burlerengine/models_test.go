package burlerengine

import "testing"

func TestRoundModels_Pick(t *testing.T) {
	t.Parallel()

	review := []ModelChoice{{Model: "r1"}, {Model: "r2", Effort: "high"}}
	fix := []ModelChoice{{Model: "f1", Version: "5"}}

	tests := []struct {
		name       string
		models     RoundModels
		round      int
		wantReview ModelChoice
		wantFix    ModelChoice
	}{
		{"round 1 takes the first entry of each list", RoundModels{Review: review, Fix: fix}, 1, review[0], fix[0]},
		{"round 2 takes the second entry of a long list and the last of a short one", RoundModels{Review: review, Fix: fix}, 2, review[1], fix[0]},
		{"a round past the list takes the last entry", RoundModels{Review: review, Fix: fix}, 9, review[1], fix[0]},
		{"a round below 1 is round 1", RoundModels{Review: review, Fix: fix}, 0, review[0], fix[0]},
		{"a negative round is round 1", RoundModels{Review: review, Fix: fix}, -3, review[0], fix[0]},
		{"an empty list yields the zero choice, independently of the other", RoundModels{Review: review}, 2, review[1], ModelChoice{}},
		{"both lists empty yield zero choices", RoundModels{}, 1, ModelChoice{}, ModelChoice{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotReview, gotFix := tt.models.Pick(tt.round)
			if gotReview != tt.wantReview {
				t.Errorf("Pick(%d) review = %+v; want %+v", tt.round, gotReview, tt.wantReview)
			}
			if gotFix != tt.wantFix {
				t.Errorf("Pick(%d) fix = %+v; want %+v", tt.round, gotFix, tt.wantFix)
			}
		})
	}
}
