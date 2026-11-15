// coverage_guard_test.go is the cross-consumer registry coverage guard: it unions every recipe
// consumer's own engine set and asserts every name in shedrecipe.Names() is reached by that
// union or is explicitly allowlisted. This guard, not any one consumer, is where a new registry
// key's coverage is now checked -- a consumer holding a full copy of this assertion would fail on
// the other consumer's engines, the same bug, doubled.
//
// It lives in package shedrecipe_test, the external test package, because that is the only place
// that may import both recipe consumers -- internal/loomrecipe and internal/battenrecipe --
// without an import cycle: neither consumer package may import the other, and this package's own
// internal test package cannot import either without producing shedrecipe -> loomrecipe ->
// shedrecipe (and the battenrecipe equivalent).

package shedrecipe_test

import (
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/testkit/shedcheckkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// coverageGuardAllowedUnreachableEngines names the registry engines this guard tolerates as
// unreferenced by any consumer's engine set. Stub joins this allowlist now that the
// last stubbed row -- Webster-Review -- is real: no loom row reaches Stub any more, and the engine
// stays registered because internal/shedrecipe's registry is generic Shed machinery shared by
// reference with a future product's producer list rather than loom's private property.
// MultiLLM is tolerated the same way: no loom row uses it yet.
var coverageGuardAllowedUnreachableEngines = map[string]bool{
	"MultiLLM": true,
	"Stub":     true,
}

// reachedEngines returns the union of every recipe consumer's engine set.
func reachedEngines() map[string]bool {
	reached := make(map[string]bool)
	for _, engine := range loomrecipe.RecipeEngines() {
		reached[engine] = true
	}
	for _, engine := range shedcheckkit.RecipeEngines(recipes.BattenRecipe) {
		reached[engine] = true
	}
	return reached
}

// TestCoverageGuard asserts every name in shedrecipe.Names() is reached or allowlisted.
// Reached means in the union of loomrecipe.RecipeEngines() and the batten recipe's engines.
// Allowlisted means on coverageGuardAllowedUnreachableEngines.
// It also guards the allowlist's drift.
// An entry naming an engine that is no longer registered fails rather than lingering.
// So does an entry naming an engine that some consumer now does reach.
//
//testtiming:keep pins that every registered engine is reached or allowlisted, and that the allowlist does not drift, a guard that fires on a new registry key
func TestCoverageGuard(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	reached := reachedEngines()
	registered := make(map[string]bool)
	for _, name := range shedrecipe.Names() {
		registered[name] = true
	}

	t.Run("EveryRegisteredEngineIsReachedOrAllowlisted", func(t *testing.T) {
		t.Parallel()
		for name := range registered {
			if reached[name] || coverageGuardAllowedUnreachableEngines[name] {
				continue
			}
			t.Errorf("shedrecipe.Names() has %q, which no consumer's engine set reaches and which is not in coverageGuardAllowedUnreachableEngines", name)
		}
	})

	t.Run("AllowlistDoesNotDrift", func(t *testing.T) {
		t.Parallel()
		for name := range coverageGuardAllowedUnreachableEngines {
			if !registered[name] {
				t.Errorf("coverageGuardAllowedUnreachableEngines names %q, which shedrecipe.Names() no longer registers", name)
			}
			if reached[name] {
				t.Errorf("coverageGuardAllowedUnreachableEngines names %q, which a consumer's engine set now reaches -- remove it from the allowlist", name)
			}
		}
	})
}
