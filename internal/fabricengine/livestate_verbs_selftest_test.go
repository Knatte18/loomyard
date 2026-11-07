// livestate_verbs_selftest_test.go checks the Verbs table's own shape:
// which cases restrict the cross product's States, and which inherit the full matrix.

package fabricengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestVerbCases_ArrangeSetsItsPreconditionField proves that Add, Checkout and Pull each arrange their fixture field.
// assertArrangePreconditions checks that field for the verb.
// That check skips an empty field.
// An Arrange that stopped setting one would leave the verb's column vacuous, unnoticed.
func TestVerbCases_ArrangeSetsItsPreconditionField(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		verb  string
		field func(VerbFixture) string
	}{
		{"Add", func(f VerbFixture) string { return f.LauncherBlocker }},
		{"Checkout", func(f VerbFixture) string { return f.CheckoutBranch }},
		{"Pull", func(f VerbFixture) string { return f.AdvancedToSHA }},
	}
	for _, tt := range tests {
		t.Run(tt.verb, func(t *testing.T) {
			t.Parallel()

			var vc *VerbCase
			for i := range Verbs {
				if Verbs[i].Name == tt.verb {
					vc = &Verbs[i]
				}
			}
			if vc == nil {
				t.Fatalf("Verbs has no %s case", tt.verb)
			}
			h := copyVerbHub(t, *vc, ".")

			if got := tt.field(vc.Arrange(t, h)); got == "" {
				t.Errorf("%s Arrange left its precondition field empty", tt.verb)
			}
		})
	}
}

// TestVerbCases_StatesRestrictionIsWellFormed proves that every hostile-input and CloneHub{Reset} case
// restricts States to exactly ["clean"], and that every ordinary verb leaves States empty.
// An empty States inherits the full state matrix.
// A case that got this backwards would vanish from the cross product, or explode it with cells that were never derived.
//
//testtiming:keep hostile-input and CloneHub{Reset} cases restricting States to clean and ordinary verbs inheriting the full matrix; coverage of its blocks by other tests does not show an assertion of this
func TestVerbCases_StatesRestrictionIsWellFormed(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	ordinary := map[string]bool{
		"Add": true, "Remove": true, "Prune": true, "Cleanup": true,
		"Checkout": true, "Reconcile": true, "UnwireJunctions": true, "Pull": true,
	}

	for _, vc := range Verbs {
		if ordinary[vc.Name] {
			if len(vc.States) != 0 {
				t.Errorf("%s: States = %v; want empty (ordinary verbs inherit the full matrix)", vc.Name, vc.States)
			}
			continue
		}

		if len(vc.States) != 1 || vc.States[0] != "clean" {
			t.Errorf("%s: States = %v; want exactly [\"clean\"]", vc.Name, vc.States)
		}
	}
}
