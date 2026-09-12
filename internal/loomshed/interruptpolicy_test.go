// interruptpolicy_test.go covers InterruptPolicyFor's own contract and the table's closed value
// vocabulary. It does not check the table against loom's assembled rows -- that agreement is
// internal/loomrecipe's interruptpolicy_meta_test.go, the only place that can see both the table and
// the unexported row-to-engine mapping the meta-test cross-checks it against.

package loomshed

import "testing"

func TestInterruptPolicyFor(t *testing.T) {
	tests := []struct {
		name string
		row  string
		want string
	}{
		{"webster handback", NameWebster, InterruptPolicyHandback},
		{"gate reinvoke", NamePreflight, InterruptPolicyReinvoke},
		{"writer reinvoke", NameDiscussionWrite, InterruptPolicyReinvoke},
		{"validator reinvoke", NameDiscussionValidate, InterruptPolicyReinvoke},
		{"bouncer reinvoke", NameDiscussionBouncer, InterruptPolicyReinvoke},
		{"burler reinvoke", NameDiscussionBurler, InterruptPolicyReinvoke},
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

// TestInterruptPolicies_ClosedVocabulary asserts every value in InterruptPolicies is one of the two
// declared constants, so a typo'd third policy word cannot ship.
func TestInterruptPolicies_ClosedVocabulary(t *testing.T) {
	for row, policy := range InterruptPolicies {
		if policy != InterruptPolicyReinvoke && policy != InterruptPolicyHandback {
			t.Errorf("InterruptPolicies[%q] = %q; want %q or %q", row, policy, InterruptPolicyReinvoke, InterruptPolicyHandback)
		}
	}
}
