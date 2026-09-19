// coverage_guard_test.go is the cross-consumer registry coverage guard: it unions every recipe
// consumer's own RecipeEngines() and asserts every name in shedrecipe.Names() is reached by that
// union or is explicitly allowlisted. This guard, not any one consumer, is where a new registry
// key's coverage is now checked -- a consumer holding a full copy of this assertion would fail on
// the other consumer's engines, the same bug, doubled.
//
// It lives in package shedrecipe_test, the external test package, because that is the only place
// that may import both recipe consumers -- internal/loomrecipe and internal/lifecyclerecipe --
// without an import cycle: neither consumer package may import the other, and this package's own
// internal test package cannot import either without producing shedrecipe -> loomrecipe ->
// shedrecipe (and the lifecyclerecipe equivalent).

package shedrecipe_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/lifecyclerecipe"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

// coverageGuardAllowedUnreachableEngines names the registry engines this guard tolerates as
// unreferenced by any consumer's RecipeEngines() union. Stub joins this allowlist now that the
// last stubbed row -- Webster-Review -- is real: no loom row reaches Stub any more, and the engine
// stays registered because internal/shedrecipe's registry is generic Shed machinery shared by
// reference with a future product's producer list rather than loom's private property.
// SingleLLM is the other tolerated entry: the two other "loom: real LLM producers" roadmap items
// (manifest/roadmap.md) have not yet landed a row that reaches it.
var coverageGuardAllowedUnreachableEngines = map[string]bool{
	"SingleLLM": true,
	"Stub":      true,
}

// TestCoverageGuard_EveryRegisteredEngineIsReachedOrAllowlisted asserts every name in
// shedrecipe.Names() is in the union of loomrecipe.RecipeEngines() and
// lifecyclerecipe.RecipeEngines(), or is on coverageGuardAllowedUnreachableEngines.
func TestCoverageGuard_EveryRegisteredEngineIsReachedOrAllowlisted(t *testing.T) {
	reached := make(map[string]bool)
	for _, engine := range loomrecipe.RecipeEngines() {
		reached[engine] = true
	}
	for _, engine := range lifecyclerecipe.RecipeEngines() {
		reached[engine] = true
	}

	for _, name := range shedrecipe.Names() {
		if reached[name] || coverageGuardAllowedUnreachableEngines[name] {
			continue
		}
		t.Errorf("shedrecipe.Names() has %q, which no consumer's RecipeEngines() reaches and which is not in coverageGuardAllowedUnreachableEngines", name)
	}
}

// TestCoverageGuard_AllowlistDoesNotDrift carries this guard's own drift direction: an allowlist
// entry naming an engine that is no longer registered, or that some consumer now does reach, fails
// rather than lingering.
func TestCoverageGuard_AllowlistDoesNotDrift(t *testing.T) {
	registered := make(map[string]bool)
	for _, name := range shedrecipe.Names() {
		registered[name] = true
	}

	reached := make(map[string]bool)
	for _, engine := range loomrecipe.RecipeEngines() {
		reached[engine] = true
	}
	for _, engine := range lifecyclerecipe.RecipeEngines() {
		reached[engine] = true
	}

	for name := range coverageGuardAllowedUnreachableEngines {
		if !registered[name] {
			t.Errorf("coverageGuardAllowedUnreachableEngines names %q, which shedrecipe.Names() no longer registers", name)
		}
		if reached[name] {
			t.Errorf("coverageGuardAllowedUnreachableEngines names %q, which a consumer's RecipeEngines() now reaches -- remove it from the allowlist", name)
		}
	}
}
