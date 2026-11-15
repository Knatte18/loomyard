package loomcli

import (
	"errors"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestArbiterEnvelopeFields asserts the verb's commits key lists the envelope's commits.
// It is an empty list when the envelope has none.
func TestArbiterEnvelopeFields(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		commits []string
	}{
		{"CommitsListed", []string{"bbb1234 fix the gate"}},
		{"NoCommitsRenderEmptyList", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fields := arbiterEnvelopeFields(loomengine.ArbiterEnvelope{Verdict: loomengine.ArbiterVerdictHandOn, Commits: tt.commits})
			got, ok := fields["commits"].([]string)
			if !ok || got == nil || !slices.Equal(got, slices.Clone(tt.commits)) {
				t.Errorf("arbiterEnvelopeFields commits = %#v; want the non-nil list %q", fields["commits"], tt.commits)
			}
		})
	}
}

// TestArbiterAmendmentTarget asserts the target is the recipe's once the plan is approved;
// it is none for an unapproved plan, an approval read error, or a recipe that sets none.
func TestArbiterAmendmentTarget(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name         string
		recipeTarget string
		approved     func() (bool, error)
		want         string
	}{
		{"ApprovedPlan", "Plan-Burler-Review", func() (bool, error) { return true, nil }, "Plan-Burler-Review"},
		{"UnapprovedPlan", "Plan-Burler-Review", func() (bool, error) { return false, nil }, "none"},
		{"ApprovalReadError", "Plan-Burler-Review", func() (bool, error) { return false, errors.New("unreadable") }, "none"},
		{"RecipeSetsNone", "", func() (bool, error) { return true, nil }, "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := arbiterAmendmentTarget(tt.recipeTarget, tt.approved); got != tt.want {
				t.Errorf("arbiterAmendmentTarget(%q, approved) = %q; want %q", tt.recipeTarget, got, tt.want)
			}
		})
	}
}
