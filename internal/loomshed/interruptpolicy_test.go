// interruptpolicy_test.go covers InterruptPolicyFor's own contract and the table's closed value
// vocabulary. It does not check the table against loom's assembled rows -- that agreement is
// internal/loomrecipe's interruptpolicy_meta_test.go, the only place that can see both the table and
// the unexported row-to-engine mapping the meta-test cross-checks it against.

package loomshed

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestInterruptPolicyFor(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name string
		row  string
		want string
	}{
		{"webster handback", NameWebster, shedverbs.InterruptPolicyHandback},
		{"gate reinvoke", NamePreflight, shedverbs.InterruptPolicyReinvoke},
		{"writer reinvoke", NameDiscussionWrite, shedverbs.InterruptPolicyReinvoke},
		{"parts write reinvoke", NamePartsWrite, shedverbs.InterruptPolicyReinvoke},
		{"parts plan write reinvoke", NamePartsPlanWrite, shedverbs.InterruptPolicyReinvoke},
		{"bouncer reinvoke", NameDiscussionBouncer, shedverbs.InterruptPolicyReinvoke},
		{"burler review reinvoke", NameDiscussionBurlerReview, shedverbs.InterruptPolicyReinvoke},
		{"burler fix reinvoke", NameCodeBurlerFix, shedverbs.InterruptPolicyReinvoke},
		{"darn reinvoke", NameDarnWrite, shedverbs.InterruptPolicyReinvoke},
		{"beam up reinvoke", pawlengine.NameBeamUp, shedverbs.InterruptPolicyReinvoke},
		{"beam await reinvoke", pawlengine.NameBeamAwait, shedverbs.InterruptPolicyReinvoke},
		{"beam down reinvoke", pawlengine.NameBeamDown, shedverbs.InterruptPolicyReinvoke},
		{"empty name", "", ""},
		{"unknown name", "Not-A-Real-Row", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InterruptPolicyFor(tt.row); got != tt.want {
				t.Errorf("InterruptPolicyFor(%q) = %q; want %q", tt.row, got, tt.want)
			}
		})
	}
}

// TestInterruptPolicies_ClosedVocabulary asserts every value in InterruptPolicies is one of the two declared constants, so a typo'd third policy word cannot ship.
//
//testtiming:keep pins that every policy value is a declared constant, which TestArchivedWebsterDirs never asserts
func TestInterruptPolicies_ClosedVocabulary(t *testing.T) {
	tiergate.Require(t, 0)
	for row, policy := range InterruptPolicies {
		if policy != shedverbs.InterruptPolicyReinvoke && policy != shedverbs.InterruptPolicyHandback {
			t.Errorf("InterruptPolicies[%q] = %q; want %q or %q", row, policy, shedverbs.InterruptPolicyReinvoke, shedverbs.InterruptPolicyHandback)
		}
	}
}
