// registry_test.go covers the unexported register/lookup pair in isolation from
// package-level init() state: each subtest builds its own registry map so a
// probe batcher's registration can never leak into — or be masked by — the real
// identity registration identity.go's init() performs on package load. Tier-1
// (pure logic, no git, no TestMain), per the go-test-tiers-and-hermetic-git
// Shared Decision.

package batcher

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// probeBatcher is a minimal Batcher stand-in used only to exercise
// register/lookup without depending on identityBatcher's own behavior.
type probeBatcher struct{ name string }

func (p probeBatcher) Batch(cards []planparser.Card) []Batch { return nil }
func (p probeBatcher) Name() string                          { return p.name }

// withEmptyRegistry swaps the package-level registry for a fresh empty map for
// the duration of the test, restoring the original via t.Cleanup so other tests
// (and identity.go's own init() registration) are unaffected.
func withEmptyRegistry(t *testing.T) {
	t.Helper()
	original := registry
	registry = make(map[string]Batcher)
	t.Cleanup(func() { registry = original })
}

func TestRegister_Lookup_RoundTrip(t *testing.T) {
	withEmptyRegistry(t)

	b := probeBatcher{name: "probe"}
	register(b)

	got, ok := lookup("probe")
	if !ok {
		t.Fatalf("lookup(%q) reported not found after register", "probe")
	}
	if got.Name() != "probe" {
		t.Errorf("lookup(%q).Name() = %q; want %q", "probe", got.Name(), "probe")
	}
}

func TestLookup_UnknownName(t *testing.T) {
	withEmptyRegistry(t)

	_, ok := lookup("does-not-exist")
	if ok {
		t.Errorf("lookup(%q) reported found; want not-found for an unregistered name", "does-not-exist")
	}
}
