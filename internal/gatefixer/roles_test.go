// roles_test.go covers the fixer role set and the numbered-form match.

package gatefixer

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestIsFixerRole pins which roles count as a fixer role, and the order Roles lists them in.
func TestIsFixerRole(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	if got, want := Roles(), []string{"gate-fixer", "verify-fixer"}; !slices.Equal(got, want) {
		t.Errorf("Roles() = %v; want %v", got, want)
	}

	tests := []struct {
		name string
		role string
		want bool
	}{
		{"GateFixer", RoleGateFixer, true},
		{"VerifyFixer", RoleVerifyFixer, true},
		{"GateFixerNumbered", "gate-fixer-2", true},
		{"VerifyFixerNumbered", "verify-fixer-3", true},
		{"GateFixerOne", "gate-fixer-1", false},
		{"GateFixerLeadingZero", "gate-fixer-02", false},
		{"OtherRole", "implementer", false},
		{"Empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsFixerRole(tt.role); got != tt.want {
				t.Errorf("IsFixerRole(%q) = %v; want %v", tt.role, got, tt.want)
			}
		})
	}
}
