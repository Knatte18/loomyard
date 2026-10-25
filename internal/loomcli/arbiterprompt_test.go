//go:build tier0

package loomcli

import (
	"errors"
	"testing"
)

// TestArbiterAmendmentTarget asserts the target is the recipe's once the plan is approved, and none for an unapproved plan, an approval read error and a recipe that sets none.
func TestArbiterAmendmentTarget(t *testing.T) {
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
