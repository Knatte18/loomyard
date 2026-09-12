// interruptpolicy_meta_test.go pins loomshed.InterruptPolicies against the production authority --
// the rows New actually assembles -- rather than against a standalone literal that could drift
// silently. It lives here rather than beside the table in internal/loomshed because loomRowEngines,
// the unexported package-level var coverage_guard_test.go already declares, is the only thing that
// can also see the engine identity behind each row, and a test in internal/loomshed cannot see it at
// all.

package loomrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
)

// TestInterruptPolicies_MatchAssembledRows asserts, in both directions, that
// loomshed.InterruptPolicies covers exactly the rows New assembles: every row New's real,
// current output has must carry an entry in the table (the direction that catches a row added to
// the recipe with no policy assigned), and every key in the table must name a row New actually
// has (the direction that keeps the table from accumulating dead entries for a renamed or removed
// row).
func TestInterruptPolicies_MatchAssembledRows(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	rowNames := make(map[string]bool, len(shed.Producers))
	for _, p := range shed.Producers {
		rowNames[p.Name] = true
		if _, ok := loomshed.InterruptPolicies[p.Name]; !ok {
			t.Errorf("New() row %q has no entry in loomshed.InterruptPolicies", p.Name)
		}
	}

	for rowName := range loomshed.InterruptPolicies {
		if !rowNames[rowName] {
			t.Errorf("loomshed.InterruptPolicies names row %q, which New() does not have", rowName)
		}
	}
}

// TestInterruptPolicies_MatchEngineIdentity cross-checks loomshed.InterruptPolicies against
// loomRowEngines, the unexported row-to-engine mapping coverage_guard_test.go declares: every row
// whose engine is "Webster" must carry loomshed.InterruptPolicyHandback, and every row whose engine
// is anything else must carry loomshed.InterruptPolicyReinvoke. This is the assertion that keeps a
// row which changes adapter from silently keeping the wrong policy, and the engine-name side is
// what makes the Webster exception derivable rather than hand-maintained.
func TestInterruptPolicies_MatchEngineIdentity(t *testing.T) {
	for rowName, engineName := range loomRowEngines {
		policy, ok := loomshed.InterruptPolicies[rowName]
		if !ok {
			t.Errorf("loomshed.InterruptPolicies has no entry for row %q", rowName)
			continue
		}

		if engineName == "Webster" {
			if policy != loomshed.InterruptPolicyHandback {
				t.Errorf("row %q (engine %q) policy = %q; want %q", rowName, engineName, policy, loomshed.InterruptPolicyHandback)
			}
			continue
		}

		if policy != loomshed.InterruptPolicyReinvoke {
			t.Errorf("row %q (engine %q) policy = %q; want %q", rowName, engineName, policy, loomshed.InterruptPolicyReinvoke)
		}
	}
}
